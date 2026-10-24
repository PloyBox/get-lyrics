// Package testutil holds test-only helpers: non-test files must not import it.
// It has no test build tag, so a production import would link testing silently.
package testutil

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// Transport is a test-only http.RoundTripper: it records every request it
// receives and hands each response decision to Respond.
type Transport struct {
	// Respond builds the response for one request.
	Respond func(*http.Request) (*http.Response, error)
	// Requests holds every request in call order.
	Requests []*http.Request
}

// RoundTrip records req, then returns the response Respond builds for it.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.Requests = append(t.Requests, req)
	return t.Respond(req)
}

// Last returns the most recent request, or nil when none was recorded.
func (t *Transport) Last() *http.Request {
	if len(t.Requests) == 0 {
		return nil
	}
	return t.Requests[len(t.Requests)-1]
}

// Respond builds a fixed-status JSON response carrying body.
func Respond(status int, body string) func(*http.Request) (*http.Response, error) {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

// Capabilities returns src's capabilities for req after checking that the
// request-aware declarations stay within the static list.
func Capabilities(t *testing.T, src source.Source,
	req source.Request,
) source.Capabilities {
	t.Helper()
	caps := src.Capabilities(req)
	static := make(map[string]bool)
	for _, spec := range src.CustomParams() {
		static[spec.Name] = true
	}
	declared := make(map[string]bool)
	for _, spec := range caps.Custom {
		declared[spec.Name] = true
		if !static[spec.Name] {
			t.Errorf("Custom key %q missing from CustomParams()", spec.Name)
		}
	}
	for _, name := range caps.RequiredCustom {
		if !declared[name] {
			t.Errorf("RequiredCustom %q missing from Custom", name)
		}
	}
	return caps
}
