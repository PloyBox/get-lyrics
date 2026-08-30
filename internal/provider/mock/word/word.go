//go:build test

package word

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-word" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch returns pseudo TTML for a word-level request and plain lyrics
// otherwise, exercising the SyncWord path end to end.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if req.SyncLevel == source.SyncWord {
		return source.Result{
			Lyrics: "<tt><body><div><p begin=\"00:00:01.000\" end=\"00:00:02.000\">" +
				"<span begin=\"00:00:01.000\" end=\"00:00:01.500\">[mock-word] </span>" +
				"<span begin=\"00:00:01.500\" end=\"00:00:02.000\">for: " + req.Song + "</span></p></div></body></tt>",
			Level:  source.SyncWord,
			Filled: source.FieldLyrics,
		}, nil
	}
	return source.Result{
		Lyrics: "[mock-word] lyrics for: " + req.Song + "\n",
		Level:  source.SyncNone,
		Filled: source.FieldLyrics,
	}, nil
}
