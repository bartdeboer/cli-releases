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

	clir "github.com/bartdeboer/go-clir"
)

type Options struct {
	Tool, Version, BinDir string
	JSON, Overwrite       bool
}

func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	r := router(out, errOut)
	if len(args) == 0 {
		return r.FPrintHelp(ctx, out, nil, clir.Depth(2))
	}
	if clir.IsHelpRequest(args) {
		if err := r.FPrintHelp(ctx, out, clir.StripHelpToken(args), clir.Depth(2)); err == nil {
			return nil
		}
		return r.FPrintHelp(ctx, out, nil, clir.Depth(2))
	}
	return r.Run(ctx, args)
}
func router(out, errOut io.Writer) *clir.Router {
	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Describe("", "Install verified public CLI releases.")
		b.Handle("install <tool>", "Install one exact tool version.", func(req *clir.Request) error {
			o, e := parse(req.Params["tool"], req.Extra)
			if e != nil {
				return e
			}
			return runInstall(req.Context(), o, out, client{})
		})
		b.Handle("bootstrap", "Install cli-get from this module source.", func(req *clir.Request) error { return bootstrap(req.Context(), req.Extra, out, errOut) })
	})
	return r
}
func parse(tool string, a []string) (Options, error) {
	o := Options{Tool: tool}
	if strings.HasPrefix(tool, "-") || !toolRE.MatchString(tool) {
		return o, errors.New("tool is not canonical")
	}
	seen := map[string]bool{}
	for i := 0; i < len(a); i++ {
		flag := a[i]
		if seen[flag] {
			return o, fmt.Errorf("%s specified twice", flag)
		}
		switch flag {
		case "--json":
			seen[flag] = true
			o.JSON = true
		case "--overwrite":
			seen[flag] = true
			o.Overwrite = true
		case "--version", "--bin-dir":
			seen[flag] = true
			if i+1 >= len(a) || strings.HasPrefix(a[i+1], "--") {
				return o, errors.New(flag + " requires value")
			}
			v := a[i+1]
			i++
			if flag == "--version" {
				o.Version = v
			} else {
				o.BinDir = v
			}
		default:
			return o, fmt.Errorf("unexpected argument %q", flag)
		}
	}
	if !verRE.MatchString(o.Version) {
		return o, errors.New("version is not canonical")
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
	return runInstallPlatform(ctx, o, out, c, runtime.GOOS, runtime.GOARCH)
}
func runInstallPlatform(ctx context.Context, o Options, out io.Writer, c client, goos, goarch string) error {
	if (goos != "linux" && goos != "darwin") || (goarch != "amd64" && goarch != "arm64") {
		return errors.New("M1 supports only linux and darwin on amd64 and arm64")
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
	m, a, e := parseManifest(mb, o.Tool, o.Version, goos, goarch)
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
	res := result{APIVersion: "cli-get.output/v1", ObservedAt: time.Now().UTC(), Status: "installed", Tool: o.Tool, Version: o.Version, Platform: goos + "/" + goarch, Destination: dst, SHA256: digest(exe), Size: int64(len(exe)), Overwritten: o.Overwrite}
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
