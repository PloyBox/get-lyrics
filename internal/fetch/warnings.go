package fetch

import (
	"fmt"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// WarningKind classifies a Warning by the stage that produced it.
type WarningKind int

const (
	// UnsupportedParam: a user-supplied optional parameter the source
	// does not honor; emitted alongside a successful result.
	UnsupportedParam WarningKind = iota
	// Downgraded: the requested sync level got no match — synced
	// was requested but only plain lyrics returned, or plain was
	// requested but only synced lyrics returned. The unmatched result
	// stays cached and can satisfy a later iteration.
	Downgraded
	// PreCheck: --lenient mode skipped a source during precheck
	// (unknown name, missing required parameter, or duplicate).
	PreCheck
	// PrecheckMismatch: a source raised RequiredParamMismatchError from
	// Fetch — its capability declaration disagrees with what Fetch
	// actually needs. Flags a source implementation bug.
	PrecheckMismatch
	// FetchFailed: the adapter returned an error; fetch moved on to the
	// next source.
	FetchFailed
	// ResultMismatch: the adapter's Filled mask disagrees with the
	// actual field contents (a declared field left empty, or a filled
	// field not declared). The result is still used as-is (trust
	// policy); the warning flags a source implementation problem.
	ResultMismatch
)

// Warning describes one issue observed while resolving lyrics. The CLI
// writes each Warning.Message to stderr verbatim — messages are
// pre-formatted here, including the [kind] tag.
type Warning struct {
	Kind      WarningKind  // which stage produced the warning
	Source    string       // name of the source the warning refers to
	Param     source.Param // typed parameter involved (UnsupportedParam / PreCheck); 0 for custom
	ParamName string       // custom --env key involved; empty for typed parameters
	Message   string       // pre-formatted, user-facing text (for stderr)
}

// detectUnsupported compares the non-empty optional fields in params
// against the adapter's filters for this request and returns one
// UnsupportedParam warning per mismatch. The sync level is deliberately
// excluded: a synced request on a plain-only source is covered by the
// Downgraded warning.
//
// Custom keys run a parallel path: every user-supplied key the adapter
// does not recognize for this request gets one warning. The map
// iteration order is unspecified on purpose — multiple unrecognized
// keys produce warnings in nondeterministic order; tests assert the
// warning set, never its order.
func detectUnsupported(params Params, src source.Source) []Warning {
	caps := src.Capabilities(requestFromParams(params))
	filters := caps.Filters
	out := make([]Warning, 0, 3)

	if strings.TrimSpace(params.Author) != "" && filters&source.ParamAuthor == 0 {
		out = append(out, Warning{
			Kind:    UnsupportedParam,
			Source:  src.Name(),
			Param:   source.ParamAuthor,
			Message: fmt.Sprintf(`warning[unsupported]: source "%s" does not support --author`, src.Name()),
		})
	}
	if strings.TrimSpace(params.Album) != "" && filters&source.ParamAlbum == 0 {
		out = append(out, Warning{
			Kind:    UnsupportedParam,
			Source:  src.Name(),
			Param:   source.ParamAlbum,
			Message: fmt.Sprintf(`warning[unsupported]: source "%s" does not support --album`, src.Name()),
		})
	}
	if strings.TrimSpace(params.ISWC) != "" && filters&source.ParamISWC == 0 {
		out = append(out, Warning{
			Kind:    UnsupportedParam,
			Source:  src.Name(),
			Param:   source.ParamISWC,
			Message: fmt.Sprintf(`warning[unsupported]: source "%s" does not support --iswc`, src.Name()),
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
				Message:   fmt.Sprintf(`warning[unsupported]: source "%s" does not support --env %s`, src.Name(), key),
			})
		}
	}
	return out
}

// resultFieldSpecs lists every field tracked by the Filled mask, with
// the accessor used by the mismatch detector.
type resultFieldSpec struct {
	bit   source.ResultField
	name  string
	value func(source.Result) string
}

var resultFieldSpecs = []resultFieldSpec{
	{source.FieldLyrics, "Lyrics", func(r source.Result) string { return r.Lyrics }},
	{source.FieldSyncedLyrics, "SyncedLyrics", func(r source.Result) string { return r.SyncedLyrics }},
	{source.FieldTitle, "Title", func(r source.Result) string { return r.Title }},
	{source.FieldArtist, "Artist", func(r source.Result) string { return r.Artist }},
	{source.FieldAlbum, "Album", func(r source.Result) string { return r.Album }},
	{source.FieldISWC, "ISWC", func(r source.Result) string { return r.ISWC }},
	{source.FieldSubSource, "SubSource", func(r source.Result) string { return r.SubSource }},
}

// detectResultMismatch compares sr.Filled against the actual field
// contents and reports one warning per inconsistency: a declared bit
// with an empty value, or a non-empty value without a declared bit.
// Either way the result is still used as-is (trust policy) — the
// warning only flags a source implementation problem.
func detectResultMismatch(srcName string, sr source.Result) []Warning {
	out := make([]Warning, 0, 2)
	for _, spec := range resultFieldSpecs {
		declared := sr.Filled&spec.bit != 0
		empty := strings.TrimSpace(spec.value(sr)) == ""
		if declared && empty {
			out = append(out, Warning{
				Kind:    ResultMismatch,
				Source:  srcName,
				Message: fmt.Sprintf(`warning[result]: source "%s" declares field %q but left it empty (source issue)`, srcName, spec.name),
			})
		} else if !declared && !empty {
			out = append(out, Warning{
				Kind:    ResultMismatch,
				Source:  srcName,
				Message: fmt.Sprintf(`warning[result]: source "%s" filled field %q without declaring it (source issue)`, srcName, spec.name),
			})
		}
	}
	return out
}
