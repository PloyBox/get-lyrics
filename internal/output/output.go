// Package output writes a fetched Result to an arbitrary io.Writer.
// main decides which Mode to use; this package only handles byte
// emission and the small invariant that SyncedLyrics must be non-empty
// when ModeSynced is requested.
package output

import (
	"errors"
	"io"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// Mode selects which lyrics track to write.
type Mode int

const (
	// ModePlain writes source.Result.Lyrics (always populated).
	ModePlain Mode = iota
	// ModeSynced writes source.Result.SyncedLyrics; returns
	// ErrEmptySynced if that field is empty (typically an adapter bug
	// rather than user error → surface as exit 4).
	ModeSynced
)

// ErrEmptySynced is returned by Write when ModeSynced is selected but
// the result has no SyncedLyrics.
var ErrEmptySynced = errors.New("output: synced lyrics requested but result has empty SyncedLyrics")

// Write writes the chosen track to w. Any I/O error from w is
// returned so main can exit with code 5.
func Write(w io.Writer, r source.Result, mode Mode) error {
	switch mode {
	case ModeSynced:
		if r.SyncedLyrics == "" {
			return ErrEmptySynced
		}
		_, err := io.WriteString(w, r.SyncedLyrics)
		return err
	default: // ModePlain
		_, err := io.WriteString(w, r.Lyrics)
		return err
	}
}
