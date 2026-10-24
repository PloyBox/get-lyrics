// Package lrclib_test covers the adapter through its exported API only.
package lrclib_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/httpx"
	"github.com/PloyBox/get-lyrics/internal/provider/real/lrclib"
	"github.com/PloyBox/get-lyrics/internal/testutil"
	"github.com/PloyBox/get-lyrics/source"
)

const testUA = "get-lyrics/test (+https://example.test)"

func TestRequest(t *testing.T) {
	const (
		searchHit = `[{"trackName":"Let It Be","plainLyrics":"x"}]`
		getHit    = `{"trackName":"Let It Be","plainLyrics":"x"}`
	)
	tests := []struct {
		name  string
		req   source.Request
		body  string
		path  string
		query url.Values
	}{
		{"search without author", source.Request{Song: "Let It Be"}, searchHit,
			"/api/search", url.Values{"q": {"Let It Be"}}},
		{"get with author", source.Request{Song: "Let It Be", Author: "The Beatles"},
			getHit, "/api/get",
			url.Values{"track_name": {"Let It Be"}, "artist_name": {"The Beatles"}}},
		{"get with album", source.Request{Song: "Let It Be", Author: "The Beatles",
			Album: "Let It Be"}, getHit, "/api/get",
			url.Values{"track_name": {"Let It Be"}, "artist_name": {"The Beatles"},
				"album_name": {"Let It Be"}}},
		{"get with duration", source.Request{Song: "Let It Be", Author: "The Beatles",
			Duration: 243}, getHit, "/api/get",
			url.Values{"track_name": {"Let It Be"}, "artist_name": {"The Beatles"},
				"duration": {"243"}}},
		{"get with album and duration", source.Request{Song: "Let It Be",
			Author: "The Beatles", Album: "Let It Be", Duration: 243},
			getHit, "/api/get",
			url.Values{"track_name": {"Let It Be"}, "artist_name": {"The Beatles"},
				"album_name": {"Let It Be"}, "duration": {"243"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.UserAgent = testUA
			tp := &testutil.Transport{Respond: testutil.Respond(http.StatusOK, tc.body)}
			a := newAdapter(tp)
			if _, err := a.Fetch(context.Background(), tc.req); err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(tp.Requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(tp.Requests))
			}
			got := tp.Last()
			if got.Method != http.MethodGet || got.URL.Path != tc.path {
				t.Errorf("request = %s %s, want GET %s",
					got.Method, got.URL.Path, tc.path)
			}
			if q := got.URL.Query().Encode(); q != tc.query.Encode() {
				t.Errorf("query = %q, want %q", q, tc.query.Encode())
			}
			if got.Header.Get("User-Agent") != testUA ||
				got.Header.Get("Accept") != "application/json" {
				t.Errorf("headers = %v", got.Header)
			}
		})
	}

	for _, tc := range []struct {
		name string
		req  source.Request
		want source.Param
		not  source.Param
	}{
		{"without author", source.Request{Song: "Let It Be"},
			source.ParamAuthor, source.ParamAlbum | source.ParamDuration},
		{"with author", source.Request{Song: "Let It Be", Author: "The Beatles"},
			source.ParamAuthor | source.ParamAlbum | source.ParamDuration, 0},
	} {
		t.Run("capabilities "+tc.name, func(t *testing.T) {
			caps := testutil.Capabilities(t, newAdapter(nil), tc.req)
			if caps.Filters&tc.want != tc.want || caps.Filters&tc.not != 0 {
				t.Errorf("Filters = %d, want %d without %d",
					caps.Filters, tc.want, tc.not)
			}
		})
	}
}

