// Package lyricsovh implements source.Source against the public
// lyrics.ovh API. See docs/source-dev/internal/providers/lyricsovh.md.
package lyricsovh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/httpx"
	"github.com/PloyBox/get-lyrics/source"
)

// defaultEndpoint is the public lyrics.ovh base path; artist and title are
// appended (URL-escaped) to form the request URL.
const defaultEndpoint = "https://api.lyrics.ovh/v1/"

// Adapter implements source.Source against api.lyrics.ovh.
type Adapter struct {
	// HTTPClient is used for every request when set. nil → a new client
	// with no timeout; the request context bounds each call.
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

	reqURL := defaultEndpoint + url.PathEscape(strings.TrimSpace(req.Author)) +
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

	body, err := httpx.ReadBody(resp.Body)
	if err != nil {
		return source.Result{}, fmt.Errorf("lyricsovh: read body: %w", err)
	}

	// 404 is the API's "no match" signal.
	if resp.StatusCode == http.StatusNotFound {
		return source.Result{}, fmt.Errorf("lyricsovh: no lyrics found for %q by %q: %w", req.Song, req.Author, source.ErrNotFound)
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
		return source.Result{}, fmt.Errorf("lyricsovh: no lyrics found for %q by %q: %w", req.Song, req.Author, source.ErrNotFound)
	}

	return source.Result{
		Lyrics: out.Lyrics,
		Level:  source.SyncNone,
		Filled: source.FieldLyrics,
	}, nil
}

func (a *Adapter) client() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{}
}

// truncate keeps an upstream error body bounded in CLI messages.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
