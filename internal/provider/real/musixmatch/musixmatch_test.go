// Black-box tests for the musixmatch adapter: request construction and
// envelope parsing through testutil.Transport, with no network access.
package musixmatch_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/provider/real/musixmatch"
	"github.com/PloyBox/get-lyrics/internal/testutil"
	"github.com/PloyBox/get-lyrics/source"
)

const (
	keyName = "MUSIXMATCH_API_KEY"
	testKey = "test-api-key"
	testUA  = "get-lyrics/test (+https://example.invalid)"
)

// stubResponse is one canned HTTP response; status 0 means 200.
type stubResponse struct {
	status int
	body   string
}

func newTransport(t *testing.T, routes map[string]stubResponse) *testutil.Transport {
	t.Helper()
	return &testutil.Transport{Respond: func(req *http.Request) (*http.Response, error) {
		r, ok := routes[path.Base(req.URL.Path)]
		if !ok {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		status := r.status
		if status == 0 {
			status = http.StatusOK
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(r.body)),
			Request:    req,
		}, nil
	}}
}

func newAdapter(tp *testutil.Transport) *musixmatch.Adapter {
	a := musixmatch.New()
	a.HTTPClient = &http.Client{Transport: tp}
	return a
}

func baseRequest() source.Request {
	return source.Request{
		Song: "Song", UserAgent: testUA,
		Custom: map[string]string{keyName: testKey},
	}
}

func authorRequest() source.Request {
	r := baseRequest()
	r.Author = "Artist"
	return r
}

func envelope(status int, body string) string {
	return `{"message":{"header":{"status_code":` + strconv.Itoa(status) +
		`},"body":` + body + `}}`
}

func envelopeHint(status int, hint, body string) string {
	return `{"message":{"header":{"status_code":` + strconv.Itoa(status) +
		`,"hint":` + strconv.Quote(hint) + `},"body":` + body + `}}`
}

func lyricsBody(lyrics string) string {
	return `{"lyrics":{"lyrics_body":` + strconv.Quote(lyrics) + `}}`
}

func lyricsEnvelope(status int, lyrics string) string {
	return envelope(status, lyricsBody(lyrics))
}

func subtitleEnvelope(status int, body string) string {
	return envelope(status, `{"subtitle":{"subtitle_body":`+strconv.Quote(body)+`}}`)
}

// trackBody is a track.get hit with metadata and a commontrack id.
const trackBody = `{"track":{"track_name":"Song","artist_name":"Artist",` +
	`"album_name":"Album","commontrack_id":42,"has_lyrics":1}}`

// searchBody lists a lyric-less hit first, so the adapter must skip it.
const searchBody = `{"track_list":[` +
	`{"track":{"track_name":"No Lyrics","commontrack_id":7,"has_lyrics":0}},` +
	`{"track":{"track_name":"Song","artist_name":"Artist",` +
	`"album_name":"Album","commontrack_id":42,"has_lyrics":1}}]}`

// wantRequest asserts request i targeted method and carried the API key.
func wantRequest(t *testing.T, tp *testutil.Transport, i int, method string) url.Values {
	t.Helper()
	if len(tp.Requests) <= i {
		t.Fatalf("got %d requests, want at least %d", len(tp.Requests), i+1)
	}
	r := tp.Requests[i]
	if got, want := r.URL.Path, "/ws/1.1/"+method; got != want {
		t.Fatalf("request %d path = %q, want %q", i, got, want)
	}
	q := r.URL.Query()
	if got := q.Get("apikey"); got != testKey {
		t.Errorf("request %d apikey = %q, want %q", i, got, testKey)
	}
	return q
}

