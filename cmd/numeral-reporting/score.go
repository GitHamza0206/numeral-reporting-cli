package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/numeral/numeral-reporting-cli/internal/entities"
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
		return fmt.Errorf("entities: subcommand required (list|show|reset)")
	}
	sub, rest := args[0], reorderFlags(args[1:])
	switch sub {
	case "list":
		return entitiesList(rest)
	case "show":
		return entitiesShow(rest)
	case "reset":
		return entitiesReset(rest)
	case "-h", "--help", "help":
		fmt.Print(entitiesUsage)
		return nil
	default:
		return fmt.Errorf("entities: unknown subcommand %q (want list|show|reset)", sub)
	}
}

const entitiesUsage = `numeral-reporting entities — manage the project entity table

Usage:
  numeral-reporting entities list  [--project DIR] [--kind KIND] [--json]
  numeral-reporting entities show  <id> [--project DIR]
  numeral-reporting entities reset [--project DIR] [--yes]

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

