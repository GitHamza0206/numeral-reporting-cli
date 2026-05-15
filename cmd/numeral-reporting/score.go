package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/numeral/numeral-reporting-cli/internal/entities"
	"github.com/numeral/numeral-reporting-cli/internal/scoring"
)

// entities.json sits at the project root, shared across versions.
func entitiesPath(root string) string {
	return filepath.Join(root, "entities.json")
}

// loadEntities reads the project store, returning an empty store if the file
// does not exist. Any other I/O or decode error bubbles up.
func loadEntities(root string) (*entities.Store, error) {
	path := entitiesPath(root)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return entities.NewStore(), nil
		}
		return nil, err
	}
	defer f.Close()
	return entities.Load(f)
}

// saveEntities writes the store to disk atomically (write-temp + rename).
func saveEntities(root string, store *entities.Store) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	path := entitiesPath(root)
	tmp, err := os.CreateTemp(root, ".entities.*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := store.Save(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// cmdEntities dispatches the `entities` subcommand. The first positional is
// the sub-subcommand; remaining args (flags interleaved with positionals) are
// reordered so flag.Parse handles them regardless of caller ordering.
func cmdEntities(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("entities: subcommand required (list|show|merge|split|rename|reset)")
	}
	sub, rest := args[0], reorderFlags(args[1:])
	switch sub {
	case "list":
		return entitiesList(rest)
	case "show":
		return entitiesShow(rest)
	case "merge":
		return entitiesMerge(rest)
	case "split":
		return entitiesSplit(rest)
	case "rename":
		return entitiesRename(rest)
	case "reset":
		return entitiesReset(rest)
	case "-h", "--help", "help":
		fmt.Print(entitiesUsage)
		return nil
	default:
		return fmt.Errorf("entities: unknown subcommand %q (want list|show|merge|split|rename|reset)", sub)
	}
}

const entitiesUsage = `numeral-reporting entities — manage the project entity table

Usage:
  numeral-reporting entities list   [--project DIR] [--kind KIND] [--json]
  numeral-reporting entities show   <id> [--project DIR]
  numeral-reporting entities merge  <src_id> <dst_id> [--project DIR]
  numeral-reporting entities split  <id> <new_name> --keys k1,k2,... [--project DIR]
  numeral-reporting entities rename <id> <new_name> [--project DIR]
  numeral-reporting entities reset  [--project DIR] [--yes]

entities.json sits at the project root, shared across versions.
`

func entitiesList(args []string) error {
	fs := flag.NewFlagSet("entities list", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	kind := fs.String("kind", "", "filter by kind (fournisseur|client|salarie|banque|fiscal|autre)")
	asJSON := fs.Bool("json", false, "emit JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	store, err := loadEntities(root)
	if err != nil {
		return err
	}

	// Filter (sorted: store.Sort already ran in Load).
	out := make([]entities.Entity, 0, len(store.Entities))
	for _, e := range store.Entities {
		if *kind != "" && string(e.Kind) != *kind {
			continue
		}
		out = append(out, e)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Entities []entities.Entity `json:"entities"`
		}{Entities: out})
	}

	if len(out) == 0 {
		fmt.Println("(no entities yet — run `numeral-reporting score --write` to populate)")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tCANONICAL\tKEYS\tIBAN\tSIRET")
	for _, e := range out {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n",
			e.ID,
			string(e.Kind),
			truncate(e.CanonicalName, 40),
			len(e.NormalizedKeys),
			len(e.IBANs),
			e.SIRET,
		)
	}
	return w.Flush()
}

