// Package lrccx is a real Source implementation backed by the legacy
// lrc.cx lyrics API (https://api.lrc.cx/jsonapi). It self-registers
// nothing; registration happens explicitly in internal/bootstrap.RegisterAll.
//
// The adapter issues a single GET against /jsonapi with freeform
// title/artist/album query parameters and picks the best-ranked hit:
//
//	GET https://api.lrc.cx/jsonapi?title=<song>&artist=<author>&album=<album>
//
// The response is a score-descending JSON array whose entries carry
// title, artist, and an "lrc" field (the LRC text). Note the field is
// named "lrc" (not "lyrics" as the legacy docs claim) and may be null
// for instrumental/missing entries.
//
// Because the API only ever returns LRC-flavoured text, synced output
// is honored: plain Lyrics are produced by stripping [mm:ss] timestamps
// and section tags ([Verse], [!text], ...), while SyncedLyrics carries
// the raw response text — but only when it actually contains timestamped
// lines. Unsynced [!text] entries therefore fall back to plain lyrics,
// matching the mock-nosync semantics at the CLI layer.
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

	"github.com/PloyBox/get-lyrics/internal/source"
)

// userAgent is sent on every request for downstream attribution.
const userAgent = "get-lyrics/0.1 (+https://github.com/PloyBox/get-lyrics)"

// requestTimeout caps each upstream call so a stalled request does not
// stall the CLI.
const requestTimeout = 10 * time.Second

// defaultEndpoint is the public lrc.cx base path; "/jsonapi" is
// appended to form the full request URL.
const defaultEndpoint = "https://api.lrc.cx"

// timestampTag matches one LRC time tag, e.g. [00:19.239] or [1:00.1].
var timestampTag = regexp.MustCompile(`\[\d{1,2}:\d{1,2}(\.\d{1,3})?\]`)

// metaTag matches any remaining bracketed tag after timestamp
// stripping: section markers ([Verse], [Chorus]) and markers such as
// [!text] that distinguish unsynced lyrics.
var metaTag = regexp.MustCompile(`\[[^\[\]]*\]`)

// Adapter implements source.Source against the lrc.cx /jsonapi endpoint.
type Adapter struct {
	// Endpoint overrides the base URL. Tests point it at an httptest
	// server; production leaves it as the zero value (the public lrc.cx
	// endpoint). Self-hosted LrcApi instances can set it too.
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

// New returns a fresh Adapter pointed at the public lrc.cx endpoint.
func New() *Adapter { return &Adapter{} }

// Name returns the stable CLI identifier.
func (a *Adapter) Name() string { return "lrccx" }

// Capabilities reports the filters this adapter uses: author and album
// both refine the /jsonapi lookup, independently of each other.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum}
}

// Fetch queries lrc.cx /jsonapi, picks the best-ranked hit with usable
// lyrics, and populates plain/synced tracks from the LRC text.
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
	httpReq.Header.Set("User-Agent", userAgent)
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

	// The response is already ranked by score descending; take the
	// first hit whose "lrc" field is present and non-empty. When every
	// hit lacks lyrics, fall back to the top entry so the caller sees a
	// deterministic "no usable lyrics" error rather than an index panic.
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
		Title:  firstNonEmpty(hit.Title, req.Song),
		Artist: firstNonEmpty(hit.Artist, req.Author),
		Lyrics: stripLRC(raw),
	}
	if req.Timestamp && hasTimestampLines(raw) {
		res.SyncedLyrics = raw
	}
	if res.Lyrics == "" && res.SyncedLyrics == "" {
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

// buildQuery encodes the non-empty optional Request fields as lrc.cx
// query parameters. The album value "[Unknown Album]" is treated as
// empty per the API docs. The "path" parameter is intentionally never
// sent — the CLI has no notion of a local music file.
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

// stripLRC removes timestamp tags and bracketed section/marker tags
// line by line, dropping lines that carry no lyrics text at all. The
// result is the plain, timestamp-free track.
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

// hasTimestampLines reports whether any line carries a [mm:ss] time
// tag. Unsynced entries (marked [!text]) contain none, so a --timestamp
// request on them falls back to plain lyrics.
func hasTimestampLines(s string) bool {
	return timestampTag.MatchString(s)
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

// lrccxHit mirrors the fields of lrc.cx's /jsonapi JSON that this
// adapter consumes. LRC is a pointer so a literal "lrc": null (which
// the API returns for instrumental entries) decodes to nil instead of
// an empty-string hit that would shadow real lyrics.
type lrccxHit struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Artist string  `json:"artist"`
	LRC    *string `json:"lrc"`
}
