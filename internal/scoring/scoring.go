package scoring

import (
	"sort"
	"strings"
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
//
// SignalActuel is a pointer to distinguish "agent did not set a value"
// (default to 0.5 neutral) from "explicitly zero" (no current signal).
type Adjustment struct {
	Reason        string   `json:"reason"`
	SignalActuel  *float64 `json:"signal_actuel,omitempty"`
	HistoricalRef string   `json:"historical_ref,omitempty"`
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
	idx := filterIndex(txs, StatusTraite)
	sort.Slice(idx, func(i, j int) bool { return txs[idx[i]].ID < txs[idx[j]].ID })

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

// ComputeUntreated scores the non-traité block. The block "score" is
// 1 - (sum(|amount| × sensitivity) / totalPnL), clamped to [0,1]. The block
// "amount" is the unweighted sum of |amount| for display.
//
// totalPnL is the global denominator: sum of |amount| across ALL transactions
// in the period. Caller computes it once and passes it in.
func ComputeUntreated(txs []Transaction, totalPnL float64) (Block, map[string]ScoreComponents) {
	idx := filterIndex(txs, StatusNonTraite)
	sort.Slice(idx, func(i, j int) bool { return txs[idx[i]].ID < txs[idx[j]].ID })

	components := make(map[string]ScoreComponents, len(idx))
	var raw, weighted float64
	for _, i := range idx {
		amt := abs(txs[i].Amount)
		coef := Sensitivity(txs[i].Category)
		components[txs[i].ID] = ScoreComponents{Sensitivity: coef}
		raw += amt
		weighted += amt * coef
	}
	block := Block{Amount: raw}
	if totalPnL > 0 {
		block.Score = clamp01(1.0 - weighted/totalPnL)
	} else {
		block.Score = 0
	}
	return block, components
}

// scorePatternHistorique rates how often the same (entity, adjustment reason)
// pair appeared in prior frozen versions. 1.0 if seen in half of versions,
// linearly interpolated below.
func scorePatternHistorique(tx Transaction, history []HistoricalSnapshot) float64 {
	if tx.Adjustment == nil || tx.EntityID == "" || len(history) == 0 {
		return 0.0
	}
	// In MVP we only check entity presence as a proxy for reason recurrence —
	// the agent supplies the reason but we don't yet persist reasons across
	// versions. Refinement is queued for S5 if needed.
	seen := 0
	for _, h := range history {
		if _, ok := h.EntityMonths[tx.EntityID]; ok {
			seen++
		}
	}
	if seen == 0 {
		return 0.0
	}
	share := float64(seen) / float64(len(history))
	if share >= 0.5 {
		return 1.0
	}
	// Linear: 0.0 at share=0, 1.0 at share=0.5.
	return clamp01(share * 2.0)
}

// scoreCoherenceMetier checks the adjustment reason against the chosen PCG
// account (FNP↔408/40x, CCA↔486, FAE↔418, PCA↔487, etc.). Returns
// 1.0 / 0.5 / 0.0.
func scoreCoherenceMetier(tx Transaction) float64 {
	if tx.Adjustment == nil {
		return 0.0
	}
	reason := strings.ToLower(strings.TrimSpace(tx.Adjustment.Reason))
	if reason == "" {
		return 0.5 // unknown reason → ambiguous
	}
	prefix := leadingDigits(strings.TrimSpace(tx.Account))
	expected := pcgBucketForAdjustment(reason)
	if len(expected) == 0 {
		return 0.5
	}
	for _, candidate := range expected {
		if strings.HasPrefix(prefix, candidate) {
			return 1.0
		}
	}
	return 0.0
}

// pcgBucketForAdjustment maps a cut-off / closing reason to the PCG account
// prefixes that legitimately carry it.
func pcgBucketForAdjustment(reason string) []string {
	switch reason {
	case "fnp":
		return []string{"408", "60", "61", "62"}
	case "cca":
		return []string{"486"}
	case "fae":
		return []string{"418"}
	case "pca":
		return []string{"487"}
	case "amortissement":
		return []string{"6811", "281", "28"}
	case "is", "impot":
		return []string{"695", "444"}
	case "tva":
		return []string{"445"}
	case "provision":
		return []string{"6815", "6817", "151", "491", "29", "39"}
	case "social", "salaires":
		return []string{"641", "644", "645", "647"}
	case "reclassement", "autre":
		return nil // accept anything → 0.5
	}
	return nil
}

// computeScoreAdj assembles the three sub-scores of an adjustment.
func computeScoreAdj(tx Transaction, history []HistoricalSnapshot) ScoreComponents {
	ph := scorePatternHistorique(tx, history)
	sa := 0.5 // default neutral when agent did not set it
	if tx.Adjustment != nil && tx.Adjustment.SignalActuel != nil {
		sa = clamp01(*tx.Adjustment.SignalActuel)
	}
	cm := scoreCoherenceMetier(tx)
	score := WeightPatternHist*ph + WeightSignalActuel*sa + WeightCoherenceMet*cm
	return ScoreComponents{
		PatternHist: ph,
		SignalAct:   sa,
		CoherenceMt: cm,
		ScoreAj:     clamp01(score),
	}
}

// ComputeAdjusted scores the ajustement block.
func ComputeAdjusted(txs []Transaction, history []HistoricalSnapshot) (Block, map[string]ScoreComponents) {
	idx := filterIndex(txs, StatusAjustement)
	sort.Slice(idx, func(i, j int) bool { return txs[idx[i]].ID < txs[idx[j]].ID })

	components := make(map[string]ScoreComponents, len(idx))
	var weighted, totalAmt float64
	for _, i := range idx {
		c := computeScoreAdj(txs[i], history)
		components[txs[i].ID] = c
		amt := abs(txs[i].Amount)
		weighted += amt * c.ScoreAj
		totalAmt += amt
	}
	block := Block{Amount: totalAmt}
	if totalAmt > 0 {
		block.Score = weighted / totalAmt
	}
	return block, components
}

// Compute runs all three blocks, aggregates the global score, builds the top
// risks list, and returns the full Result + a merged components map keyed by
// transaction ID. Pure function.
func Compute(txs []Transaction, history []HistoricalSnapshot, cfg Config) (Result, map[string]ScoreComponents) {
	totalPnL := 0.0
	for _, tx := range txs {
		totalPnL += abs(tx.Amount)
	}

	treated, compT := ComputeTreated(txs, history, cfg)
	untreated, compU := ComputeUntreated(txs, totalPnL)
	adjusted, compA := ComputeAdjusted(txs, history)

	denom := treated.Amount + untreated.Amount + adjusted.Amount
	global := 0.0
	if denom > 0 {
		global = (treated.Amount*treated.Score +
			untreated.Amount*untreated.Score +
			adjusted.Amount*adjusted.Score) / denom
	}
	level, label := Level(global)

	mat := Materiality(totalPnL)
	risks := buildTopRisks(txs, compT, compU, compA, totalPnL, mat)

	// Merge components, deterministic precedence: traité > non_traité > ajusté.
	components := make(map[string]ScoreComponents, len(compT)+len(compU)+len(compA))
	for k, v := range compA {
		components[k] = v
	}
	for k, v := range compU {
		components[k] = v
	}
	for k, v := range compT {
		components[k] = v
	}

	return Result{
		Treated:     treated,
		Untreated:   untreated,
		Adjusted:    adjusted,
		Global:      global,
		Level:       level,
		Label:       label,
		TotalPnL:    totalPnL,
		Materiality: mat,
		TopRisks:    risks,
	}, components
}

// buildTopRisks groups transactions by (Kind, EntityID), computes impact in
// € on the global score, and returns the top 5 sorted deterministically.
func buildTopRisks(
	txs []Transaction,
	compT, compU, compA map[string]ScoreComponents,
	totalPnL, materiality float64,
) []Risk {
	if totalPnL <= 0 {
		return nil
	}
	type groupKey struct{ Kind, EntityID string }
	type group struct {
		Risk
		hits int
	}
	groups := map[groupKey]*group{}

	addRisk := func(kind, entityID, label string, amount, impact float64, txID string) {
		if amount < materiality {
			return
		}
		k := groupKey{Kind: kind, EntityID: entityID}
		g, ok := groups[k]
		if !ok {
			g = &group{Risk: Risk{Kind: kind, EntityID: entityID, Label: label}}
			groups[k] = g
		}
		g.Amount += amount
		g.ImpactPct += 100.0 * impact / totalPnL
		if len(g.TxIDs) < 3 {
			g.TxIDs = append(g.TxIDs, txID)
		}
		g.hits++
	}

	// Iterate in ID order for deterministic group construction (impact sums
	// are the same regardless of order, but TxIDs and labels need order).
	sortedTxs := make([]Transaction, len(txs))
	copy(sortedTxs, txs)
	sort.Slice(sortedTxs, func(i, j int) bool { return sortedTxs[i].ID < sortedTxs[j].ID })

	for _, tx := range sortedTxs {
		amt := abs(tx.Amount)
		switch tx.Status {
		case StatusTraite:
			c := compT[tx.ID]
			impact := amt * (1.0 - c.ScoreTx)
			if c.Identite < 0.5 {
				addRisk("low_identity", tx.EntityID, "Libellé non reconnu", amt, impact, tx.ID)
				continue
			}
			if c.Coherence < 0.5 {
				addRisk("category_mismatch", tx.EntityID, "Compte/catégorie ambigus", amt, impact, tx.ID)
				continue
			}
			if c.ScoreTx < 0.85 {
				addRisk("low_score_treated", tx.EntityID, "Classification fragile", amt, impact, tx.ID)
			}
		case StatusNonTraite:
			c := compU[tx.ID]
			impact := amt * c.Sensitivity
			label := "Écriture non catégorisée"
			if tx.Category != "" {
				label = "Non catégorisé — sensibilité " + tx.Category
			}
			addRisk("unrecognized_amount", tx.EntityID, label, amt, impact, tx.ID)
		case StatusAjustement:
			c := compA[tx.ID]
			impact := amt * (1.0 - c.ScoreAj)
			if c.ScoreAj < 0.5 {
				addRisk("adjustment_weak", tx.EntityID, "Ajustement faiblement étayé", amt, impact, tx.ID)
			}
		}
	}

	if len(groups) == 0 {
		return nil
	}
	// Deterministic ordering: keys sorted, then result sorted by impact desc,
	// tie-break by kind asc / label asc / entity asc.
	keys := make([]groupKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Kind != keys[j].Kind {
			return keys[i].Kind < keys[j].Kind
		}
		return keys[i].EntityID < keys[j].EntityID
	})

	risks := make([]Risk, 0, len(keys))
	for _, k := range keys {
		risks = append(risks, groups[k].Risk)
	}
	sort.SliceStable(risks, func(i, j int) bool {
		if risks[i].ImpactPct != risks[j].ImpactPct {
			return risks[i].ImpactPct > risks[j].ImpactPct
		}
		if risks[i].Kind != risks[j].Kind {
			return risks[i].Kind < risks[j].Kind
		}
		if risks[i].Label != risks[j].Label {
			return risks[i].Label < risks[j].Label
		}
		return risks[i].EntityID < risks[j].EntityID
	})
	if len(risks) > 5 {
		risks = risks[:5]
	}
	return risks
}

func filterIndex(txs []Transaction, status string) []int {
	out := make([]int, 0, len(txs))
	for i := range txs {
		if txs[i].Status == status {
			out = append(out, i)
		}
	}
	return out
}
