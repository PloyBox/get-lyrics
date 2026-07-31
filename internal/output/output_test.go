package output

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

func TestWrite_PlainWritesLyrics(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, source.Result{Lyrics: "hello\n"}, ModePlain)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if buf.String() != "hello\n" {
		t.Fatalf("got %q; want %q", buf.String(), "hello\n")
	}
}

func TestWrite_SyncedWritesSyncedLyrics(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, source.Result{SyncedLyrics: "[00:00.00] hi\n"}, ModeSynced)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if buf.String() != "[00:00.00] hi\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestWrite_PropagatesWriterError(t *testing.T) {
	ew := errWriter{err: errors.New("disk full")}
	err := Write(ew, source.Result{Lyrics: "x"}, ModePlain)
	if err == nil || err.Error() != "disk full" {
		t.Fatalf("err = %v; want disk full", err)
	}
}

func TestWrite_EmptyLyricsIsNoop(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, source.Result{}, ModePlain)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("buf = %q; want empty", buf.String())
	}
}

func TestWrite_SyncedButEmptyReturnsErr(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, source.Result{Lyrics: "plain only"}, ModeSynced)
	if !errors.Is(err, ErrEmptySynced) {
		t.Fatalf("err = %v; want ErrEmptySynced", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("buf should remain empty, got %q", buf.String())
	}
}

// errWriter satisfies io.Writer and always returns the configured error.
type errWriter struct{ err error }

func (e errWriter) Write(_ []byte) (int, error) { return 0, e.err }

// Ensure errWriter still satisfies io.Writer even when unused.
var _ io.Writer = errWriter{}
