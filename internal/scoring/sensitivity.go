// Package scoring implements the deterministic reliability scoring engine.
// All functions in this package are pure: no I/O, no time.Now(), no map
// iteration without sort. Same inputs → same outputs.
//
// The scoring spec lives in skills/numeral-reporting-agent/scoring.md.
// Key invariants:
//   - score_transaction = 0.40·identité + 0.30·cohérence + 0.20·récurrence + 0.10·montant
//   - Score weighted by € amounts, never by line count.
//   - Three exclusive blocks (traité / non_traité / ajustement) in €.
//   - Global = weighted mean of block scores by their € weight.
//   - Materiality threshold = max(100€, 0.05% × Total_PnL).
package scoring

import (
	"math"
	"strings"
)

// SchemaVersion is the on-disk version of computed score data. Bump on
// breaking changes to the formula or coefficients.
const SchemaVersion = 1

// Block weights inside score_transaction. These constants are checked against
// the rendered HTML footer in static.go to prevent drift between code and
// narrative (see TestWeightsMatchRendererFooter in scoring_test.go).
const (
	WeightIdentite   = 0.40
	WeightCoherence  = 0.30
	WeightRecurrence = 0.20
	WeightMontant    = 0.10
)

// Block weights inside score_ajustement.
const (
	WeightPatternHist  = 0.45
	WeightSignalActuel = 0.35
	WeightCoherenceMet = 0.20
)

// Materiality returns max(100€, 0.0005 × totalPnL). Used to filter
// negligible lines out of the top-risks ranking.
func Materiality(totalPnL float64) float64 {
	threshold := 0.0005 * totalPnL
	if threshold < 100 {
		return 100
	}
	return threshold
}

// Category buckets used for sensitivity weighting on non-traité amounts.
// Stored as a sorted, exclusive enum so callers always pass the same key.
const (
	CategoryCA      = "ca"
	CategorySalaire = "salaires"
	CategoryLoyer   = "loyer"
	CategoryAchats  = "achats"
	CategoryDivers  = "divers"
)

// Sensitivity returns the coefficient applied to the |amount| of a
// non-traité transaction when computing Score_non_traité. Higher = more
// painful to leave unclassified.
//
// Spec: CA × 1.5, salaires × 1.3, loyer × 1.2, achats × 1.0, divers × 0.8.
func Sensitivity(category string) float64 {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case CategoryCA:
		return 1.5
	case CategorySalaire:
		return 1.3
	case CategoryLoyer:
		return 1.2
	case CategoryAchats:
		return 1.0
	default:
		return 0.8
	}
}

// Status values for transactions. Mutually exclusive: a transaction sits in
// exactly one block.
const (
	StatusTraite     = "traite"
	StatusNonTraite  = "non_traite"
	StatusAjustement = "ajustement"
)

// CoherencePCG returns the comptable-coherence sub-score for a transaction:
// 1.0 if the PCG account matches the expected category, 0.5 if the account is
// known but the category is plausible-but-uncertain, 0.0 if account is empty
// or category is incompatible.
//
// The mapping is intentionally coarse (PCG account-range → expected category)
// — the goal is to detect gross misclassifications, not to second-guess a
// chartered accountant. Spec details live in skills/.../categorize.md.
func CoherencePCG(account string, category string) float64 {
	account = strings.TrimSpace(account)
	category = strings.ToLower(strings.TrimSpace(category))
	if account == "" {
		return 0.0
	}
	// We only look at the leading digits.
	prefix := leadingDigits(account)
	if prefix == "" {
		return 0.0
	}
	bucket := pcgBucket(prefix)
	if bucket == "" {
		return 0.0 // unknown account range
	}
	switch bucket {
	case "ca":
		if category == CategoryCA {
			return 1.0
		}
	case "salaires":
		if category == CategorySalaire {
			return 1.0
		}
	case "loyer":
		if category == CategoryLoyer {
			return 1.0
		}
		// Loyer accounts are within 61x — if categorized as "achats" that's
		// possible but ambiguous (other 61x are services externes).
		if category == CategoryAchats {
			return 0.5
		}
	case "achats":
		if category == CategoryAchats {
			return 1.0
		}
	case "divers":
		if category == CategoryDivers {
			return 1.0
		}
	}
	return 0.0
}

// pcgBucket maps a PCG account prefix to its coarse expected category.
// Reflects the mapping documented in skills/.../categorize.md.
func pcgBucket(prefix string) string {
	// Order matters: more specific prefixes first.
	switch {
	case strings.HasPrefix(prefix, "613"):
		return "loyer"
	case strings.HasPrefix(prefix, "641"), strings.HasPrefix(prefix, "644"),
		strings.HasPrefix(prefix, "645"), strings.HasPrefix(prefix, "647"):
		return "salaires"
	case strings.HasPrefix(prefix, "60"), strings.HasPrefix(prefix, "61"),
		strings.HasPrefix(prefix, "62"):
		return "achats"
	case strings.HasPrefix(prefix, "63"), strings.HasPrefix(prefix, "65"),
		strings.HasPrefix(prefix, "66"), strings.HasPrefix(prefix, "67"),
		strings.HasPrefix(prefix, "68"), strings.HasPrefix(prefix, "69"):
		return "divers"
	case strings.HasPrefix(prefix, "70"), strings.HasPrefix(prefix, "71"),
		strings.HasPrefix(prefix, "72"), strings.HasPrefix(prefix, "74"),
		strings.HasPrefix(prefix, "75"), strings.HasPrefix(prefix, "76"),
		strings.HasPrefix(prefix, "77"), strings.HasPrefix(prefix, "78"):
		return "ca"
	}
	return ""
}

func leadingDigits(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s[:i]
		}
	}
	return s
}

// clamp01 keeps a score within [0, 1].
func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// abs returns the absolute value of a float64.
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// Level maps a 0..1 global score to a discrete level + label pair.
// Threshold buckets straight from the spec.
func Level(global float64) (level, label string) {
	switch {
	case global >= 0.90:
		return "tres_fiable", "Très fiable — publication immédiate"
	case global >= 0.85:
		return "envoyable", "Envoyable au client"
	case global >= 0.80:
		return "acceptable", "Acceptable avec revue"
	case global >= 0.70:
		return "fragile", "Fragile — revue recommandée"
	default:
		return "non_fiable", "Non fiable — ne pas publier"
	}
}
