package scoring

import (
	"math"
	"runtime"
	"testing"
)

func TestMateriality(t *testing.T) {
	cases := []struct {
		pnl  float64
		want float64
	}{
		{0, 100},          // floor
		{100_000, 100},    // 0.05% = 50€ → floor wins
		{200_000, 100},    // 0.05% = 100€ → still floor
		{1_000_000, 500},  // 0.05% = 500€ wins
		{10_000_000, 5000},
	}
	for _, c := range cases {
		got := Materiality(c.pnl)
		if got != c.want {
			t.Errorf("Materiality(%v) = %v want %v", c.pnl, got, c.want)
		}
	}
}

func TestSensitivity(t *testing.T) {
	cases := map[string]float64{
		"ca":            1.5,
		"CA":            1.5,
		"  salaires  ":  1.3,
		"loyer":         1.2,
		"achats":        1.0,
		"divers":        0.8,
		"":              0.8,
		"unknown_cat":   0.8,
	}
	for in, want := range cases {
		if got := Sensitivity(in); got != want {
			t.Errorf("Sensitivity(%q) = %v want %v", in, got, want)
		}
	}
}

func TestCoherencePCG(t *testing.T) {
	cases := []struct {
		account, category string
		want              float64
	}{
		{"706000", "ca", 1.0},
		{"707", "ca", 1.0},
		{"707", "achats", 0.0},      // produit catégorisé en charge
		{"706", "", 0.0},            // no category
		{"606300", "achats", 1.0},
		{"606300", "ca", 0.0},
		{"613000", "loyer", 1.0},
		{"613000", "achats", 0.5},   // loyer plausibly classed as services externes
		{"641000", "salaires", 1.0},
		{"645000", "salaires", 1.0},
		{"647000", "salaires", 1.0},
		{"695000", "divers", 1.0},   // IS
		{"", "ca", 0.0},             // no account
		{"abc", "ca", 0.0},          // garbage account
	}
	for _, c := range cases {
		got := CoherencePCG(c.account, c.category)
		if got != c.want {
			t.Errorf("CoherencePCG(%q,%q) = %v want %v", c.account, c.category, got, c.want)
		}
	}
}

func TestLevel(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{1.0, "tres_fiable"},
		{0.90, "tres_fiable"},
		{0.85, "envoyable"},
		{0.80, "acceptable"},
		{0.70, "fragile"},
		{0.50, "non_fiable"},
		{0.0, "non_fiable"},
	}
	for _, c := range cases {
		got, _ := Level(c.score)
		if got != c.want {
			t.Errorf("Level(%v) = %q want %q", c.score, got, c.want)
		}
	}
}

func TestComputeTreatedEmpty(t *testing.T) {
	block, comp := ComputeTreated(nil, nil, Config{})
	if block.Amount != 0 || block.Score != 0 {
		t.Errorf("empty input must yield zero block, got %+v", block)
	}
	if len(comp) != 0 {
		t.Errorf("expected zero components, got %d", len(comp))
	}
}

