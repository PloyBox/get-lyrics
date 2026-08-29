package fetch

import (
	"fmt"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// NoResultError is returned when every source was skipped or failed and
// no result matched any requested sync level. The CLI maps it to
// exit code 4 and prints the collected warnings first.
type NoResultError struct{}

func (NoResultError) Error() string { return "no source returned a valid result" }

// InvalidSyncLevelError is returned by precheck when Params.SyncLevels
// contains SyncUnknown, which classifies results but is not a
// requestable level — callers must request only SyncNone/SyncLine.
// It is a caller bug (the CLI rejects such values at parse time), so
// it aborts in BOTH strict and lenient mode.
type InvalidSyncLevelError struct{}

func (InvalidSyncLevelError) Error() string {
	return `invalid sync level "unknown" (want "line" or "none")`
}

// UnknownSourceError identifies the requested but unregistered source
// name. It unwraps to source.ErrNotFound so callers can match with
// errors.Is and still render the offending name.
type UnknownSourceError struct {
	Name string
}

func (e UnknownSourceError) Error() string {
	return fmt.Sprintf("source %q not found", e.Name)
}

func (e UnknownSourceError) Unwrap() error { return source.ErrNotFound }

// DuplicateSourceError identifies a source name listed more than once.
// The CLI maps it to a dedicated exit code (8); --lenient mode instead
// emits a PreCheck warning and drops the duplicate.
type DuplicateSourceError struct {
	Name string
}

func (e DuplicateSourceError) Error() string {
	return fmt.Sprintf("source %q is listed more than once", e.Name)
}

// RequiredParamError reports a source whose Capabilities.Required list
// (or RequiredCustom list) includes a parameter the caller did not
// supply. The precheck stage builds it for the first missing field;
// adapters never return it themselves. The CLI maps it to exit code 6.
type RequiredParamError struct {
	Source    string       // adapter Name() that requires the parameter
	Param     source.Param // typed Param bit; 0 for a custom key
	ParamName string       // custom key name; empty for a typed parameter
	Flag      string       // CLI flag spelling: "--author" etc., or "--env <KEY>"
}

// Error renders a stable message; main prints it verbatim after the
// error[required] tag.
func (e RequiredParamError) Error() string {
	return "source \"" + e.Source + "\" requires " + e.Flag
}
