package fetch

import (
	"fmt"

	"github.com/PloyBox/get-lyrics/source"
)

// Params bundles all CLI inputs the fetch layer needs. Source and
// SyncLevels are priority-ordered; Lenient affects the precheck only.
type Params struct {
	Song       string
	Source     []string
	Author     string
	Album      string
	ISRC       string
	Duration   int // whole seconds; 0 means not provided
	SyncLevels []SyncLevel
	Lenient    bool
	UserAgent  string
	Custom     map[string]string
}

// SyncLevel classifies the lyrics content a fetch.Result carries by its
// sync level.
type SyncLevel uint8

const (
	SyncUnknown SyncLevel = iota // no populated lyrics track
	SyncNone                     // plain (non-timestamped) lyrics
	SyncLine                     // synced (LRC timestamped) lyrics
	SyncWord                     // word-level (TTML timeline) lyrics
)

// requestFromParams projects Params onto a source.Request for capability
// queries. SyncLevel is omitted: its zero value is SyncNone, since
// synced output is a runtime property. Custom is projected so
// conditional declarations hold.
func requestFromParams(params Params) source.Request {
	return source.Request{
		Song:     params.Song,
		Author:   params.Author,
		Album:    params.Album,
		ISRC:     params.ISRC,
		Duration: params.Duration,
		Custom:   params.Custom,
	}
}

// sourceSyncLevel maps a requested fetch.SyncLevel onto its source
// counterpart one-to-one.
func sourceSyncLevel(want SyncLevel) source.SyncLevel {
	switch want {
	case SyncNone:
		return source.SyncNone
	case SyncLine:
		return source.SyncLine
	case SyncWord:
		return source.SyncWord
	}
	panic(fmt.Sprintf("fetch: invalid sync level %d (precheck must reject SyncUnknown)", want))
}
