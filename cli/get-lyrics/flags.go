package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PloyBox/get-lyrics/fetch"
)

// parsedFlags holds the parsed CLI inputs.
type parsedFlags struct {
	song          string
	source        string
	author        string
	album         string
	isrc          string
	duration      int64 // whole seconds; normalized at parse time
	timeout       int64 // per-source request timeout in seconds; 0 = no timeout
	timeoutGlobal int64 // whole-fetch budget in seconds; 0 = off
	output        string
	json          bool
	syncLevels    []fetch.SyncLevel // parsed from --sync-level at parse time
	userAgent     string
	lenient       bool
	quiet         bool
	overwrite     bool
	help          bool
	version       bool
	env           map[string]string // validated at parse time
}

// parseFlags parses argv. An unknown flag maps to exitUsage; positional
// arguments are returned raw. help/version return before any validation.
func parseFlags(argv []string) (parsedFlags, []string, error) {
	fs := flag.NewFlagSet("get-lyrics", flag.ContinueOnError)
	// Silence flag's own usage writer; Run writes its own on error.
	fs.SetOutput(io.Discard)
	var f parsedFlags

	// repeats records each flag given more than once, in argv order; only
	// --env may be repeated.
	repeats := make([]string, 0, 1)

	addString := func(long, short, def, usage string, dst *string) {
		*dst = def
		seen := false
		set := func(v string) error {
			if seen {
				repeats = append(repeats, "--"+long)
			}
			seen = true
			*dst = v
			return nil
		}
		fs.Func(long, usage, set)
		fs.Func(short, usage+" (short)", set)
	}

	addBool := func(long, short, usage string, dst *bool) {
		seen := false
		set := func(v string) error {
			if seen {
				repeats = append(repeats, "--"+long)
			}
			seen = true
			b, err := strconv.ParseBool(v)
			if err != nil {
				return err
			}
			*dst = b
			return nil
		}
		fs.BoolFunc(long, usage, set)
		fs.BoolFunc(short, usage+" (short)", set)
	}

	var durationRaw, timeoutRaw, timeoutGlobalRaw, syncLevel string
	addString("name", "n", "", "song title", &f.song)
	addString("source", "s", "lrclib", "lyrics source name", &f.source)
	addString("author", "a", "", "author/artist filter", &f.author)
	addString("album", "A", "", "album filter", &f.album)
	addString("isrc", "i", "", "ISRC identifier", &f.isrc)
	addString("duration", "d", "", "track duration filter (seconds or mm:ss)", &durationRaw)
	addString("timeout", "t", "10", "per-source request timeout in seconds (0 = no timeout)", &timeoutRaw)
	addString("timeout-global", "T", "0", "overall fetch timeout in seconds (0 = off)", &timeoutGlobalRaw)
	addString("output", "o", "", "output file path", &f.output)
	addBool("json", "j", "write complete result as JSON", &f.json)
	addString("sync-level", "S", "line,none", "synced (line/word) or plain (none) lyrics", &syncLevel)
	addString("user-agent", "u", defaultUserAgent(), "User-Agent header for HTTP requests", &f.userAgent)
	addBool("lenient", "l", "skip invalid sources instead of failing", &f.lenient)
	addBool("quiet", "q", "suppress all stderr output", &f.quiet)
	addBool("overwrite", "O", "overwrite an existing output file", &f.overwrite)
	addBool("help", "h", "show help", &f.help)
	addBool("version", "v", "print version and exit", &f.version)

	var envs envList
	fs.Var(&envs, "env", "custom source parameter key=value (repeatable)")
	fs.Var(&envs, "e", "custom source parameter key=value (repeatable, short)")

	if err := fs.Parse(argv); err != nil {
		return f, nil, err
	}
	// help/version win: skip repeat and value validation.
	if f.help || f.version {
		return f, fs.Args(), nil
	}
	if len(repeats) > 0 {
		return f, nil, fmt.Errorf("flag %s given more than once (only --env may be repeated)", repeats[0])
	}
	f.song = strings.TrimSpace(f.song)
	f.source = strings.TrimSpace(f.source)
	syncLevels, err := parseSyncLevels(syncLevel)
	if err != nil {
		return f, nil, err
	}
	f.syncLevels = syncLevels
	duration, err := parseDuration(durationRaw)
	if err != nil {
		return f, nil, err
	}
	f.duration = duration
	env, err := validateEnv(envs)
	if err != nil {
		return f, nil, err
	}
	f.env = env
	timeout, err := parseTimeout(timeoutRaw)
	if err != nil {
		return f, nil, err
	}
	f.timeout = timeout
	timeoutGlobal, err := parseTimeout(timeoutGlobalRaw)
	if err != nil {
		return f, nil, err
	}
	f.timeoutGlobal = timeoutGlobal
	return f, fs.Args(), nil
}

// parseSyncLevels converts a comma-separated --sync-level value into the
// ordered SyncLevels the fetch layer consumes; entries are trimmed and
// empty ones dropped. Invalid values are usage errors (exit 2); an empty
// result is rejected later by parsedFlagsToParams.
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

// maxMinute/maxSecond bound an mm:ss value to fetch.MaxSeconds without
// multiplying first; the caller guarantees sec <= 59.
const (
	maxMinute = fetch.MaxSeconds / 60
	maxSecond = fetch.MaxSeconds % 60
)

// parseDuration turns a --duration value (seconds or mm:ss) into whole
// seconds; empty means 0 and anything outside 1-MaxSeconds is a usage error.
func parseDuration(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	bad := fmt.Errorf("invalid duration value %q (want 1-%d seconds or mm:ss)",
		s, fetch.MaxSeconds)
	if !strings.Contains(s, ":") {
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil || sec < 1 || sec > fetch.MaxSeconds {
			return 0, bad
		}
		return sec, nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, bad
	}
	m, errM := strconv.ParseInt(parts[0], 10, 64)
	sec, errS := strconv.ParseInt(parts[1], 10, 64)
	if errM != nil || errS != nil || m < 0 || sec < 0 || sec > 59 {
		return 0, bad
	}
	// Tuple compare: the overflow check must precede m*60.
	if m > maxMinute || (m == maxMinute && sec > maxSecond) {
		return 0, bad
	}
	total := m*60 + sec
	if total < 1 {
		return 0, bad
	}
	return total, nil
}

// parseTimeout turns a timeout flag value into whole seconds; empty means
// 0 (no timeout) and anything outside 0-MaxSeconds is a usage error.
func parseTimeout(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || n > fetch.MaxSeconds {
		return 0, fmt.Errorf("invalid timeout value %q (want 0-%d seconds)",
			s, fetch.MaxSeconds)
	}
	return n, nil
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

// parsedFlagsToParams converts the raw flags into fetch.Params; defaults
// live in parseFlags. It splits --source and rejects empty lists.
func parsedFlagsToParams(f parsedFlags) (fetch.Params, error) {
	sources := splitTrimmed(f.source)
	if len(sources) == 0 {
		return fetch.Params{}, fmt.Errorf(
			"invalid --source value %q: at least one source name is required", f.source)
	}
	if len(f.syncLevels) == 0 {
		return fetch.Params{}, errors.New(
			`invalid --sync-level value: at least one of "line", "word" or "none" is required`)
	}
	return fetch.Params{
		Song:       f.song,
		Source:     sources,
		Author:     f.author,
		Album:      f.album,
		ISRC:       f.isrc,
		Duration:   f.duration,
		SyncLevels: f.syncLevels,
		UserAgent:  f.userAgent,
		Lenient:    f.lenient,
		Custom:     f.env,
		Timeout:    f.timeout,
	}, nil
}
