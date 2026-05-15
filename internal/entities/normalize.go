// Package entities provides deterministic entity resolution for bank libellés
// and FEC entries. All functions in this package are pure: same inputs yield
// the same outputs, no I/O, no time.Now(), no map iteration without sort.
package entities

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// sepaDenyList holds tokens that carry no entity signal in bank libellés.
// Kept sorted for readability; lookup is via a precomputed set.
var sepaDenyList = []string{
	"carte",
	"cb",
	"date",
	"du",
	"fact",
	"facture",
	"le",
	"n",
	"no",
	"num",
	"prelevement",
	"prelvt",
	"ref",
	"reference",
	"sepa",
	"vir",
	"virement",
	"virt",
}

var sepaDenySet map[string]struct{}

// Date and digit patterns are matched on the lowered Unicode form, BEFORE
// non-alphanumerics are turned into spaces — otherwise "12/03/2024" would
// already be "12 03 2024" and the date shape would be lost.
var (
	// ISO: YYYY-MM-DD with realistic month/day ranges.
	reDateISO = regexp.MustCompile(`\b(19|20)\d{2}[-/.](0[1-9]|1[0-2])[-/.](0[1-9]|[12]\d|3[01])\b`)
	// JJ[/-.]MM[/-.]YYYY or JJ[/-.]MM[/-.]YY or JJ[/-.]MM, with valid ranges.
	reDateFR     = regexp.MustCompile(`\b(0?[1-9]|[12]\d|3[01])[-/.](0?[1-9]|1[0-2])([-/.](\d{2}|\d{4}))?\b`)
	reLongDigits = regexp.MustCompile(`\b\d{4,}\b`)
	reWhitespace = regexp.MustCompile(`\s+`)
)

func init() {
	sepaDenySet = make(map[string]struct{}, len(sepaDenyList))
	for _, t := range sepaDenyList {
		sepaDenySet[t] = struct{}{}
	}
}

// Normalize collapses a raw libellé into a stable lowercase ASCII form
// suitable for exact-match comparison across entities.
//
// Pipeline:
//  1. NFD decomposition and strip combining marks (handles accents).
//  2. Lowercase (locale-independent because step 5 restricts output to ASCII).
//  3. Strip date-like tokens (DD/MM/YYYY, ISO YYYY-MM-DD, DD-MM, etc.).
//  4. Strip long integer tokens (>=4 digits, usually invoice numbers).
//  5. Replace runes outside [a-z0-9 ] with spaces.
//  6. Drop SEPA noise tokens and tokens shorter than 2 chars.
//  7. Collapse whitespace.
func Normalize(raw string) string {
	if raw == "" {
		return ""
	}

	// Step 1+2: NFD + strip marks + lowercase.
	decomposed := norm.NFD.String(raw)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	s := b.String()

	// Step 3: strip dates BEFORE separators are erased.
	s = reDateISO.ReplaceAllString(s, " ")
	s = reDateFR.ReplaceAllString(s, " ")

	// Step 4: strip long digit runs (invoice numbers, refs).
	s = reLongDigits.ReplaceAllString(s, " ")

	// Step 5: keep only [a-z0-9 ], replace everything else with a space.
	var c strings.Builder
	c.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			c.WriteRune(r)
		case r >= '0' && r <= '9':
			c.WriteRune(r)
		default:
			c.WriteByte(' ')
		}
	}
	s = c.String()

	// Step 6: tokenize, drop deny-list, short tokens, and bare digit tokens
	// shorter than 4 chars (year suffixes like "24" left over from "12/03/24"
	// that the date regex failed to fully consume).
	fields := strings.Fields(s)
	out := fields[:0]
	for _, t := range fields {
		if len(t) < 2 {
			continue
		}
		if _, deny := sepaDenySet[t]; deny {
			continue
		}
		if isPureDigits(t) && len(t) < 4 {
			// Short standalone numbers without context: drop. Allows "tva20"
			// (single token) but drops noise like "12 03" left from a date.
			continue
		}
		out = append(out, t)
	}

	// Step 7: rejoin with single spaces.
	joined := strings.Join(out, " ")
	joined = reWhitespace.ReplaceAllString(joined, " ")
	return strings.TrimSpace(joined)
}

func isPureDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