func entitiesShow(args []string) error {
	fs := flag.NewFlagSet("entities show", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("entities show: expected exactly one entity id, got %d", len(rest))
	}
	id := rest[0]
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	store, err := loadEntities(root)
	if err != nil {
		return err
	}
	e := store.FindByID(id)
	if e == nil {
		return fmt.Errorf("entity %q not found", id)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(e)
}

func entitiesReset(args []string) error {
	fs := flag.NewFlagSet("entities reset", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	path := entitiesPath(root)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("entities.json does not exist — nothing to reset")
		return nil
	}
	if !*yes {
		return fmt.Errorf("entities reset: pass --yes to confirm (will delete %s)", path)
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	fmt.Println("removed", path)
	return nil
}

func truncate(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// ---- entities merge / split / rename ----

func entitiesMerge(args []string) error {
	fs := flag.NewFlagSet("entities merge", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("entities merge: expected <src_id> <dst_id>, got %d args", len(rest))
	}
	srcID, dstID := rest[0], rest[1]
	if srcID == dstID {
		return fmt.Errorf("entities merge: src and dst must differ")
	}
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	store, err := loadEntities(root)
	if err != nil {
		return err
	}
	src := store.FindByID(srcID)
	dst := store.FindByID(dstID)
	if src == nil {
		return fmt.Errorf("entities merge: source entity %q not found", srcID)
	}
	if dst == nil {
		return fmt.Errorf("entities merge: destination entity %q not found", dstID)
	}

	// Union keys/IBANs/aliases on dst, drop src.
	dst.NormalizedKeys = append(dst.NormalizedKeys, src.NormalizedKeys...)
	dst.IBANs = append(dst.IBANs, src.IBANs...)
	dst.Aliases = append(dst.Aliases, src.Aliases...)
	if dst.SIRET == "" {
		dst.SIRET = src.SIRET
	}
	dst.ManualOverrides = append(dst.ManualOverrides, entities.Override{
		Kind:   entities.OverrideMergeInto,
		Source: src.ID,
		Target: dst.ID,
		Note:   "merged from " + src.CanonicalName,
		Date:   time.Now().UTC().Format(time.RFC3339),
	})

	// Remove src.
	out := store.Entities[:0]
	for _, e := range store.Entities {
		if e.ID == src.ID {
			continue
		}
		out = append(out, e)
	}
	store.Entities = out

	if err := saveEntities(root, store); err != nil {
		return err
	}
	fmt.Printf("merged %s → %s (%d keys, %d ibans)\n",
		srcID, dstID, len(dst.NormalizedKeys), len(dst.IBANs))
	return nil
}

func entitiesSplit(args []string) error {
	fs := flag.NewFlagSet("entities split", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	keys := fs.String("keys", "", "comma-separated normalized keys to move into the new entity (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("entities split: expected <id> <new_canonical_name>, got %d args", len(rest))
	}
	id, newName := rest[0], rest[1]
	if *keys == "" {
		return fmt.Errorf("entities split: --keys is required")
	}
	splitKeys := splitCSV(*keys)
	if len(splitKeys) == 0 {
		return fmt.Errorf("entities split: --keys produced no entries")
	}
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	store, err := loadEntities(root)
	if err != nil {
		return err
	}
	parent := store.FindByID(id)
	if parent == nil {
		return fmt.Errorf("entities split: entity %q not found", id)
	}

	// Validate every requested key belongs to the parent.
	have := map[string]struct{}{}
	for _, k := range parent.NormalizedKeys {
		have[k] = struct{}{}
	}
	for _, k := range splitKeys {
		if _, ok := have[k]; !ok {
			return fmt.Errorf("entities split: key %q does not belong to %s", k, id)
		}
	}

	// Build the new entity. ID derived from new canonical name (stable).
	newID := entities.NewEntityID(newName)
	if newID == id {
		return fmt.Errorf("entities split: new canonical name yields the same ID as the source")
	}
	newKeysSet := map[string]struct{}{}
	for _, k := range splitKeys {
		newKeysSet[k] = struct{}{}
	}
	keepOnParent := parent.NormalizedKeys[:0]
	for _, k := range parent.NormalizedKeys {
		if _, ok := newKeysSet[k]; ok {
			continue
		}
		keepOnParent = append(keepOnParent, k)
	}
	parent.NormalizedKeys = keepOnParent
	parent.ManualOverrides = append(parent.ManualOverrides, entities.Override{
		Kind:   entities.OverrideSplitFrom,
		Source: parent.ID,
		Target: newID,
		Note:   "split keys: " + *keys,
		Date:   time.Now().UTC().Format(time.RFC3339),
	})

	newEntity := entities.Entity{
		ID:             newID,
		CanonicalName:  newName,
		Kind:           parent.Kind,
		NormalizedKeys: splitKeys,
		CreatedByRun:   "manual_split",
		FirstSeen:      time.Now().UTC().Format(time.RFC3339),
	}
	store.Entities = append(store.Entities, newEntity)

	if err := saveEntities(root, store); err != nil {
		return err
	}
	fmt.Printf("split %s → new %s (%d keys moved)\n", id, newID, len(splitKeys))
	return nil
}

func entitiesRename(args []string) error {
	fs := flag.NewFlagSet("entities rename", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("entities rename: expected <id> <new_canonical_name>, got %d args", len(rest))
	}
	id, newName := rest[0], rest[1]
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	store, err := loadEntities(root)
	if err != nil {
		return err
	}
	e := store.FindByID(id)
	if e == nil {
		return fmt.Errorf("entities rename: entity %q not found", id)
	}
	old := e.CanonicalName
	e.CanonicalName = newName
	if err := saveEntities(root, store); err != nil {
		return err
	}
	fmt.Printf("renamed %s: %q → %q\n", id, old, newName)
	return nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := []string{}
	current := []rune{}
	for _, r := range s {
		if r == ',' {
			if t := trimRunes(current); len(t) > 0 {
				parts = append(parts, string(t))
			}
			current = current[:0]
			continue
		}
		current = append(current, r)
	}
	if t := trimRunes(current); len(t) > 0 {
		parts = append(parts, string(t))
	}
	return parts
}

func trimRunes(in []rune) []rune {
	start, end := 0, len(in)
	for start < end && (in[start] == ' ' || in[start] == '\t') {
		start++
	}
	for end > start && (in[end-1] == ' ' || in[end-1] == '\t') {
		end--
	}
	return in[start:end]
}

// ---- score subcommand ----

// transactionsFile mirrors versions/vN/transactions.json on disk.
type transactionsFile struct {
	SchemaVersion int                     `json:"schema_version"`
	Period        string                  `json:"period,omitempty"`
	TotalPnL      float64                 `json:"total_pnl,omitempty"`
	Materiality   float64                 `json:"materiality_threshold,omitempty"`
	GeneratedAt   string                  `json:"generated_at,omitempty"`
	Transactions  []scoring.Transaction   `json:"transactions"`
}

func transactionsPath(root string, n int) string {
	return filepath.Join(staticVersionDir(root, n), "transactions.json")
}

func loadTransactions(root string, n int) (*transactionsFile, error) {
	path := transactionsPath(root, n)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: no transactions.json — run the agent's categorize step first", path)
		}
		return nil, err
	}
	var f transactionsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.SchemaVersion == 0 {
		f.SchemaVersion = scoring.SchemaVersion
	}
	return &f, nil
}

func saveTransactions(root string, n int, f *transactionsFile) error {
	path := transactionsPath(root, n)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".transactions.*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(f); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// cmdScore implements `numeral-reporting score`.
func cmdScore(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	version := fs.String("version", "", "version (default: active)")
	asJSON := fs.Bool("json", false, "emit JSON to stdout")
	write := fs.Bool("write", false, "persist computed scores back into report.json and transactions.json")
	threshold := fs.Int("score-threshold", 0, "if >0, exit 3 when global score < threshold (in percent)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	n, err := staticVersionFromFlag(root, *version)
	if err != nil {
		return err
	}

	txFile, err := loadTransactions(root, n)
	if err != nil {
		return err
	}
	store, err := loadEntities(root)
	if err != nil {
		return err
	}

	// Re-run Resolve on every transaction. This is idempotent: same store +
	// same libelle → same match. Persists EntityID/MatchConfidence/MatchKind
	// only on --write.
	for i := range txFile.Transactions {
		m := entities.Resolve(store, txFile.Transactions[i].LibelleRaw)
		txFile.Transactions[i].LibelleNorm = m.Norm
		txFile.Transactions[i].EntityID = m.EntityID
		txFile.Transactions[i].MatchConfidence = m.Confidence
		txFile.Transactions[i].MatchKind = string(m.Kind)
	}

	// History: load all frozen versions strictly older than n, deterministically.
	history, err := loadHistorySnapshots(root, n)
	if err != nil {
		return err
	}

	result, components := scoring.Compute(txFile.Transactions, history, scoring.Config{})

	if *write {
		// Persist components into transactions.json.
		for i := range txFile.Transactions {
			c, ok := components[txFile.Transactions[i].ID]
			if !ok {
				txFile.Transactions[i].Components = nil
				continue
			}
			cp := c
			txFile.Transactions[i].Components = &cp
		}
		txFile.SchemaVersion = scoring.SchemaVersion
		txFile.TotalPnL = result.TotalPnL
		txFile.Materiality = result.Materiality
		txFile.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		if err := saveTransactions(root, n, txFile); err != nil {
			return err
		}
		if err := persistReportScore(root, n, result); err != nil {
			return err
		}
	}

	if *asJSON {
		out := struct {
			Version     string                 `json:"version"`
			Result      scoring.Result         `json:"result"`
			TopRisks    []scoring.Risk         `json:"top_risks"`
			ComputedAt  string                 `json:"computed_at,omitempty"`
		}{
			Version:    fmt.Sprintf("v%d", n),
			Result:     result,
			TopRisks:   result.TopRisks,
			ComputedAt: txFile.GeneratedAt,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(out); err != nil {
			return err
		}
	} else {
		printScoreSummary(n, result)
	}

	if *threshold > 0 && int(math.Round(result.Global*100)) < *threshold {
		os.Exit(3)
	}
	return nil
}

func printScoreSummary(n int, r scoring.Result) {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintf(w, "score v%d — Total P&L %.0f €  (materiality %.0f €)\n", n, r.TotalPnL, r.Materiality)
	fmt.Fprintf(w, "  traité\t%.0f €\t%.1f %%\n", r.Treated.Amount, r.Treated.Score*100)
	fmt.Fprintf(w, "  non traité\t%.0f €\t%.1f %%\n", r.Untreated.Amount, r.Untreated.Score*100)
	fmt.Fprintf(w, "  ajusté\t%.0f €\t%.1f %%\n", r.Adjusted.Amount, r.Adjusted.Score*100)
	fmt.Fprintf(w, "  global\t\t%.0f %%  (%s)\n", r.Global*100, r.Level)
	w.Flush()
	if len(r.TopRisks) > 0 {
		fmt.Println("\ntop risks")
		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		for i, rk := range r.TopRisks {
			fmt.Fprintf(w, "  %d.\t%s\t%.0f €\t(%.1f %%)\n", i+1, rk.Label, rk.Amount, rk.ImpactPct)
		}
		w.Flush()
	}
}

// loadHistorySnapshots reads frozen versions < n and returns snapshots in
// ascending order. Missing transactions.json in a prior version is OK (pre-
// engine state) — that version simply contributes nothing.
func loadHistorySnapshots(root string, current int) ([]scoring.HistoricalSnapshot, error) {
	meta, err := readStaticMeta(root)
	if err != nil {
		return nil, err
	}
	type frozenN struct{ N int }
	var frozen []frozenN
	for _, v := range meta.Versions {
		if v.N < current && v.Frozen {
			frozen = append(frozen, frozenN{N: v.N})
		}
	}
	// Sort ascending by N for deterministic iteration.
	for i := 1; i < len(frozen); i++ {
		for j := i; j > 0 && frozen[j-1].N > frozen[j].N; j-- {
			frozen[j-1], frozen[j] = frozen[j], frozen[j-1]
		}
	}
	snaps := make([]scoring.HistoricalSnapshot, 0, len(frozen))
	for _, fz := range frozen {
		tx, err := loadTransactions(root, fz.N)
		if err != nil {
			// Missing transactions.json in older version: pre-engine, skip.
			var pe *fs.PathError
			if errors.As(err, &pe) {
				continue
			}
			// loadTransactions wraps with a custom "no transactions.json"
			// message when fs.ErrNotExist — detect via fs.ErrNotExist too.
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			// Other errors (malformed JSON): surface, don't silently swallow.
			return nil, err
		}
		snap := scoring.HistoricalSnapshot{
			Version:       fmt.Sprintf("v%d", fz.N),
			EntityAmounts: map[string][]float64{},
			EntityMonths:  map[string]map[string]int{},
		}
		for _, t := range tx.Transactions {
			if t.EntityID == "" {
				continue
			}
			snap.EntityAmounts[t.EntityID] = append(snap.EntityAmounts[t.EntityID], t.Amount)
			if snap.EntityMonths[t.EntityID] == nil {
				snap.EntityMonths[t.EntityID] = map[string]int{}
			}
			if t.PeriodMonth != "" {
				snap.EntityMonths[t.EntityID][t.PeriodMonth]++
			}
		}
		snaps = append(snaps, snap)
	}
	return snaps, nil
}

// persistReportScore overlays the score block into versions/vN/report.json.
// It preserves every other field via a json.RawMessage merge.
func persistReportScore(root string, n int, r scoring.Result) error {
	path := staticReportPath(root, n)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	score := buildReportScore(r)
	scoreJSON, err := json.MarshalIndent(score, "", "  ")
	if err != nil {
		return err
	}
	doc["score"] = scoreJSON

	// Re-emit document with sorted keys for byte-stable output.
	// json.Marshal of a map is sorted by key in stdlib.
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

type reportBlock struct {
	Amount      float64 `json:"amount"`
	Score       float64 `json:"score"`
	Percent     int     `json:"percent"`
	Description string  `json:"description"`
}

type reportRisk struct {
	Kind      string   `json:"kind"`
	Label     string   `json:"label"`
	EntityID  string   `json:"entity_id,omitempty"`
	Amount    float64  `json:"amount"`
	ImpactPct float64  `json:"impact_pct"`
	TxIDs     []string `json:"tx_ids,omitempty"`
}

type reportScore struct {
	Global              int           `json:"global"`
	Level               string        `json:"level"`
	Label               string        `json:"label"`
	Treated             *reportBlock  `json:"treated,omitempty"`
	Untreated           *reportBlock  `json:"untreated,omitempty"`
	Adjusted            *reportBlock  `json:"adjusted,omitempty"`
	TopRisks            []reportRisk  `json:"top_risks,omitempty"`
	Materiality         float64       `json:"materiality_threshold,omitempty"`
	ComputedAt          string        `json:"computed_at,omitempty"`
	ScoreSchemaVersion  int           `json:"score_schema_version,omitempty"`
}

func buildReportScore(r scoring.Result) reportScore {
	mkBlock := func(b scoring.Block) *reportBlock {
		return &reportBlock{
			Amount:      b.Amount,
			Score:       b.Score,
			Percent:     int(math.Round(b.Score * 100)),
			Description: fmt.Sprintf("%.0f € (%.0f %%)", b.Amount, b.Score*100),
		}
	}
	risks := make([]reportRisk, 0, len(r.TopRisks))
	for _, rk := range r.TopRisks {
		risks = append(risks, reportRisk{
			Kind:      rk.Kind,
			Label:     rk.Label,
			EntityID:  rk.EntityID,
			Amount:    rk.Amount,
			ImpactPct: rk.ImpactPct,
			TxIDs:     rk.TxIDs,
		})
	}
	return reportScore{
		Global:             int(math.Round(r.Global * 100)),
		Level:              r.Level,
		Label:              r.Label,
		Treated:            mkBlock(r.Treated),
		Untreated:          mkBlock(r.Untreated),
		Adjusted:           mkBlock(r.Adjusted),
		TopRisks:           risks,
		Materiality:        r.Materiality,
		ComputedAt:         time.Now().UTC().Format(time.RFC3339),
		ScoreSchemaVersion: scoring.SchemaVersion,
	}
}

