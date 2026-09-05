package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/PloyBox/get-lyrics/source"
)

// printUsage writes the help text. Examples use the long (--) form per
// the plan; the underlying flag library also accepts short forms.
// decls, when non-nil, feeds the "Source parameters:" section: a
// per-source list of the static --env keys and their descriptions.
func printUsage(w io.Writer, reg *source.Registry, decls map[string][]source.ParamSpec) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "Usage: get-lyrics [--source <names>] [--author <name>] [--album <name>]")
	fmt.Fprintln(&b, "                   [--isrc <code>] [--duration <secs>] [--output <file>]")
	fmt.Fprintln(&b, "                   [--user-agent <ua>] [--sync-level <levels>] [--json] <song>")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Options:")
	fmt.Fprintln(&b, "  --source <names>, -s <names> Lyrics source names (default: lrclib)")
	fmt.Fprintln(&b, "  --author <name>,  -a <name>  Author / artist filter")
	fmt.Fprintln(&b, "  --album <name>,   -A <name>  Album filter")
	fmt.Fprintln(&b, "  --isrc <code>,    -i <code>  ISRC identifier")
	fmt.Fprintln(&b, "  --duration <secs>, -d <secs>  Track duration (seconds or mm:ss)")
	fmt.Fprintln(&b, "  --output <file>,  -o <file>  Write lyrics to file (default: stdout; refuses to overwrite an existing file)")
	fmt.Fprintln(&b, "  --overwrite, -O               Overwrite an existing --output file")
	fmt.Fprintln(&b, "  --json, -j                    Write complete fetch result as JSON (default: plain lyrics)")
	fmt.Fprintln(&b, "  --sync-level <levels>, -S <levels> Sync levels: line, word or none (default: line,none)")
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
