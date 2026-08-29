package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// userAgentSource is a test-only fixture that echoes the incoming
// Request.UserAgent so a CLI run can prove --user-agent reaches the
// adapter. It registers no custom params and requires nothing.
type userAgentSource struct{ name string }

func (u *userAgentSource) Name() string { return u.name }
func (u *userAgentSource) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}
func (u *userAgentSource) CustomParams() []source.ParamSpec { return nil }
func (u *userAgentSource) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		Lyrics: "[user-agent] " + req.UserAgent + "\n",
		Title:  req.Song,
		Filled: source.FieldLyrics | source.FieldTitle,
	}, nil
}

// TestRun_UserAgentReachesAdapter drives the --user-agent flag end to
// end: the header value the user supplies must reach the adapter's
// Request.UserAgent.
func TestRun_UserAgentReachesAdapter(t *testing.T) {
	if err := registry.Register(&userAgentSource{name: "mock-useragent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-useragent") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-useragent", "--user-agent", "CustomUA/1.0", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[user-agent] CustomUA/1.0") {
		t.Fatalf("stdout missing user-agent value: %q", stdout.String())
	}
}

// TestRun_UserAgentShortFlagWorks proves the -u short form is accepted.
func TestRun_UserAgentShortFlagWorks(t *testing.T) {
	if err := registry.Register(&userAgentSource{name: "mock-useragent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-useragent") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-useragent", "-u", "ShortUA/2.0", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[user-agent] ShortUA/2.0") {
		t.Fatalf("stdout missing user-agent value: %q", stdout.String())
	}
}

// TestRun_UserAgentDefaultReachesAdapter proves an unset --user-agent
// falls back to the CLI's default UA (version-injected) rather than an
// empty value — the sources no longer supply their own default.
func TestRun_UserAgentDefaultReachesAdapter(t *testing.T) {
	if err := registry.Register(&userAgentSource{name: "mock-useragent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-useragent") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-useragent", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[user-agent] "+defaultUserAgent()) {
		t.Fatalf("stdout should echo the default user-agent %q: %q", defaultUserAgent(), stdout.String())
	}
}
