package fetch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PloyBox/get-lyrics/source"
)

// blocking returns a source whose Fetch blocks until ctx is done, then
// reports the context error — simulating a hung upstream.
func blocking(name string) *fakeSrc {
	return &fakeSrc{
		name: name,
		fetch: func(ctx context.Context, _ source.Request) (source.Result, error) {
			<-ctx.Done()
			return source.Result{}, ctx.Err()
		},
	}
}

// TestFetch_GlobalDeadlineReturnsStopped locks the whole-fetch budget: a
// deadline on the caller's context makes Fetch stop and report a
// StoppedError wrapping context.DeadlineExceeded.
func TestFetch_GlobalDeadlineReturnsStopped(t *testing.T) {
	r := newRegistry(t, blocking("slow"))
	svc := New(r)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := svc.Fetch(ctx, Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"slow"}})

	var stop StoppedError
	if !errors.As(err, &stop) {
		t.Fatalf("err = %v; want StoppedError", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v; want wrapping context.DeadlineExceeded", err)
	}
}

// TestFetch_CanceledContextReturnsStopped locks the "asked to stop"
// semantics: a canceled caller context also stops the fetch immediately.
func TestFetch_CanceledContextReturnsStopped(t *testing.T) {
	r := newRegistry(t, blocking("slow"))
	svc := New(r)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := svc.Fetch(ctx, Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"slow"}})

	var stop StoppedError
	if !errors.As(err, &stop) {
		t.Fatalf("err = %v; want StoppedError", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want wrapping context.Canceled", err)
	}
}

// TestFetch_PerSourceTimeoutFailsOver locks the per-source budget: it
// expires only that call, so fetch fails over to the next source instead
// of stopping.
func TestFetch_PerSourceTimeoutFailsOver(t *testing.T) {
	ok := &fakeSrc{
		name: "ok",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L", Filled: source.FieldLyrics}, nil
		},
	}
	r := newRegistry(t, blocking("bad"), ok)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{
		Song:       "S",
		SyncLevels: []SyncLevel{SyncNone},
		Source:     []string{"bad", "ok"},
		Timeout:    1,
	})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if res.Source != "ok" || res.Lyrics != "L" {
		t.Fatalf("res = %+v; want result from ok", res)
	}
	if len(warnings) != 1 || warnings[0].Kind != FetchFailed || warnings[0].Source != "bad" {
		t.Fatalf("warnings = %+v; want one FetchFailed warning for bad", warnings)
	}
}
