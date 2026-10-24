//go:build test

package bootstrap

import (
	"github.com/PloyBox/get-lyrics/internal/provider/mock"
	"github.com/PloyBox/get-lyrics/source"
)

// RegisterAllMock registers every mock adapter; test builds only.
func RegisterAllMock(r *source.Registry) error {
	for _, src := range mock.All() {
		if err := r.Register(src); err != nil {
			return err
		}
	}
	return nil
}
