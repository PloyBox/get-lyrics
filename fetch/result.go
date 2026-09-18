package fetch

import (
	"strings"

	"github.com/PloyBox/get-lyrics/source"
)

// Result is the fetch layer's output: one lyrics track plus its source
// attribution and sync level.
type Result struct {
	Lyrics    string
	Title     string
	Artist    string
	Album     string
	ISRC      string
	Source    string // adapter that produced the result
	SubSource string // sub-source for aggregate adapters, else empty
	Level     SyncLevel
}

// resultLevel maps the adapter-declared source.SyncLevel onto its fetch
// counterpart one-to-one; an unknown value (a source bug) becomes
// SyncUnknown, which can never match a request.
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

// filtResult builds the fetch.Result track an adapter result legitimately
// contains, following the adapter's Filled mask only — never string
// contents. A track whose Level equals want is returned as match and not
// stored; any other track is returned as storable for the per-call cache.
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

// findCached returns the first cached track from name whose Level matches
// want, or nil.
func findCached(cache []Result, name string, want SyncLevel) *Result {
	for i := range cache {
		if cache[i].Source == name && cache[i].Level == want {
			return &cache[i]
		}
	}
	return nil
}