func wantQuery(t *testing.T, q url.Values, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func wantCount(t *testing.T, tp *testutil.Transport, n int) {
	t.Helper()
	if len(tp.Requests) != n {
		t.Fatalf("got %d requests, want %d", len(tp.Requests), n)
	}
}

func TestRequest(t *testing.T) {
	t.Run("line_author_subtitle", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"matcher.subtitle.get": {body: subtitleEnvelope(200, "[00:01.00]line")},
		})
		req := authorRequest()
		req.SyncLevel = source.SyncLine
		if _, err := newAdapter(tp).Fetch(context.Background(), req); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		q := wantRequest(t, tp, 0, "matcher.subtitle.get")
		wantQuery(t, q, map[string]string{"q_track": "Song", "q_artist": "Artist"})
		if got := tp.Requests[0].Header.Get("User-Agent"); got != testUA {
			t.Errorf("User-Agent = %q, want %q", got, testUA)
		}
		wantCount(t, tp, 1)
	})

	t.Run("none_author_lyrics", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"matcher.lyrics.get": {body: lyricsEnvelope(200, "plain")},
		})
		if _, err := newAdapter(tp).Fetch(context.Background(),
			authorRequest()); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		q := wantRequest(t, tp, 0, "matcher.lyrics.get")
		wantQuery(t, q, map[string]string{"q_track": "Song", "q_artist": "Artist"})
		wantCount(t, tp, 1)
	})

	t.Run("isrc_over_author", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"track.get":        {body: envelope(200, trackBody)},
			"track.lyrics.get": {body: lyricsEnvelope(200, "plain")},
		})
		req := authorRequest()
		req.ISRC = "USRC17607839"
		if _, err := newAdapter(tp).Fetch(context.Background(), req); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		q := wantRequest(t, tp, 0, "track.get")
		wantQuery(t, q, map[string]string{"track_isrc": "USRC17607839",
			"q_artist": ""})
		wantRequest(t, tp, 1, "track.lyrics.get")
		wantCount(t, tp, 2)
	})

	t.Run("title_search", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"track.search":     {body: envelope(200, searchBody)},
			"track.lyrics.get": {body: lyricsEnvelope(200, "plain")},
		})
		if _, err := newAdapter(tp).Fetch(context.Background(), baseRequest()); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		q := wantRequest(t, tp, 0, "track.search")
		wantQuery(t, q, map[string]string{"q_track": "Song", "f_has_lyrics": "1",
			"page_size": "5", "s_track_rating": "desc"})
		q = wantRequest(t, tp, 1, "track.lyrics.get")
		wantQuery(t, q, map[string]string{"commontrack_id": "42"})
		wantCount(t, tp, 2)
	})

	t.Run("capabilities", func(t *testing.T) {
		author := authorRequest()
		isrc := baseRequest()
		isrc.ISRC = "USRC17607839"
		for _, tc := range []struct {
			name string
			req  source.Request
			want source.Param
		}{
			{"author", author, source.ParamAuthor},
			{"isrc", isrc, source.ParamISRC},
		} {
			t.Run(tc.name, func(t *testing.T) {
				caps := testutil.Capabilities(t, musixmatch.New(), tc.req)
				if caps.Filters != tc.want {
					t.Errorf("Filters = %d, want %d", caps.Filters, tc.want)
				}
				declared := make(map[string]bool, len(caps.Custom))
				for _, spec := range caps.Custom {
					declared[spec.Name] = true
				}
				if !declared[keyName] {
					t.Errorf("Custom = %v, want key %q", declared, keyName)
				}
				if len(caps.RequiredCustom) != 1 ||
					caps.RequiredCustom[0] != keyName {
					t.Errorf("RequiredCustom = %v, want [%q]",
						caps.RequiredCustom, keyName)
				}
			})
		}
	})
}