func TestComputeTreatedHappyPath(t *testing.T) {
	// Three transactions, all traité. Identity high, coherence varies.
	txs := []Transaction{
		{
			ID:              "tx_001",
			Amount:          -1000.0, // charge
			Status:          StatusTraite,
			Account:         "606300",
			Category:        "achats",
			EntityID:        "ent_aaa",
			MatchConfidence: 1.0,
		},
		{
			ID:              "tx_002",
			Amount:          5000.0,
			Status:          StatusTraite,
			Account:         "706000",
			Category:        "ca",
			EntityID:        "ent_bbb",
			MatchConfidence: 0.95,
		},
		{
			ID:              "tx_003",
			Amount:          -300.0,
			Status:          StatusTraite,
			Account:         "",         // no PCG account
			Category:        "achats",
			EntityID:        "",         // unmatched
			MatchConfidence: 0.0,
		},
	}
	block, components := ComputeTreated(txs, nil, Config{SkipRecurrence: true, SkipAmountCoherence: true})

	// Total amount = |1000| + |5000| + |300| = 6300
	if block.Amount != 6300 {
		t.Errorf("Amount = %v want 6300", block.Amount)
	}

	// score_tx for tx_001:
	//   identité 1.0, cohérence 1.0, récurrence 0.4 (skipped → fixed), montant 0.6 (skipped → fixed)
	//   = 0.4 + 0.3 + 0.08 + 0.06 = 0.84
	// score_tx for tx_002:
	//   identité 0.95, cohérence 1.0, rec 0.4, mt 0.6
	//   = 0.38 + 0.3 + 0.08 + 0.06 = 0.82
	// score_tx for tx_003:
	//   identité 0.0, cohérence 0.0, rec 0.4, mt 0.6
	//   = 0 + 0 + 0.08 + 0.06 = 0.14
	c1 := components["tx_001"].ScoreTx
	c2 := components["tx_002"].ScoreTx
	c3 := components["tx_003"].ScoreTx
	if !approx(c1, 0.84, 1e-9) {
		t.Errorf("score_tx[tx_001] = %v want 0.84", c1)
	}
	if !approx(c2, 0.82, 1e-9) {
		t.Errorf("score_tx[tx_002] = %v want 0.82", c2)
	}
	if !approx(c3, 0.14, 1e-9) {
		t.Errorf("score_tx[tx_003] = %v want 0.14", c3)
	}

	// Weighted block score = (1000·0.84 + 5000·0.82 + 300·0.14) / 6300
	//                      = (840 + 4100 + 42) / 6300 = 4982 / 6300 ≈ 0.7908
	expected := (1000*0.84 + 5000*0.82 + 300*0.14) / 6300
	if !approx(block.Score, expected, 1e-9) {
		t.Errorf("block.Score = %v want %v", block.Score, expected)
	}
}

func TestComputeTreatedSkipsNonTreated(t *testing.T) {
	// Mix of statuses: only traité should be folded into the block.
	txs := []Transaction{
		{ID: "a", Amount: 1000, Status: StatusTraite, MatchConfidence: 1.0, Account: "706", Category: "ca"},
		{ID: "b", Amount: 2000, Status: StatusNonTraite},
		{ID: "c", Amount: 500, Status: StatusAjustement},
	}
	block, components := ComputeTreated(txs, nil, Config{SkipRecurrence: true, SkipAmountCoherence: true})
	if block.Amount != 1000 {
		t.Errorf("block.Amount = %v want 1000 (only traité tx counts)", block.Amount)
	}
	if _, ok := components["a"]; !ok {
		t.Errorf("traité tx 'a' missing from components")
	}
	if _, ok := components["b"]; ok {
		t.Errorf("non_traite tx 'b' must not appear in components")
	}
	if _, ok := components["c"]; ok {
		t.Errorf("ajustement tx 'c' must not appear in components")
	}
}

