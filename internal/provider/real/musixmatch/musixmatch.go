// Package musixmatch is a real Source implementation backed by the
// Musixmatch API (https://api.musixmatch.com). It self-registers
// nothing; registration happens explicitly in internal/bootstrap.RegisterAll.
//
// Every request carries the API key as the apikey query parameter, taken
// from the required custom --env key MUSIXMATCH_API_KEY. The lookup path
// depends on which Request fields are non-empty:
//
//	matcher.lyrics.get / matcher.subtitle.get   when Song + Author are set
//	track.search → track.lyrics.get / track.subtitle.get
//	                                            when only Song is set
//
// The matcher endpoints are fuzzy lookups keyed by title + artist; with
// no artist the API cannot match, so a title-only request searches for
// the track and fetches lyrics/subtitles by commontrack id. Subtitle
// endpoints require the paid Scale plan; on cheaper plans they return
// 402/403, which the adapter treats as "no synced lyrics" and falls back
// to the plain track (the fetch layer reports the downgrade).
package musixmatch

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

// requestTimeout caps each upstream call so a stalled request does not
// stall the CLI.
const requestTimeout = 10 * time.Second

// defaultEndpoint is the Musixmatch REST base path; the method name
// (e.g. "matcher.lyrics.get") is appended to form the full request URL.
const defaultEndpoint = "https://api.musixmatch.com/ws/1.1"

// apiKeyParam is the custom --env key carrying the API key. Musixmatch
// requires it on every request.
const apiKeyParam = "MUSIXMATCH_API_KEY"

// errNotFound marks an API-level 404 or an empty result body; callers
// distinguish it from transport/hard failures so a missing synced track
// can fall back to plain lyrics.
var errNotFound = errors.New("musixmatch: not found")

// Adapter implements source.Source against api.musixmatch.com.
type Adapter struct {
	// Endpoint overrides the REST base path. Tests point it at an
	// httptest server; production leaves it as the zero value (the
	// public Musixmatch endpoint).
	Endpoint string

	// HTTPClient is reused across calls. nil → http.DefaultClient.
	HTTPClient *http.Client
}

// New returns a fresh Adapter pointed at the public Musixmatch endpoint.
func New() *Adapter { return &Adapter{} }

// Name returns the stable CLI identifier.
func (a *Adapter) Name() string { return "musixmatch" }

// Capabilities reports the filters this adapter uses: author refines the
// lookup. Album is not supported — neither the matcher nor the search
// endpoints take an album parameter — and ISWC is not supported either:
// Musixmatch identifies recordings by ISRC (track_isrc), a different
// standard than the CLI's ISWC.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{
		Filters:        source.ParamAuthor,
		Custom:         []source.ParamSpec{{Name: apiKeyParam, Description: "Musixmatch API key (https://developer.musixmatch.com)"}},
		RequiredCustom: []string{apiKeyParam},
	}
}

func (a *Adapter) CustomParams() []source.ParamSpec {
	return []source.ParamSpec{{Name: apiKeyParam, Description: "Musixmatch API key (https://developer.musixmatch.com)"}}
}

