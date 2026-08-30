package main

import (
	"fmt"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

// flagForParam maps a Param bit to the CLI flag spelling used in
// rendered messages.
func flagForParam(p source.Param) string {
	switch p {
	case source.ParamAuthor:
		return "--author"
	case source.ParamAlbum:
		return "--album"
	case source.ParamISWC:
		return "--iswc"
	case source.ParamDuration:
		return "--duration"
	}
	return ""
}

// flagFor renders the flag spelling for a parameter reference: a
// custom key renders as "--env <KEY>", a typed bit via flagForParam.
func flagFor(p source.Param, custom string) string {
	if custom != "" {
		return "--env " + custom
	}
	return flagForParam(p)
}

// renderWarning renders one fetch.Warning into the exact stderr line,
// [kind] tag included: the fetch layer supplies structured data only,
// the CLI owns every byte of display text.
func renderWarning(w fetch.Warning) string {
	switch w.Kind {
	case fetch.UnsupportedParam:
		return fmt.Sprintf(`warning[unsupported]: source "%s" does not support %s`, w.Source, flagFor(w.Param, w.ParamName))
	case fetch.Downgraded:
		msg := "returned no synced lyrics"
		if w.Want == fetch.SyncNone {
			msg = "returned only synced lyrics"
		}
		return fmt.Sprintf(`warning[downgraded]: source "%s" %s`, w.Source, msg)
	case fetch.PreCheck:
		if w.Param != 0 || w.ParamName != "" {
			return fmt.Sprintf(`warning[precheck]: source "%s" skipped: requires %s`, w.Source, flagFor(w.Param, w.ParamName))
		}
		if w.Err != nil {
			return fmt.Sprintf(`warning[precheck]: source "%s" skipped: not found`, w.Source)
		}
		return fmt.Sprintf(`warning[precheck]: source "%s" skipped: duplicate`, w.Source)
	case fetch.PrecheckMismatch:
		if w.Err != nil {
			return fmt.Sprintf(`warning[precheck-mismatch]: source "%s" requires %s but precheck did not enforce it (source bug); trying next source`, w.Source, flagFor(w.Param, w.ParamName))
		}
		return fmt.Sprintf(`warning[precheck-mismatch]: source "%s" declared invalid --env key %q (source bug)`, w.Source, w.ParamName)
	case fetch.FetchFailed:
		return fmt.Sprintf(`warning[fetch]: source "%s" failed: %v; trying next source`, w.Source, w.Err)
	case fetch.ResultMismatch:
		if w.Declared {
			return fmt.Sprintf(`warning[result]: source "%s" declares field %q but left it empty (source issue)`, w.Source, w.Field.String())
		}
		return fmt.Sprintf(`warning[result]: source "%s" filled field %q without declaring it (source issue)`, w.Source, w.Field.String())
	}
	// Unreachable for the WarningKind set above; a safety net so a
	// future kind never renders as an empty line.
	return fmt.Sprintf(`warning[unknown]: source "%s" (kind %d)`, w.Source, int(w.Kind))
}

// renderRequiredError renders a fetch.RequiredParamError as the body of
// the error[required] line.
func renderRequiredError(e fetch.RequiredParamError) string {
	return fmt.Sprintf("source %q requires %s", e.Source, flagFor(e.Param, e.ParamName))
}
