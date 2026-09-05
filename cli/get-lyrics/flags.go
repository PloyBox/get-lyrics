package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PloyBox/get-lyrics/fetch"
)

// parsedFlags holds the parsed CLI inputs. song is kept separate because
// it is a positional argument, not a flag.
type parsedFlags struct {
	source     string
	author     string
	album      string
	isrc       string
	duration   int // whole seconds; normalized from --duration at parse time
	output     string
	json       bool
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
	fs.StringVar(&f.isrc, "isrc", "", "ISRC identifier")
	fs.StringVar(&f.isrc, "i", "", "ISRC identifier (short)")
	var durationRaw string
	fs.StringVar(&durationRaw, "duration", "", "track duration filter (seconds or mm:ss)")
	fs.StringVar(&durationRaw, "d", "", "track duration filter (seconds or mm:ss) (short)")
	fs.StringVar(&f.output, "output", "", "output file path")
	fs.StringVar(&f.output, "o", "", "output file path (short)")
	fs.BoolVar(&f.json, "json", false, "write complete result as JSON")
	fs.BoolVar(&f.json, "j", false, "write complete result as JSON (short)")
	var syncLevel string
	fs.StringVar(&syncLevel, "sync-level", "line,none", "synced (line/word) or plain (none) lyrics")
	fs.StringVar(&syncLevel, "S", "line,none", "synced (line/word) or plain (none) lyrics (short)")
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
	duration, err := parseDuration(durationRaw)
	if err != nil {
		return parsedFlags{}, "", err
	}
	f.duration = duration
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
// "word" → SyncWord, "none" → SyncNone. Whitespace around entries is
// trimmed and empty entries are dropped; any other value is a usage
// error (exit 2).
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
		case "word":
			out = append(out, fetch.SyncWord)
		case "none":
			out = append(out, fetch.SyncNone)
		default:
			return nil, fmt.Errorf("invalid sync level value %q (want \"line\", \"word\" or \"none\")", p)
		}
	}
	return out, nil
}

// parseDuration converts a --duration value into whole seconds: a plain
// positive integer ("225") or mm:ss ("3:45"). Whitespace-only input is
// treated as not provided (0, mirroring the --author whitespace
// precedent); any other value is a usage error (exit 2).
func parseDuration(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if !strings.Contains(s, ":") {
		sec, err := strconv.Atoi(s)
		if err != nil || sec < 1 {
			return 0, fmt.Errorf("invalid duration value %q (want seconds or mm:ss)", s)
		}
		return sec, nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid duration value %q (want seconds or mm:ss)", s)
	}
	m, errM := strconv.Atoi(parts[0])
	sec, errS := strconv.Atoi(parts[1])
	if errM != nil || errS != nil || m < 0 || sec < 0 || sec > 59 || m*60+sec < 1 {
		return 0, fmt.Errorf("invalid duration value %q (want seconds or mm:ss)", s)
	}
	return m*60 + sec, nil
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
		ISRC:       f.isrc,
		Duration:   f.duration,
		SyncLevels: f.syncLevels,
		UserAgent:  f.userAgent,
		Lenient:    f.lenient,
		Custom:     f.env,
	}
}
