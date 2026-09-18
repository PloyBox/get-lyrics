// Package lyricsovh implements source.Source against the public
// lyrics.ovh API. See docs/refs/providers/lyricsovh.md.
package lyricsovh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PloyBox/get-lyrics/source"
)

// requestTimeout caps each upstream call.
const requestTimeout = 10 * time.Second

// defaultEndpoint is the public lyrics.ovh base path; artist and title are
// appended (URL-escaped) to form the request URL.
const defaultEndpoint = "https://api.lyrics.ovh/v1/"

// Adapter implements source.Source against api.lyrics.ovh.
type Adapter struct {
	// Endpoint overrides the base URL; tests point it at an httptest server.
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "lyricsovh" }

// Capabilities requires --author: the API has no title-only search, so a
// fetch without an artist cannot form a valid request.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Filters: source.ParamAuthor, Required: source.ParamAuthor}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if strings.TrimSpace(req.Song) == "" {
		return source.Result{}, errors.New("lyricsovh: song title is required")
	}

	reqURL := a.endpoint() + url.PathEscape(strings.TrimSpace(req.Author)) +
		"/" + url.PathEscape(strings.TrimSpace(req.Song))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return source.Result{}, fmt.Errorf("lyricsovh: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", req.UserAgent)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.client().Do(httpReq)
	if err != nil {
		return source.Result{}, fmt.Errorf("lyricsovh: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return source.Result{}, fmt.Errorf("lyricsovh: read body: %w", err)
	}

	// 404 is the API's "no match" signal.
	if resp.StatusCode == http.StatusNotFound {
		return source.Result{}, fmt.Errorf("lyricsovh: no lyrics found for %q by %q", req.Song, req.Author)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return source.Result{}, fmt.Errorf("lyricsovh: HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}

	var out struct {
		Lyrics string `json:"lyrics"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return source.Result{}, fmt.Errorf("lyricsovh: decode response: %w", err)
	}
	// The API sometimes pairs a 200 with an error field; honor it.
	if strings.TrimSpace(out.Error) != "" {
		return source.Result{}, fmt.Errorf("lyricsovh: %s", out.Error)
	}
	if strings.TrimSpace(out.Lyrics) == "" {
		return source.Result{}, fmt.Errorf("lyricsovh: no lyrics found for %q by %q", req.Song, req.Author)
	}

	return source.Result{
		Lyrics: out.Lyrics,
		Level:  source.SyncNone,
		Filled: source.FieldLyrics,
	}, nil
}

func (a *Adapter) endpoint() string {
	if a.Endpoint != "" {
		return strings.TrimRight(a.Endpoint, "/") + "/"
	}
	return defaultEndpoint
}

func (a *Adapter) client() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{Timeout: requestTimeout}
}

// truncate keeps an upstream error body bounded in CLI messages.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