func TestComputeTreatedDeterminism(t *testing.T) {
	// Same input → same output, regardless of GOMAXPROCS or call count.
	txs := []Transaction{
		{ID: "x", Amount: 100, Status: StatusTraite, MatchConfidence: 1.0, Account: "706", Category: "ca"},
		{ID: "y", Amount: 200, Status: StatusTraite, MatchConfidence: 0.8, Account: "606", Category: "achats"},
		{ID: "z", Amount: 50, Status: StatusTraite, MatchConfidence: 0.5, Account: "613", Category: "loyer"},
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	prev := runtime.GOMAXPROCS(1)
	first, _ := ComputeTreated(txs, nil, Config{SkipRecurrence: true, SkipAmountCoherence: true})
	runtime.GOMAXPROCS(prev)

	for procs := 1; procs <= 8; procs++ {
		runtime.GOMAXPROCS(procs)
		for i := 0; i < 25; i++ {
			got, _ := ComputeTreated(txs, nil, Config{SkipRecurrence: true, SkipAmountCoherence: true})
			if got != first {
				t.Fatalf("ComputeTreated drifted at procs=%d iter=%d:\n  first: %+v\n  got:   %+v", procs, i, first, got)
			}
		}
	}
}

func TestComputeTreatedRecurrence(t *testing.T) {
	// Same entity, three prior versions → recurrence = 1.0.
	history := []HistoricalSnapshot{
		{Version: "v1", EntityMonths: map[string]map[string]int{"ent_recur": {"2024-01": 1}}},
		{Version: "v2", EntityMonths: map[string]map[string]int{"ent_recur": {"2024-02": 1}}},
		{Version: "v3", EntityMonths: map[string]map[string]int{"ent_recur": {"2024-03": 1}}},
	}
	tx := Transaction{
		ID: "t", Amount: 100, Status: StatusTraite,
		EntityID: "ent_recur", MatchConfidence: 1.0,
		Account: "706", Category: "ca",
	}
	_, comp := ComputeTreated([]Transaction{tx}, history, Config{SkipAmountCoherence: true})
	if got := comp["t"].Recurrence; got != 1.0 {
		t.Errorf("recurrence = %v want 1.0", got)
	}
	if got := comp["t"].ScoreTx; !approx(got, 0.40+0.30+0.20+0.06, 1e-9) {
		t.Errorf("score_tx = %v want 0.96", got)
	}
}

func TestComputeTreatedAmountCoherence(t *testing.T) {
	history := []HistoricalSnapshot{
		{Version: "v1", EntityAmounts: map[string][]float64{
			"ent_x": {1000, 1000, 1000, 1000, 1000},
		}},
	}
	// Amount matches mean exactly → z=0 → 1.0
	tx := Transaction{
		ID: "t1", Amount: 1000, Status: StatusTraite,
		EntityID: "ent_x", MatchConfidence: 1.0,
		Account: "606", Category: "achats",
	}
	_, comp := ComputeTreated([]Transaction{tx}, history, Config{SkipRecurrence: true})
	if got := comp["t1"].MontantCoh; got != 1.0 {
		t.Errorf("montant coherence at z=0 = %v want 1.0", got)
	}

	// Way outside → 0.1
	tx2 := tx
	tx2.ID = "t2"
	tx2.Amount = 1_000_000
	_, comp2 := ComputeTreated([]Transaction{tx2}, history, Config{SkipRecurrence: true})
	if got := comp2["t2"].MontantCoh; got != 0.1 {
		t.Errorf("montant coherence at extreme = %v want 0.1", got)
	}
}

// approx returns true if a and b are within epsilon of each other.
func approx(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

func sameRisk(a, b Risk) bool {
	if a.Kind != b.Kind || a.Label != b.Label || a.EntityID != b.EntityID {
		return false
	}
	if a.Amount != b.Amount || a.ImpactPct != b.ImpactPct {
		return false
	}
	if len(a.TxIDs) != len(b.TxIDs) {
		return false
	}
	for i := range a.TxIDs {
		if a.TxIDs[i] != b.TxIDs[i] {
			return false
		}
	}
	return true
}

func TestComputeUntreatedSensitivity(t *testing.T) {
	// totalPnL = 10 000 ; one non_traité tx with 1000 CA (×1.5 = 1500),
	// one with 500 divers (×0.8 = 400). Weighted = 1900.
	// Score = 1 - 1900/10000 = 0.81. Amount = 1500 unweighted.
	txs := []Transaction{
		{ID: "a", Amount: 1000, Status: StatusNonTraite, Category: "ca"},
		{ID: "b", Amount: -500, Status: StatusNonTraite, Category: "divers"},
	}
	block, _ := ComputeUntreated(txs, 10_000)
	if block.Amount != 1500 {
		t.Errorf("block.Amount = %v want 1500", block.Amount)
	}
	if !approx(block.Score, 0.81, 1e-9) {
		t.Errorf("block.Score = %v want 0.81", block.Score)
	}
}

func TestComputeUntreatedZeroPnL(t *testing.T) {
	block, _ := ComputeUntreated(nil, 0)
	if block.Score != 0 || block.Amount != 0 {
		t.Errorf("zero input must yield zero block, got %+v", block)
	}
}

func TestComputeAdjustedHappyPath(t *testing.T) {
	// Adjustment with pattern_historique 1.0 (entity in 1 of 1 prior version),
	// signal_actuel 0.9, coherence_metier 1.0 (FNP on 408x).
	// score_aj = 0.45*1 + 0.35*0.9 + 0.20*1 = 0.45 + 0.315 + 0.20 = 0.965
	history := []HistoricalSnapshot{
		{Version: "v1", EntityMonths: map[string]map[string]int{"ent_x": {"2024-01": 1}}},
	}
	sa := 0.9
	txs := []Transaction{{
		ID: "aj1", Amount: -2000, Status: StatusAjustement,
		EntityID: "ent_x", Account: "408100",
		Adjustment: &Adjustment{Reason: "fnp", SignalActuel: &sa},
	}}
	block, comp := ComputeAdjusted(txs, history)
	if block.Amount != 2000 {
		t.Errorf("Amount = %v want 2000", block.Amount)
	}
	if !approx(comp["aj1"].ScoreAj, 0.965, 1e-9) {
		t.Errorf("score_aj = %v want 0.965", comp["aj1"].ScoreAj)
	}
}

func TestComputeAdjustedDefaultSignal(t *testing.T) {
	// No signal_actuel from agent → default 0.5. Pattern hist = 0 (no history).
	// Coherence_metier with no reason → 0.5.
	// score_aj = 0.45*0 + 0.35*0.5 + 0.20*0.5 = 0 + 0.175 + 0.10 = 0.275
	txs := []Transaction{{
		ID:         "aj1",
		Amount:     -1000,
		Status:     StatusAjustement,
		Adjustment: &Adjustment{},
	}}
	_, comp := ComputeAdjusted(txs, nil)
	if !approx(comp["aj1"].ScoreAj, 0.275, 1e-9) {
		t.Errorf("score_aj = %v want 0.275", comp["aj1"].ScoreAj)
	}
}

func TestComputeGlobal(t *testing.T) {
	// Scenario: 80% traité bien classé, 15% non traité divers, 5% ajustement.
	// totalPnL = 10 000.
	txs := []Transaction{
		// 8 traité × 1000 ; identité 1.0, cohérence 1.0
		{ID: "t01", Amount: 1000, Status: StatusTraite, Account: "706", Category: "ca", MatchConfidence: 1.0},
		{ID: "t02", Amount: 1000, Status: StatusTraite, Account: "706", Category: "ca", MatchConfidence: 1.0},
		{ID: "t03", Amount: 1000, Status: StatusTraite, Account: "706", Category: "ca", MatchConfidence: 1.0},
		{ID: "t04", Amount: 1000, Status: StatusTraite, Account: "706", Category: "ca", MatchConfidence: 1.0},
		{ID: "t05", Amount: -1000, Status: StatusTraite, Account: "606", Category: "achats", MatchConfidence: 1.0},
		{ID: "t06", Amount: -1000, Status: StatusTraite, Account: "606", Category: "achats", MatchConfidence: 1.0},
		{ID: "t07", Amount: -1000, Status: StatusTraite, Account: "606", Category: "achats", MatchConfidence: 1.0},
		{ID: "t08", Amount: -1000, Status: StatusTraite, Account: "606", Category: "achats", MatchConfidence: 1.0},
		// 1500 non traité divers (×0.8)
		{ID: "u01", Amount: -1500, Status: StatusNonTraite, Category: "divers"},
		// 500 ajustement (default neutral)
		{ID: "a01", Amount: -500, Status: StatusAjustement, Adjustment: &Adjustment{}},
	}
	r, _ := Compute(txs, nil, Config{SkipRecurrence: true, SkipAmountCoherence: true})
	if r.TotalPnL != 10_000 {
		t.Errorf("TotalPnL = %v want 10000", r.TotalPnL)
	}
	// traité Amount = 8000
	if r.Treated.Amount != 8000 {
		t.Errorf("Treated.Amount = %v want 8000", r.Treated.Amount)
	}
	// Untreated Amount = 1500, Score = 1 - (1500*0.8)/10000 = 1 - 0.12 = 0.88
	if r.Untreated.Amount != 1500 || !approx(r.Untreated.Score, 0.88, 1e-9) {
		t.Errorf("Untreated bloc = %+v", r.Untreated)
	}
	// All three blocks fold into the weighted global.
	denom := r.Treated.Amount + r.Untreated.Amount + r.Adjusted.Amount
	expectedGlobal := (r.Treated.Amount*r.Treated.Score +
		r.Untreated.Amount*r.Untreated.Score +
		r.Adjusted.Amount*r.Adjusted.Score) / denom
	if !approx(r.Global, expectedGlobal, 1e-9) {
		t.Errorf("Global = %v want %v", r.Global, expectedGlobal)
	}
	if r.Materiality != 100 {
		t.Errorf("Materiality = %v want 100", r.Materiality)
	}
}

func TestComputeTopRisksOrderedByImpact(t *testing.T) {
	// Two non-traité on distinct entities: each gets its own group.
	// CA × 1.5 × 3000 = 4500 impact ; divers × 0.8 × 200 = 160 impact.
	// totalPnL = 3200, so CA risk has highest ImpactPct.
	txs := []Transaction{
		{ID: "small", Amount: -200, Status: StatusNonTraite, Category: "divers", EntityID: "ent_small"},
		{ID: "big", Amount: -3000, Status: StatusNonTraite, Category: "ca", EntityID: "ent_big"},
	}
	r, _ := Compute(txs, nil, Config{})
	if len(r.TopRisks) < 2 {
		t.Fatalf("expected 2 risks, got %d", len(r.TopRisks))
	}
	if r.TopRisks[0].Amount != 3000 {
		t.Errorf("first risk Amount = %v want 3000", r.TopRisks[0].Amount)
	}
	if r.TopRisks[1].Amount != 200 {
		t.Errorf("second risk Amount = %v want 200", r.TopRisks[1].Amount)
	}
}

func TestComputeTopRisksGroupsWhenSameEntity(t *testing.T) {
	// Same entity ID → grouped into a single risk.
	txs := []Transaction{
		{ID: "a", Amount: -200, Status: StatusNonTraite, Category: "divers"},
		{ID: "b", Amount: -300, Status: StatusNonTraite, Category: "divers"},
	}
	r, _ := Compute(txs, nil, Config{})
	if len(r.TopRisks) != 1 {
		t.Fatalf("expected 1 grouped risk, got %d", len(r.TopRisks))
	}
	if r.TopRisks[0].Amount != 500 {
		t.Errorf("grouped risk Amount = %v want 500", r.TopRisks[0].Amount)
	}
	if len(r.TopRisks[0].TxIDs) != 2 {
		t.Errorf("expected 2 tx ids in grouped risk, got %d", len(r.TopRisks[0].TxIDs))
	}
}

// ptr returns a pointer to the given float64 (test convenience).
func ptr(v float64) *float64 { return &v }

func TestComputeDeterminism(t *testing.T) {
	// Same fixture, 100 runs, varying GOMAXPROCS. Result must be identical.
	txs := []Transaction{
		{ID: "x", Amount: 1000, Status: StatusTraite, Account: "706", Category: "ca", MatchConfidence: 0.9},
		{ID: "y", Amount: -200, Status: StatusNonTraite, Category: "salaires"},
		{ID: "z", Amount: -50, Status: StatusAjustement, EntityID: "ent_e",
			Account: "486", Adjustment: &Adjustment{Reason: "cca", SignalActuel: ptr(0.7)}},
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	first, _ := Compute(txs, nil, Config{})
	for procs := 1; procs <= 8; procs++ {
		runtime.GOMAXPROCS(procs)
		for i := 0; i < 25; i++ {
			r, _ := Compute(txs, nil, Config{})
			// Result is a struct with slices; deep equal by re-marshaling
			// (the marshaled forms are deterministic too).
			if r.Global != first.Global || r.Treated != first.Treated ||
				r.Untreated != first.Untreated || r.Adjusted != first.Adjusted ||
				r.TotalPnL != first.TotalPnL || r.Materiality != first.Materiality ||
				len(r.TopRisks) != len(first.TopRisks) {
				t.Fatalf("Compute drifted at procs=%d iter=%d", procs, i)
			}
			for k := range r.TopRisks {
				if !sameRisk(r.TopRisks[k], first.TopRisks[k]) {
					t.Fatalf("TopRisk[%d] drifted at procs=%d iter=%d", k, procs, i)
				}
			}
		}
	}
}
