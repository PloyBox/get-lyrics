package stub

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

func TestStub_Name(t *testing.T) {
	if got := New().Name(); got != "stub" {
		t.Fatalf("Name() = %q; want %q", got, "stub")
	}
}

func TestStub_SupportedParamsIsAuthorOnly(t *testing.T) {
	got := New().SupportedParams()
	if got&source.ParamAuthor == 0 {
		t.Fatalf("missing ParamAuthor bit: %b", got)
	}
	for _, p := range []source.Param{source.ParamAlbum, source.ParamISWC, source.ParamTimestamp} {
		if got&p != 0 {
			t.Fatalf("should not advertise %b", p)
		}
	}
}

func TestStub_FetchEchoesSongAndAuthor(t *testing.T) {
	res, err := New().Fetch(context.Background(), source.Request{
		Song:   "TEST_SONG",
		Author: "TEST_AUTHOR",
	})
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if res.Title != "TEST_SONG" {
		t.Fatalf("Title = %q; want %q", res.Title, "TEST_SONG")
	}
	if res.Artist != "TEST_AUTHOR" {
		t.Fatalf("Artist = %q; want %q", res.Artist, "TEST_AUTHOR")
	}
	if res.Source != "stub" {
		t.Fatalf("Source = %q; want %q", res.Source, "stub")
	}
	if res.Lyrics == "" || !strings.Contains(res.Lyrics, "TEST_SONG") {
		t.Fatalf("Lyrics = %q; want it to mention TEST_SONG", res.Lyrics)
	}
}

func TestStub_FetchMissingAuthorReturnsRequiredParamError(t *testing.T) {
	_, err := New().Fetch(context.Background(), source.Request{Song: "TEST_SONG"})
	if err == nil {
		t.Fatalf("Fetch should fail when Author is empty")
	}
	if !errors.Is(err, source.ErrRequiredParam) {
		t.Fatalf("err = %v; want it to match ErrRequiredParam", err)
	}
	var reqErr source.RequiredParamError
	if !errors.As(err, &reqErr) {
		t.Fatalf("err = %v; want it to be a RequiredParamError", err)
	}
	if reqErr.Param != source.ParamAuthor || reqErr.Flag != "--author" {
		t.Fatalf("reqErr = %+v; want Param=ParamAuthor, Flag=--author", reqErr)
	}
}
