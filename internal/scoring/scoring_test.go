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
