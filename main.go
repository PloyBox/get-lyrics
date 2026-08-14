// Command get-lyrics fetches song lyrics from a registered source.
//
// Usage: get-lyrics --source <name> [--author <name>] [--album <name>]
//
//	[--iswc <code>] [--output <file>] [--timestamp] <song>
//
// Use --help or -h for the same summary plus the list of registered sources.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/bootstrap"
	"github.com/PloyBox/get-lyrics/internal/fetch"
	"github.com/PloyBox/get-lyrics/internal/source"
)

// Exit codes documented for shell consumers:
//
//	0 → success (stderr may still carry warnings)
//	2 → usage error (missing song, unknown/typo flag, invalid --timestamp value)
//	3 → unknown source (strict precheck)
//	4 → no valid result: every source skipped (lenient) or failed, or no
//	     result matched the requested timestamp formats
//	5 → output failure (file create or write)
//	6 → source-required parameter missing in strict precheck (e.g. the
//	     caller did not supply --author to a source that requires it)
const (
	exitOK           = 0
	exitUsage        = 2
	exitUnknownSrc   = 3
	exitFetchFailed  = 4
	exitOutputFailed = 5
	exitRequired     = 6
)

// version is stamped at release build time via
// -ldflags "-X main.version=<tag>"; "dev" is the local-build default.
var version = "dev"

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

// parsedFlags holds the parsed CLI inputs. song is kept separate because
// it is a positional argument, not a flag.
type parsedFlags struct {
	source    string
	author    string
	album     string
	iswc      string
	output    string
	timestamp string
	lenient   bool
	help      bool
	version   bool
}

// Run is the testable core: it takes argv (excluding the program name)
// and explicit writers for stdout/stderr, returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	parsed, song, err := parseFlags(argv)
	if err != nil {
		fmt.Fprintln(stderr, "error[usage]:", err)
		printUsage(stderr, nil)
		return exitUsage
	}
	if parsed.help {
		printUsage(stdout, registry)
		return exitOK
	}
	if parsed.version {
		fmt.Fprintf(stdout, "get-lyrics %s\n", version)
		return exitOK
	}
	if song == "" {
		fmt.Fprintln(stderr, "error[usage]: song title is required")
		printUsage(stderr, nil)
		return exitUsage
	}

	out, closer, err := openOutput(parsed.output, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "error[output]:", err)
		return exitOutputFailed
	}
	defer func() { _ = closer() }()

	svc := fetch.New(registry)
	params := parsedFlagsToParams(parsed, song)
	res, warnings, err := svc.Fetch(context.Background(), params)
	var dupErr fetch.DuplicateSourceError
	if errors.As(err, &dupErr) {
		fmt.Fprintln(stderr, "error[usage]:", dupErr.Error())
		return exitUsage
	}
	if errors.Is(err, source.ErrNotFound) {
		fmt.Fprintln(stderr, "error[unknown]:", err.Error())
		return exitUnknownSrc
	}
	var reqErr source.RequiredParamError
	if errors.As(err, &reqErr) {
		fmt.Fprintln(stderr, "error[required]:", reqErr.Error())
		return exitRequired
	}
	var noRes fetch.NoResultError
	if errors.As(err, &noRes) {
		// Failure path: in-flight warnings still tell the user why each
		// source was skipped or failed, printed before the error.
		for _, w := range warnings {
			fmt.Fprintln(stderr, w.Message)
		}
		fmt.Fprintln(stderr, "error[no-result]:", noRes.Error())
		return exitFetchFailed
	}
	if err != nil {
		fmt.Fprintln(stderr, "error[fetch]:", err)
		return exitFetchFailed
	}

	for _, w := range warnings {
		fmt.Fprintln(stderr, w.Message)
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
	if err := closer(); err != nil {
		fmt.Fprintln(stderr, "error[output]:", err)
		return exitOutputFailed
	}
	return exitOK
}

func main() {
	code := Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

// parsedFlagsToParams converts the raw CLI flags and positional song
// argument into the fetch.Params struct, without applying defaults
// (those live in parseFlags). Source and timestamp lists are trimmed and
// empty entries dropped here so they match validateTimestamp's behavior.
func parsedFlagsToParams(f parsedFlags, song string) fetch.Params {
	return fetch.Params{
		Song:      song,
		Source:    splitTrimmed(f.source),
		Author:    f.author,
		Album:     f.album,
		ISWC:      f.iswc,
		Timestamp: splitTrimmed(f.timestamp),
		Lenient:   f.lenient,
	}
}

// splitTrimmed splits a comma-separated flag value, trimming whitespace
// and dropping empty entries.
func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validateTimestamp rejects any comma-separated --timestamp value other
// than "line" or "none"; empty entries are ignored.
func validateTimestamp(s string) error {
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if v != "line" && v != "none" {
			return fmt.Errorf("invalid timestamp value %q (want \"line\" or \"none\")", v)
		}
	}
	return nil
}

