//go:build test

package bootstrap

import (
	"github.com/PloyBox/get-lyrics/internal/provider/mock/custom"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/fail"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/lrc"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/mismatch"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/nosupport"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/nosync"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/require"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/success"
	"github.com/PloyBox/get-lyrics/internal/provider/mock/synconly"
	"github.com/PloyBox/get-lyrics/internal/source"
)

// RegisterAllMock registers every mock/test-only adapter into r.
// Use in place of RegisterAll when running tests that need stub sources.
func RegisterAllMock(r *source.Registry) error {
	adapters := []source.Source{
		success.New(),
		require.New(),
		nosupport.New(),
		fail.New(),
		lrc.New(),
		nosync.New(),
		synconly.New(),
		mismatch.New(),
		custom.New(),
	}
	for _, a := range adapters {
		if err := r.Register(a); err != nil {
			return err
		}
	}
	return nil
}