func TestFetch(t *testing.T) {
	const (
		plainHit = `{"trackName":"Song","artistName":"Artist",` +
			`"plainLyrics":"plain words"}`
		syncedHit = `{"trackName":"Song","artistName":"Artist",` +
			`"syncedLyrics":"[00:01]synced words"}`
	)
	const meta = source.FieldTitle | source.FieldArtist
	tests := []struct {
		name       string
		status     int
		body       string
		req        source.Request
		wantLyrics string
		wantLevel  source.SyncLevel
		wantTitle  string
		wantArtist string
		wantAlbum  string
		wantFilled source.ResultField
		wantErrIs  error
		wantErrSub string
	}{
		{"sync request picks synced hit", http.StatusOK,
			"[" + plainHit + "," + syncedHit + "]",
			source.Request{Song: "Song", SyncLevel: source.SyncLine},
			"[00:01]synced words", source.SyncLine, "Song", "Artist", "",
			source.FieldLyrics | meta, nil, ""},
		{"sync request falls back to plain", http.StatusOK, "[" + plainHit + "]",
			source.Request{Song: "Song", SyncLevel: source.SyncLine},
			"plain words", source.SyncNone, "Song", "Artist", "",
			source.FieldLyrics | meta, nil, ""},
		{"plain request with only synced hits", http.StatusOK, "[" + syncedHit + "]",
			source.Request{Song: "Song", SyncLevel: source.SyncNone},
			"", source.SyncNone, "", "", "", 0, source.ErrNotFound, ""},
		{"metadata fills only non-empty fields", http.StatusOK,
			`[{"trackName":"Real Title","artistName":"Real Artist",` +
				`"plainLyrics":"words"}]`,
			source.Request{Song: "Song"},
			"words", source.SyncNone, "Real Title", "Real Artist", "",
			source.FieldLyrics | meta, nil, ""},
		{"get endpoint object decodes", http.StatusOK,
			`{"trackName":"Get Title","artistName":"Get Artist",` +
				`"albumName":"Get Album","plainLyrics":"get words"}`,
			source.Request{Song: "Get Title", Author: "Get Artist",
				Album: "Get Album", Duration: 200},
			"get words", source.SyncNone, "Get Title", "Get Artist", "Get Album",
			source.FieldLyrics | meta | source.FieldAlbum, nil, ""},
		{"http 404 is not found", http.StatusNotFound, `{"error":"not found"}`,
			source.Request{Song: "Song"}, "", 0, "", "", "", 0, source.ErrNotFound, ""},
		{"http 500 is an error", http.StatusInternalServerError, "boom",
			source.Request{Song: "Song"}, "", 0, "", "", "", 0, nil, "500"},
		{"invalid json is a decode error", http.StatusOK, `{"trackName":`,
			source.Request{Song: "Song"}, "", 0, "", "", "", 0, nil, "decode"},
		{"oversized body is rejected", http.StatusOK,
			strings.Repeat("x", httpx.MaxBodyBytes+1), source.Request{Song: "Song"},
			"", 0, "", "", "", 0, source.ErrBodyTooLarge, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &testutil.Transport{Respond: testutil.Respond(tc.status, tc.body)}
			res, err := newAdapter(tp).Fetch(context.Background(), tc.req)
			if len(tp.Requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(tp.Requests))
			}
			if tc.wantErrIs != nil {
				if !errors.Is(err, tc.wantErrIs) {
					t.Errorf("err = %v, want %v", err, tc.wantErrIs)
				}
				return
			}
			if tc.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("err = %v, want one containing %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if res.Lyrics != tc.wantLyrics || res.Level != tc.wantLevel {
				t.Errorf("lyrics/level = %q/%v, want %q/%v",
					res.Lyrics, res.Level, tc.wantLyrics, tc.wantLevel)
			}
			if res.Title != tc.wantTitle || res.Artist != tc.wantArtist ||
				res.Album != tc.wantAlbum || res.Filled != tc.wantFilled {
				t.Errorf("metadata = %+v, want %q/%q/%q filled %d",
					res, tc.wantTitle, tc.wantArtist, tc.wantAlbum, tc.wantFilled)
			}
		})
	}
}

func newAdapter(tp *testutil.Transport) *lrclib.Adapter {
	a := lrclib.New()
	a.HTTPClient = &http.Client{Transport: tp}
	return a
}
