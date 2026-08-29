// Command get-lyrics fetches song lyrics from a registered source.
//
// Usage: get-lyrics --source <name> [--author <name>] [--album <name>]
//
//	[--iswc <code>] [--output <file>] [--user-agent <ua>] [--sync-level <levels>] <song>
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
	"io/fs"
	"os"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/bootstrap"
	"github.com/PloyBox/get-lyrics/internal/fetch"
	"github.com/PloyBox/get-lyrics/internal/source"
)

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

// parsedFlags holds the parsed CLI inputs. song is kept separate because
// it is a positional argument, not a flag.
type parsedFlags struct {
	source     string
	author     string
	album      string
	iswc       string
	output     string
	syncLevels []fetch.SyncLevel // parsed from --sync-level at parse time
	userAgent  string
	lenient    bool
	overwrite  bool
	help       bool
	version    bool
	env        map[string]string // validated --env keys; validated at parse time
}

// envList collects repeated --env key=value flags. flag.Value calls Set
// once per occurrence, so both --env LANG=en and --env=LANG=en work.
type envList []string

func (e *envList) String() string { return strings.Join(*e, ",") }

func (e *envList) Set(value string) error {
	*e = append(*e, value)
	return nil
}

// outputExistsError reports that --output points to an existing file
// while --overwrite was not given. Run maps it to exit code 7.
type outputExistsError struct{ path string }

func (e outputExistsError) Error() string {
	return fmt.Sprintf("file %q already exists (use --overwrite to replace it)", e.path)
}

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
			fmt.Fprintln(stderr, w.Message)
		}
		fmt.Fprintln(stderr, "error[usage]:", dupErr.Error())
		return exitDuplicateSrc
	}
	if errors.Is(err, source.ErrNotFound) {
		for _, w := range warnings {
			fmt.Fprintln(stderr, w.Message)
		}
		fmt.Fprintln(stderr, "error[unknown]:", err.Error())
		return exitUnknownSrc
	}
	var reqErr fetch.RequiredParamError
	if errors.As(err, &reqErr) {
		for _, w := range warnings {
			fmt.Fprintln(stderr, w.Message)
		}
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
		for _, w := range warnings {
			fmt.Fprintln(stderr, w.Message)
		}
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
	return exitOK
}

func main() {
	code := Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

// parsedFlagsToParams converts the raw CLI flags and positional song
// argument into the fetch.Params struct, without applying defaults
// (those live in parseFlags). Source lists are trimmed and empty
// entries dropped here; sync levels are already parsed into SyncLevels
// by parseSyncLevels.
func parsedFlagsToParams(f parsedFlags, song string) fetch.Params {
	return fetch.Params{
		Song:       song,
		Source:     splitTrimmed(f.source),
		Author:     f.author,
		Album:      f.album,
		ISWC:       f.iswc,
		SyncLevels: f.syncLevels,
		UserAgent:  f.userAgent,
		Lenient:    f.lenient,
		Custom:     f.env,
	}
}

// validateEnv validates the collected --env entries at parse time and
// returns them as a key→value map. Each entry is split on the first '=';
// the key must match ParamNamePattern, the value must be non-empty after
// trimming (a whitespace-only value counts as empty, mirroring the typed
// params' TrimSpace semantics), and duplicate keys are rejected. Any
// violation is a usage error (exit 2).
func validateEnv(envs envList) (map[string]string, error) {
	out := make(map[string]string, len(envs))
	for _, entry := range envs {
		key, value, _ := strings.Cut(entry, "=")
		if !source.ValidParamName(key) {
			return nil, fmt.Errorf("invalid --env key %q (must match %s)", key, source.ParamNamePattern)
		}
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("invalid --env entry %q: value must be non-empty", entry)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate --env key %q", key)
		}
		out[key] = value
	}
	return out, nil
}

// mergeEnv fills every key any requested source declares from the
// process environment when the user did not supply it via --env.
// Precedence: --env > environment > missing. An environment variable
// that exists but is empty (e.g. LANG=) counts as missing and is not
// injected. Injected keys are treated exactly like user-provided ones —
// a source that does not declare the key still warns unsupported.
func mergeEnv(custom map[string]string, decls map[string][]source.ParamSpec) map[string]string {
	for _, specs := range decls {
		for _, spec := range specs {
			if _, provided := custom[spec.Name]; provided {
				continue
			}
			if v, ok := os.LookupEnv(spec.Name); ok && strings.TrimSpace(v) != "" {
				if custom == nil {
					custom = make(map[string]string)
				}
				custom[spec.Name] = v
			}
		}
	}
	return custom
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

// parseSyncLevels converts a comma-separated --sync-level value into
// the ordered SyncLevels the fetch layer consumes: "line" → SyncLine,
// "none" → SyncNone. Whitespace around entries is trimmed and empty
// entries are dropped; any other value is a usage error (exit 2).
func parseSyncLevels(s string) ([]fetch.SyncLevel, error) {
	parts := strings.Split(s, ",")
	out := make([]fetch.SyncLevel, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "":
			continue
		case "line":
			out = append(out, fetch.SyncLine)
		case "none":
			out = append(out, fetch.SyncNone)
		default:
			return nil, fmt.Errorf("invalid sync level value %q (want \"line\" or \"none\")", p)
		}
	}
	return out, nil
}

