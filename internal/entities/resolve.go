package entities

import (
	"regexp"
	"sort"
	"strings"
)

// SimilarityThreshold is the minimum ratio a fuzzy candidate must reach to win.
// ratio = 1 - dist/maxLen where dist is Damerau-Levenshtein distance.
// Changing this constant requires bumping the scoring schema version.
const SimilarityThreshold = 0.85

// MatchKind enumerates the resolution paths in priority order.
type MatchKind string

const (
	MatchNone   MatchKind = "none"
	MatchManual MatchKind = "manual"
	MatchExact  MatchKind = "exact"
	MatchIBAN   MatchKind = "iban"
	MatchSIRET  MatchKind = "siret"
	MatchFuzzy  MatchKind = "fuzzy"
)

// Match is the outcome of a Resolve call.
type Match struct {
	EntityID   string
	Confidence float64
	Kind       MatchKind
	Norm       string // the normalized form used for matching, returned for caller persistence
}

var (
	// IBAN: ISO 13616 — two-letter country, 2 check digits, then 11-30 alphanumerics.
	// Matched on the uppercased raw (space-preserving) so word boundaries work.
	// Grouping spaces inside the IBAN body are not handled here; an agent
	// pre-normalizes those before feeding to Resolve.
	reIBAN = regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b`)
	// SIRET: 14 contiguous digits, with word boundaries.
	reSIRET = regexp.MustCompile(`\b\d{14}\b`)
)

// ExtractIBAN returns the first IBAN-shaped token in raw, uppercased, or ""
// if none. Matching runs on the space-preserving uppercased form so word
// boundaries function correctly.
func ExtractIBAN(raw string) string {
	return reIBAN.FindString(strings.ToUpper(raw))
}

// ExtractSIRET returns the first 14-digit SIRET-shaped token in raw, or "".
func ExtractSIRET(raw string) string {
	return reSIRET.FindString(raw)
}

// Resolve maps a raw libellé to an entity using a deterministic priority
// pipeline: manual override > exact normalized match > IBAN > SIRET > fuzzy.
//
// The store MUST have been sorted (Load and modifier helpers do this).
func Resolve(store *Store, raw string) Match {
	norm := Normalize(raw)

	// A — manual override wins.
	if id, ok := store.lookupOverride(norm); ok {
		if id == "" {
			return Match{Kind: MatchNone, Norm: norm}
		}
		return Match{EntityID: id, Confidence: 1.0, Kind: MatchManual, Norm: norm}
	}

	// B — exact normalized match (entities sorted by ID, keys sorted within entity).
	for i := range store.Entities {
		for _, k := range store.Entities[i].NormalizedKeys {
			if k == norm && norm != "" {
				return Match{EntityID: store.Entities[i].ID, Confidence: 1.0, Kind: MatchExact, Norm: norm}
			}
		}
	}

	// C — strong identifier match (IBAN beats SIRET).
	if iban := ExtractIBAN(raw); iban != "" {
		if e := store.FindByIBAN(iban); e != nil {
			return Match{EntityID: e.ID, Confidence: 0.98, Kind: MatchIBAN, Norm: norm}
		}
	}
	if siret := ExtractSIRET(raw); siret != "" {
		if e := store.FindBySIRET(siret); e != nil {
			return Match{EntityID: e.ID, Confidence: 0.97, Kind: MatchSIRET, Norm: norm}
		}
	}

	// D — fuzzy match on normalized keys (deterministic tie-break by ID).
	if norm == "" || len(norm) < 4 {
		return Match{Kind: MatchNone, Norm: norm}
	}
	bestRatio := -1.0
	bestID := ""
	for i := range store.Entities {
		e := &store.Entities[i]
		for _, k := range e.NormalizedKeys {
			if len(k) < 4 {
				continue
			}
			ratio := similarity(norm, k)
			switch {
			case ratio > bestRatio:
				bestRatio = ratio
				bestID = e.ID
			case ratio == bestRatio && e.ID < bestID:
				bestID = e.ID
			}
		}
	}
	if bestID != "" && bestRatio >= SimilarityThreshold {
		return Match{EntityID: bestID, Confidence: bestRatio, Kind: MatchFuzzy, Norm: norm}
	}

	return Match{Kind: MatchNone, Norm: norm}
}

// similarity returns 1 - dist/maxLen, where dist is Damerau-Levenshtein.
// Pure: same inputs → same float.
func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 || lb == 0 {
		return 0
	}
	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}
	dist := damerauLevenshtein(ra, rb)
	return 1.0 - float64(dist)/float64(maxLen)
}

// damerauLevenshtein computes the Damerau-Levenshtein distance with adjacent
// transposition between two rune slices.
func damerauLevenshtein(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := d[i-1][j] + 1
			if d[i][j-1]+1 < v {
				v = d[i][j-1] + 1
			}
			if d[i-1][j-1]+cost < v {
				v = d[i-1][j-1] + cost
			}
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				if t := d[i-2][j-2] + 1; t < v {
					v = t
				}
			}
			d[i][j] = v
		}
	}
	return d[la][lb]
}

// ClusterAction is a deterministic instruction emitted by Cluster.
type ClusterAction struct {
	Kind          string // "create_entity" | "attach_alias"
	CanonicalName string
	EntityID      string
	Keys          []string // for create_entity
	Aliases       []string // raw libellés that contributed
	RawSamples    []string // up to 3 sorted raw libellés for traceability
}

// Cluster proposes new entities for libellés that Resolve returned MatchNone.
// Pure: deterministic on the input. Caller decides whether to apply.
//
// Two libellés cluster together when:
//   - their normalized forms are identical, OR
//   - their similarity >= SimilarityThreshold and both have len >= 4.
//
// The canonical name is the most common raw form among the cluster, with
// ties broken alphabetically.
func Cluster(rawLibelles []string) []ClusterAction {
	// Group by normalized form first.
	groups := map[string][]string{}
	for _, raw := range rawLibelles {
		n := Normalize(raw)
		if n == "" {
			continue
		}
		groups[n] = append(groups[n], raw)
	}

	// Merge near-duplicate norms. Iterate norms in sorted order for determinism.
	norms := make([]string, 0, len(groups))
	for n := range groups {
		norms = append(norms, n)
	}
	sortStrings(norms)

	// Union-find on the sorted norm list.
	parent := make([]int, len(norms))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	for i := 0; i < len(norms); i++ {
		for j := i + 1; j < len(norms); j++ {
			a, b := norms[i], norms[j]
			if len(a) < 4 || len(b) < 4 {
				continue
			}
			if similarity(a, b) >= SimilarityThreshold {
				union(i, j)
			}
		}
	}

	clusters := map[int][]string{} // root index → list of norms
	for i := range norms {
		r := find(i)
		clusters[r] = append(clusters[r], norms[i])
	}

	roots := make([]int, 0, len(clusters))
	for r := range clusters {
		roots = append(roots, r)
	}
	sortInts(roots)

	actions := make([]ClusterAction, 0, len(roots))
	for _, r := range roots {
		members := clusters[r]
		sortStrings(members)
		var raws []string
		for _, m := range members {
			raws = append(raws, groups[m]...)
		}
		canonical := pickCanonical(raws)
		samples := uniqueSortedFirstN(raws, 3)
		actions = append(actions, ClusterAction{
			Kind:          "create_entity",
			CanonicalName: canonical,
			EntityID:      NewEntityID(canonical),
			Keys:          members,
			Aliases:       sortDedup(raws),
			RawSamples:    samples,
		})
	}
	return actions
}

// pickCanonical returns the most common raw libellé from the slice, tie-broken
// alphabetically. Deterministic.
func pickCanonical(raws []string) string {
	if len(raws) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, r := range raws {
		counts[r]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sortStrings(keys)
	best := keys[0]
	bestCount := counts[best]
	for _, k := range keys[1:] {
		if counts[k] > bestCount {
			best = k
			bestCount = counts[k]
		}
	}
	return best
}

func uniqueSortedFirstN(in []string, n int) []string {
	deduped := sortDedup(in)
	if len(deduped) <= n {
		return deduped
	}
	return deduped[:n]
}

func sortStrings(s []string) { sort.Strings(s) }
func sortInts(s []int)       { sort.Ints(s) }
