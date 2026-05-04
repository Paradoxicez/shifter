// Package codecs ships the embedded vendor codec_js source files for the
// three D-07 seed device profiles. Open Question #5 resolution from
// 02-RESEARCH.md — codec_js sources live alongside Go source under normal
// version control and are pulled into the binary via //go:embed, NOT inlined
// into a SQL seed migration. This avoids pg_dump escaping headaches and keeps
// codec diffs reviewable in the same PRs as their Go consumers.
//
// Pitfall 6 — Acrel ADL200 + ADW300 are part of the same vendor family and
// share ONE codec_js. CodecBySlug routes both slugs to AcrelFamily; the two
// profile rows differ only in capabilities + mapping subset (DATA-09).
package codecs

import _ "embed"

//go:embed axioma_w1.js
var AxiomaW1 string

//go:embed acrel_family.js
var AcrelFamily string

// CodecBySlug returns the embedded JS source for a seed profile slug.
// Returns "" for unknown slug — the seed routine logs and skips.
func CodecBySlug(slug string) string {
	switch slug {
	case "axioma_w1":
		return AxiomaW1
	case "acrel_adl200", "acrel_adw300": // Pitfall 6 — shared codec
		return AcrelFamily
	default:
		return ""
	}
}