func TestFetch(t *testing.T) {
	t.Run("subtitle_hit", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"matcher.subtitle.get": {body: subtitleEnvelope(200, "[00:01.00]line")},
		})
		req := authorRequest()
		req.SyncLevel = source.SyncLine
		res, err := newAdapter(tp).Fetch(context.Background(), req)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if res.Lyrics != "[00:01.00]line" || res.Level != source.SyncLine ||
			res.Filled != source.FieldLyrics {
			t.Errorf("result = %+v, want the subtitle body", res)
		}
	})

	t.Run("subtitle_404_falls_back", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"matcher.subtitle.get": {body: envelope(404, `{}`)},
			"matcher.lyrics.get":   {body: lyricsEnvelope(200, "plain lyric")},
		})
		req := authorRequest()
		req.SyncLevel = source.SyncLine
		res, err := newAdapter(tp).Fetch(context.Background(), req)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if res.Lyrics != "plain lyric" || res.Level != source.SyncNone {
			t.Errorf("result = %q/%d, want plain/SyncNone", res.Lyrics, res.Level)
		}
		wantRequest(t, tp, 0, "matcher.subtitle.get")
		wantRequest(t, tp, 1, "matcher.lyrics.get")
		wantCount(t, tp, 2)
	})

	t.Run("envelope_status", func(t *testing.T) {
		cases := []struct {
			name       string
			httpStatus int
			body       string
			wantErr    error
			wantSub    string
			wantLyrics string
		}{
			{"not_found", 200, envelope(404, `{}`), source.ErrNotFound, "", ""},
			{"unauthorized", 200, envelope(401, `{}`), nil, "HTTP 401", ""},
			{"payment_required", 200, envelope(402, `{}`), nil, "HTTP 402", ""},
			{"forbidden", 200, envelope(403, `{}`), nil, "HTTP 403", ""},
			{"hint", 200, envelopeHint(500, "upstream exploded", `{}`),
				nil, "upstream exploded", ""},
			{"status_zero_http_500", 500, envelope(0, `{}`), nil, "500", ""},
			{"status_zero_http_200", 200, envelope(0, lyricsBody("plain")),
				nil, "", "plain"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				tp := newTransport(t, map[string]stubResponse{
					"matcher.lyrics.get": {status: tc.httpStatus, body: tc.body},
				})
				res, err := newAdapter(tp).Fetch(context.Background(),
					authorRequest())
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("Fetch error = %v, want %v", err, tc.wantErr)
					}
					return
				}
				if tc.wantSub != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
						t.Fatalf("error = %v, want it to contain %q",
							err, tc.wantSub)
					}
					return
				}
				if err != nil {
					t.Fatalf("Fetch: %v", err)
				}
				if res.Lyrics != tc.wantLyrics || res.Level != source.SyncNone {
					t.Errorf("result = %q/%d, want %q/SyncNone",
						res.Lyrics, res.Level, tc.wantLyrics)
				}
			})
		}
	})

	t.Run("empty_message_body", func(t *testing.T) {
		for _, tc := range []struct{ name, body string }{
			{"empty", `{"message":{"header":{"status_code":200}}}`},
			{"empty_array", envelope(200, `[]`)},
			{"empty_object", envelope(200, `{}`)},
			{"null", envelope(200, `null`)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tp := newTransport(t, map[string]stubResponse{
					"matcher.lyrics.get": {body: tc.body},
				})
				_, err := newAdapter(tp).Fetch(context.Background(),
					authorRequest())
				if !errors.Is(err, source.ErrNotFound) {
					t.Fatalf("Fetch error = %v, want source.ErrNotFound", err)
				}
			})
		}
	})

	t.Run("empty_http_body", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"matcher.lyrics.get": {body: ""},
		})
		_, err := newAdapter(tp).Fetch(context.Background(), authorRequest())
		if err == nil || errors.Is(err, source.ErrNotFound) ||
			!strings.Contains(err.Error(), "decode") {
			t.Fatalf("Fetch error = %v, want a decode error", err)
		}
	})

	t.Run("lyrics_cleaning", func(t *testing.T) {
		for _, tc := range []struct {
			name, body string
			want       string
			notFound   bool
		}{
			{"notice_trimmed",
				"line one\nline two\n*******\nFree lyrics, not for commercial use",
				"line one\nline two", false},
			{"instrumental_placeholder", "....", "", true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tp := newTransport(t, map[string]stubResponse{
					"matcher.lyrics.get": {body: lyricsEnvelope(200, tc.body)},
				})
				res, err := newAdapter(tp).Fetch(context.Background(),
					authorRequest())
				if tc.notFound {
					if !errors.Is(err, source.ErrNotFound) {
						t.Fatalf("Fetch error = %v, want ErrNotFound", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("Fetch: %v", err)
				}
				if res.Lyrics != tc.want {
					t.Errorf("Lyrics = %q, want %q", res.Lyrics, tc.want)
				}
			})
		}
	})

	t.Run("isrc_subtitle", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"track.get":          {body: envelope(200, trackBody)},
			"track.subtitle.get": {body: subtitleEnvelope(200, "[00:02.00]line")},
		})
		req := baseRequest()
		req.ISRC, req.SyncLevel = "USRC17607839", source.SyncLine
		res, err := newAdapter(tp).Fetch(context.Background(), req)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if res.Lyrics != "[00:02.00]line" || res.Level != source.SyncLine ||
			res.Title != "Song" {
			t.Errorf("result = %+v, want subtitle/SyncLine with title", res)
		}
		wantRequest(t, tp, 0, "track.get")
		q := wantRequest(t, tp, 1, "track.subtitle.get")
		wantQuery(t, q, map[string]string{"commontrack_id": "42"})
		wantCount(t, tp, 2)
	})

	t.Run("title_search", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{
			"track.search":     {body: envelope(200, searchBody)},
			"track.lyrics.get": {body: lyricsEnvelope(200, "plain")},
		})
		res, err := newAdapter(tp).Fetch(context.Background(), baseRequest())
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if res.Lyrics != "plain" || res.Level != source.SyncNone ||
			res.Title != "Song" || res.Artist != "Artist" {
			t.Errorf("result = %+v, want plain lyrics plus metadata", res)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		tp := newTransport(t, map[string]stubResponse{})
		req := baseRequest()
		req.Custom = nil
		_, err := newAdapter(tp).Fetch(context.Background(), req)
		var mismatch source.RequiredParamMismatchError
		if !errors.As(err, &mismatch) || mismatch.ParamName != keyName {
			t.Fatalf("Fetch error = %v, want mismatch for %q", err, keyName)
		}
		wantCount(t, tp, 0)
	})
}

func TestAPIKeyNotLeaked(t *testing.T) {
	const secret = "SUPERSECRET"
	tp := &testutil.Transport{Respond: func(*http.Request) (*http.Response,
		error) {
		return nil, errors.New("dial fail")
	}}
	req := authorRequest()
	req.Custom = map[string]string{keyName: secret}
	_, err := newAdapter(tp).Fetch(context.Background(), req)
	if err == nil {
		t.Fatal("Fetch succeeded, want a transport error")
	}
	if strings.Contains(err.Error(), secret) ||
		!strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("error = %v, want the key redacted", err)
	}
}
