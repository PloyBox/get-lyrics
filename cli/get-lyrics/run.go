// Command get-lyrics fetches song lyrics from a registered source.
// See docs/refs/cli.md.
package main

import (
	"context"
	"encoding/json"
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
)

// Run is the testable core: argv excludes the program name; stdout and
// stderr are explicit writers. It returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) (code int) {
	parsed, song, err := parseFlags(argv)
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
	if song == "" {
		fmt.Fprintln(stderr, "error[usage]: song title is required")
		printUsage(stderr, nil, nil)
		return exitUsage
	}

	svc := fetch.New(registry)
	params := parsedFlagsToParams(parsed, song)

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
		// compare the path's current inode with the open fd first so a
		// file that replaced ours is never deleted.
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
	if parsed.json {
		data, merr := json.Marshal(struct {
			FormatVersion int `json:"formatVersion"`
			fetch.Result
		}{FormatVersion: 1, Result: res})
		if merr != nil {
			fmt.Fprintln(stderr, "error[output]:", merr)
			return exitOutputFailed
		}
		data = append(data, '\n')
		if _, werr := out.Write(data); werr != nil {
			fmt.Fprintln(stderr, "error[output]:", werr)
			return exitOutputFailed
		}
	} else if _, werr := io.WriteString(out, res.Lyrics); werr != nil {
		fmt.Fprintln(stderr, "error[output]:", werr)
		return exitOutputFailed
	}
	return exitOK
}

func main() {
	code := Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}
