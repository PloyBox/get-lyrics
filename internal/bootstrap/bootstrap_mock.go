//go:build test

package bootstrap

import (
	"github.com/PloyBox/get-lyrics/internal/source"
	"github.com/PloyBox/get-lyrics/internal/source/mock/custom"
	"github.com/PloyBox/get-lyrics/internal/source/mock/fail"
	"github.com/PloyBox/get-lyrics/internal/source/mock/lrc"
	"github.com/PloyBox/get-lyrics/internal/source/mock/mismatch"
	"github.com/PloyBox/get-lyrics/internal/source/mock/nosupport"
	"github.com/PloyBox/get-lyrics/internal/source/mock/nosync"
	"github.com/PloyBox/get-lyrics/internal/source/mock/require"
	"github.com/PloyBox/get-lyrics/internal/source/mock/success"
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
