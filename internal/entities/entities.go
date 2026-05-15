package entities

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// SchemaVersion is the on-disk version of entities.json. Bump on breaking changes.
const SchemaVersion = 1

// Kind classifies the role of an entity in the accounting picture.
type Kind string

const (
	KindFournisseur Kind = "fournisseur"
	KindClient      Kind = "client"
	KindSalarie     Kind = "salarie"
	KindBanque      Kind = "banque"
	KindFiscal      Kind = "fiscal"
	KindAutre       Kind = "autre"
)

// Entity is a stable identity that groups one or more libellés.
type Entity struct {
	ID              string     `json:"id"`
	CanonicalName   string     `json:"canonical_name"`
	Kind            Kind       `json:"kind"`
	NormalizedKeys  []string   `json:"normalized_keys"`
	IBANs           []string   `json:"ibans,omitempty"`
	SIRET           string     `json:"siret,omitempty"`
	Aliases         []string   `json:"aliases,omitempty"`
	TypicalAccount  string     `json:"typical_account,omitempty"`
	TypicalCategory string     `json:"typical_category,omitempty"`
	FirstSeen       string     `json:"first_seen,omitempty"`
	LastSeen        string     `json:"last_seen,omitempty"`
	CreatedByRun    string     `json:"created_by_run,omitempty"`
	ManualOverrides []Override `json:"manual_overrides,omitempty"`
}

// OverrideKind enumerates the manual operations that can pin a libellé.
type OverrideKind string

const (
	OverrideMergeInto    OverrideKind = "merge_into"
	OverrideSplitFrom    OverrideKind = "split_from"
	OverrideForceMatch   OverrideKind = "force_match"
	OverrideForceUnmatch OverrideKind = "force_unmatch"
)

// Override records a human (or agent) decision that should beat automatic
// resolution on the next run.
type Override struct {
	Kind   OverrideKind `json:"kind"`
	Source string       `json:"source"`
	Target string       `json:"target,omitempty"`
	Note   string       `json:"note,omitempty"`
	Date   string       `json:"date,omitempty"`
}

// Store is the persistent collection of entities for a project.
type Store struct {
	Version  int      `json:"schema_version"`
	Entities []Entity `json:"entities"`
}

// NewStore returns an empty, schema-versioned store.
func NewStore() *Store {
	return &Store{Version: SchemaVersion, Entities: []Entity{}}
}

// Load decodes a Store from JSON. An empty reader yields an empty store.
func Load(r io.Reader) (*Store, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	s := &Store{}
	if err := dec.Decode(s); err != nil {
		if err == io.EOF {
			return NewStore(), nil
		}
		return nil, fmt.Errorf("decode entities.json: %w", err)
	}
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	s.Sort()
	return s, nil
}

// Save writes a Store as canonical JSON: sorted entities, sorted slices,
// stable indentation. The output is byte-identical for byte-identical input.
func (s *Store) Save(w io.Writer) error {
	s.Sort()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(s)
}

// Sort canonicalizes the store: entities by ID, all string slices ascending,
// deduped. Safe to call multiple times.
func (s *Store) Sort() {
	for i := range s.Entities {
		s.Entities[i].normalize()
	}
	sort.Slice(s.Entities, func(i, j int) bool {
		return s.Entities[i].ID < s.Entities[j].ID
	})
}

func (e *Entity) normalize() {
	e.NormalizedKeys = sortDedup(e.NormalizedKeys)
	e.IBANs = sortDedup(e.IBANs)
	e.Aliases = sortDedup(e.Aliases)
	// Manual overrides: stable order by (Kind, Source, Target, Date).
	sort.Slice(e.ManualOverrides, func(i, j int) bool {
		a, b := e.ManualOverrides[i], e.ManualOverrides[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Date < b.Date
	})
}

func sortDedup(in []string) []string {
	if len(in) == 0 {
		return in
	}
	cp := make([]string, len(in))
	copy(cp, in)
	sort.Strings(cp)
	out := cp[:0]
	var last string
	for i, v := range cp {
		if i == 0 || v != last {
			out = append(out, v)
			last = v
		}
	}
	return out
}

// FindByID returns the entity with the given ID, or nil.
func (s *Store) FindByID(id string) *Entity {
	for i := range s.Entities {
		if s.Entities[i].ID == id {
			return &s.Entities[i]
		}
	}
	return nil
}

// FindByIBAN returns the entity whose IBAN list contains the (uppercase) iban.
func (s *Store) FindByIBAN(iban string) *Entity {
	iban = strings.ToUpper(strings.ReplaceAll(iban, " ", ""))
	for i := range s.Entities {
		for _, candidate := range s.Entities[i].IBANs {
			if strings.ToUpper(candidate) == iban {
				return &s.Entities[i]
			}
		}
	}
	return nil
}

// FindBySIRET returns the entity matching the given SIRET, or nil.
func (s *Store) FindBySIRET(siret string) *Entity {
	if siret == "" {
		return nil
	}
	for i := range s.Entities {
		if s.Entities[i].SIRET == siret {
			return &s.Entities[i]
		}
	}
	return nil
}

// lookupOverride returns the entity that an active manual override pins the
// given normalized libellé to, or nil. ForceUnmatch overrides also resolve
// here and return a sentinel ID "" with ok=true.
func (s *Store) lookupOverride(norm string) (entityID string, ok bool) {
	// Iterate entities in sorted order (s.Sort() is required to be called
	// before this function — Load and modifier helpers always do).
	for i := range s.Entities {
		for _, ov := range s.Entities[i].ManualOverrides {
			if ov.Source != norm {
				continue
			}
			switch ov.Kind {
			case OverrideForceMatch:
				return s.Entities[i].ID, true
			case OverrideForceUnmatch:
				return "", true
			}
		}
	}
	return "", false
}

// NewEntityID derives a stable 12-char hex ID from the canonical name.
// Identical canonical names always produce identical IDs across runs/machines.
func NewEntityID(canonicalName string) string {
	if canonicalName == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(canonicalName))
	return "ent_" + hex.EncodeToString(sum[:6]) // 12 hex chars
}
