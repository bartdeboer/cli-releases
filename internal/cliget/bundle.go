package cliget

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

var toolRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var verRE = regexp.MustCompile(`^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$`)
var hexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sourceRE = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func parseManifest(data []byte, tool, version, goos, arch string) (Manifest, Artifact, error) {
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return m, Artifact{}, errors.New("invalid manifest JSON")
	}
	var x any
	if !errors.Is(d.Decode(&x), io.EOF) {
		return m, Artifact{}, errors.New("manifest has trailing data")
	}
	canonical, _ := json.MarshalIndent(m, "", "  ")
	if !bytes.Equal(data, append(canonical, '\n')) {
		return m, Artifact{}, errors.New("manifest is not canonical")
	}
	stamp, e := time.Parse(time.RFC3339, m.BuildTime)
	if m.Schema != manifestSchema || m.Tool != tool || m.Version != version || !toolRE.MatchString(tool) || !verRE.MatchString(version) || !sourceRE.MatchString(m.SourceCommit) || e != nil || stamp.UTC().Format(time.RFC3339) != m.BuildTime || len(m.Artifacts) < 1 || len(m.Artifacts) > 32 {
		return m, Artifact{}, errors.New("manifest identity or provenance mismatch")
	}
	last := ""
	var selected Artifact
	found := 0
	seen := map[string]bool{}
	for _, a := range m.Artifacts {
		key := a.OS + "/" + a.Arch
		expected := tool + "_" + version + "_" + a.OS + "_" + a.Arch
		validName := a.File == expected+".tar.gz" || a.File == expected+".zip"
		if a.File <= last || path.Base(a.File) != a.File || seen[key] || !validName || a.Executable != tool || a.Size < 1 || a.Size > 512<<20 || !hexRE.MatchString(a.SHA256) {
			return m, Artifact{}, errors.New("invalid artifact declaration")
		}
		last = a.File
		seen[key] = true
		if a.OS == goos && a.Arch == arch {
			selected = a
			found++
		}
	}
	if found != 1 {
		return m, Artifact{}, errors.New("bundle does not contain exactly one current platform artifact")
	}
	return m, selected, nil
}
func parseChecksums(data, manifest []byte, m Manifest) error {
	sum := sha256.Sum256(manifest)
	lines := []string{hex.EncodeToString(sum[:]) + "  manifest.json"}
	for _, a := range m.Artifacts {
		lines = append(lines, a.SHA256+"  "+a.File)
	}
	sort.Strings(lines)
	if string(data) != strings.Join(lines, "\n")+"\n" {
		return errors.New("SHA256SUMS.txt is not canonical or disagrees with manifest")
	}
	return nil
}
func extractOne(data []byte, name, expected string) ([]byte, error) {
	if strings.HasSuffix(name, ".tar.gz") {
		g, e := gzip.NewReader(bytes.NewReader(data))
		if e != nil {
			return nil, errors.New("invalid gzip archive")
		}
		defer g.Close()
		t := tar.NewReader(g)
		h, e := t.Next()
		if e != nil || h.Name != expected || h.Typeflag != tar.TypeReg || h.Mode&0111 == 0 || h.Size < 1 || h.Size > 512<<20 {
			return nil, errors.New("archive must contain one normalized executable")
		}
		b, e := io.ReadAll(io.LimitReader(t, h.Size+1))
		if e != nil || int64(len(b)) != h.Size {
			return nil, errors.New("invalid archive member")
		}
		if _, e = t.Next(); !errors.Is(e, io.EOF) {
			return nil, errors.New("archive contains extra members")
		}
		return b, nil
	}
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil || len(z.File) != 1 {
		return nil, errors.New("zip must contain one member")
	}
	f := z.File[0]
	if f.Name != expected || !f.Mode().IsRegular() || f.Mode().Perm()&0111 == 0 || f.UncompressedSize64 > 512<<20 {
		return nil, errors.New("invalid zip member")
	}
	r, e := f.Open()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, 512<<20+1))
	if e != nil || len(b) > 512<<20 {
		return nil, fmt.Errorf("invalid zip data")
	}
	return b, nil
}