// printUsage writes the help text. Examples use the long (--) form per
// the plan; the underlying flag library also accepts short forms.
func printUsage(w io.Writer, reg *source.Registry) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "Usage: get-lyrics [--source <names>] [--author <name>] [--album <name>]")
	fmt.Fprintln(&b, "                   [--iswc <code>] [--output <file>] [--timestamp <fmts>] <song>")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Options:")
	fmt.Fprintln(&b, "  --source <names>, -s <names> Lyrics source names (default: lrclib)")
	fmt.Fprintln(&b, "  --author <name>,  -a <name>  Author / artist filter")
	fmt.Fprintln(&b, "  --album <name>,   -A <name>  Album filter")
	fmt.Fprintln(&b, "  --iswc <code>,    -i <code>  ISWC identifier")
	fmt.Fprintln(&b, "  --output <file>,  -o <file>  Write lyrics to file (default: stdout)")
	fmt.Fprintln(&b, "  --timestamp <fmts>, -t <fmts> Timestamp formats (default: line,none)")
	fmt.Fprintln(&b, "  --lenient, -l               Skip invalid sources instead of failing")
	fmt.Fprintln(&b, "  --help, -h                   Show this help and exit")
	fmt.Fprintln(&b, "  --version                    Print version and exit")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Positionals:")
	fmt.Fprintln(&b, "  <song>                       Song title (required)")
	if reg != nil {
		fmt.Fprintln(&b, "")
		fmt.Fprintln(&b, "Available sources:")
		for _, n := range reg.Names() {
			fmt.Fprintf(&b, "  %s\n", n)
		}
	}
	_, _ = io.Copy(w, &b)
}

// openOutput returns the lyrics sink: stdout when path is empty, an
// os.File when path is set. The file is opened for writing without
// O_TRUNC so a fetch failure leaves existing content intact; the caller
// truncates it only after a successful fetch. The caller must invoke the
// closer.
func openOutput(path string, fallback io.Writer) (io.Writer, func() error, error) {
	if path == "" {
		return fallback, func() error { return nil }, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return f, f.Close, nil
}

// parseFlags handles both -x/--x and POSIX-mixed positional/flag order,
// using Go flag's default behavior. Unknown flags become a non-nil
// error which Run maps to exitUsage.
func parseFlags(argv []string) (parsedFlags, string, error) {
	fs := flag.NewFlagSet("get-lyrics", flag.ContinueOnError)
	// Silence flag's own usage writer; Run writes its own on error.
	fs.SetOutput(io.Discard)
	var f parsedFlags
	fs.StringVar(&f.source, "source", "lrclib", "lyrics source name")
	fs.StringVar(&f.source, "s", "lrclib", "lyrics source name (short)")
	fs.StringVar(&f.author, "author", "", "author/artist filter")
	fs.StringVar(&f.author, "a", "", "author/artist filter (short)")
	fs.StringVar(&f.album, "album", "", "album filter")
	fs.StringVar(&f.album, "A", "", "album filter (short)")
	fs.StringVar(&f.iswc, "iswc", "", "ISWC identifier")
	fs.StringVar(&f.iswc, "i", "", "ISWC identifier (short)")
	fs.StringVar(&f.output, "output", "", "output file path")
	fs.StringVar(&f.output, "o", "", "output file path (short)")
	fs.StringVar(&f.timestamp, "timestamp", "line,none", "request timestamped lyrics")
	fs.StringVar(&f.timestamp, "t", "line,none", "request timestamped lyrics (short)")
	fs.BoolVar(&f.lenient, "lenient", false, "skip invalid sources instead of failing")
	fs.BoolVar(&f.lenient, "l", false, "skip invalid sources instead of failing (short)")
	fs.BoolVar(&f.help, "help", false, "show help")
	fs.BoolVar(&f.help, "h", false, "show help (short)")
	fs.BoolVar(&f.version, "version", false, "print version and exit")
	fs.BoolVar(&f.version, "v", false, "print version and exit (short)")

	if err := fs.Parse(argv); err != nil {
		return parsedFlags{}, "", err
	}
	if err := validateTimestamp(f.timestamp); err != nil {
		return parsedFlags{}, "", err
	}
	positional := fs.Args()
	if len(positional) == 0 {
		return f, "", nil
	}
	return f, strings.Join(positional, " "), nil
}
