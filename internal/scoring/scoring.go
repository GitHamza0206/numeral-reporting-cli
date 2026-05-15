package scoring

import (
	"sort"
)

// Transaction is the input unit consumed by the scoring engine. Mirrors
// versions/vN/transactions.json on disk. The engine takes it by value and
// never writes back — the caller layer (cmd/numeral-reporting/score.go)
// owns persistence.
type Transaction struct {
	ID              string  `json:"id"`
	Date            string  `json:"date"`
	PeriodMonth     string  `json:"period_month"`
	Amount          float64 `json:"amount"`
	LibelleRaw      string  `json:"libelle_raw"`
	LibelleNorm     string  `json:"libelle_norm,omitempty"`
	EntityID        string  `json:"entity_id,omitempty"`
	MatchConfidence float64 `json:"match_confidence"`
	MatchKind       string  `json:"match_kind,omitempty"`
	Status          string  `json:"status"`
	Account         string  `json:"account,omitempty"`
	Category        string  `json:"category,omitempty"`
	Source          string  `json:"source,omitempty"`
	Adjustment      *Adjustment      `json:"adjustment,omitempty"`
	Components      *ScoreComponents `json:"components,omitempty"`
}

// Adjustment carries the agent-supplied signal used to score auto-adjustments
// (CCA, FNP, FAE, PCA, social reconstitutions, amortissements reproduced).
type Adjustment struct {
	Reason        string  `json:"reason"`
	SignalActuel  float64 `json:"signal_actuel"`
	HistoricalRef string  `json:"historical_ref,omitempty"`
}

// ScoreComponents records the sub-scores assigned to a transaction by the
// engine. Populated when the caller persists with --write.
type ScoreComponents struct {
	Identite    float64 `json:"identite,omitempty"`
	Coherence   float64 `json:"coherence,omitempty"`
	Recurrence  float64 `json:"recurrence,omitempty"`
	MontantCoh  float64 `json:"montant,omitempty"`
	ScoreTx     float64 `json:"score_tx,omitempty"`
	PatternHist float64 `json:"pattern_historique,omitempty"`
	SignalAct   float64 `json:"signal_actuel,omitempty"`
	CoherenceMt float64 `json:"coherence_metier,omitempty"`
	ScoreAj     float64 `json:"score_aj,omitempty"`
	Sensitivity float64 `json:"sensitivity,omitempty"`
}

// HistoricalSnapshot is what the engine knows about prior frozen versions.
// The caller layer builds and supplies these — the engine never touches the
// filesystem.
type HistoricalSnapshot struct {
	Version       string                    // "v2"
	EntityAmounts map[string][]float64      // entity_id → sorted prior amounts
	EntityMonths  map[string]map[string]int // entity_id → period_month → count
}

// Block is the result of scoring one of the three exclusive groups
// (traité / non_traité / ajusté). Amount is in absolute €.
type Block struct {
	Amount float64 // Σ |tx.Amount| within this block
	Score  float64 // 0..1 weighted by |amount| within the block
}

// Result is the full engine output for one period.
type Result struct {
	Treated     Block
	Untreated   Block
	Adjusted    Block
	Global      float64
	Level       string
	Label       string
	TotalPnL    float64
	Materiality float64
	TopRisks    []Risk
}

// Risk is one of the explanatory contributors to a sub-90 score.
type Risk struct {
	Kind      string
	Label     string
	EntityID  string
	Amount    float64
	ImpactPct float64
	TxIDs     []string
}

// Config tweaks the compute pipeline. Zero value is a valid configuration.
type Config struct {
	// SkipRecurrence and SkipAmountCoherence let the caller short-circuit
	// the history-dependent sub-scores. Used in S3 (before history is wired
	// in S5) and in unit tests.
	SkipRecurrence      bool
	SkipAmountCoherence bool
}

// scoreIdentite returns the identity sub-score for a transaction.
// Direct passthrough of the resolver confidence, clamped.
func scoreIdentite(tx Transaction) float64 {
	return clamp01(tx.MatchConfidence)
}

// scoreCoherence delegates to the PCG table in sensitivity.go.
func scoreCoherence(tx Transaction) float64 {
	return CoherencePCG(tx.Account, tx.Category)
}