// Fetch looks up lyrics by title + artist (matcher endpoints) or, when
// no artist is given, by searching for the track and fetching by
// commontrack id. A synced request tries the subtitle endpoint first and
// silently degrades to the plain track when subtitles are unavailable
// (not found, or 402/403 from a plan without subtitle access).
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if strings.TrimSpace(req.Song) == "" {
		return source.Result{}, errors.New("musixmatch: song title is required")
	}
	apiKey := strings.TrimSpace(req.Custom[apiKeyParam])
	if apiKey == "" {
		return source.Result{}, source.RequiredParamMismatchError{
			Source:    a.Name(),
			ParamName: apiKeyParam,
		}
	}

	res := source.Result{Title: req.Song, Filled: source.FieldTitle}
	ua := req.UserAgent

	if strings.TrimSpace(req.Author) != "" {
		res.Artist = req.Author
		res.Filled |= source.FieldArtist

		if req.SyncLevel == source.SyncLine {
			if sub, err := a.fetchMatcherSubtitle(ctx, apiKey, ua, req); err == nil {
				res.SyncedLyrics = sub
				res.Filled |= source.FieldSyncedLyrics
			}
		}
		lyrics, err := a.fetchMatcherLyrics(ctx, apiKey, ua, req)
		if err != nil && !errors.Is(err, errNotFound) {
			return source.Result{}, err
		}
		if lyrics != "" {
			res.Lyrics = lyrics
			res.Filled |= source.FieldLyrics
		}
	} else {
		track, err := a.searchTrack(ctx, apiKey, ua, req.Song)
		if err != nil && !errors.Is(err, errNotFound) {
			return source.Result{}, err
		}
		res.Title = firstNonEmpty(track.TrackName, req.Song)
		res.Artist = track.ArtistName
		res.Album = track.AlbumName
		res.Filled = source.FieldTitle
		if strings.TrimSpace(res.Artist) != "" {
			res.Filled |= source.FieldArtist
		}
		if strings.TrimSpace(res.Album) != "" {
			res.Filled |= source.FieldAlbum
		}

		if req.SyncLevel == source.SyncLine && track.CommontrackID != 0 {
			if sub, err := a.fetchTrackSubtitle(ctx, apiKey, ua, track.CommontrackID); err == nil {
				res.SyncedLyrics = sub
				res.Filled |= source.FieldSyncedLyrics
			}
		}
		if track.CommontrackID != 0 {
			lyrics, err := a.fetchTrackLyrics(ctx, apiKey, ua, track.CommontrackID)
			if err != nil && !errors.Is(err, errNotFound) {
				return source.Result{}, err
			}
			if lyrics != "" {
				res.Lyrics = lyrics
				res.Filled |= source.FieldLyrics
			}
		}
	}

	if res.Filled&(source.FieldLyrics|source.FieldSyncedLyrics) == 0 {
		return source.Result{}, fmt.Errorf("musixmatch: no usable lyrics for %q", req.Song)
	}
	return res, nil
}

// fetchMatcherLyrics returns the plain track via matcher.lyrics.get
// (fuzzy match by title + artist).
func (a *Adapter) fetchMatcherLyrics(ctx context.Context, apiKey, ua string, req source.Request) (string, error) {
	q := url.Values{}
	q.Set("q_track", strings.TrimSpace(req.Song))
	q.Set("q_artist", strings.TrimSpace(req.Author))
	var out lyricsResponse
	if err := a.do(ctx, apiKey, ua, "matcher.lyrics.get", q, &out); err != nil {
		return "", err
	}
	return cleanLyrics(out.Lyrics.LyricsBody), nil
}

// fetchMatcherSubtitle returns the LRC track via matcher.subtitle.get.
func (a *Adapter) fetchMatcherSubtitle(ctx context.Context, apiKey, ua string, req source.Request) (string, error) {
	q := url.Values{}
	q.Set("q_track", strings.TrimSpace(req.Song))
	q.Set("q_artist", strings.TrimSpace(req.Author))
	var out subtitleResponse
	if err := a.do(ctx, apiKey, ua, "matcher.subtitle.get", q, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Subtitle.SubtitleBody), nil
}

// searchTrack finds the first search hit that carries lyrics, ranked by
// track rating so the best-known match comes first.
func (a *Adapter) searchTrack(ctx context.Context, apiKey, ua, song string) (musixmatchTrack, error) {
	q := url.Values{}
	q.Set("q_track", strings.TrimSpace(song))
	q.Set("f_has_lyrics", "1")
	q.Set("page_size", "5")
	q.Set("s_track_rating", "desc")
	var out searchResponse
	if err := a.do(ctx, apiKey, ua, "track.search", q, &out); err != nil {
		return musixmatchTrack{}, err
	}
	for _, item := range out.TrackList {
		if item.Track.HasLyrics == 1 && item.Track.CommontrackID != 0 {
			return item.Track, nil
		}
	}
	return musixmatchTrack{}, errNotFound
}

// fetchTrackLyrics returns the plain track by commontrack id.
func (a *Adapter) fetchTrackLyrics(ctx context.Context, apiKey, ua string, trackID int64) (string, error) {
	q := url.Values{}
	q.Set("commontrack_id", strconv.FormatInt(trackID, 10))
	var out lyricsResponse
	if err := a.do(ctx, apiKey, ua, "track.lyrics.get", q, &out); err != nil {
		return "", err
	}
	return cleanLyrics(out.Lyrics.LyricsBody), nil
}

