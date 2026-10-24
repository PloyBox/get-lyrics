package lyricsovh_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/provider/real/lyricsovh"
	"github.com/PloyBox/get-lyrics/internal/testutil"
	"github.com/PloyBox/get-lyrics/source"
)

const testUA = "get-lyrics/test (+https://example.test)"

func TestRequest(t *testing.T) {
	tp := &testutil.Transport{Respond: testutil.Respond(http.StatusOK,
		`{"lyrics":"x"}`)}
	req := source.Request{Song: "Back In Black", Author: "AC/DC", UserAgent: testUA}
	a := newAdapter(tp)
	if _, err := a.Fetch(context.Background(), req); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(tp.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(tp.Requests))
	}
	got := tp.Last()
	if got.Method != http.MethodGet || got.URL.Scheme != "https" ||
		got.URL.Host != "api.lyrics.ovh" {
		t.Errorf("request = %s %s", got.Method, got.URL)
	}
	if p := got.URL.EscapedPath(); p != "/v1/AC%2FDC/Back%20In%20Black" {
		t.Errorf("path = %q, want escaped artist and song", p)
	}
	if got.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty", got.URL.RawQuery)
	}
	if got.Header.Get("User-Agent") != testUA ||
		got.Header.Get("Accept") != "application/json" {
		t.Errorf("headers = %v", got.Header)
	}

	caps := testutil.Capabilities(t, a, req)
	if caps.Required&source.ParamAuthor == 0 ||
		caps.Filters&source.ParamAuthor == 0 {
		t.Errorf("caps = %+v, want author required and supported", caps)
	}
}

func TestEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		custom map[string]string
		host   string
	}{
		{"default", nil, "api.lyrics.ovh"},
		{"custom", map[string]string{
			"LYRICSOVH_ENDPOINT": "https://mirror.example/",
		}, "mirror.example"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &testutil.Transport{Respond: testutil.Respond(http.StatusOK,
				`{"lyrics":"x"}`)}
			_, err := newAdapter(tp).Fetch(context.Background(), source.Request{
				Song: "Song", Author: "Artist", Custom: tc.custom,
			})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			got := tp.Last()
			if got.URL.Host != tc.host ||
				got.URL.Path != "/v1/Artist/Song" {
				t.Errorf("URL = %s, want host %q path /v1/Artist/Song",
					got.URL, tc.host)
			}
		})
	}
}

func TestFetch(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		body         string
		wantLyrics   string
		wantErr      bool
		wantNotFound bool
		wantErrSub   string
	}{
		{"success", http.StatusOK, `{"lyrics":"Hello\nWorld"}`,
			"Hello\nWorld", false, false, ""},
		{"error field on 200", http.StatusOK, `{"error":"upstream refused"}`,
			"", true, false, "upstream refused"},
		{"whitespace lyrics", http.StatusOK, `{"lyrics":" \n\t "}`,
			"", true, true, ""},
		{"not found", http.StatusNotFound, `{"error":"No lyrics found"}`,
			"", true, true, ""},
		{"server error", http.StatusInternalServerError, "boom",
			"", true, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &testutil.Transport{Respond: testutil.Respond(tc.status, tc.body)}
			res, err := newAdapter(tp).Fetch(context.Background(),
				source.Request{Song: "Back In Black", Author: "AC/DC"})
			if len(tp.Requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(tp.Requests))
			}
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("Fetch: %v", err)
				}
				if res.Lyrics != tc.wantLyrics || res.Level != source.SyncNone ||
					res.Filled != source.FieldLyrics || res.Title != "" ||
					res.Artist != "" || res.Album != "" || res.ISRC != "" ||
					res.SubSource != "" {
					t.Errorf("result = %+v", res)
				}
				return
			}
			if err == nil {
				t.Fatalf("Fetch = %+v, want error", res)
			}
			if got := errors.Is(err, source.ErrNotFound); got != tc.wantNotFound {
				t.Errorf("errors.Is(err, ErrNotFound) = %v, want %v (err: %v)",
					got, tc.wantNotFound, err)
			}
			if tc.wantErrSub != "" &&
				!strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("err = %q, want it to contain %q", err, tc.wantErrSub)
			}
		})
	}
}

func newAdapter(tp *testutil.Transport) *lyricsovh.Adapter {
	a := lyricsovh.New()
	a.HTTPClient = &http.Client{Transport: tp}
	return a
}
