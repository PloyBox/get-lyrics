//go:build test

package bootstrap

import (
	"github.com/PloyBox/get-lyrics/internal/source"
	"github.com/PloyBox/get-lyrics/internal/source/mock/stub"
)

// RegisterAllMock registers every mock/test-only adapter into r.
// Use in place of RegisterAll when running tests that need stub sources.
func RegisterAllMock(r *source.Registry) error {
	if err := r.Register(stub.New()); err != nil {
		return err
	}
	return nil
}
