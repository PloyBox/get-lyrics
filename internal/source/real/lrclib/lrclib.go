// Package lrclib is a real Source implementation backed by the public
// lrclib.net API. It self-registers nothing; registration happens
// explicitly in internal/bootstrap.RegisterAll.
//
// The adapter picks the endpoint based on which Request fields are
// non-empty:
//
//	GET /api/search?q=<song>            when only Song is set
//	GET /api/get?track_name=...&artist_name=...
//	                                    when Song + Author are set
//
// Both endpoints return a single track (or first hit) with plainLyrics
// and syncedLyrics (LRC) fields. Synced output is produced when
// Request.SyncLevel is SyncLine.
package lrclib

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

	"github.com/PloyBox/get-lyrics/internal/source"
)

// requestTimeout caps each upstream call. The value is short on
// purpose — a stalled request should not stall the CLI.
const requestTimeout = 10 * time.Second

// Adapter implements source.Source against lrclib.net.
type Adapter struct {
	// Endpoint is the search URL. Tests override it to point at an
	// httptest server; production leaves it as the zero value (the
	// public lrclib endpoint).
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

// New returns a fresh Adapter pointed at the public lrclib endpoint.
func New() *Adapter { return &Adapter{} }

// Name returns the stable CLI identifier.
func (a *Adapter) Name() string { return "lrclib" }

// Capabilities reports which filters are honored for req. The album
// filter only takes effect on the /api/get path (author present); a
// search-only request drops it, so the fetch layer can warn the user
// that --album is being ignored.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	c := source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum}
	if strings.TrimSpace(req.Author) == "" {
		c.Filters &^= source.ParamAlbum
	}
	return c
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch calls lrclib /api/search, picks the best candidate, and
// populates plain/synced lyrics accordingly.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if strings.TrimSpace(req.Song) == "" {
		return source.Result{}, errors.New("lrclib: song title is required")
	}

	endpoint := a.endpoint(req)
	client := a.client()

	query := buildQuery(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query, nil)
	if err != nil {
		return source.Result{}, fmt.Errorf("lrclib: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", req.UserAgent)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return source.Result{}, fmt.Errorf("lrclib: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return source.Result{}, fmt.Errorf("lrclib: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return source.Result{}, fmt.Errorf("lrclib: HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}

	// The response shape is dictated by the endpoint, not by sniffing
	// the body: /api/get returns a single object, /api/search returns an
	// array. Branch on the same condition endpoint()/buildQuery() use so
	// a server that pretty-prints (leading whitespace/newline) cannot
	// confuse the two.
	usedGetEndpoint := strings.TrimSpace(req.Author) != ""
	var hits []lrclibHit
	if usedGetEndpoint {
		var single lrclibHit
		if err := json.Unmarshal(body, &single); err != nil {
			return source.Result{}, fmt.Errorf("lrclib: decode response: %w", err)
		}
		if single.PlainLyrics == "" && single.SyncedLyrics == "" {
			return source.Result{}, fmt.Errorf("lrclib: no lyrics found for %q", req.Song)
		}
		hits = []lrclibHit{single}
	} else if err := json.Unmarshal(body, &hits); err != nil {
		return source.Result{}, fmt.Errorf("lrclib: decode response: %w", err)
	}
	if len(hits) == 0 {
		return source.Result{}, fmt.Errorf("lrclib: no lyrics found for %q", req.Song)
	}

	// Prefer the first hit with non-empty plainLyrics. lrclib commonly
	// returns instrumental/synced-only entries at the front; fall back
	// to index 0 if nothing fills the plain track.
	best := -1
	for i := range hits {
		if strings.TrimSpace(hits[i].PlainLyrics) != "" {
			best = i
			break
		}
	}
	if best == -1 {
		best = 0
	}
	hit := hits[best]

	res := source.Result{
		Title:  firstNonEmpty(hit.TrackName, req.Song),
		Artist: firstNonEmpty(hit.ArtistName, req.Author),
		Album:  hit.AlbumName,
		Lyrics: hit.PlainLyrics,
		Filled: source.FieldTitle, // Title always falls back to req.Song
	}
	if strings.TrimSpace(res.Artist) != "" {
		res.Filled |= source.FieldArtist
	}
	if strings.TrimSpace(res.Album) != "" {
		res.Filled |= source.FieldAlbum
	}
	if strings.TrimSpace(res.Lyrics) != "" {
		res.Filled |= source.FieldLyrics
	}
	if req.SyncLevel == source.SyncLine && strings.TrimSpace(hit.SyncedLyrics) != "" {
		res.SyncedLyrics = hit.SyncedLyrics
		res.Filled |= source.FieldSyncedLyrics
	}
	if res.Filled&(source.FieldLyrics|source.FieldSyncedLyrics) == 0 {
		return source.Result{}, fmt.Errorf("lrclib: no usable lyrics for %q", req.Song)
	}
	return res, nil
}

func (a *Adapter) endpoint(req source.Request) string {
	if a.Endpoint != "" {
		return a.Endpoint
	}
	if strings.TrimSpace(req.Author) != "" {
		return "https://lrclib.net/api/get"
	}
	return "https://lrclib.net/api/search"
}

func (a *Adapter) client() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{Timeout: requestTimeout}
}

// buildQuery picks the query encoding that matches the chosen endpoint:
//   - /api/search uses freeform q=
//   - /api/get uses structured track_name + artist_name (+ album_name)
func buildQuery(req source.Request) string {
	q := url.Values{}
	if strings.TrimSpace(req.Author) == "" {
		q.Set("q", strings.TrimSpace(req.Song))
		return q.Encode()
	}
	q.Set("track_name", strings.TrimSpace(req.Song))
	q.Set("artist_name", strings.TrimSpace(req.Author))
	if a := strings.TrimSpace(req.Album); a != "" {
		q.Set("album_name", a)
	}
	return q.Encode()
}

// firstNonEmpty returns a if non-empty, otherwise b.
func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// truncate keeps an upstream error body bounded when emitted in a CLI
// message.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// lrclibHit mirrors the relevant fields of lrclib's /api/search JSON.
// We intentionally keep this struct private and minimal so a future
// schema change is a one-file diff.
type lrclibHit struct {
	TrackName    string `json:"trackName"`
	ArtistName   string `json:"artistName"`
	AlbumName    string `json:"albumName"`
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
}
