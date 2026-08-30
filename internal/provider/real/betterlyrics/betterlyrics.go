// Package betterlyrics is a real Source implementation backed by the
// Better Lyrics API (https://lyrics-api.boidu.dev). It self-registers
// nothing; registration happens explicitly in internal/bootstrap.RegisterAll.
//
// betterlyrics is an aggregate adapter: it fronts two distinct upstream
// endpoints and reports which one served the result via Result.SubSource
// (along with FieldSubSource). The endpoint is selected by the requested
// sync level:
//
//	GET /ttml/getLyrics?s=<song>&a=<author>...   word-level request (SyncWord)
//	                                            → TTML document, preserved verbatim
//	                                            SubSource: "ttml"
//	GET /kugou/getLyrics?s=<song>&a=<author>...  line/plain request (SyncLine/SyncNone)
//	                                            → LRC-style "lyrics" text; a line request
//	                                            keeps timestamped lines, otherwise the
//	                                            text is stripped to plain
//	                                            SubSource: "kugou"
//
// A word-level request returns the raw TTML string unchanged (whitespace
// between spans is significant) with Level SyncWord; line/plain requests
// produce exactly one track — SyncLine when the response actually carries
// timestamped lines, SyncNone otherwise. Album and duration refine the
// match; --author is required (the API has no title-only query).
package betterlyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PloyBox/get-lyrics/source"
)

// requestTimeout caps each upstream call so a stalled request does not
// stall the CLI.
const requestTimeout = 10 * time.Second

// defaultEndpoint is the public Better Lyrics API base path; the
// provider-specific path is appended to form the full request URL.
const defaultEndpoint = "https://lyrics-api.boidu.dev"

// ttmlPath serves syllable-level (word) lyrics.
const ttmlPath = "/ttml/getLyrics"

// kugouPath serves line-level LRC lyrics.
const kugouPath = "/kugou/getLyrics"

// subSourceTTML / subSourceKugou are the Result.SubSource identifiers
// reported alongside FieldSubSource so consumers can tell which
// endpoint served the result. They mirror the URL path suffix
// (ttml/kugou) to keep the provenance self-describing.
const (
	subSourceTTML  = "ttml"
	subSourceKugou = "kugou"
)

// timestampTag matches one LRC time tag, e.g. [00:19.239] or [1:00.1].
var timestampTag = regexp.MustCompile(`\[\d{1,2}:\d{1,2}(\.\d{1,3})?\]`)

// metaTag matches any remaining bracketed tag after timestamp
// stripping: section markers ([Verse], [Chorus]) and markers such as
// [!text] that distinguish unsynced lyrics.
var metaTag = regexp.MustCompile(`\[[^\[\]]*\]`)

// Adapter implements source.Source against the Better Lyrics API.
type Adapter struct {
	// Endpoint overrides the base URL. Tests point it at an httptest
	// server; production leaves it as the zero value (the public
	// Better Lyrics endpoint).
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

// New returns a fresh Adapter pointed at the public Better Lyrics endpoint.
func New() *Adapter { return &Adapter{} }

// Name returns the stable CLI identifier.
func (a *Adapter) Name() string { return "betterlyrics" }

// Capabilities requires --author (the API has no title-only query) and
// honors album + duration as matching hints.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{
		Filters:  source.ParamAuthor | source.ParamAlbum | source.ParamDuration,
		Required: source.ParamAuthor,
	}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch queries the provider endpoint matching the requested sync level
// and returns the single lyrics track the response carries.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if strings.TrimSpace(req.Song) == "" {
		return source.Result{}, errors.New("betterlyrics: song title is required")
	}

	path := kugouPath
	sub := subSourceKugou
	if req.SyncLevel == source.SyncWord {
		path = ttmlPath
		sub = subSourceTTML
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, a.endpoint()+path+"?"+buildQuery(req), nil)
	if err != nil {
		return source.Result{}, fmt.Errorf("betterlyrics: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", req.UserAgent)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.client().Do(httpReq)
	if err != nil {
		return source.Result{}, fmt.Errorf("betterlyrics: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return source.Result{}, fmt.Errorf("betterlyrics: read body: %w", err)
	}

	// 404 is the API's "no match" signal; surface it as a not-found
	// error rather than a generic HTTP-status failure.
	if resp.StatusCode == http.StatusNotFound {
		return source.Result{}, fmt.Errorf("betterlyrics: no lyrics found for %q by %q", req.Song, req.Author)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return source.Result{}, fmt.Errorf("betterlyrics: HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}

	if req.SyncLevel == source.SyncWord {
		// The provider-specific endpoint reports the TTML document
		// under different keys depending on deployment: the documented
		// shape uses "ttml", the live API serves it under "lyrics".
		// Accept either, preferring the documented one.
		var out struct {
			TTML   string `json:"ttml"`
			Lyrics string `json:"lyrics"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return source.Result{}, fmt.Errorf("betterlyrics: decode response: %w", err)
		}
		doc := out.TTML
		if strings.TrimSpace(doc) == "" {
			doc = out.Lyrics
		}
		if strings.TrimSpace(doc) == "" {
			return source.Result{}, fmt.Errorf("betterlyrics: no usable lyrics for %q", req.Song)
		}
		// The TTML document is whitespace-sensitive: span-separating
		// spaces live in bare text nodes, so the raw string is kept
		// exactly as the server returned it.
		return source.Result{
			Lyrics:    doc,
			Level:     source.SyncWord,
			SubSource: sub,
			Filled:    source.FieldLyrics | source.FieldSubSource,
		}, nil
	}

	var out struct {
		Lyrics string `json:"lyrics"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return source.Result{}, fmt.Errorf("betterlyrics: decode response: %w", err)
	}
	if strings.TrimSpace(out.Lyrics) == "" {
		return source.Result{}, fmt.Errorf("betterlyrics: no usable lyrics for %q", req.Song)
	}
	res := source.Result{}
	if req.SyncLevel == source.SyncLine && hasTimestampLines(out.Lyrics) {
		res.Lyrics = out.Lyrics
		res.Level = source.SyncLine
	} else {
		res.Lyrics = stripLRC(out.Lyrics)
		res.Level = source.SyncNone
	}
	res.SubSource = sub
	res.Filled = source.FieldLyrics | source.FieldSubSource
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

// buildQuery encodes the request fields as Better Lyrics query
// parameters: s (song) and a (author) are always sent; al (album) and
// d (duration in seconds) refine the match when provided.
func buildQuery(req source.Request) string {
	q := url.Values{}
	q.Set("s", strings.TrimSpace(req.Song))
	q.Set("a", strings.TrimSpace(req.Author))
	if al := strings.TrimSpace(req.Album); al != "" {
		q.Set("al", al)
	}
	if req.Duration > 0 {
		q.Set("d", strconv.Itoa(req.Duration))
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
// tag. Unsynced entries contain none, so a line request on them falls
// back to plain lyrics.
func hasTimestampLines(s string) bool {
	return timestampTag.MatchString(s)
}

// truncate keeps an upstream error body bounded when emitted in a CLI
// message.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
