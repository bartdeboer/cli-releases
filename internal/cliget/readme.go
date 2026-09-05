package cliget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

func validateDocumentation(m Manifest) error {
	if m.Schema == manifestSchema {
		if m.Documentation != nil {
			return errors.New("v1 cannot declare documentation")
		}
		return nil
	}
	d := m.Documentation
	if len(d) < 1 || len(d) > 2 {
		return errors.New("v2 requires README and optional docs.zip")
	}
	for i, v := range d {
		name, limit := "README.md", int64(1<<20)
		if i == 1 {
			name, limit = "docs.zip", 32<<20
		}
		if v.File != name || v.Size < 1 || v.Size > limit || !hexRE.MatchString(v.SHA256) {
			return errors.New("invalid documentation declaration")
		}
	}
	return nil
}

func parseReadme(tool string, args []string) (Options, error) {
	o := Options{Tool: tool}
	if !toolRE.MatchString(tool) {
		return o, errors.New("tool is not canonical")
	}
	if len(args) == 0 {
		return o, nil
	}
	if len(args) != 2 || args[0] != "--version" || !verRE.MatchString(args[1]) {
		return o, errors.New("usage: cli-get readme TOOL [--version vX.Y.Z]")
	}
	o.Version = args[1]
	return o, nil
}

// Discovery is bounded and tool-specific. A full final page fails closed:
// a partial scan must never silently become a claim about the newest version.
func (c client) newestVersion(ctx context.Context, tool string) (string, error) {
	newest := ""
	for page := 1; page <= 10; page++ {
		b, err := c.do(ctx, "GET", fmt.Sprintf("/repos/bartdeboer/cli-releases/releases?per_page=100&page=%d", page), "application/vnd.github+json", 4<<20)
		if err != nil {
			return "", err
		}
		var releases []releaseResponse
		if json.Unmarshal(b, &releases) != nil || len(releases) > 100 {
			return "", errors.New("invalid release discovery metadata")
		}
		for _, r := range releases {
			if r.Draft || r.Prerelease || !strings.HasPrefix(r.Tag, tool+"/") {
				continue
			}
			version := strings.TrimPrefix(r.Tag, tool+"/")
			if verRE.MatchString(version) && (newest == "" || versionGreater(version, newest)) {
				newest = version
			}
		}
		if len(releases) < 100 {
			if newest == "" {
				return "", errors.New("no stable version found for tool")
			}
			return newest, nil
		}
	}
	return "", errors.New("release discovery exceeds bound; specify --version")
}

// Compare arbitrary-length canonical decimal components without overflow.
func versionGreater(a, b string) bool {
	aa, bb := strings.Split(a[1:], "."), strings.Split(b[1:], ".")
	for i := range aa {
		if len(aa[i]) != len(bb[i]) {
			return len(aa[i]) > len(bb[i])
		}
		if aa[i] != bb[i] {
			return aa[i] > bb[i]
		}
	}
	return false
}

func runReadme(ctx context.Context, o Options, out io.Writer, c client) error {
	var err error
	if o.Version == "" {
		o.Version, err = c.newestVersion(ctx, o.Tool)
		if err != nil {
			return err
		}
	}
	r, m, err := verifiedMetadata(ctx, c, o.Tool, o.Version)
	if err != nil {
		return err
	}
	if len(m.Documentation) == 0 {
		return errors.New("release has no README")
	}
	doc := m.Documentation[0]
	asset, err := findAsset(r, doc.File)
	if err != nil {
		return err
	}
	data, err := c.asset(ctx, asset.ID, doc.Size)
	if err != nil {
		return err
	}
	if int64(len(data)) != doc.Size || digest(data) != doc.SHA256 || !utf8.Valid(data) {
		return errors.New("README size, checksum or UTF-8 mismatch")
	}
	// No Markdown rendering, hyperlink interpretation or escape execution.
	// Escape terminal controls and Unicode format/bidi controls, retaining LF/TAB.
	var safe strings.Builder
	for _, r := range string(data) {
		if (unicode.IsControl(r) || unicode.In(r, unicode.Cf)) && r != '\n' && r != '\t' {
			fmt.Fprintf(&safe, "\\u{%04x}", r)
		} else {
			safe.WriteRune(r)
		}
	}
	_, err = io.WriteString(out, safe.String())
	return err
}
