//go:build test

package bootstrap

import "github.com/PloyBox/get-lyrics/source"

// RegisterAllMock no longer registers any adapter: all mocks were removed.
// It stays as a no-op so test-tag wiring keeps compiling.
func RegisterAllMock(*source.Registry) error {
	return nil
}
