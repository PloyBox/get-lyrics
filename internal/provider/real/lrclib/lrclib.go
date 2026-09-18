// Package lrclib implements source.Source against the public lrclib.net API.
// See docs/refs/providers/lrclib.md.
package lrclib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PloyBox/get-lyrics/source"
)

// requestTimeout caps each upstream call.
const requestTimeout = 10 * time.Second

// Adapter implements source.Source against lrclib.net.
type Adapter struct {
	// Endpoint overrides the API base URL; tests point it at an httptest
	// server.
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "lrclib" }

// Capabilities honors author, album and duration — album and duration
// only on the /api/get path (author present).
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	c := source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum | source.ParamDuration}
	if strings.TrimSpace(req.Author) == "" {
		c.Filters &^= source.ParamAlbum | source.ParamDuration
	}
	return c
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

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

	// The response shape is dictated by the endpoint, not by sniffing the
	// body: /api/get returns an object, /api/search an array.
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

	// Prefer the first hit with non-empty plainLyrics; fall back to the
	// first entry when nothing fills the plain track.
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
		Title:  hit.TrackName,
		Artist: hit.ArtistName,
		Album:  hit.AlbumName,
	}
	if strings.TrimSpace(res.Title) != "" {
		res.Filled |= source.FieldTitle
	}
	if strings.TrimSpace(res.Artist) != "" {
		res.Filled |= source.FieldArtist
	}
	if strings.TrimSpace(res.Album) != "" {
		res.Filled |= source.FieldAlbum
	}
	// A synced request takes the LRC track when available; otherwise the
	// plain track. Exactly one lyrics track per Fetch.
	if req.SyncLevel == source.SyncLine && strings.TrimSpace(hit.SyncedLyrics) != "" {
		res.Lyrics = hit.SyncedLyrics
		res.Level = source.SyncLine
		res.Filled |= source.FieldLyrics
	} else if strings.TrimSpace(hit.PlainLyrics) != "" {
		res.Lyrics = hit.PlainLyrics
		res.Level = source.SyncNone
		res.Filled |= source.FieldLyrics
	}
	if res.Filled&source.FieldLyrics == 0 {
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

// buildQuery picks the query encoding matching the chosen endpoint:
// /api/search uses freeform q=, /api/get uses structured track_name +
// artist_name (+ album_name, duration).
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
	if req.Duration > 0 {
		q.Set("duration", strconv.Itoa(req.Duration))
	}
	return q.Encode()
}

// truncate keeps an upstream error body bounded in CLI messages.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// lrclibHit mirrors the fields of lrclib's JSON that this adapter consumes.
type lrclibHit struct {
	TrackName    string `json:"trackName"`
	ArtistName   string `json:"artistName"`
	AlbumName    string `json:"albumName"`
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
}
