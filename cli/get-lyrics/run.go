// Command get-lyrics fetches song lyrics from a registered source.
// See docs/refs/cli.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PloyBox/get-lyrics/bootstrap"
	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

// version is stamped at release build time via
// -ldflags "-X main.version=<tag>"; "dev" is the local-build default.
var version = "dev"

func defaultUserAgent() string {
	return "get-lyrics/" + version + " (+https://github.com/PloyBox/get-lyrics)"
}

// registry is populated at package-init time, before main() runs.
var registry = mustRegisterAll()

func mustRegisterAll() *source.Registry {
	r := source.NewRegistry()
	if err := bootstrap.RegisterAll(r); err != nil {
		// Registration failure is a programmer error.
		panic(fmt.Sprintf("get-lyrics: source registration failed: %v", err))
	}
	return r
}

const (
	exitOK           = 0
	exitUsage        = 2
	exitUnknownSrc   = 3
	exitFetchFailed  = 4
	exitOutputFailed = 5
	exitRequired     = 6
	exitFileExists   = 7
	exitDuplicateSrc = 8
	exitTimeout      = 9
)

// Run is the testable core: argv excludes the program name; stdout and
// stderr are explicit writers. It returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	return RunContext(context.Background(), argv, stdout, stderr)
}

// RunContext is Run with a caller-supplied cancellation context.
func RunContext(ctx context.Context, argv []string, stdout, stderr io.Writer) (code int) {
	parsed, positional, err := parseFlags(argv)
	if parsed.quiet {
		stderr = io.Discard
	}
	if err != nil {
		fmt.Fprintln(stderr, "error[usage]:", err)
		printUsage(stderr, nil, nil)
		return exitUsage
	}
	if parsed.help {
		// Declaration for rendering only; no env fallback.
		decls, _ := fetch.New(registry).CustomParamsFor(fetch.Params{Source: registry.Names(), Lenient: true})
		printUsage(stdout, registry, decls)
		return exitOK
	}
	if parsed.version {
		fmt.Fprintf(stdout, "get-lyrics %s\n", version)
		return exitOK
	}
	if len(positional) > 0 {
		fmt.Fprintf(stderr, "error[usage]: unexpected positional argument %q (use --name for the song title)\n", positional[0])
		printUsage(stderr, nil, nil)
		return exitUsage
	}
	if parsed.song == "" {
		fmt.Fprintln(stderr, "error[usage]: song title is required (use --name)")
		printUsage(stderr, nil, nil)
		return exitUsage
	}

	svc := fetch.New(registry)
	params, perr := parsedFlagsToParams(parsed)
	if perr != nil {
		fmt.Fprintln(stderr, "error[usage]:", perr)
		printUsage(stderr, nil, nil)
		return exitUsage
	}

	decls, err := svc.CustomParamsFor(params)
	if err != nil {
		var dupErr fetch.DuplicateSourceError
		if errors.As(err, &dupErr) {
			fmt.Fprintln(stderr, "error[usage]:", dupErr.Error())
			return exitDuplicateSrc
		}
		if errors.Is(err, source.ErrNotFound) {
			fmt.Fprintln(stderr, "error[unknown]:", err.Error())
			return exitUnknownSrc
		}
		fmt.Fprintln(stderr, "error[fetch]:", err)
		return exitFetchFailed
	}
	params.Custom = mergeEnv(params.Custom, decls)

	sink, err := openOutput(parsed.output, parsed.overwrite, stdout)
	if err != nil {
		var existsErr outputExistsError
		if errors.As(err, &existsErr) {
			fmt.Fprintln(stderr, "error[output]:", err)
			return exitFileExists
		}
		fmt.Fprintln(stderr, "error[output]:", err)
		return exitOutputFailed
	}
	defer func() {
		if aerr := sink.abort(); aerr != nil {
			fmt.Fprintf(stderr, "warning[cleanup]: %v\n", aerr)
		}
	}()

	// The whole-fetch budget rides on the context: when it ends, fetch
	// stops and reports a StoppedError.
	if parsed.timeoutGlobal > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(parsed.timeoutGlobal)*time.Second)
		defer cancel()
	}

	res, warnings, err := svc.Fetch(ctx, params)
	var dupErr fetch.DuplicateSourceError
	if errors.As(err, &dupErr) {
		// In-flight warnings (e.g. a gate-2 source-bug warning emitted
		// before the strict abort) are printed before the error.
		for _, w := range warnings {
			fmt.Fprintln(stderr, renderWarning(w))
		}
		fmt.Fprintln(stderr, "error[usage]:", dupErr.Error())
		return exitDuplicateSrc
	}
	if errors.Is(err, source.ErrNotFound) {
		for _, w := range warnings {
			fmt.Fprintln(stderr, renderWarning(w))
		}
		fmt.Fprintln(stderr, "error[unknown]:", err.Error())
		return exitUnknownSrc
	}
	var stopErr fetch.StoppedError
	if errors.As(err, &stopErr) {
		for _, w := range warnings {
			fmt.Fprintln(stderr, renderWarning(w))
		}
		fmt.Fprintln(stderr, "error[timeout]:", stopErr.Error())
		return exitTimeout
	}
	var reqErr fetch.RequiredParamError
	if errors.As(err, &reqErr) {
		for _, w := range warnings {
			fmt.Fprintln(stderr, renderWarning(w))
		}
		fmt.Fprintln(stderr, "error[required]:", renderRequiredError(reqErr))
		return exitRequired
	}
	var noRes fetch.NoResultError
	if errors.As(err, &noRes) {
		for _, w := range warnings {
			fmt.Fprintln(stderr, renderWarning(w))
		}
		fmt.Fprintln(stderr, "error[no-result]:", noRes.Error())
		return exitFetchFailed
	}
	if err != nil {
		for _, w := range warnings {
			fmt.Fprintln(stderr, renderWarning(w))
		}
		fmt.Fprintln(stderr, "error[fetch]:", err)
		return exitFetchFailed
	}

	for _, w := range warnings {
		fmt.Fprintln(stderr, renderWarning(w))
	}

	if parsed.json {
		var level string
		switch res.Level {
		case fetch.SyncUnknown:
			level = "unknown"
		case fetch.SyncNone:
			level = "none"
		case fetch.SyncLine:
			level = "line"
		case fetch.SyncWord:
			level = "word"
		}
		data, merr := json.Marshal(struct {
			FormatVersion int `json:"formatVersion"`
			Level         string
			fetch.Result
		}{FormatVersion: 2, Level: level, Result: res})
		if merr != nil {
			fmt.Fprintln(stderr, "error[output]:", merr)
			return exitOutputFailed
		}
		data = append(data, '\n')
		if _, werr := sink.Write(data); werr != nil {
			fmt.Fprintln(stderr, "error[output]:", werr)
			return exitOutputFailed
		}
	} else if _, werr := io.WriteString(sink, res.Lyrics); werr != nil {
		fmt.Fprintln(stderr, "error[output]:", werr)
		return exitOutputFailed
	}
	if err := sink.commit(); err != nil {
		var existsErr outputExistsError
		if errors.As(err, &existsErr) {
			fmt.Fprintln(stderr, "error[output]:", err)
			return exitFileExists
		}
		fmt.Fprintln(stderr, "error[output]:", err)
		return exitOutputFailed
	}
	return exitOK
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := RunContext(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
