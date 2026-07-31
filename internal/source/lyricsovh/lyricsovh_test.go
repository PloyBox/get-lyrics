package lyricsovh

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

func newTestAdapter(t *testing.T, h http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a := New()
	a.Endpoint = srv.URL + "/v1"
	return a
}

func TestLyricsovh_Name(t *testing.T) {
	if got := New().Name(); got != "lyricsovh" {
		t.Fatalf("Name() = %q; want %q", got, "lyricsovh")
	}
}

func TestLyricsovh_SupportedParamsIsAuthorOnly(t *testing.T) {
	got := New().SupportedParams()
	if got&source.ParamAuthor == 0 {
		t.Fatalf("missing ParamAuthor bit: %b", got)
	}
	for _, p := range []source.Param{source.ParamAlbum, source.ParamISWC, source.ParamTimestamp} {
		if got&p != 0 {
			t.Fatalf("should not advertise %b", p)
		}
	}
}

func TestLyricsovh_FetchReturnsLyrics(t *testing.T) {
	var gotPath string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"lyrics":"look how they shine for you"}`)
	})

	res, err := a.Fetch(context.Background(), source.Request{Song: "Yellow", Author: "Coldplay"})
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if gotPath != "/v1/Coldplay/Yellow" {
		t.Fatalf("path = %q; want /v1/Coldplay/Yellow", gotPath)
	}
	if res.Lyrics != "look how they shine for you" {
		t.Fatalf("Lyrics = %q; want the fetched lyrics", res.Lyrics)
	}
	if res.Title != "Yellow" || res.Artist != "Coldplay" || res.Source != "lyricsovh" {
		t.Fatalf("metadata mismatch: %+v", res)
	}
}

func TestLyricsovh_FetchEscapesURLSegments(t *testing.T) {
	var gotPath string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"lyrics":"x"}`)
	})

	if _, err := a.Fetch(context.Background(), source.Request{Song: "A B", Author: "C D"}); err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if gotPath != "/v1/C%20D/A%20B" {
		t.Fatalf("path = %q; want URL-escaped segments /v1/C%%20D/A%%20B", gotPath)
	}
}

func TestLyricsovh_FetchMissingAuthorReturnsRequiredParamError(t *testing.T) {
	_, err := New().Fetch(context.Background(), source.Request{Song: "Yellow"})
	if err == nil {
		t.Fatalf("Fetch should fail when Author is empty")
	}
	var reqErr source.RequiredParamError
	if !errors.As(err, &reqErr) {
		t.Fatalf("err = %v; want a RequiredParamError", err)
	}
	if reqErr.Param != source.ParamAuthor || reqErr.Flag != "--author" {
		t.Fatalf("reqErr = %+v; want Param=ParamAuthor, Flag=--author", reqErr)
	}
}

func TestLyricsovh_FetchMissingSongReturnsError(t *testing.T) {
	_, err := New().Fetch(context.Background(), source.Request{Author: "Coldplay"})
	if err == nil {
		t.Fatalf("missing song should be rejected")
	}
}

func TestLyricsovh_FetchNotFoundReturnsError(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"error":"No lyrics found"}`)
	})
	_, err := a.Fetch(context.Background(), source.Request{Song: "nope", Author: "nobody"})
	if err == nil || !strings.Contains(err.Error(), "no lyrics found") {
		t.Fatalf("err = %v; want 'no lyrics found' message", err)
	}
}

func TestLyricsovh_FetchEmptyLyricsReturnsError(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"lyrics": "   "})
	})
	_, err := a.Fetch(context.Background(), source.Request{Song: "X", Author: "Y"})
	if err == nil {
		t.Fatalf("empty lyrics should be rejected")
	}
}

func TestLyricsovh_FetchSetsUserAgent(t *testing.T) {
	var seenUA string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"lyrics":"x"}`)
	})
	if _, err := a.Fetch(context.Background(), source.Request{Song: "X", Author: "Y"}); err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if !strings.HasPrefix(seenUA, "get-lyrics/") {
		t.Fatalf("User-Agent = %q; want get-lyrics/ prefix", seenUA)
	}
}
