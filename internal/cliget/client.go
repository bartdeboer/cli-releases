package cliget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type client struct {
	base     string
	http     *http.Client
	testHost string
}

func (c client) endpoint(p string) string {
	b := c.base
	if b == "" {
		b = "https://api.github.com"
	}
	return strings.TrimRight(b, "/") + p
}
func (c client) do(ctx context.Context, method, p, accept string, max int64) ([]byte, error) {
	u := c.endpoint(p)
	req, e := http.NewRequestWithContext(ctx, method, u, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	h := &http.Client{Timeout: 30 * time.Second}
	if c.http != nil {
		*h = *c.http
		if h.Timeout == 0 {
			h.Timeout = 30 * time.Second
		}
	}
	origin, _ := url.Parse(c.endpoint(""))
	h.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		host := strings.ToLower(r.URL.Hostname())
		allowed := r.URL.Scheme == "https" && r.URL.User == nil && r.URL.Port() == "" && (host == "api.github.com" || host == "github.com" || host == "objects.githubusercontent.com" || host == "release-assets.githubusercontent.com")
		if c.testHost != "" {
			allowed = r.URL.Scheme == origin.Scheme && r.URL.Host == origin.Host
		}
		if !allowed {
			return errors.New("download redirect left approved origin")
		}
		return nil
	}
	resp, e := h.Do(req)
	if e != nil {
		return nil, errors.New("GitHub request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub returned status %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if e != nil || int64(len(b)) > max {
		return nil, errors.New("GitHub response exceeds bound")
	}
	return b, nil
}
func (c client) release(ctx context.Context, tag string) (releaseResponse, error) {
	b, e := c.do(ctx, "GET", "/repos/bartdeboer/cli-releases/releases/tags/"+url.PathEscape(tag), "application/vnd.github+json", 1<<20)
	if e != nil {
		return releaseResponse{}, e
	}
	var r releaseResponse
	if e = json.Unmarshal(b, &r); e != nil {
		return r, errors.New("invalid GitHub release metadata")
	}
	return r, nil
}
func (c client) asset(ctx context.Context, id int64, max int64) ([]byte, error) {
	return c.do(ctx, "GET", "/repos/bartdeboer/cli-releases/releases/assets/"+strconv.FormatInt(id, 10), "application/octet-stream", max)
}
func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func findAsset(r releaseResponse, name string) (releaseAsset, error) {
	var got releaseAsset
	found := 0
	for _, a := range r.Assets {
		if a.Name == name {
			got = a
			found++
		}
	}
	if found != 1 || got.ID < 1 {
		return got, fmt.Errorf("release does not contain exact asset %q", name)
	}
	return got, nil
}
