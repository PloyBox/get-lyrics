package fetch

import (
	"fmt"

	"github.com/PloyBox/get-lyrics/source"
)

// NoResultError is returned when every source was skipped or failed and
// no result matched any requested sync level. Exit code 4.
type NoResultError struct{}

func (NoResultError) Error() string { return "no source returned a valid result" }

// InvalidSyncLevelError is returned by precheck when Params.SyncLevels
// contains SyncUnknown, which classifies results but is not a
// requestable level — a caller bug, so it aborts in BOTH modes.
type InvalidSyncLevelError struct{}

func (InvalidSyncLevelError) Error() string {
	return "invalid sync level: SyncUnknown is not requestable"
}

// UnknownSourceError identifies the requested but unregistered source
// name. Exit code 3; unwraps to source.ErrNotFound.
type UnknownSourceError struct {
	Name string
}

func (e UnknownSourceError) Error() string {
	return fmt.Sprintf("source %q not found", e.Name)
}

func (e UnknownSourceError) Unwrap() error { return source.ErrNotFound }

// DuplicateSourceError identifies a source name listed more than once.
// Exit code 8.
type DuplicateSourceError struct {
	Name string
}

func (e DuplicateSourceError) Error() string {
	return fmt.Sprintf("source %q is listed more than once", e.Name)
}

// RequiredParamError reports the first required parameter the caller did
// not supply. Exit code 6.
type RequiredParamError struct {
	Source    string       // adapter Name() that requires the parameter
	Param     source.Param // typed Param bit; 0 for a custom key
	ParamName string       // custom key name; empty for a typed parameter
}

// Error renders a neutral message; the CLI renders the user-facing text
// from the structured fields.
func (e RequiredParamError) Error() string {
	return fmt.Sprintf("source %q requires a parameter the caller did not supply", e.Source)
}