// printUsage writes the help text. Examples use the long (--) form per
// the plan; the underlying flag library also accepts short forms.
// decls, when non-nil, feeds the "Source parameters:" section: a
// per-source list of the static --env keys and their descriptions.
func printUsage(w io.Writer, reg *source.Registry, decls map[string][]source.ParamSpec) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "Usage: get-lyrics [--source <names>] [--author <name>] [--album <name>]")
	fmt.Fprintln(&b, "                   [--iswc <code>] [--output <file>] [--user-agent <ua>] [--sync-level <levels>] <song>")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Options:")
	fmt.Fprintln(&b, "  --source <names>, -s <names> Lyrics source names (default: lrclib)")
	fmt.Fprintln(&b, "  --author <name>,  -a <name>  Author / artist filter")
	fmt.Fprintln(&b, "  --album <name>,   -A <name>  Album filter")
	fmt.Fprintln(&b, "  --iswc <code>,    -i <code>  ISWC identifier")
	fmt.Fprintln(&b, "  --output <file>,  -o <file>  Write lyrics to file (default: stdout; refuses to overwrite an existing file)")
	fmt.Fprintln(&b, "  --overwrite, -O               Overwrite an existing --output file")
	fmt.Fprintln(&b, "  --sync-level <levels>, -S <levels> Sync levels (default: line,none)")
	fmt.Fprintf(&b, "  --user-agent <ua>, -u <ua>    User-Agent header for HTTP requests (default: %s)\n", defaultUserAgent())
	fmt.Fprintln(&b, "  --env <key=value>, -e <key=value> Custom source parameter (repeatable; key must match ^[A-Z][A-Z0-9_]*$)")
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
		any := false
		var paramsBuf bytes.Buffer
		for _, n := range reg.Names() {
			specs := decls[n]
			if len(specs) == 0 {
				continue
			}
			any = true
			fmt.Fprintf(&paramsBuf, "  %s:\n", n)
			for _, spec := range specs {
				fmt.Fprintf(&paramsBuf, "    --env %-10s %s\n", spec.Name, spec.Description)
			}
		}
		if any {
			fmt.Fprintln(&b, "")
			fmt.Fprintln(&b, "Source parameters:")
			_, _ = paramsBuf.WriteTo(&b)
		}
	}
	_, _ = io.Copy(w, &b)
}

// openOutput returns the lyrics sink: stdout when path is empty, an
// os.File when path is set. A new file is created exclusively
// (O_CREATE|O_EXCL) and reported via created so the caller can remove
// it on failure. An existing file is only opened when overwrite is set,
// and never with O_TRUNC — truncation happens only after a successful
// fetch, so a failed run leaves existing content intact. The caller
// must invoke the closer.
func openOutput(path string, overwrite bool, fallback io.Writer) (io.Writer, func() error, bool, error) {
	if path == "" {
		return fallback, func() error { return nil }, false, nil
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		// WARNING: O_EXCL is not completely safe; file races cannot be
		// fully eliminated.
		if err == nil {
			// Created by this process: on failure the caller removes it.
			return f, f.Close, true, nil
		}
		if !os.IsExist(err) {
			return nil, func() error { return nil }, false, err
		}
		if !overwrite {
			return nil, func() error { return nil }, false, outputExistsError{path}
		}
		// The file exists; reopen it without O_CREATE and without
		// O_TRUNC (the caller truncates only after a successful fetch).
		f, err = os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			return f, f.Close, false, nil
		}
		lastErr = err
		if !os.IsNotExist(err) {
			return nil, func() error { return nil }, false, err
		}
		// The file vanished between the two opens; retry the exclusive
		// create.
	}
	return nil, func() error { return nil }, false, lastErr
}

// parseFlags handles both -x/--x forms using Go flag's default
// behavior: parsing stops at the first positional argument, so flags
// must precede it. Unknown flags become a non-nil error which Run maps
// to exitUsage.
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
	var syncLevel string
	fs.StringVar(&syncLevel, "sync-level", "line,none", "synced (line) or plain (none) lyrics")
	fs.StringVar(&syncLevel, "S", "line,none", "synced (line) or plain (none) lyrics (short)")
	fs.StringVar(&f.userAgent, "user-agent", defaultUserAgent(), "User-Agent header for HTTP requests")
	fs.StringVar(&f.userAgent, "u", defaultUserAgent(), "User-Agent header for HTTP requests (short)")
	fs.BoolVar(&f.lenient, "lenient", false, "skip invalid sources instead of failing")
	fs.BoolVar(&f.lenient, "l", false, "skip invalid sources instead of failing (short)")
	fs.BoolVar(&f.overwrite, "overwrite", false, "overwrite an existing output file")
	fs.BoolVar(&f.overwrite, "O", false, "overwrite an existing output file (short)")
	fs.BoolVar(&f.help, "help", false, "show help")
	fs.BoolVar(&f.help, "h", false, "show help (short)")
	fs.BoolVar(&f.version, "version", false, "print version and exit")
	fs.BoolVar(&f.version, "v", false, "print version and exit (short)")
	var envs envList
	fs.Var(&envs, "env", "custom source parameter key=value (repeatable)")
	fs.Var(&envs, "e", "custom source parameter key=value (repeatable, short)")

	if err := fs.Parse(argv); err != nil {
		return parsedFlags{}, "", err
	}
	syncLevels, err := parseSyncLevels(syncLevel)
	if err != nil {
		return parsedFlags{}, "", err
	}
	f.syncLevels = syncLevels
	env, err := validateEnv(envs)
	if err != nil {
		return parsedFlags{}, "", err
	}
	f.env = env
	positional := fs.Args()
	if len(positional) == 0 {
		return f, "", nil
	}
	return f, strings.Join(positional, " "), nil
}