// fetchTrackSubtitle returns the LRC track by commontrack id.
func (a *Adapter) fetchTrackSubtitle(ctx context.Context, apiKey, ua string, trackID int64) (string, error) {
	q := url.Values{}
	q.Set("commontrack_id", strconv.FormatInt(trackID, 10))
	var out subtitleResponse
	if err := a.do(ctx, apiKey, ua, "track.subtitle.get", q, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Subtitle.SubtitleBody), nil
}

// do issues one GET against method, injecting the API key, and decodes
// the JSON envelope. The effective status is message.header.status_code
// when present — Musixmatch returns HTTP 200 with an error code in the
// body for many failures — falling back to the HTTP status.
func (a *Adapter) do(ctx context.Context, apiKey, ua, method string, q url.Values, out any) error {
	q.Set("apikey", apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint()+"/"+method+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("musixmatch: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", ua)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.client().Do(httpReq)
	if err != nil {
		return fmt.Errorf("musixmatch: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("musixmatch: read body: %w", err)
	}
	return decodeResponse(body, resp.StatusCode, out)
}

// decodeResponse maps a Musixmatch response to an error or decodes the
// body into out. An empty/[]/{} body on success is the API's no-match
// signal (errNotFound); 401/402/403 get explicit, actionable messages.
func decodeResponse(body []byte, httpStatus int, out any) error {
	if strings.TrimSpace(string(body)) == "[]" {
		return errNotFound
	}
	var api apiResponse
	if err := json.Unmarshal(body, &api); err != nil {
		return fmt.Errorf("musixmatch: decode response: %w", err)
	}
	code := api.Message.Header.StatusCode
	if code == 0 {
		code = httpStatus
	}
	switch {
	case code == http.StatusOK:
		raw := strings.TrimSpace(string(api.Message.Body))
		if raw == "" || raw == "[]" || raw == "{}" || raw == "null" {
			return errNotFound
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(api.Message.Body, out); err != nil {
			return fmt.Errorf("musixmatch: decode response body: %w", err)
		}
		return nil
	case code == http.StatusUnauthorized:
		return fmt.Errorf("musixmatch: invalid or missing API key (HTTP 401)")
	case code == http.StatusPaymentRequired:
		return fmt.Errorf("musixmatch: rate limit or insufficient plan balance (HTTP 402)")
	case code == http.StatusForbidden:
		return fmt.Errorf("musixmatch: not authorized for this endpoint (HTTP 403)")
	case code == http.StatusNotFound:
		return errNotFound
	default:
		if hint := strings.TrimSpace(api.Message.Header.Hint); hint != "" {
			return fmt.Errorf("musixmatch: API status %d: %s", code, hint)
		}
		return fmt.Errorf("musixmatch: API status %d: %s", code, truncate(body, 200))
	}
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

// cleanLyrics trims Musixmatch's appended "*******" usage notice (API
// boilerplate, not lyrics) and treats the instrumental placeholder
// "...." as empty.
func cleanLyrics(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n*******"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "...." {
		return ""
	}
	return s
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

// apiResponse is the shared JSON envelope: every endpoint wraps its
// payload in message.body and reports the outcome in message.header.
type apiResponse struct {
	Message struct {
		Header struct {
			StatusCode int    `json:"status_code"`
			Hint       string `json:"hint"`
		} `json:"header"`
		Body json.RawMessage `json:"body"`
	} `json:"message"`
}

// lyricsResponse mirrors the body of matcher.lyrics.get / track.lyrics.get.
type lyricsResponse struct {
	Lyrics struct {
		LyricsBody string `json:"lyrics_body"`
	} `json:"lyrics"`
}

// subtitleResponse mirrors the body of matcher.subtitle.get / track.subtitle.get.
type subtitleResponse struct {
	Subtitle struct {
		SubtitleBody string `json:"subtitle_body"`
	} `json:"subtitle"`
}

// searchResponse mirrors the body of track.search.
type searchResponse struct {
	TrackList []struct {
		Track musixmatchTrack `json:"track"`
	} `json:"track_list"`
}

// musixmatchTrack mirrors the fields of a track.search hit that this
// adapter consumes.
type musixmatchTrack struct {
	TrackName     string `json:"track_name"`
	ArtistName    string `json:"artist_name"`
	AlbumName     string `json:"album_name"`
	CommontrackID int64  `json:"commontrack_id"`
	HasLyrics     int    `json:"has_lyrics"`
}
