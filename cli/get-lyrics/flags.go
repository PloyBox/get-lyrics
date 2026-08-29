package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/fetch"
)

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
