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

	"github.com/PloyBox/get-lyrics/internal/fetch"
	"github.com/PloyBox/get-lyrics/internal/output"
	"github.com/PloyBox/get-lyrics/internal/source"
	"github.com/PloyBox/get-lyrics/internal/bootstrap"
)

// Exit codes documented for shell consumers:
//
//	0 → success (stderr may still carry warnings)
//	2 → usage error (missing song, unknown flag, etc.)
//	3 → unknown source
//	4 → fetch failure (network / parse / synced-empty invariant)
//	5 → output failure (file create or write)
//	6 → source-required parameter missing (e.g. an adapter refused
//	     because the caller did not supply --author/--iswc/…)
const (
	exitOK           = 0
	exitUsage        = 2
	exitUnknownSrc   = 3
	exitFetchFailed  = 4
	exitOutputFailed = 5
	exitRequired     = 6
)

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
	timestamp bool
	help      bool
}

// Run is the testable core: it takes argv (excluding the program name)
// and explicit writers for stdout/stderr, returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	parsed, song, err := parseFlags(argv)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		printUsage(stderr, nil)
		return exitUsage
	}
	if parsed.help {
		printUsage(stdout, registry)
		return exitOK
	}
	if song == "" {
		fmt.Fprintln(stderr, "error: song title is required")
		printUsage(stderr, nil)
		return exitUsage
	}

	out, closer, err := openOutput(parsed.output, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitOutputFailed
	}
	defer func() { _ = closer() }()

	svc := fetch.New(registry)
	req := source.Request{
		Song:      song,
		Author:    parsed.author,
		Album:     parsed.album,
		ISWC:      parsed.iswc,
		Timestamp: parsed.timestamp,
	}
	res, warnings, err := svc.Fetch(context.Background(), req, parsed.source)
	if errors.Is(err, source.ErrNotFound) {
		fmt.Fprintf(stderr, "error: unknown source %q\n", parsed.source)
		return exitUnknownSrc
	}
	var reqErr source.RequiredParamError
	if errors.As(err, &reqErr) {
		fmt.Fprintf(stderr, "error: source %q requires %s\n", reqErr.Source, reqErr.Flag)
		return exitRequired
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitFetchFailed
	}

	for _, w := range warnings {
		fmt.Fprintln(stderr, w.Message)
	}

	src, _ := registry.Get(parsed.source)
	mode := output.ModePlain
	if parsed.timestamp && src != nil && src.SupportedParams()&source.ParamTimestamp != 0 && res.SyncedLyrics != "" {
		mode = output.ModeSynced
	}
	if werr := output.Write(out, res, mode); werr != nil {
		fmt.Fprintln(stderr, "error:", werr)
		return exitOutputFailed
	}
	return exitOK
}

func main() {
	code := Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

// printUsage writes the help text. Examples use the long (--) form per
// the plan; the underlying flag library also accepts short forms.
func printUsage(w io.Writer, reg *source.Registry) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "Usage: get-lyrics --source <name> [--author <name>] [--album <name>]")
	fmt.Fprintln(&b, "                   [--iswc <code>] [--output <file>] [--timestamp] <song>")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Options:")
	fmt.Fprintln(&b, "  --source <name>, -s <name>   Lyrics source name (required)")
	fmt.Fprintln(&b, "  --author <name>, -a <name>   Author / artist filter")
	fmt.Fprintln(&b, "  --album <name>,  -A <name>   Album filter")
	fmt.Fprintln(&b, "  --iswc <code>,  -i <code>    ISWC identifier")
	fmt.Fprintln(&b, "  --output <file>, -o <file>   Write lyrics to file (default: stdout)")
	fmt.Fprintln(&b, "  --timestamp,    -t           Request timestamped (LRC) lyrics when supported")
	fmt.Fprintln(&b, "  --help, -h                   Show this help and exit")
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
// os.File when path is set. The caller must invoke the closer.
func openOutput(path string, fallback io.Writer) (io.Writer, func() error, error) {
	if path == "" {
		return fallback, func() error { return nil }, nil
	}
	f, err := os.Create(path)
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
	fs.StringVar(&f.source, "source", "", "lyrics source name")
	fs.StringVar(&f.source, "s", "", "lyrics source name (short)")
	fs.StringVar(&f.author, "author", "", "author/artist filter")
	fs.StringVar(&f.author, "a", "", "author/artist filter (short)")
	fs.StringVar(&f.album, "album", "", "album filter")
	fs.StringVar(&f.album, "A", "", "album filter (short)")
	fs.StringVar(&f.iswc, "iswc", "", "ISWC identifier")
	fs.StringVar(&f.iswc, "i", "", "ISWC identifier (short)")
	fs.StringVar(&f.output, "output", "", "output file path")
	fs.StringVar(&f.output, "o", "", "output file path (short)")
	fs.BoolVar(&f.timestamp, "timestamp", false, "request timestamped lyrics")
	fs.BoolVar(&f.timestamp, "t", false, "request timestamped lyrics (short)")
	fs.BoolVar(&f.help, "help", false, "show help")
	fs.BoolVar(&f.help, "h", false, "show help (short)")

	if err := fs.Parse(argv); err != nil {
		return parsedFlags{}, "", err
	}
	positional := fs.Args()
	if len(positional) == 0 {
		return f, "", nil
	}
	return f, positional[0], nil
}
