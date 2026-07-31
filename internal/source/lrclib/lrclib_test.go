package lrclib

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

func newTestAdapter(t *testing.T, route string, h http.HandlerFunc) *Adapter {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(route, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	a := New()
	a.Endpoint = srv.URL + route
	return a
}

func TestLrclib_Name(t *testing.T) {
	if got := New().Name(); got != "lrclib" {
		t.Fatalf("Name() = %q; want %q", got, "lrclib")
	}
}

func TestLrclib_SupportedParamsIsAuthorPlusTimestamp(t *testing.T) {
	got := New().SupportedParams()
	if got&source.ParamAuthor == 0 {
		t.Fatalf("missing ParamAuthor bit: %b", got)
	}
	if got&source.ParamTimestamp == 0 {
		t.Fatalf("missing ParamTimestamp bit: %b", got)
	}
	if got&source.ParamAlbum != 0 {
		t.Fatalf("should not advertise ParamAlbum: %b", got)
	}
	if got&source.ParamISWC != 0 {
		t.Fatalf("should not advertise ParamISWC: %b", got)
	}
}

func TestLrclib_FetchPicksFirstNonEmptyPlainHit(t *testing.T) {
	a := newTestAdapter(t, "/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "Blinding Lights" {
			t.Errorf("q = %q; want %q", r.URL.Query().Get("q"), "Blinding Lights")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]lrclibHit{
			{TrackName: "Blinding Lights", PlainLyrics: ""},
			{TrackName: "Blinding Lights", ArtistName: "The Weeknd",
				PlainLyrics:  "I said, ooh I'm blinded by the lights",
				SyncedLyrics: "[00:00.00] I said, ooh I'm blinded by the lights"},
		})
	})

	res, err := a.Fetch(context.Background(), source.Request{Song: "Blinding Lights"})
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if res.Title != "Blinding Lights" {
		t.Fatalf("Title = %q", res.Title)
	}
	if res.Artist != "The Weeknd" {
		t.Fatalf("Artist = %q", res.Artist)
	}
	if res.Lyrics == "" {
		t.Fatalf("Lyrics empty")
	}
	if res.Source != "lrclib" {
		t.Fatalf("Source = %q", res.Source)
	}
}

func TestLrclib_FetchWithAuthorUsesGetEndpoint(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	a := newTestAdapter(t, "/api/get", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		// /api/get returns a single object, not an array.
		_, _ = io.WriteString(w, `{"id":1,"trackName":"Blinding Lights","artistName":"The Weeknd","plainLyrics":"I said, ooh I'm blinded by the lights","syncedLyrics":"[00:00.00] I said, ooh I'm blinded by the lights"}`)
	})

	res, err := a.Fetch(context.Background(), source.Request{Song: "Blinding Lights", Author: "The Weeknd"})
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if gotPath != "/api/get" {
		t.Fatalf("path = %q; want /api/get", gotPath)
	}
	if gotQuery.Get("track_name") != "Blinding Lights" || gotQuery.Get("artist_name") != "The Weeknd" {
		t.Fatalf("query = %v; want track_name+artist_name", gotQuery)
	}
	if res.Lyrics == "" {
		t.Fatalf("Lyrics empty")
	}
	if res.SyncedLyrics != "" {
		t.Fatalf("SyncedLyrics unexpectedly populated without --timestamp: %q", res.SyncedLyrics)
	}
}

func TestLrclib_FetchFillsSyncedOnlyWhenTimestampRequested(t *testing.T) {
	a := newTestAdapter(t, "/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]lrclibHit{{
			TrackName:    "X",
			PlainLyrics:  "plain",
			SyncedLyrics: "[00:00.00] synced",
		}})
	})

	plain, err := a.Fetch(context.Background(), source.Request{Song: "X"})
	if err != nil {
		t.Fatalf("plain Fetch err: %v", err)
	}
	if plain.SyncedLyrics != "" {
		t.Fatalf("SyncedLyrics unexpectedly populated without --timestamp: %q", plain.SyncedLyrics)
	}

	synced, err := a.Fetch(context.Background(), source.Request{Song: "X", Timestamp: true})
	if err != nil {
		t.Fatalf("synced Fetch err: %v", err)
	}
	if synced.SyncedLyrics == "" {
		t.Fatalf("SyncedLyrics empty when Timestamp requested")
	}
}

func TestLrclib_FetchEmptyResultReturnsError(t *testing.T) {
	a := newTestAdapter(t, "/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	_, err := a.Fetch(context.Background(), source.Request{Song: "nope"})
	if err == nil || !strings.Contains(err.Error(), "no lyrics") {
		t.Fatalf("err = %v; want 'no lyrics' message", err)
	}
}

func TestLrclib_FetchNon2xxReturnsError(t *testing.T) {
	a := newTestAdapter(t, "/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "upstream busy")
	})
	_, err := a.Fetch(context.Background(), source.Request{Song: "X"})
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v; want status code message", err)
	}
}

func TestLrclib_FetchMissingSongReturnsError(t *testing.T) {
	_, err := New().Fetch(context.Background(), source.Request{Song: "   "})
	if err == nil {
		t.Fatalf("missing song should be rejected")
	}
}

func TestLrclib_FetchSetsUserAgent(t *testing.T) {
	var seenUA string
	a := newTestAdapter(t, "/api/search", func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "[]")
	})
	if _, err := a.Fetch(context.Background(), source.Request{Song: "X"}); err == nil {
		t.Fatalf("expected error from empty hits, got nil")
	}
	if !strings.HasPrefix(seenUA, "get-lyrics/") {
		t.Fatalf("User-Agent = %q; want get-lyrics/ prefix", seenUA)
	}
}
