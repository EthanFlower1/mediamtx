package lpr

import (
	"regexp"
	"strings"
)

// regionPattern pairs a region identifier with the compiled regex for valid
// plate texts in that region.
type regionPattern struct {
	Region string
	Re     *regexp.Regexp
}

// regionPatterns holds the priority-ordered list of known plate formats.
// The list covers ~20 of the most common regional formats. Texts are
// matched after upper-casing and stripping hyphens/spaces so that "ABC 123"
// and "ABC-123" both match the same pattern.
//
// Ordering is critical: more specific (longer / rarer) patterns appear first
// so that they win over broad fallback patterns.
//
// Sources / references:
//   - US formats: AAMVA plate design standards
//   - EU: European Commission vehicle registration plate regulations
//   - UK: DVLA plate format specification
//   - AU: Austroads plate numbering guideline
var regionPatterns = []regionPattern{
	// --- United Kingdom ---
	// Current (post-2001): AA51 AAA  (2 letters, 2 digits, 3 letters = 7 chars)
	{Region: "UK", Re: regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z]{3}$`)},
	// Prefix format (1983-2001): A123 BBB (1 letter + 1-3 digits + 3 letters)
	{Region: "UK-prefix", Re: regexp.MustCompile(`^[A-Z][0-9]{1,3}[A-Z]{3}$`)},

	// --- EU countries (ISO 3166-1 alpha-2) ---
	// Germany: 1-3 letters (Kreis) + 1-2 letters (identifier) + 1-4 digits
	// with an optional H or E suffix, total 4-11 chars.
	// Placed after UK and before AU so that German formats that look like
	// generic alphanumeric take priority over looser regional patterns.
	{Region: "EU-DE", Re: regexp.MustCompile(`^[A-Z]{1,3}[A-Z]{1,2}[0-9]{1,4}[HE]?$`)},

	// France: AA-001-AA (since 2009) — exactly 2L 3D 2L = 7 chars
	{Region: "EU-FR", Re: regexp.MustCompile(`^[A-Z]{2}[0-9]{3}[A-Z]{2}$`)},

	// Spain: 0000-AAA (since 2000) — exactly 4D 3L = 7 chars
	{Region: "EU-ES", Re: regexp.MustCompile(`^[0-9]{4}[A-Z]{3}$`)},

	// Belgium: 1-ABC-234 — exactly 1D 3L 3D = 7 chars
	{Region: "EU-BE", Re: regexp.MustCompile(`^[0-9][A-Z]{3}[0-9]{3}$`)},

	// Italy: AA 000 AA — exactly 2L 3D 2L = 7 chars (same shape as FR)
	// Must come AFTER FR; distinguish by putting Italy after France in ordering.
	// In practice the two are indistinguishable by format alone; Italy is a
	// lower-priority match. Callers may enrich with country hint from camera GPS.
	{Region: "EU-IT", Re: regexp.MustCompile(`^[A-Z]{2}[0-9]{3}[A-Z]{2}$`)},

	// Sweden: AAA 000 or AAA 00A — exactly 3L 2D 1(D|L) = 6 chars
	{Region: "EU-SE", Re: regexp.MustCompile(`^[A-Z]{3}[0-9]{2}[A-Z0-9]$`)},

	// Poland: AA 00000 or AAA 0000 — 2-3 letters + 4-5 digits
	{Region: "EU-PL", Re: regexp.MustCompile(`^[A-Z]{2,3}[0-9]{4,5}$`)},

	// --- Australia ---
	// ACT: Y AA 00 A — 1L 2L 2D 1L = 6 chars (must be before generic AU)
	{Region: "AU-ACT", Re: regexp.MustCompile(`^[A-Z]{3}[0-9]{2}[A-Z]$`)},
	// NSW/VIC/QLD: 3L 3D (most common) — 6 chars
	{Region: "AU", Re: regexp.MustCompile(`^[A-Z]{3}[0-9]{3}$`)},

	// --- United States ---
	// California: 7 chars — 1D 3L 3D e.g. 7ABC123
	{Region: "US-CA", Re: regexp.MustCompile(`^[0-9][A-Z]{3}[0-9]{3}$`)},
	// New York / Texas: 3L 4D e.g. ABC1234 — 7 chars
	{Region: "US-NY", Re: regexp.MustCompile(`^[A-Z]{3}[0-9]{4}$`)},
	// Florida: AAA 000 or AAA 0000 — 6-7 chars
	{Region: "US-FL", Re: regexp.MustCompile(`^[A-Z]{3}[0-9]{3,4}$`)},
	// Illinois: A 000000 or AA 000000 — 7-8 chars
	{Region: "US-IL", Re: regexp.MustCompile(`^[A-Z]{1,2}[0-9]{5,6}$`)},
	// Generic US: 2-7 alphanumeric chars
	{Region: "US", Re: regexp.MustCompile(`^[A-Z0-9]{2,7}$`)},
}

// Normalise strips spaces, hyphens and dots then upper-cases the text so that
// "abc 123", "ABC-123" and "ABC.123" all normalise to "ABC123".
func Normalise(plate string) string {
	r := strings.NewReplacer(" ", "", "-", "", ".", "")
	return strings.ToUpper(r.Replace(plate))
}

// MatchRegion returns the first region identifier whose pattern matches the
// normalised plate text. Returns "" if no pattern matches.
func MatchRegion(normalised string) string {
	for _, p := range regionPatterns {
		if p.Re.MatchString(normalised) {
			return p.Region
		}
	}
	return ""
}

// AllRegions returns the full list of region identifiers in priority order.
// Used by tests and documentation tooling.
func AllRegions() []string {
	out := make([]string, len(regionPatterns))
	for i, p := range regionPatterns {
		out[i] = p.Region
	}
	return out
}
