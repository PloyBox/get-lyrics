package fetch

import (
	"strings"

	"github.com/PloyBox/get-lyrics/source"
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

// Warning describes one issue observed while resolving lyrics. It
// carries structured data only — the CLI renders all display text,
// including the [kind] tag, from these fields.
type Warning struct {
	Kind      WarningKind  // which stage produced the warning
	Source    string       // name of the source the warning refers to
	Param     source.Param // typed parameter involved; 0 for a custom key or no parameter
	ParamName string       // custom parameter key involved; empty for typed parameters
	// Want is the sync level requested by the iteration that produced a
	// Downgraded warning: SyncLine means the source returned no synced
	// lyrics, SyncNone means it returned only synced lyrics. Zero for
	// every other kind.
	Want SyncLevel
	// Field is the result field a ResultMismatch warning refers to.
	Field source.ResultField
	// Declared reports, for a ResultMismatch warning, whether the source
	// declared Field via its Filled mask but left it empty (true), or
	// filled it without declaring it (false).
	Declared bool
	// Err is the underlying cause when one exists: the adapter error for
	// FetchFailed, the registry error for a not-found PreCheck, the
	// RequiredParamMismatchError for a fetch-time PrecheckMismatch; nil
	// otherwise.
	Err error
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
	if strings.TrimSpace(params.ISWC) != "" && filters&source.ParamISWC == 0 {
		out = append(out, Warning{
			Kind:   UnsupportedParam,
			Source: src.Name(),
			Param:  source.ParamISWC,
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

// resultFieldSpecs lists every field tracked by the Filled mask, with
// the accessor used by the mismatch detector.
type resultFieldSpec struct {
	bit   source.ResultField
	value func(source.Result) string
}

var resultFieldSpecs = []resultFieldSpec{
	{source.FieldLyrics, func(r source.Result) string { return r.Lyrics }},
	{source.FieldSyncedLyrics, func(r source.Result) string { return r.SyncedLyrics }},
	{source.FieldTitle, func(r source.Result) string { return r.Title }},
	{source.FieldArtist, func(r source.Result) string { return r.Artist }},
	{source.FieldAlbum, func(r source.Result) string { return r.Album }},
	{source.FieldISWC, func(r source.Result) string { return r.ISWC }},
	{source.FieldSubSource, func(r source.Result) string { return r.SubSource }},
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
