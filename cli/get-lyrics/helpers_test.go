//go:build test

package main_test

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"testing"

	app "github.com/PloyBox/get-lyrics/cli/get-lyrics"
)

// offlineTransport rejects every request: CLI tests never use the network.
type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("tests must not perform real network requests")
}

// TestMain blocks accidental real network requests for the whole package.
func TestMain(m *testing.M) {
	http.DefaultTransport = offlineTransport{}
	os.Exit(m.Run())
}

// run executes app.Run with env applied and returns code and output.
func run(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	applyEnv(t, env)
	var out, errOut bytes.Buffer
	code = app.Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// applyEnv clears the mock-only keys, then sets every env entry.
func applyEnv(t *testing.T, env map[string]string) {
	t.Helper()
	t.Setenv("MOCK_KEY", "")
	t.Setenv("MOCK_OPT", "")
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// srcArgs builds name and source argv plus extra flags.
func srcArgs(source string, extra ...string) []string {
	return append([]string{"--name", "x", "--source", source}, extra...)
}

// stdArgs is srcArgs with the explicit plain sync level.
func stdArgs(source string, extra ...string) []string {
	return srcArgs(source, append([]string{"--sync-level", "none"}, extra...)...)
}
