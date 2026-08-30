// Command get-lyrics fetches song lyrics from a registered source.
//
// Usage: get-lyrics --source <name> [--author <name>] [--album <name>]
//
//	[--iswc <code>] [--duration <secs>] [--output <file>]
//	[--user-agent <ua>] [--sync-level <levels>] <song>
//
// Use --help or -h for the same summary plus the list of registered sources.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/PloyBox/get-lyrics/bootstrap"
	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

// version is stamped at release build time via
// -ldflags "-X main.version=<tag>"; "dev" is the local-build default.
var version = "dev"

// defaultUserAgent is the User-Agent the CLI sends on every upstream
// request unless the caller overrides it with --user-agent. The version
// is injected automatically (from version, stamped at build time) — it
// replaces the UA the built-in sources previously hardcoded; the sources
// now trust whatever they are handed.
func defaultUserAgent() string {
	return "get-lyrics/" + version + " (+https://github.com/PloyBox/get-lyrics)"
}

// registry is populated at package-init time so RegisterAll runs
// before main() — matches the "init()-style" plan without hidden globals
// inside adapter packages.
var registry = mustRegisterAll()

func mustRegisterAll() *source.Registry {
	r := source.NewRegistry()
	if err := bootstrap.RegisterAll(r); err != nil {
		// Adapter init failure is a programmer error; bail out before
		// any CLI handling runs.
		panic(fmt.Sprintf("get-lyrics: source registration failed: %v", err))
	}
	return r
}

// Exit codes documented for shell consumers:
//
//	0 → success (stderr may still carry warnings)
//	2 → usage error (missing song, unknown/typo flag, invalid --sync-level value)
//	3 → unknown source (strict precheck)
//	4 → no valid result: every source skipped (lenient) or failed, or no
//	     result matched the requested sync levels
//	5 → output failure (file open, write, or close)
//	6 → source-required parameter missing in strict precheck (e.g. the
//	     caller did not supply --author to a source that requires it)
//	7 → --output points to an existing file and --overwrite was not given
//	8 → duplicate --source entry (strict precheck)
const (
	exitOK           = 0
	exitUsage        = 2
	exitUnknownSrc   = 3
	exitFetchFailed  = 4
	exitOutputFailed = 5
	exitRequired     = 6
	exitFileExists   = 7
	exitDuplicateSrc = 8
)

// Run is the testable core: it takes argv (excluding the program name)
// and explicit writers for stdout/stderr, returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) (code int) {
	parsed, song, err := parseFlags(argv)
	if err != nil {
		fmt.Fprintln(stderr, "error[usage]:", err)
		printUsage(stderr, nil, nil)
		return exitUsage
	}
	if parsed.help {
		// Full declaration for rendering only; no env fallback. Lenient
		// mode and the sorted registry names make this query infallible.
		decls, _ := fetch.New(registry).CustomParamsFor(fetch.Params{Source: registry.Names(), Lenient: true})
		printUsage(stdout, registry, decls)
		return exitOK
	}
	if parsed.version {
		fmt.Fprintf(stdout, "get-lyrics %s\n", version)
		return exitOK
	}
	if song == "" {
		fmt.Fprintln(stderr, "error[usage]: song title is required")
		printUsage(stderr, nil, nil)
		return exitUsage
	}

	svc := fetch.New(registry)
	params := parsedFlagsToParams(parsed, song)

	// Static declarations for the requested sources (exit 3/8 in strict
	// mode, same codes as the fetch precheck but reported first), then
	// fill any undeclared-by-flag key from the process environment.
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

	out, closer, created, err := openOutput(parsed.output, parsed.overwrite, stdout)
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
		// Only files this process created via O_EXCL are ever removed;
		// before removing, compare the path's current inode with the
		// open fd so a file that replaced ours is never deleted.
		same := false
		if created {
			if f, ok := out.(*os.File); ok {
				fi1, e1 := f.Stat()
				fi2, e2 := os.Lstat(parsed.output)
				same = e1 == nil && e2 == nil && os.SameFile(fi1, fi2)
			}
		}
		cerr := closer()
		if code == exitOK && cerr != nil {
			fmt.Fprintln(stderr, "error[output]:", cerr)
			code = exitOutputFailed
		}
		// A failed run must not leave a freshly created empty file
		// behind; pre-existing files are never touched here.
		if code != exitOK && created && same {
			if rerr := os.Remove(parsed.output); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
				fmt.Fprintf(stderr, "warning[cleanup]: remove %q: %v\n", parsed.output, rerr)
			}
		}
	}()

	res, warnings, err := svc.Fetch(context.Background(), params)
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
		// Failure path: in-flight warnings still tell the user why each
		// source was skipped or failed, printed before the error.
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

	// Only the real output file is truncated here — stdout (the
	// fallback) must never be truncated or seeked.
	if parsed.output != "" {
		if f, ok := out.(*os.File); ok {
			if err := f.Truncate(0); err != nil {
				fmt.Fprintln(stderr, "error[output]:", err)
				return exitOutputFailed
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				fmt.Fprintln(stderr, "error[output]:", err)
				return exitOutputFailed
			}
		}
	}
	if _, werr := io.WriteString(out, res.Lyrics); werr != nil {
		fmt.Fprintln(stderr, "error[output]:", werr)
		return exitOutputFailed
	}
	return exitOK
}

func main() {
	code := Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}
