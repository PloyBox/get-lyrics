package main

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// durationSource is a test-only fixture that echoes the incoming
// Request.Duration so a CLI run can prove --duration reaches the
// adapter as whole seconds. It declares the Duration filter and
// requires nothing else.
type durationSource struct{ name string }

func (d *durationSource) Name() string { return d.name }
func (d *durationSource) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Filters: source.ParamDuration}
}
func (d *durationSource) CustomParams() []source.ParamSpec { return nil }
func (d *durationSource) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		Lyrics: "[duration] " + strconv.Itoa(req.Duration) + "\n",
		Title:  req.Song,
		Filled: source.FieldLyrics | source.FieldTitle,
	}, nil
}

// TestRun_DurationMmssReachesAdapterAsSeconds drives --duration 3:45
// end to end: the CLI must normalize it to whole seconds (225) before
// the adapter sees it.
func TestRun_DurationMmssReachesAdapterAsSeconds(t *testing.T) {
	if err := registry.Register(&durationSource{name: "mock-duration"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-duration") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-duration", "--duration", "3:45", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[duration] 225") {
		t.Fatalf("stdout missing normalized duration: %q", stdout.String())
	}
}

// TestRun_DurationShortFlagWorks proves the -d short form is accepted
// and passes whole seconds straight through.
func TestRun_DurationShortFlagWorks(t *testing.T) {
	if err := registry.Register(&durationSource{name: "mock-duration"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-duration") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-duration", "-d", "225", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[duration] 225") {
		t.Fatalf("stdout missing duration value: %q", stdout.String())
	}
}
