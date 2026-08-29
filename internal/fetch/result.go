package fetch

import (
	"strings"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// Result is the fetch layer's output, consolidating the two lyrics
// tracks into a single field. Level records which track Lyrics
// carries: SyncNone for plain text, SyncLine for synced (LRC) content,
// SyncUnknown when neither track was populated.
//
// Source always names the adapter that produced the result. SubSource
// is the sub-source identifier reported by aggregate sources (e.g. a
// multi-provider aggregator); standalone adapters leave it empty.
type Result struct {
	Lyrics    string
	Title     string
	Artist    string
	Album     string
	ISWC      string
	Source    string // adapter that produced the result
	SubSource string // sub-source for aggregate adapters, copied only when the adapter declared FieldSubSource; empty otherwise
	Level     SyncLevel
}

// filtResult converts an adapter result into the fetch.Result tracks it
// legitimately contains. Field population follows the adapter's Filled
// mask — never string contents: a field whose bit is unset is treated
// as empty. A declared-and-populated Lyrics yields one SyncNone track,
// a declared-and-populated SyncedLyrics one SyncLine track; both
// together yield two tracks sharing the adapter's metadata.
//
// Matching is level-based: when any produced track's Level equals want,
// it is returned as match and nothing is stored — the caller returns
// immediately. Only when nothing matches does the caller receive every
// produced track as storable for the per-call cache: want decides
// matching, never storage. When the adapter populated neither lyrics
// track (despite declaring one — a detectResultMismatch case), nothing
// is produced and nothing is stored.
func filtResult(srcName string, sr source.Result, want SyncLevel) (match *Result, storable []Result) {
	var tracks []Result
	addTrack := func(bit source.ResultField, text string, level SyncLevel) {
		if sr.Filled&bit == 0 || strings.TrimSpace(text) == "" {
			return
		}
		r := Result{Source: srcName, Lyrics: text, Level: level}
		if sr.Filled&source.FieldTitle != 0 {
			r.Title = sr.Title
		}
		if sr.Filled&source.FieldArtist != 0 {
			r.Artist = sr.Artist
		}
		if sr.Filled&source.FieldAlbum != 0 {
			r.Album = sr.Album
		}
		if sr.Filled&source.FieldISWC != 0 {
			r.ISWC = sr.ISWC
		}
		if sr.Filled&source.FieldSubSource != 0 {
			r.SubSource = sr.SubSource
		}
		tracks = append(tracks, r)
	}
	addTrack(source.FieldLyrics, sr.Lyrics, SyncNone)
	addTrack(source.FieldSyncedLyrics, sr.SyncedLyrics, SyncLine)
	if len(tracks) == 0 {
		return nil, nil
	}
	for i := range tracks {
		if tracks[i].Level == want {
			return &tracks[i], nil
		}
	}
	return nil, tracks
}

// findCached returns the first cached track produced by name whose
// Level matches want, or nil.
func findCached(cache []Result, name string, want SyncLevel) *Result {
	for i := range cache {
		if cache[i].Source == name && cache[i].Level == want {
			return &cache[i]
		}
	}
	return nil
}
