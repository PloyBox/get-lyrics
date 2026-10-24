//go:build test

package main_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const (
	mockPlainLyrics = "plain lyrics line one\nplain lyrics line two"
	mockTTMLLyrics  = `<tt xmlns="http://www.w3.org/ns/ttml">` +
		`<body><p>word one</p></body></tt>`
	defaultUA = "get-lyrics/dev (+https://github.com/PloyBox/get-lyrics)"
)

// TestSuccess locks byte-for-byte plain lyrics on stdout with clean stderr.
func TestSuccess(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"plain", stdArgs("mock-plain")},
		{"trimmed source list", stdArgs(" mock-plain , ")},
		{"trimmed sync levels", []string{"--name", "x", "--source", "mock-plain",
			"--sync-level", " none , , "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, nil, tc.args...)
			if code != 0 || stderr != "" {
				t.Fatalf("code/stderr = %d/%q, want 0 and empty", code, stderr)
			}
			if stdout != mockPlainLyrics {
				t.Errorf("stdout = %q, want %q", stdout, mockPlainLyrics)
			}
		})
	}
}

// TestJSON locks the FormatVersion, the nine keys and the trailing newline.
func TestJSON(t *testing.T) {
	code, stdout, stderr := run(t, nil, stdArgs("mock-plain", "--json")...)
	if code != 0 || stderr != "" {
		t.Fatalf("code/stderr = %d/%q, want 0 and empty", code, stderr)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &keys); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	wantKeys := []string{
		"FormatVersion", "Level", "Lyrics", "Title", "Artist",
		"Album", "ISRC", "Source", "SubSource",
	}
	if len(keys) != len(wantKeys) {
		t.Errorf("top-level key count = %d, want %d: %v",
			len(keys), len(wantKeys), keys)
	}
	for _, k := range wantKeys {
		if _, ok := keys[k]; !ok {
			t.Errorf("missing top-level key %q", k)
		}
	}
	var doc struct {
		FormatVersion         int
		Level, Lyrics, Source string
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode typed document: %v", err)
	}
	if doc.FormatVersion != 3 || doc.Level != "none" ||
		doc.Lyrics != mockPlainLyrics || doc.Source != "mock-plain" {
		t.Errorf("document = %+v, want version 3 plain mock-plain lyrics", doc)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout = %q, want a trailing newline", stdout)
	}
}

// TestSyncLevels locks level matching, downgrade warnings and no-result.
func TestSyncLevels(t *testing.T) {
	const noResult = "error[no-result]: no source returned a valid result\n"
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"default downgrades to plain", srcArgs("mock-plain"), 0, mockPlainLyrics,
			`warning[downgraded]: source "mock-plain" returned no synced lyrics` +
				"\n"},
		{"line on plain", srcArgs("mock-plain", "--sync-level", "line"), 4, "",
			`warning[downgraded]: source "mock-plain" returned no synced lyrics` +
				"\n" + noResult},
		{"none on line", srcArgs("mock-lrc", "--sync-level", "none"), 4, "",
			`warning[downgraded]: source "mock-lrc" returned only synced lyrics` +
				"\n" + noResult},
		{"word on line", srcArgs("mock-lrc", "--sync-level", "word"), 4, "",
			`warning[downgraded]: source "mock-lrc" returned no word-synced lyrics` +
				"\n" + noResult},
		{"word hit", srcArgs("mock-word", "--sync-level", "word"), 0,
			mockTTMLLyrics, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, nil, tc.args...)
			if code != tc.wantCode || stdout != tc.wantOut || stderr != tc.wantErr {
				t.Errorf("code/stdout/stderr = %d/%q/%q, want %d/%q/%q",
					code, stdout, stderr, tc.wantCode, tc.wantOut, tc.wantErr)
			}
		})
	}
}

// capturedRequest mirrors the mock-capture JSON payload.
type capturedRequest struct {
	Song      string
	Author    string
	Album     string
	ISRC      string
	Duration  int64
	SyncLevel uint8
	UserAgent string
	Custom    map[string]string
}

// TestRequestFields locks field pass-through, duration and User-Agent.
func TestRequestFields(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want capturedRequest
	}{
		{"metadata pass-through", []string{"--name", "Song title", "--author",
			"Artist", "--album", "Album", "--isrc", "ISRC123"},
			capturedRequest{Song: "Song title", Author: "Artist", Album: "Album",
				ISRC: "ISRC123", UserAgent: defaultUA}},
		{"duration mm:ss", []string{"--name", "x", "--duration", "3:45"},
			capturedRequest{Song: "x", Duration: 225, UserAgent: defaultUA}},
		{"duration at max", []string{"--name", "x", "--duration", "153722867:16"},
			capturedRequest{Song: "x", Duration: 9223372036, UserAgent: defaultUA}},
		{"user agent override", []string{"--name", "x", "--user-agent",
			"Custom/1.0"},
			capturedRequest{Song: "x", UserAgent: "Custom/1.0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argv := append([]string{"--source", "mock-capture",
				"--sync-level", "none"}, tc.args...)
			code, stdout, stderr := run(t, nil, argv...)
			if code != 0 || stderr != "" {
				t.Fatalf("code/stderr = %d/%q, want 0 and empty", code, stderr)
			}
			var req capturedRequest
			if err := json.Unmarshal([]byte(stdout), &req); err != nil {
				t.Fatalf("decode request: %v (stdout %q)", err, stdout)
			}
			req.Custom = nil // custom keys are covered by TestEnv
			if !reflect.DeepEqual(req, tc.want) {
				t.Errorf("request = %+v, want %+v", req, tc.want)
			}
		})
	}
}