// scoreRecurrence rates how often the entity appears in prior frozen versions.
// 1.0 if seen in ≥2 prior versions, 0.7 if in 1, 0.4 if entity exists but has
// no prior usage in history, 0.0 if entity is empty.
func scoreRecurrence(tx Transaction, history []HistoricalSnapshot) float64 {
	if tx.EntityID == "" {
		return 0.0
	}
	if len(history) == 0 {
		return 0.4 // entity recognized this period but no history to confirm yet
	}
	seen := 0
	for _, h := range history {
		if _, ok := h.EntityMonths[tx.EntityID]; ok {
			seen++
		}
	}
	switch {
	case seen >= 2:
		return 1.0
	case seen == 1:
		return 0.7
	default:
		return 0.4
	}
}

// scoreMontant rates how close the current amount is to the entity's
// historical mean. Z-score based: ≤1σ → 1.0, ≤2σ → 0.7, ≤3σ → 0.4, else 0.1.
// If history is sparse (<3 samples) returns 0.6 (neutral / "no comparable").
func scoreMontant(tx Transaction, history []HistoricalSnapshot) float64 {
	if tx.EntityID == "" {
		return 0.6
	}
	samples := []float64{}
	for _, h := range history {
		samples = append(samples, h.EntityAmounts[tx.EntityID]...)
	}
	if len(samples) < 3 {
		return 0.6
	}
	mean, stddev := meanStddev(samples)
	if stddev == 0 {
		// All historical amounts identical → deviation = 0 only if current matches.
		if abs(tx.Amount-mean) < 0.01 {
			return 1.0
		}
		return 0.1
	}
	z := abs(tx.Amount-mean) / stddev
	switch {
	case z <= 1:
		return 1.0
	case z <= 2:
		return 0.7
	case z <= 3:
		return 0.4
	default:
		return 0.1
	}
}

func meanStddev(xs []float64) (mean, stddev float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	for _, x := range xs {
		d := x - mean
		stddev += d * d
	}
	stddev = stddev / float64(len(xs))
	// sqrt without importing math directly to keep dependency surface tiny.
	stddev = sqrt(stddev)
	return mean, stddev
}

// sqrt — Newton-Raphson, deterministic, no math import.
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 32; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// computeScoreTx assembles the four sub-scores into a single weighted score.
// Pure function: no I/O, no time.
func computeScoreTx(tx Transaction, history []HistoricalSnapshot, cfg Config) ScoreComponents {
	id := scoreIdentite(tx)
	co := scoreCoherence(tx)
	rec := 0.4
	if !cfg.SkipRecurrence {
		rec = scoreRecurrence(tx, history)
	}
	mt := 0.6
	if !cfg.SkipAmountCoherence {
		mt = scoreMontant(tx, history)
	}
	score := WeightIdentite*id + WeightCoherence*co + WeightRecurrence*rec + WeightMontant*mt
	return ScoreComponents{
		Identite:   id,
		Coherence:  co,
		Recurrence: rec,
		MontantCoh: mt,
		ScoreTx:    clamp01(score),
	}
}

// ComputeTreated scores the traité block only. Returns the block result and a
// per-transaction map of components for caller-side persistence.
//
// txs may contain transactions of any status; only traité ones are folded in.
// The caller filters to the right Status before persistence.
func ComputeTreated(txs []Transaction, history []HistoricalSnapshot, cfg Config) (Block, map[string]ScoreComponents) {
	// Sort by ID for determinism. Operate on a copy so callers can keep their
	// own ordering (e.g. by date).
	idx := make([]int, 0, len(txs))
	for i := range txs {
		if txs[i].Status == StatusTraite {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(i, j int) bool {
		return txs[idx[i]].ID < txs[idx[j]].ID
	})

	components := make(map[string]ScoreComponents, len(idx))
	var weighted, totalAmt float64
	for _, i := range idx {
		c := computeScoreTx(txs[i], history, cfg)
		components[txs[i].ID] = c
		amt := abs(txs[i].Amount)
		weighted += amt * c.ScoreTx
		totalAmt += amt
	}
	block := Block{Amount: totalAmt}
	if totalAmt > 0 {
		block.Score = weighted / totalAmt
	}
	return block, components
}
