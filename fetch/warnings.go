package fetch

import (
	"strings"

	"github.com/PloyBox/get-lyrics/source"
)

// WarningKind classifies a Warning by the stage that produced it.
type WarningKind int

const (
	UnsupportedParam WarningKind = iota // supplied parameter the source does not honor
	Downgraded                          // requested level unmatched; Want names the direction
	PreCheck                            // --lenient skipped a source during precheck
	PrecheckMismatch                    // a source's declaration disagrees with its Fetch
	FetchFailed                         // adapter error; fetch moved on to the next source
	ResultMismatch                      // Filled mask disagrees with the field contents
)

// Warning describes one issue observed while resolving lyrics. It carries
// structured data only — the CLI renders all display text, including the
// [kind] tag.
type Warning struct {
	Kind      WarningKind
	Source    string
	Param     source.Param // typed parameter; 0 for a custom key or none
	ParamName string       // custom parameter key; empty for typed parameters
	// Want is the level requested by the iteration that produced a
	// Downgraded warning; zero for every other kind.
	Want SyncLevel
	// Field is the result field a ResultMismatch refers to.
	Field source.ResultField
	// Declared reports, for a ResultMismatch, whether the source declared
	// Field but left it empty (true) or filled it without declaring it.
	Declared bool
	// Err is the underlying cause when one exists; nil otherwise.
	Err error
}

// detectUnsupported returns one UnsupportedParam warning per non-empty
// optional parameter the adapter does not honor for this request. The
// sync level is excluded (covered by Downgraded). Custom keys iterate in
// unspecified order, so multiple unrecognized keys warn nondeterministically.
func detectUnsupported(params Params, src source.Source) []Warning {
	caps := src.Capabilities(requestFromParams(params))
	filters := caps.Filters
	out := make([]Warning, 0, 4)

	if strings.TrimSpace(params.Author) != "" && filters&source.ParamAuthor == 0 {
		out = append(out, Warning{
			Kind:   UnsupportedParam,
			Source: src.Name(),
			Param:  source.ParamAuthor,
		})
	}
	if strings.TrimSpace(params.Album) != "" && filters&source.ParamAlbum == 0 {
		out = append(out, Warning{
			Kind:   UnsupportedParam,
			Source: src.Name(),
			Param:  source.ParamAlbum,
		})
	}
	if strings.TrimSpace(params.ISRC) != "" && filters&source.ParamISRC == 0 {
		out = append(out, Warning{
			Kind:   UnsupportedParam,
			Source: src.Name(),
			Param:  source.ParamISRC,
		})
	}
	if params.Duration > 0 && filters&source.ParamDuration == 0 {
		out = append(out, Warning{
			Kind:   UnsupportedParam,
			Source: src.Name(),
			Param:  source.ParamDuration,
		})
	}

	recognized := make(map[string]bool, len(caps.Custom))
	for _, spec := range caps.Custom {
		recognized[spec.Name] = true
	}
	for key, value := range params.Custom {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if !recognized[key] {
			out = append(out, Warning{
				Kind:      UnsupportedParam,
				Source:    src.Name(),
				ParamName: key,
			})
		}
	}
	return out
}

// resultFieldSpecs lists every field tracked by the Filled mask, with the
// accessor used by the mismatch detector.
type resultFieldSpec struct {
	bit   source.ResultField
	value func(source.Result) string
}

var resultFieldSpecs = []resultFieldSpec{
	{source.FieldLyrics, func(r source.Result) string { return r.Lyrics }},
	{source.FieldTitle, func(r source.Result) string { return r.Title }},
	{source.FieldArtist, func(r source.Result) string { return r.Artist }},
	{source.FieldAlbum, func(r source.Result) string { return r.Album }},
	{source.FieldISRC, func(r source.Result) string { return r.ISRC }},
	{source.FieldSubSource, func(r source.Result) string { return r.SubSource }},
}

// detectResultMismatch reports one warning per Filled-vs-content
// inconsistency: a declared bit with an empty value, or a non-empty value
// without a declared bit. The result is still used as-is (trust policy).
func detectResultMismatch(srcName string, sr source.Result) []Warning {
	out := make([]Warning, 0, 2)
	for _, spec := range resultFieldSpecs {
		declared := sr.Filled&spec.bit != 0
		empty := strings.TrimSpace(spec.value(sr)) == ""
		if declared && empty {
			out = append(out, Warning{
				Kind:     ResultMismatch,
				Source:   srcName,
				Field:    spec.bit,
				Declared: true,
			})
		} else if !declared && !empty {
			out = append(out, Warning{
				Kind:   ResultMismatch,
				Source: srcName,
				Field:  spec.bit,
			})
		}
	}
	return out
}
