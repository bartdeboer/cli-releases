package cliget

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func fixture(t *testing.T, tamper string) (client, Options) {
	t.Helper()
	tool, ver := "demo", "v1.2.3"
	exe := []byte("#!/bin/sh\nexit 0\n")
	var raw bytes.Buffer
	g := gzip.NewWriter(&raw)
	tw := tar.NewWriter(g)
	tw.WriteHeader(&tar.Header{Name: tool, Mode: 0755, Size: int64(len(exe)), Typeflag: tar.TypeReg})
	tw.Write(exe)
	tw.Close()
	g.Close()
	archive := raw.Bytes()
	an := fmt.Sprintf("%s_%s_%s_%s.tar.gz", tool, ver, runtime.GOOS, runtime.GOARCH)
	ah := digest(archive)
	m := Manifest{Schema: manifestSchema, Tool: tool, Version: ver, SourceCommit: "0123456789abcdef0123456789abcdef01234567", BuildTime: "2026-09-03T00:00:00Z", GoVersion: "go1.24.0", Artifacts: []Artifact{{OS: runtime.GOOS, Arch: runtime.GOARCH, File: an, Size: int64(len(archive)), SHA256: ah, Executable: tool}}}
	mb, _ := json.MarshalIndent(m, "", "  ")
	mb = append(mb, '\n')
	lines := []string{digest(mb) + "  manifest.json", ah + "  " + an}
	sort.Strings(lines)
	cb := []byte(lines[0] + "\n" + lines[1] + "\n")
	assets := map[int64][]byte{1: mb, 2: cb, 3: archive}
	if tamper == "archive" {
		assets[3] = append([]byte{}, archive...)
		assets[3][0] ^= 1
	}
	meta := releaseResponse{Tag: tool + "/" + ver, Assets: []releaseAsset{{ID: 1, Name: "manifest.json", Size: int64(len(mb)), Digest: "sha256:" + digest(mb), ContentType: "application/json"}, {ID: 2, Name: "SHA256SUMS.txt", Size: int64(len(cb)), Digest: "sha256:" + digest(cb), ContentType: "text/plain; charset=utf-8"}, {ID: 3, Name: an, Size: int64(len(archive)), Digest: "sha256:" + ah, ContentType: "application/gzip"}}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bytes.Contains([]byte(r.URL.Path), []byte("/releases/tags/")) {
			json.NewEncoder(w).Encode(meta)
			return
		}
		var id int64
		fmt.Sscanf(filepath.Base(r.URL.Path), "%d", &id)
		w.Write(assets[id])
	}))
	t.Cleanup(srv.Close)
	return client{base: srv.URL, http: srv.Client(), testHost: "yes"}, Options{Tool: tool, Version: ver, BinDir: t.TempDir(), JSON: true}
}
func TestPilotInstallAndNoOverwrite(t *testing.T) {
	c, o := fixture(t, "")
	var out bytes.Buffer
	if e := runInstall(context.Background(), o, &out, c); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(o.BinDir, o.Tool))
	if e != nil || len(b) == 0 {
		t.Fatal(e)
	}
	if e = runInstall(context.Background(), o, &out, c); e == nil {
		t.Fatal("overwrote without flag")
	}
	o.Overwrite = true
	if e = runInstall(context.Background(), o, &out, c); e != nil {
		t.Fatal(e)
	}
}
func TestTamperedArchiveRejectedAndCleaned(t *testing.T) {
	c, o := fixture(t, "archive")
	if e := runInstall(context.Background(), o, &bytes.Buffer{}, c); e == nil {
		t.Fatal("accepted tamper")
	}
	if _, e := os.Stat(filepath.Join(o.BinDir, o.Tool)); !os.IsNotExist(e) {
		t.Fatal("left destination")
	}
}
func TestSymlinkDestinationRejected(t *testing.T) {
	c, o := fixture(t, "")
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("x"), 0600)
	os.Symlink(target, filepath.Join(o.BinDir, o.Tool))
	o.Overwrite = true
	if e := runInstall(context.Background(), o, &bytes.Buffer{}, c); e == nil {
		t.Fatal("accepted symlink")
	}
}
func TestManifestAndChecksumTamper(t *testing.T) {
	m := []byte(`{}`)
	if _, _, e := parseManifest(m, "x", "v1.0.0", "linux", "amd64"); e == nil {
		t.Fatal("accepted manifest")
	}
	sum := sha256.Sum256([]byte("x"))
	_ = hex.EncodeToString(sum[:])
	if e := parseChecksums([]byte("bad\n"), []byte("{}"), Manifest{}); e == nil {
		t.Fatal("accepted checksums")
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var b bytes.Buffer
	if e := WriteError(&b, fmt.Errorf("safe failure")); e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(b.Bytes(), []byte(`"status":"error"`)) || !bytes.Contains(b.Bytes(), []byte("safe failure")) {
		t.Fatal(b.String())
	}
}

func TestRedirectDowngradeRejected(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	c := client{base: source.URL, http: source.Client()}
	if _, e := c.do(context.Background(), "GET", "/x", "application/json", 100); e == nil || reached {
		t.Fatalf("err=%v reached=%v", e, reached)
	}
}
