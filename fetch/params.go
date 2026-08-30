package fetch

import (
	"fmt"

	"github.com/PloyBox/get-lyrics/source"
)

// Params bundles all CLI inputs the fetch layer needs. Source is the
// ordered list of source names to try (failover order). SyncLevels is
// the ordered list of requested sync levels (SyncLine → synced,
// SyncNone → plain) — the CLI parses the --sync-level level names into
// it; the first match wins. SyncUnknown is not a legal request value;
// precheck rejects it with InvalidSyncLevelError. Lenient controls the
// precheck stage only: when false, the first precheck problem aborts; when true,
// problem sources are skipped with a PreCheck warning.
type Params struct {
	Song       string
	Source     []string
	Author     string
	Album      string
	ISRC       string
	Duration   int // whole seconds; 0 means not provided
	SyncLevels []SyncLevel
	Lenient    bool
	// UserAgent is the HTTP User-Agent header to send on upstream
	// requests (from --user-agent). It is passed to every requested
	// source; empty means the source uses its own default UA.
	UserAgent string
	// Custom carries the user-supplied --env keys (plus process-
	// environment fallbacks injected by the CLI). Keys the caller did
	// not provide are absent; env-injected keys are treated exactly
	// like user-provided ones.
	Custom map[string]string
}

// SyncLevel classifies the lyrics content a fetch.Result carries by its
// sync level.
type SyncLevel uint8

const (
	// SyncUnknown: unknown / no valid lyrics content (neither lyrics
	// track was populated).
	SyncUnknown SyncLevel = iota
	// SyncNone: plain (non-timestamped) lyrics.
	SyncNone
	// SyncLine: synced (LRC timestamped) lyrics.
	SyncLine
)

// requestFromParams projects the CLI params onto a source.Request for
// capability queries. SyncLevel is deliberately omitted — zero value is
// SyncNone (plain), synced output is a runtime property.
// Custom is projected so capability queries see the user-supplied keys
// — conditional recognition/requirements (e.g. mock-custom's COUNTRY
// depending on LANG) would otherwise never hold in precheck and
// detectUnsupported.
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

// sourceSyncLevel maps a requested fetch.SyncLevel onto its
// source.Request counterpart one-to-one. SyncUnknown never reaches
// this point: precheck rejects it before the fetch loop runs.
func sourceSyncLevel(want SyncLevel) source.SyncLevel {
	switch want {
	case SyncNone:
		return source.SyncNone
	case SyncLine:
		return source.SyncLine
	}
	panic(fmt.Sprintf("fetch: invalid sync level %d (precheck must reject SyncUnknown)", want))
}
