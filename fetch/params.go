package fetch

import (
	"fmt"
	"math"
	"time"

	"github.com/PloyBox/get-lyrics/source"
)

// MaxSeconds is the largest whole-second timeout whose multiplication by
// time.Second stays positive; a larger value wraps in a plain multiply.
const MaxSeconds = int64(math.MaxInt64) / int64(time.Second)

// Params bundles all CLI inputs the fetch layer needs. Source and
// SyncLevels are priority-ordered; Lenient affects the precheck only.
type Params struct {
	Song       string
	Source     []string
	Author     string
	Album      string
	ISRC       string
	Duration   int64 // whole seconds; 0 means not provided
	SyncLevels []SyncLevel
	Lenient    bool
	UserAgent  string
	Custom     map[string]string
	Timeout    int64 // per-source call timeout in seconds; 0 = no timeout
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
