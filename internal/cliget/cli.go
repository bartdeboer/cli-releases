package cliget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Options struct {
	Tool, Version, BinDir string
	JSON, Overwrite       bool
}

func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" {
		help(out)
		return nil
	}
	if args[0] == "bootstrap" {
		return bootstrap(ctx, args[1:], out, errOut)
	}
	if args[0] != "install" {
		return errors.New("expected install or bootstrap")
	}
	o, e := parse(args[1:])
	if e != nil {
		return e
	}
	return runInstall(ctx, o, out, client{})
}
func help(w io.Writer) {
	fmt.Fprintln(w, "cli-get install TOOL --version vX.Y.Z [--bin-dir DIR] [--overwrite] [--json]\ncli-get bootstrap [--source MODULE_ROOT]")
}
func parse(a []string) (Options, error) {
	var o Options
	if len(a) < 1 {
		return o, errors.New("install requires TOOL")
	}
	o.Tool = a[0]
	for i := 1; i < len(a); i++ {
		switch a[i] {
		case "--json":
			o.JSON = true
		case "--overwrite":
			o.Overwrite = true
		case "--version", "--bin-dir":
			if i+1 >= len(a) {
				return o, errors.New(a[i] + " requires value")
			}
			v := a[i+1]
			i++
			if a[i-1] == "--version" {
				o.Version = v
			} else {
				o.BinDir = v
			}
		default:
			return o, fmt.Errorf("unexpected argument %q", a[i])
		}
	}
	if !toolRE.MatchString(o.Tool) || !verRE.MatchString(o.Version) {
		return o, errors.New("tool or version is not canonical")
	}
	if o.BinDir == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return o, e
		}
		o.BinDir = filepath.Join(home, "go", "bin")
	}
	return o, nil
}
func runInstall(ctx context.Context, o Options, out io.Writer, c client) error {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errors.New("M1 supports only linux/amd64 and linux/arm64")
	}
	tag := o.Tool + "/" + o.Version
	r, e := c.release(ctx, tag)
	if e != nil {
		return e
	}
	if r.Tag != tag || r.Draft || r.Prerelease {
		return errors.New("release identity or state mismatch")
	}
	ma, e := findAsset(r, "manifest.json")
	if e != nil {
		return e
	}
	mb, e := c.asset(ctx, ma.ID, 1<<20)
	if e != nil {
		return e
	}
	if int64(len(mb)) != ma.Size || ma.Digest != "sha256:"+digest(mb) || ma.ContentType != "application/json" {
		return errors.New("manifest provider metadata mismatch")
	}
	m, a, e := parseManifest(mb, o.Tool, o.Version, runtime.GOOS, runtime.GOARCH)
	if e != nil {
		return e
	}
	if e = validateRemoteAssets(r, m); e != nil {
		return e
	}
	ca, e := findAsset(r, "SHA256SUMS.txt")
	if e != nil {
		return e
	}
	cb, e := c.asset(ctx, ca.ID, 1<<20)
	if e != nil {
		return e
	}
	if int64(len(cb)) != ca.Size || ca.Digest != "sha256:"+digest(cb) || ca.ContentType != "text/plain; charset=utf-8" || parseChecksums(cb, mb, m) != nil {
		return errors.New("checksum asset mismatch")
	}
	aa, e := findAsset(r, a.File)
	if e != nil {
		return e
	}
	expectedType := "application/gzip"
	if strings.HasSuffix(a.File, ".zip") {
		expectedType = "application/zip"
	}
	if aa.Size != a.Size || aa.Digest != "sha256:"+a.SHA256 || aa.ContentType != expectedType {
		return errors.New("archive provider metadata mismatch")
	}
	ab, e := c.asset(ctx, aa.ID, a.Size)
	if e != nil {
		return e
	}
	if int64(len(ab)) != a.Size || digest(ab) != a.SHA256 {
		return errors.New("archive digest mismatch")
	}
	exe, e := extractOne(ab, a.File, a.Executable)
	if e != nil {
		return e
	}
	dst, e := installFile(o.BinDir, o.Tool, exe, o.Overwrite)
	if e != nil {
		return e
	}
	res := result{APIVersion: "cli-get.output/v1", ObservedAt: time.Now().UTC(), Status: "installed", Tool: o.Tool, Version: o.Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, Destination: dst, SHA256: digest(exe), Size: int64(len(exe)), Overwritten: o.Overwrite}
	if o.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	fmt.Fprintf(out, "Installed %s %s to %s\n", o.Tool, o.Version, dst)
	return nil
}
func validateRemoteAssets(r releaseResponse, m Manifest) error {
	want := map[string]Artifact{}
	for _, a := range m.Artifacts {
		want[a.File] = a
	}
	seen := map[string]bool{}
	for _, a := range r.Assets {
		if seen[a.Name] {
			return errors.New("duplicate release asset")
		}
		seen[a.Name] = true
		if a.Name == "manifest.json" || a.Name == "SHA256SUMS.txt" {
			continue
		}
		v, ok := want[a.Name]
		if !ok || a.Size != v.Size || a.Digest != "sha256:"+v.SHA256 {
			return errors.New("release asset set disagrees with manifest")
		}
	}
	if len(seen) != len(want)+2 || !seen["manifest.json"] || !seen["SHA256SUMS.txt"] {
		return errors.New("release asset set is not exact")
	}
	for n := range want {
		if !seen[n] {
			return errors.New("release asset missing")
		}
	}
	return nil
}
func bootstrap(ctx context.Context, a []string, out, errOut io.Writer) error {
	source := ""
	if len(a) == 2 && a[0] == "--source" {
		source = a[1]
	} else if len(a) != 0 {
		return errors.New("bootstrap accepts only --source MODULE_ROOT")
	}
	if source == "" {
		source = "."
	}
	abs, e := filepath.Abs(source)
	if e != nil {
		return e
	}
	data, e := os.ReadFile(filepath.Join(abs, "go.mod"))
	if e != nil || !strings.HasPrefix(string(data), "module github.com/bartdeboer/cli-releases\n") {
		return errors.New("bootstrap requires cli-releases module root")
	}
	cmd := exec.CommandContext(ctx, "go", "install", "-mod=readonly", "-trimpath", "-ldflags=-s -w", "./cmd/cli-get")
	cmd.Dir = abs
	cmd.Env = append(removeEnv(os.Environ(), "CGO_ENABLED="), "CGO_ENABLED=0")
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}
func removeEnv(a []string, p string) []string {
	out := []string{}
	for _, v := range a {
		if !strings.HasPrefix(v, p) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func JSONRequested(args []string) bool {
	for _, v := range args {
		if v == "--json" {
			return true
		}
	}
	return false
}
func WriteError(w io.Writer, err error) error {
	return json.NewEncoder(w).Encode(map[string]any{"apiVersion": "cli-get.output/v1", "status": "error", "error": map[string]string{"code": "failed", "message": err.Error()}})
}
