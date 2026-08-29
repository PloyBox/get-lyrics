// Package lyricsovh is a real Source implementation backed by the public
// lyrics.ovh API (api.lyrics.ovh). It self-registers nothing; registration
// happens explicitly in internal/bootstrap.RegisterAll.
//
// The API exposes a single endpoint keyed by artist and title:
//
//	GET https://api.lyrics.ovh/v1/{artist}/{title}
//
// which returns {"lyrics": "..."} on success, or 404 with
// {"error": "No lyrics found"} when no match exists. Because the path
// requires an artist, this adapter requires --author: a fetch without
// it cannot form a valid request, so the fetch layer enforces the
// requirement during precheck (exit code 6).
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

// requestTimeout caps each upstream call so a stalled request does not
// stall the CLI.
const requestTimeout = 10 * time.Second

// defaultEndpoint is the public lyrics.ovh base path; artist and title
// are appended (URL-escaped) to form the full request URL.
const defaultEndpoint = "https://api.lyrics.ovh/v1/"

// Adapter implements source.Source against api.lyrics.ovh.
type Adapter struct {
	// Endpoint overrides the base URL. Tests point it at an httptest
	// server; production leaves it as the zero value.
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

// New returns a fresh Adapter pointed at the public lyrics.ovh endpoint.
func New() *Adapter { return &Adapter{} }

// Name returns the stable CLI identifier.
func (a *Adapter) Name() string { return "lyricsovh" }

// Capabilities requires --author: the API has no title-only search,
// so a fetch without an artist cannot form a valid request. The fetch
// layer enforces this during precheck (exit code 6).
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Filters: source.ParamAuthor, Required: source.ParamAuthor}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch looks up lyrics via api.lyrics.ovh/v1/{artist}/{title}. The
// author is guaranteed non-empty by the precheck; the adapter no longer
// validates it itself.
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

	// 404 is the API's "no match" signal; surface it as a not-found
	// error rather than a generic HTTP-status failure.
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

	res := source.Result{
		Lyrics: out.Lyrics,
		Title:  req.Song,
		Artist: req.Author,
		Filled: source.FieldLyrics | source.FieldTitle,
	}
	if strings.TrimSpace(res.Artist) != "" {
		res.Filled |= source.FieldArtist
	}
	return res, nil
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

// truncate keeps an upstream error body bounded when emitted in a CLI
// message.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
