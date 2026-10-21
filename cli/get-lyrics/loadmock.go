//go:build test

// Test-only wiring: calls bootstrap.RegisterAllMock, currently a no-op.
// Production builds exclude this file.
package main

import "github.com/PloyBox/get-lyrics/bootstrap"

func init() {
	if err := bootstrap.RegisterAllMock(registry); err != nil {
		panic(err)
	}
}
