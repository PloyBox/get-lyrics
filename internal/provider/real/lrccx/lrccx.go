// Package lrccx implements source.Source against the legacy lrc.cx lyrics
// API. See docs/refs/providers/lrccx.md.
package lrccx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PloyBox/get-lyrics/source"
)

// requestTimeout caps each upstream call.
const requestTimeout = 10 * time.Second

// defaultEndpoint is the public lrc.cx base path; "/jsonapi" is appended.
const defaultEndpoint = "https://api.lrc.cx"

// timestampTag matches one LRC time tag, e.g. [00:19.239] or [1:00.1].
var timestampTag = regexp.MustCompile(`\[\d{1,2}:\d{1,2}(\.\d{1,3})?\]`)

// metaTag matches any remaining bracketed tag: section markers ([Verse],
// [Chorus]) and markers such as [!text] that distinguish unsynced lyrics.
var metaTag = regexp.MustCompile(`\[[^\[\]]*\]`)

// Adapter implements source.Source against the lrc.cx /jsonapi endpoint.
type Adapter struct {
	// Endpoint overrides the base URL; tests point it at an httptest
	// server, and self-hosted LrcApi instances can set it too.
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "lrccx" }

// Capabilities honors author and album, independently of each other.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if strings.TrimSpace(req.Song) == "" {
		return source.Result{}, errors.New("lrccx: song title is required")
	}

	query := buildQuery(req)

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, a.endpoint()+"/jsonapi?"+query, nil)
	if err != nil {
		return source.Result{}, fmt.Errorf("lrccx: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", req.UserAgent)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.client().Do(httpReq)
	if err != nil {
		return source.Result{}, fmt.Errorf("lrccx: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return source.Result{}, fmt.Errorf("lrccx: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return source.Result{}, fmt.Errorf("lrccx: HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}

	var hits []lrccxHit
	if err := json.Unmarshal(body, &hits); err != nil {
		return source.Result{}, fmt.Errorf("lrccx: decode response: %w", err)
	}
	if len(hits) == 0 {
		return source.Result{}, fmt.Errorf("lrccx: no lyrics found for %q", req.Song)
	}

	// The response is ranked by score descending; take the first hit whose
	// "lrc" field is present and non-empty, else the top entry so the
	// caller sees a deterministic error rather than an index panic.
	best := -1
	for i := range hits {
		if hits[i].LRC != nil && strings.TrimSpace(*hits[i].LRC) != "" {
			best = i
			break
		}
	}
	if best == -1 {
		best = 0
	}
	hit := hits[best]

	raw := ""
	if hit.LRC != nil {
		raw = *hit.LRC
	}
	res := source.Result{
		Title:  hit.Title,
		Artist: hit.Artist,
	}
	if strings.TrimSpace(res.Title) != "" {
		res.Filled |= source.FieldTitle
	}
	if strings.TrimSpace(res.Artist) != "" {
		res.Filled |= source.FieldArtist
	}
	// A synced request returns the raw LRC text only when it actually
	// carries timestamped lines; otherwise — and for plain requests — the
	// text is stripped to plain lyrics. Exactly one track per Fetch.
	if req.SyncLevel == source.SyncLine && hasTimestampLines(raw) {
		res.Lyrics = raw
		res.Level = source.SyncLine
		res.Filled |= source.FieldLyrics
	} else if plain := stripLRC(raw); plain != "" {
		res.Lyrics = plain
		res.Level = source.SyncNone
		res.Filled |= source.FieldLyrics
	}
	if res.Filled&source.FieldLyrics == 0 {
		return source.Result{}, fmt.Errorf("lrccx: no usable lyrics for %q", req.Song)
	}
	return res, nil
}

func (a *Adapter) endpoint() string {
	if a.Endpoint != "" {
		return strings.TrimSuffix(a.Endpoint, "/")
	}
	return defaultEndpoint
}

func (a *Adapter) client() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{Timeout: requestTimeout}
}

// buildQuery encodes the non-empty optional fields as lrc.cx query
// parameters; the album value "[Unknown Album]" is treated as empty. The
// "path" parameter is intentionally never sent.
func buildQuery(req source.Request) string {
	q := url.Values{}
	q.Set("title", strings.TrimSpace(req.Song))
	if a := strings.TrimSpace(req.Author); a != "" {
		q.Set("artist", a)
	}
	if a := strings.TrimSpace(req.Album); a != "" && a != "[Unknown Album]" {
		q.Set("album", a)
	}
	return q.Encode()
}

// stripLRC removes timestamp and bracketed marker tags line by line,
// dropping lines that carry no lyrics text.
func stripLRC(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = timestampTag.ReplaceAllString(line, "")
		line = metaTag.ReplaceAllString(line, "")
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// hasTimestampLines reports whether any line carries a [mm:ss] time tag.
func hasTimestampLines(s string) bool {
	return timestampTag.MatchString(s)
}

// truncate keeps an upstream error body bounded in CLI messages.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// lrccxHit mirrors the fields of lrc.cx's /jsonapi JSON that this adapter
// consumes. LRC is a pointer so a literal "lrc": null decodes to nil
// instead of an empty-string hit.
type lrccxHit struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Artist string  `json:"artist"`
	LRC    *string `json:"lrc"`
}
