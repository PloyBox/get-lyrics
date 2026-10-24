// Black-box tests for the lrccx adapter. Requests go through
// testutil.Transport only: no test server, no network.
package lrccx_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/provider/real/lrccx"
	"github.com/PloyBox/get-lyrics/internal/testutil"
	"github.com/PloyBox/get-lyrics/source"
)

const testUA = "get-lyrics/test"

func TestRequest(t *testing.T) {
	tests := []struct {
		name   string
		song   string
		author string
		album  string
		want   url.Values
	}{
		{"title only", "Song", "", "", url.Values{"title": {"Song"}}},
		{"author only", "Song", "Artist", "",
			url.Values{"title": {"Song"}, "artist": {"Artist"}}},
		{"album only", "Song", "", "Album",
			url.Values{"title": {"Song"}, "album": {"Album"}}},
		{"author and album", "Song", "Artist", "Album",
			url.Values{"title": {"Song"}, "artist": {"Artist"},
				"album": {"Album"}}},
		{"unknown album omitted", "Song", "Artist", "[Unknown Album]",
			url.Values{"title": {"Song"}, "artist": {"Artist"}}},
		{"values escaped", "Rock & Roll", "A&B", "C & D",
			url.Values{"title": {"Rock & Roll"}, "artist": {"A&B"},
				"album": {"C & D"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &testutil.Transport{Respond: testutil.Respond(http.StatusOK, "[]")}
			a := newAdapter(tp)
			_, _ = a.Fetch(context.Background(), source.Request{
				Song: tc.song, Author: tc.author, Album: tc.album,
				SyncLevel: source.SyncLine, UserAgent: testUA,
			})
			req := tp.Last()
			if req == nil {
				t.Fatal("adapter sent no request")
			}
			if req.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", req.Method)
			}
			if req.URL.Path != "/jsonapi" {
				t.Errorf("path = %q, want /jsonapi", req.URL.Path)
			}
			if got := req.URL.RawQuery; got != tc.want.Encode() {
				t.Errorf("query = %q, want %q", got, tc.want.Encode())
			}
			if req.UserAgent() != testUA ||
				req.Header.Get("Accept") != "application/json" {
				t.Errorf("headers = %v", req.Header)
			}
		})
	}

	t.Run("capabilities", func(t *testing.T) {
		req := source.Request{Song: "Song", Author: "Artist", Album: "Album"}
		caps := testutil.Capabilities(t, newAdapter(nil), req)
		want := source.ParamAuthor | source.ParamAlbum
		if caps.Filters&want != want || caps.Required != 0 {
			t.Errorf("caps = %+v, want filters %d and no required", caps, want)
		}
	})
}

func TestFetch(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		body         string
		level        source.SyncLevel
		wantLyrics   string
		wantLevel    source.SyncLevel
		wantTitle    string
		wantArtist   string
		wantErr      bool
		wantNotFound bool
	}{
		{"skips null lrc for later hit", http.StatusOK,
			`[
  {"title":"Wrong","artist":"Wrong","lrc":null},
  {"title":"Right","artist":"Right",
   "lrc":"[00:10.000]line one\n[00:20.000]line two\n"}
]`, source.SyncLine,
			"[00:10.000]line one\n[00:20.000]line two\n", source.SyncLine,
			"Right", "Right", false, false},
		{"sync line keeps raw lrc", http.StatusOK,
			`[{"title":"T","artist":"A",
  "lrc":"[00:01.500]intro\n[00:05.250]verse\n"}]`, source.SyncLine,
			"[00:01.500]intro\n[00:05.250]verse\n", source.SyncLine,
			"T", "A", false, false},
		{"sync line strips untimed text", http.StatusOK,
			`[{"title":"T","artist":"A",
  "lrc":"[Verse]\nplain one\n\nplain two\n"}]`, source.SyncLine,
			"plain one\nplain two", source.SyncNone, "T", "A", false, false},
		{"sync none strips tags", http.StatusOK,
			`[{"title":"T","artist":"A",
  "lrc":"[Verse]\n[00:01.500]intro\n[Chorus]\n[00:05.250]chorus\n"}]`,
			source.SyncNone, "intro\nchorus", source.SyncNone,
			"T", "A", false, false},
		{"empty array", http.StatusOK, `[]`, source.SyncLine,
			"", 0, "", "", true, true},
		{"all hits lack lrc", http.StatusOK,
			`[{"title":"T","artist":"A","lrc":null},` +
				`{"title":"U","artist":"B","lrc":""}]`,
			source.SyncLine, "", 0, "", "", true, true},
		{"http 500", http.StatusInternalServerError, `{"error":"boom"}`,
			source.SyncLine, "", 0, "", "", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &testutil.Transport{Respond: testutil.Respond(tc.status, tc.body)}
			res, err := newAdapter(tp).Fetch(context.Background(),
				source.Request{Song: "Song", SyncLevel: tc.level, UserAgent: testUA})
			if tc.wantErr {
				if err == nil {
					t.Fatal("Fetch() error = nil, want error")
				}
				if got := errors.Is(err, source.ErrNotFound); got != tc.wantNotFound {
					t.Fatalf("errors.Is(err, ErrNotFound) = %v, want %v: %v",
						got, tc.wantNotFound, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if len(tp.Requests) != 1 {
				t.Errorf("sent %d requests, want 1", len(tp.Requests))
			}
			if res.Lyrics != tc.wantLyrics || res.Level != tc.wantLevel {
				t.Errorf("lyrics/level = %q/%d, want %q/%d",
					res.Lyrics, res.Level, tc.wantLyrics, tc.wantLevel)
			}
			if res.Title != tc.wantTitle || res.Artist != tc.wantArtist {
				t.Errorf("metadata = %q/%q, want %q/%q",
					res.Title, res.Artist, tc.wantTitle, tc.wantArtist)
			}
			wantFilled := source.FieldLyrics
			if tc.wantTitle != "" {
				wantFilled |= source.FieldTitle
			}
			if tc.wantArtist != "" {
				wantFilled |= source.FieldArtist
			}
			if res.Filled != wantFilled {
				t.Errorf("Filled = %d, want %d", res.Filled, wantFilled)
			}
		})
	}
}

func newAdapter(tp *testutil.Transport) *lrccx.Adapter {
	a := lrccx.New()
	a.HTTPClient = &http.Client{Transport: tp}
	return a
}
