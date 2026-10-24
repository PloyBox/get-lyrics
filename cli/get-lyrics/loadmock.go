//go:build test

// Test-only wiring: registers every mock adapter via RegisterAllMock.
// Production builds exclude this file.
package main

import "github.com/PloyBox/get-lyrics/bootstrap"

func init() {
	if err := bootstrap.RegisterAllMock(registry); err != nil {
		panic(err)
	}
}
