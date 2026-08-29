//go:build test

// Test-only wiring: registers the mock/test-only sources so that
// binaries built with `-tags test` (and the test suite itself) can
// exercise them. Production builds exclude this file.
package main

import (
	"github.com/PloyBox/get-lyrics/internal/bootstrap"
)

func init() {
	if err := bootstrap.RegisterAllMock(registry); err != nil {
		panic(err)
	}
}
