package entities

import (
	"bytes"
	"strings"
	"testing"
)

func makeStore(t *testing.T, entities ...Entity) *Store {
	t.Helper()
	s := NewStore()
	s.Entities = entities
	s.Sort()
	return s
}

func TestNewEntityIDIsStable(t *testing.T) {
	a := NewEntityID("URSSAF Ile-de-France")
	b := NewEntityID("URSSAF Ile-de-France")
	if a != b {
		t.Fatalf("NewEntityID not stable: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "ent_") || len(a) != 16 {
		t.Errorf("ID has unexpected shape: %q (expected ent_+12hex)", a)
	}
	if NewEntityID("") != "" {
		t.Errorf("empty canonical name should yield empty ID")
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	src := makeStore(t,
		Entity{ID: "ent_b", CanonicalName: "EDF", Kind: KindFournisseur, NormalizedKeys: []string{"edf"}},
		Entity{ID: "ent_a", CanonicalName: "OVH", Kind: KindFournisseur, NormalizedKeys: []string{"ovh", "ovh sas"}},
	)
	var buf bytes.Buffer
	if err := src.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	first := buf.String()

	dst, err := Load(strings.NewReader(first))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(dst.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(dst.Entities))
	}
	if dst.Entities[0].ID != "ent_a" || dst.Entities[1].ID != "ent_b" {
		t.Errorf("entities not sorted by ID after Load: %+v", dst.Entities)
	}

	// Round-trip: second Save must match the first byte-for-byte.
	var buf2 bytes.Buffer
	if err := dst.Save(&buf2); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if buf2.String() != first {
		t.Errorf("Save not byte-deterministic:\nfirst:  %s\nsecond: %s", first, buf2.String())
	}
}

func TestLoadEmpty(t *testing.T) {
	s, err := Load(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if s == nil || s.Version != SchemaVersion || len(s.Entities) != 0 {
		t.Errorf("expected empty store, got %+v", s)
	}
}

func TestResolvePriority(t *testing.T) {
	// Setup: an entity with normalized key "edf", IBAN, SIRET, plus a fuzzy
	// neighbor "edfx" that should never win when an exact match exists.
	edf := Entity{
		ID:             "ent_aaaaaa000001",
		CanonicalName:  "EDF",
		Kind:           KindFournisseur,
		NormalizedKeys: []string{"edf", "edf sa"},
		IBANs:          []string{"FR7630001007941234567890185"},
		SIRET:          "55208131766522",
	}
	other := Entity{
		ID:             "ent_aaaaaa000002",
		CanonicalName:  "EDFX similar",
		Kind:           KindFournisseur,
		NormalizedKeys: []string{"edfx similar"},
	}
	store := makeStore(t, edf, other)

	cases := []struct {
		name       string
		raw        string
		wantKind   MatchKind
		wantEntity string
	}{
		{"exact_match", "EDF", MatchExact, edf.ID},
		{"exact_match_variant", "EDF FACTURE 88412", MatchExact, edf.ID},
		{"iban_match", "VIREMENT IBAN FR7630001007941234567890185", MatchIBAN, edf.ID},
		{"siret_match", "PMT 55208131766522 facture", MatchSIRET, edf.ID},
		{"none", "ZZZ UNKNOWN COMPANY", MatchNone, ""},
		{"too_short", "AB", MatchNone, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := Resolve(store, c.raw)
			if m.Kind != c.wantKind {
				t.Errorf("Resolve(%q).Kind = %q want %q", c.raw, m.Kind, c.wantKind)
			}
			if m.EntityID != c.wantEntity {
				t.Errorf("Resolve(%q).EntityID = %q want %q", c.raw, m.EntityID, c.wantEntity)
			}
		})
	}
}

func TestResolveFuzzyAndTieBreak(t *testing.T) {
	a := Entity{
		ID:             "ent_aaaaaa000001",
		CanonicalName:  "Starbucks",
		Kind:           KindFournisseur,
		NormalizedKeys: []string{"starbucks coffee"},
	}
	b := Entity{
		ID:             "ent_bbbbbb000001",
		CanonicalName:  "Starbukcs",
		Kind:           KindFournisseur,
		NormalizedKeys: []string{"starbukcs coffee"}, // typo, same distance to "starbuks"
	}
	store := makeStore(t, a, b)

	// "starbuks coffee" is 1 edit from "starbucks coffee" (delete c)
	// and 1 transposition from "starbukcs coffee" (kc <-> ck).
	// Both have similar Damerau-Levenshtein distance; tie-break must
	// pick the smaller ID (ent_aaa... < ent_bbb...).
	m := Resolve(store, "STARBUKS COFFEE")
	if m.Kind != MatchFuzzy {
		t.Fatalf("expected fuzzy match, got %q", m.Kind)
	}
	if m.EntityID != a.ID {
		t.Errorf("expected lower ID %q to win tie-break, got %q", a.ID, m.EntityID)
	}

	// Below threshold → none.
	m2 := Resolve(store, "TOTALLY DIFFERENT THING")
	if m2.Kind != MatchNone {
		t.Errorf("expected MatchNone for unrelated libellé, got %q", m2.Kind)
	}
}

func TestResolveManualOverride(t *testing.T) {
	target := Entity{
		ID:             "ent_aaaaaa000001",
		CanonicalName:  "URSSAF",
		Kind:           KindFiscal,
		NormalizedKeys: []string{"urssaf"},
		ManualOverrides: []Override{
			{Kind: OverrideForceMatch, Source: "cotis sociales", Target: "ent_aaaaaa000001"},
		},
	}
	store := makeStore(t, target)

	m := Resolve(store, "Cotis Sociales")
	if m.Kind != MatchManual {
		t.Errorf("expected manual override, got %q", m.Kind)
	}
	if m.EntityID != target.ID {
		t.Errorf("expected entity %q, got %q", target.ID, m.EntityID)
	}
	if m.Confidence != 1.0 {
		t.Errorf("manual override confidence should be 1.0, got %v", m.Confidence)
	}
}

func TestResolveForceUnmatch(t *testing.T) {
	e := Entity{
		ID:             "ent_aaaaaa000001",
		CanonicalName:  "Bank Fee",
		NormalizedKeys: []string{"frais bancaires"},
		ManualOverrides: []Override{
			{Kind: OverrideForceUnmatch, Source: "frais bancaires"},
		},
	}
	store := makeStore(t, e)

	m := Resolve(store, "Frais bancaires")
	if m.Kind != MatchNone {
		t.Errorf("force_unmatch must yield MatchNone, got %q", m.Kind)
	}
}

func TestResolveIsDeterministic(t *testing.T) {
	store := makeStore(t,
		Entity{ID: "ent_a01", CanonicalName: "OVH", NormalizedKeys: []string{"ovh sas"}},
		Entity{ID: "ent_a02", CanonicalName: "OVH Cloud", NormalizedKeys: []string{"ovh cloud"}},
		Entity{ID: "ent_a03", CanonicalName: "EDF", NormalizedKeys: []string{"edf"}},
	)
	inputs := []string{"OVH SAS FACT 12345", "edf 03/2024", "OVH CLOUD"}
	first := make([]Match, len(inputs))
	for i, in := range inputs {
		first[i] = Resolve(store, in)
	}
	for run := 0; run < 100; run++ {
		for i, in := range inputs {
			got := Resolve(store, in)
			if got != first[i] {
				t.Fatalf("Resolve drifted on run %d for %q:\n  first:  %+v\n  now:    %+v", run, in, first[i], got)
			}
		}
	}
}

func TestCluster(t *testing.T) {
	raws := []string{
		"SE DOMICILIE FACTURE 48-99",
		"SE DOMICILIE FACTURE 49-12",
		"SE DOMICILIE",
		"OVH SAS",
		"OVH SAS FACT 12345",
		"UNRELATED ONE",
	}
	actions := Cluster(raws)
	// Expect 3 clusters: "se domicilie", "ovh sas", "unrelated one".
	if len(actions) != 3 {
		t.Fatalf("expected 3 clusters, got %d:\n%+v", len(actions), actions)
	}
	// Determinism: cluster again, expect identical output.
	again := Cluster(raws)
	if len(again) != len(actions) {
		t.Fatalf("Cluster non-deterministic count")
	}
	for i := range actions {
		if actions[i].EntityID != again[i].EntityID {
			t.Errorf("Cluster[%d].EntityID drifted: %q vs %q", i, actions[i].EntityID, again[i].EntityID)
		}
	}
}
