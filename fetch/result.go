package fetch

import (
	"strings"

	"github.com/PloyBox/get-lyrics/source"
)

// Result is the fetch layer's output. Level records the sync level of
// Lyrics: SyncNone for plain text, SyncLine for synced (LRC) content,
// SyncWord for word-level (TTML) content, SyncUnknown when no lyrics
// track was populated.
//
// Source always names the adapter that produced the result. SubSource
// is the sub-source identifier reported by aggregate sources (e.g. a
// multi-provider aggregator); standalone adapters leave it empty.
type Result struct {
	Lyrics    string
	Title     string
	Artist    string
	Album     string
	ISRC      string
	Source    string // adapter that produced the result
	SubSource string // sub-source for aggregate adapters, copied only when the adapter declared FieldSubSource; empty otherwise
	Level     SyncLevel
}

// resultLevel maps the adapter-declared source.SyncLevel onto its
// fetch.Result counterpart one-to-one. The two types deliberately do
// not share numeric values, so mapping is explicit. An unknown value
// (a source bug) maps to SyncUnknown — the result can then never match
// a request, which the cache/downgrade machinery handles safely.
func resultLevel(l source.SyncLevel) SyncLevel {
	switch l {
	case source.SyncNone:
		return SyncNone
	case source.SyncLine:
		return SyncLine
	case source.SyncWord:
		return SyncWord
	}
	return SyncUnknown
}

// filtResult converts an adapter result into the fetch.Result track it
// legitimately contains. Field population follows the adapter's Filled
// mask — never string contents: a field whose bit is unset is treated
// as empty.
//
// An adapter produces exactly one lyrics track per Fetch; Level is
// copied from the adapter's declaration via resultLevel. Matching is
// level-based: when the track's Level equals want, it is returned as
// match and nothing is stored — the caller returns immediately. Only
// when it does not match does the caller receive it as storable for
// the per-call cache: want decides matching, never storage. When the
// adapter declared FieldLyrics but left it empty (a detectResultMismatch
// case), nothing is produced and nothing is stored.
func filtResult(srcName string, sr source.Result, want SyncLevel) (match *Result, storable []Result) {
	if sr.Filled&source.FieldLyrics == 0 || strings.TrimSpace(sr.Lyrics) == "" {
		return nil, nil
	}
	r := Result{
		Source: srcName,
		Lyrics: sr.Lyrics,
		Level:  resultLevel(sr.Level),
	}
	if sr.Filled&source.FieldTitle != 0 {
		r.Title = sr.Title
	}
	if sr.Filled&source.FieldArtist != 0 {
		r.Artist = sr.Artist
	}
	if sr.Filled&source.FieldAlbum != 0 {
		r.Album = sr.Album
	}
	if sr.Filled&source.FieldISRC != 0 {
		r.ISRC = sr.ISRC
	}
	if sr.Filled&source.FieldSubSource != 0 {
		r.SubSource = sr.SubSource
	}
	if r.Level == want {
		return &r, nil
	}
	return nil, []Result{r}
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
