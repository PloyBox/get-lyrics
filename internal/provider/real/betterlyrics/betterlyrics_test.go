package betterlyrics_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/provider/real/betterlyrics"
	"github.com/PloyBox/get-lyrics/internal/testutil"
	"github.com/PloyBox/get-lyrics/source"
)

const testUA = "get-lyrics/test-agent"

func TestRequest(t *testing.T) {
	body := `{"ttml":"<tt/>","lyrics":"[00:01.00]hi"}`
	tests := []struct {
		name     string
		sync     source.SyncLevel
		album    string
		duration int64
		wantPath string
	}{
		{"word", source.SyncWord, "", 0, "/ttml/getLyrics"},
		{"line with album and duration", source.SyncLine, " 25 ", 225,
			"/kugou/getLyrics"},
		{"none", source.SyncNone, "", 0, "/kugou/getLyrics"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &testutil.Transport{
				Respond: testutil.Respond(http.StatusOK, body),
			}
			a := newAdapter(tp)
			if _, err := a.Fetch(context.Background(), source.Request{
				Song: " Hello ", Author: " Adele ", Album: tc.album,
				Duration: tc.duration, SyncLevel: tc.sync, UserAgent: testUA,
			}); err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			got := tp.Last()
			if got == nil {
				t.Fatal("adapter sent no HTTP request")
			}
			if got.URL.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", got.URL.Path, tc.wantPath)
			}
			q := got.URL.Query()
			if q.Get("s") != "Hello" || q.Get("a") != "Adele" {
				t.Errorf("query = %v, want trimmed song and author", q)
			}
			wantAl, wantD := "", ""
			if tc.album != "" {
				wantAl = "25"
			}
			if tc.duration != 0 {
				wantD = "225"
			}
			if q.Get("al") != wantAl || q.Get("d") != wantD {
				t.Errorf("al/d = %q/%q, want %q/%q",
					q.Get("al"), q.Get("d"), wantAl, wantD)
			}
			if got.Header.Get("User-Agent") != testUA ||
				got.Header.Get("Accept") != "application/json" {
				t.Errorf("headers = %v", got.Header)
			}
		})
	}

	t.Run("capabilities", func(t *testing.T) {
		caps := testutil.Capabilities(t, newAdapter(nil), source.Request{})
		want := source.ParamAuthor | source.ParamAlbum | source.ParamDuration
		if caps.Filters&want != want || caps.Required&source.ParamAuthor == 0 {
			t.Errorf("caps = %+v, want filters %d and author required",
				caps, want)
		}
	})
}

func TestFetch(t *testing.T) {
	ttml := "\n<tt>\n  <body><p begin=\"0s\">Hi</p></body>\n</tt>\n"
	lrc := "[00:01.00]First line\n[00:02.50]Second line"
	plain := "First line\nSecond line"
	tests := []struct {
		name         string
		sync         source.SyncLevel
		status       int
		body         string
		wantLyrics   string
		wantLevel    source.SyncLevel
		wantSub      string
		wantNotFound bool
		wantErr      bool
	}{
		{"word prefers ttml", source.SyncWord, 0,
			fmt.Sprintf(`{"ttml":%q,"lyrics":"<tt>ignored</tt>"}`, ttml),
			ttml, source.SyncWord, "ttml", false, false},
		{"word falls back to lyrics", source.SyncWord, 0,
			fmt.Sprintf(`{"ttml":"  ","lyrics":%q}`, ttml),
			ttml, source.SyncWord, "ttml", false, false},
		{"line keeps timestamped lrc", source.SyncLine, 0,
			fmt.Sprintf(`{"lyrics":%q}`, lrc),
			lrc, source.SyncLine, "kugou", false, false},
		{"line strips text without timestamps", source.SyncLine, 0,
			`{"lyrics":"[Verse]\nFirst line\n\n[Chorus]\nSecond line"}`,
			plain, source.SyncNone, "kugou", false, false},
		{"none strips lrc", source.SyncNone, 0,
			fmt.Sprintf(`{"lyrics":%q}`, lrc),
			plain, source.SyncNone, "kugou", false, false},
		{"word empty payload", source.SyncWord, 0,
			`{"ttml":"","lyrics":"  \n\t"}`, "", 0, "", true, false},
		{"line whitespace payload", source.SyncLine, 0,
			`{"lyrics":"   "}`, "", 0, "", true, false},
		{"http 404", source.SyncLine, http.StatusNotFound,
			`{"error":"not found"}`, "", 0, "", true, false},
		{"http 500", source.SyncLine, http.StatusInternalServerError,
			"upstream failed", "", 0, "", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := tc.status
			if status == 0 {
				status = http.StatusOK
			}
			tp := &testutil.Transport{
				Respond: testutil.Respond(status, tc.body),
			}
			res, err := newAdapter(tp).Fetch(context.Background(),
				source.Request{Song: "Hello", Author: "Adele",
					SyncLevel: tc.sync, UserAgent: testUA})
			switch {
			case tc.wantNotFound:
				if !errors.Is(err, source.ErrNotFound) {
					t.Fatalf("err = %v, want ErrNotFound", err)
				}
				return
			case tc.wantErr:
				if err == nil || errors.Is(err, source.ErrNotFound) {
					t.Fatalf("err = %v, want a non-not-found error", err)
				}
				return
			case err != nil:
				t.Fatalf("Fetch: %v", err)
			}
			if res.Lyrics != tc.wantLyrics || res.Level != tc.wantLevel ||
				res.SubSource != tc.wantSub {
				t.Errorf("result = %q/%d/%q, want %q/%d/%q", res.Lyrics,
					res.Level, res.SubSource, tc.wantLyrics, tc.wantLevel,
					tc.wantSub)
			}
			wantFilled := source.FieldLyrics | source.FieldSubSource
			if res.Filled != wantFilled {
				t.Errorf("Filled = %d, want %d", res.Filled, wantFilled)
			}
		})
	}
}

func TestEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		sync   source.SyncLevel
		custom map[string]string
		host   string
		path   string
	}{
		{"default line", source.SyncLine, nil,
			"lyrics-api.boidu.dev", "/kugou/getLyrics"},
		{"default word", source.SyncWord, nil,
			"lyrics-api.boidu.dev", "/ttml/getLyrics"},
		{"custom line", source.SyncLine, map[string]string{
			"BETTERLYRICS_ENDPOINT": "https://mirror.example/",
		}, "mirror.example", "/kugou/getLyrics"},
		{"custom word with base path", source.SyncWord, map[string]string{
			"BETTERLYRICS_ENDPOINT": "https://mirror.example/api",
		}, "mirror.example", "/api/ttml/getLyrics"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &testutil.Transport{Respond: testutil.Respond(http.StatusOK,
				`{"lyrics":"x"}`)}
			_, err := newAdapter(tp).Fetch(context.Background(), source.Request{
				Song: "Hello", Author: "Adele", SyncLevel: tc.sync,
				Custom: tc.custom,
			})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			got := tp.Last()
			if got.URL.Host != tc.host || got.URL.Path != tc.path {
				t.Errorf("URL = %s, want host %q path %q",
					got.URL, tc.host, tc.path)
			}
		})
	}
}

func newAdapter(tp *testutil.Transport) *betterlyrics.Adapter {
	a := betterlyrics.New()
	a.HTTPClient = &http.Client{Transport: tp}
	return a
}
