package cliget

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func documentationFixture(t *testing.T, tamper string) (client, *[]string) {
	t.Helper()
	files, err := os.ReadDir("testdata/bundle-v2")
	if err != nil {
		t.Fatal(err)
	}
	data := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join("testdata/bundle-v2", f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		data[f.Name()] = b
	}
	if tamper == "controls" {
		data["README.md"] = []byte("safe\n\x1b]52;c;evil\a\r\u202eend\n")
		var m Manifest
		json.Unmarshal(data["manifest.json"], &m)
		m.Documentation[0].Size = int64(len(data["README.md"]))
		m.Documentation[0].SHA256 = digest(data["README.md"])
		b, _ := json.MarshalIndent(m, "", "  ")
		data["manifest.json"] = append(b, '\n')
		lines := []string{}
		for name, b := range data {
			if name != "SHA256SUMS.txt" {
				lines = append(lines, digest(b)+"  "+name)
			}
		}
		sort.Strings(lines)
		data["SHA256SUMS.txt"] = []byte(strings.Join(lines, "\n") + "\n")
	}
	meta := releaseResponse{Tag: "example/v1.2.3"}
	assetData := map[string][]byte{}
	requested := []string{}
	names := []string{}
	for name := range data {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		b := data[name]
		typ := "application/gzip"
		switch name {
		case "manifest.json":
			typ = "application/json"
		case "README.md", "SHA256SUMS.txt":
			typ = "text/plain; charset=utf-8"
		case "docs.zip":
			typ = "application/zip"
		}
		id := int64(i + 1)
		meta.Assets = append(meta.Assets, releaseAsset{ID: id, Name: name, Size: int64(len(b)), Digest: "sha256:" + digest(b), ContentType: typ})
		assetData[fmt.Sprint(id)] = b
	}
	if tamper == "inventory" {
		meta.Assets = append(meta.Assets, releaseAsset{ID: 99, Name: "extra", Size: 1})
	}
	if tamper == "readme" {
		for _, a := range meta.Assets {
			if a.Name == "README.md" {
				assetData[fmt.Sprint(a.ID)] = []byte("tampered")
			}
		}
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("credential header")
		}
		if strings.Contains(r.URL.Path, "/releases/tags/") {
			json.NewEncoder(w).Encode(meta)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/releases") {
			json.NewEncoder(w).Encode([]releaseResponse{{Tag: "other/v99.0.0"}, {Tag: "example/v1.2.2"}, meta})
			return
		}
		id := filepath.Base(r.URL.Path)
		for _, a := range meta.Assets {
			if fmt.Sprint(a.ID) == id {
				requested = append(requested, a.Name)
			}
		}
		w.Write(assetData[id])
	}))
	t.Cleanup(srv.Close)
	return client{base: srv.URL, http: srv.Client(), testHost: "yes"}, &requested
}
func TestReadmeOnlyDownloadsMetadataAndReadme(t *testing.T) {
	for _, version := range []string{"v1.2.3", ""} {
		c, requested := documentationFixture(t, "")
		var out bytes.Buffer
		if err := runReadme(context.Background(), Options{Tool: "example", Version: version}, &out, c); err != nil {
			t.Fatal(err)
		}
		want, _ := os.ReadFile("testdata/bundle-v2/README.md")
		if !bytes.Equal(want, out.Bytes()) {
			t.Fatal(out.String())
		}
		if strings.Join(*requested, ",") != "manifest.json,SHA256SUMS.txt,README.md" {
			t.Fatal(*requested)
		}
	}
}
func TestReadmeFailureEmitsNothing(t *testing.T) {
	for _, tamper := range []string{"readme", "inventory"} {
		c, _ := documentationFixture(t, tamper)
		var out bytes.Buffer
		if runReadme(context.Background(), Options{Tool: "example", Version: "v1.2.3"}, &out, c) == nil || out.Len() != 0 {
			t.Fatal("accepted tamper/emitted bytes")
		}
	}
	c, o := fixture(t, "")
	var out bytes.Buffer
	if runReadme(context.Background(), o, &out, c) == nil || out.Len() != 0 {
		t.Fatal("v1 README should be missing")
	}
}
func TestReadmeEscapesTerminalControls(t *testing.T) {
	c, _ := documentationFixture(t, "controls")
	var out bytes.Buffer
	if err := runReadme(context.Background(), Options{Tool: "example", Version: "v1.2.3"}, &out, c); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out.String(), "\x1b\a\r\u202e") || !strings.Contains(out.String(), "\\u{001b}") {
		t.Fatal(out.String())
	}
}
func TestV2InstallDoesNotDownloadDocumentation(t *testing.T) {
	c, requested := documentationFixture(t, "")
	dir := t.TempDir()
	if err := runInstallPlatform(context.Background(), Options{Tool: "example", Version: "v1.2.3", BinDir: dir}, &bytes.Buffer{}, c, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, name := range *requested {
		if name == "README.md" || name == "docs.zip" {
			t.Fatal("downloaded documentation during install")
		}
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 || files[0].Name() != "example" {
		t.Fatal(files)
	}
}
func TestDocumentationVersionValidation(t *testing.T) {
	data, _ := os.ReadFile("testdata/bundle-v2/manifest.json")
	var m Manifest
	json.Unmarshal(data, &m)
	m.Schema = manifestSchema
	b, _ := json.MarshalIndent(m, "", "  ")
	if _, _, err := parseManifest(append(b, '\n'), "example", "v1.2.3", "", ""); err == nil {
		t.Fatal("v1 accepted docs")
	}
	for _, args := range [][]string{{"--version", "v1.2.3", "--version", "v1.2.4"}, {"--overwrite"}, {"--version", "bad"}, {"extra"}} {
		if _, err := parseReadme("example", args); err == nil {
			t.Fatal(args)
		}
	}
	if !versionGreater("v100000000000000000000.0.0", "v9.9.9") || versionGreater("v1.2.3", "v1.10.0") {
		t.Fatal("incorrect semver comparison")
	}
}

func TestSharedV1Golden(t *testing.T) {
	mb, err := os.ReadFile("testdata/bundle-v1/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := parseManifest(mb, "github-client", "v0.1.0", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	cb, err := os.ReadFile("testdata/bundle-v1/SHA256SUMS.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err = parseChecksums(cb, mb, m); err != nil {
		t.Fatal(err)
	}
}
func TestDiscoveryFailsClosedAtPageLimit(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		rows := make([]releaseResponse, 100)
		rows[0] = releaseResponse{Tag: "example/v1.2.3"}
		json.NewEncoder(w).Encode(rows)
	}))
	defer srv.Close()
	c := client{base: srv.URL, http: srv.Client(), testHost: "yes"}
	if _, err := c.newestVersion(context.Background(), "example"); err == nil || calls != 10 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
