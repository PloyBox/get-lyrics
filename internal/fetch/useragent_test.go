package fetch

import (
	"context"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// TestFetch_PassesUserAgentToRequest locks the data flow:
// params.UserAgent reaches the adapter's Request.UserAgent verbatim,
// including an empty value — the fetch layer never substitutes a default;
// the CLI owns the default UA now.
func TestFetch_PassesUserAgentToRequest(t *testing.T) {
	var gotUA string
	seen := &fakeSrc{
		name: "seen",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			gotUA = r.UserAgent
			return source.Result{Lyrics: "L", Filled: source.FieldLyrics}, nil
		},
	}
	r := newRegistry(t, seen)
	svc := New(r)

	_, _, err := svc.Fetch(context.Background(), Params{Song: "S", UserAgent: "custom/1.0", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"seen"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotUA != "custom/1.0" {
		t.Fatalf("adapter UserAgent = %q; want %q", gotUA, "custom/1.0")
	}

	// Empty params.UserAgent must propagate as empty — the fetch layer
	// never invents a default; the CLI owns it.
	_, _, err = svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"seen"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotUA != "" {
		t.Fatalf("adapter UserAgent = %q; want empty for default", gotUA)
	}
}
