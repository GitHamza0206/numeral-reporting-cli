// numeral-reporting is a CLI for scaffolding and managing Numeral
// reporting projects (the Next.js template that lives under
// reports/template, reports/v0, reports/v1, ...).
//
// Subcommands:
//
//	init <dir> [--template PATH]   clone the template to a new project dir
//	create <dir> --kind KIND       create a static app report project
//	doctor [--version vN]          run report integrity and agent-safety checks
//	render [--version vN]          render a static report to dist/
//	app [--addr ADDR]              serve the static report app locally
//	list [--json]                  list versions in the current project
//	new [--from N] [--name NOTE]   create a new vN from `from` (default: tip)
//	freeze <N>                     mark vN immutable
//	delete <N>                     drop vN (not v0, not frozen)
//	activate <N>                   set active_version to N
//	refresh                        rewrite reports/registry.ts from disk
//	export <N> <out.pdf> [--url URL]
//	                               print http://localhost:8080/vN to PDF
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/numeral/numeral-reporting-cli/internal/pdf"
	"github.com/numeral/numeral-reporting-cli/internal/reports"
)

const usage = `numeral-reporting — manage Numeral reporting projects

Usage:
  numeral-reporting init <dir> [--template PATH]
  numeral-reporting create <dir> --kind demo-saas|restaurant|cabinet-client [--mode static|next] [--template PATH]
  numeral-reporting doctor [--version vN] [--strict] [--json] [--project DIR]
  numeral-reporting render [--version vN] [--out DIR] [--project DIR]
  numeral-reporting app [--addr 127.0.0.1:8787] [--project DIR]
  numeral-reporting list [--json] [--project DIR]
  numeral-reporting new [--from N] [--name NOTE] [--project DIR]
  numeral-reporting freeze <N> [--project DIR]
  numeral-reporting delete <N> [--project DIR]
  numeral-reporting activate <N> [--project DIR]
  numeral-reporting refresh [--project DIR]
  numeral-reporting export <N> <out.pdf> [--url URL] [--project DIR]

Run inside a project directory, or pass --project DIR.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], reorderFlags(os.Args[2:])
	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "create":
		err = cmdCreate(args)
	case "doctor":
		err = cmdDoctor(args)
	case "render":
		err = cmdRender(args)
	case "app":
		err = cmdApp(args)
	case "list", "ls":
		err = cmdList(args)
	case "new":
		err = cmdNew(args)
	case "freeze":
		err = cmdFreeze(args)
	case "delete", "rm":
		err = cmdDelete(args)
	case "activate":
		err = cmdActivate(args)
	case "refresh":
		err = cmdRefresh(args)
	case "export":
		err = cmdExport(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	template := fs.String("template", defaultTemplate(), "path to the numeral-reporting template directory to copy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("init: missing target directory")
	}
	target, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("target %s already exists", target)
	}
	if _, err := os.Stat(*template); err != nil {
		return fmt.Errorf("template %s not found: %w", *template, err)
	}
	if err := copyTreeSkippingHeavy(*template, target); err != nil {
		return err
	}
	// Make sure reports/v0 + meta.json exist.
	p, _ := reports.Open(target)
	if _, err := p.ReadMeta(); err != nil {
		return err
	}
	fmt.Printf("initialized numeral-reporting project at %s\n", target)
	fmt.Println("next: cd", target, "&& npm install && npm run dev")
	return nil
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	version := fs.String("version", "v0", "version to inspect (for example v0 or 0)")
	strict := fs.Bool("strict", false, "treat warnings as blocking")
	asJSON := fs.Bool("json", false, "output machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	if isStaticProject(root) {
		return doctorStaticProject(root, *version, *strict, *asJSON)
	}
	script := filepath.Join(root, "scripts", "reporting-doctor.mjs")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("reporting doctor script not found at %s", script)
	}

	cmdArgs := []string{
		script,
		"--project", root,
		"--version", *version,
	}
	if *strict {
		cmdArgs = append(cmdArgs, "--strict")
	}
	if *asJSON {
		cmdArgs = append(cmdArgs, "--json")
	}

	cmd := exec.Command("node", cmdArgs...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

// defaultTemplate returns the bundled template path. Prefer
// $NUMERAL_REPORTING_TEMPLATE, otherwise look for a sibling
// numeral-reporting/ next to the CLI source tree (works in dev).
func defaultTemplate() string {
	if v := os.Getenv("NUMERAL_REPORTING_TEMPLATE"); v != "" {
		return v
	}
	exe, err := os.Executable()
	if err == nil {
		// Walk up looking for ../../numeral-reporting (dev layout under numeral-templates/).
		dir := filepath.Dir(exe)
		for i := 0; i < 6; i++ {
			candidate := filepath.Join(dir, "numeral-reporting")
			if _, err := os.Stat(filepath.Join(candidate, "reports", "template")); err == nil {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "./numeral-reporting"
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "output meta.json as JSON")
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if root, err := filepath.Abs(*project); err == nil && isStaticProject(root) {
		return listStaticProject(root, *asJSON)
	}
	p, err := reports.Open(*project)
	if err != nil {
		return err
	}
	m, err := p.ReadMeta()
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(m)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "tip\tv%d\nactive\tv%d\n\n", m.Tip, m.ActiveVersion)
	fmt.Fprintln(w, "VERSION\tPARENT\tFROZEN\tNOTE\tCREATED")
	for _, v := range m.Versions {
		parent := "-"
		if v.Parent != nil {
			parent = fmt.Sprintf("v%d", *v.Parent)
		}
		note := "-"
		if v.ChangeNote != nil {
			note = *v.ChangeNote
		}
		created := "-"
		if v.CreatedAt != nil {
			created = *v.CreatedAt
		}
		fmt.Fprintf(w, "v%d\t%s\t%v\t%s\t%s\n", v.N, parent, v.Frozen, note, created)
	}
	return w.Flush()
}

func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	from := fs.Int("from", -1, "source version (default: tip)")
	name := fs.String("name", "", "change note")
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := reports.Open(*project)
	if err != nil {
		return err
	}
	var fromPtr *int
	if *from >= 0 {
		fromPtr = from
	}
	if root, err := filepath.Abs(*project); err == nil && isStaticProject(root) {
		v, parent, err := newStaticVersion(root, fromPtr, *name)
		if err != nil {
			return err
		}
		fmt.Printf("created v%d from v%d\n", v, parent)
		return nil
	}
	v, parent, err := p.NewVersion(fromPtr, *name)
	if err != nil {
		return err
	}
	fmt.Printf("created v%d from v%d\n", v, parent)
	return nil
}

func cmdFreeze(args []string) error {
	fs := flag.NewFlagSet("freeze", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	n, err := parseVersionArg(fs.Arg(0))
	if err != nil {
		return err
	}
	if root, err := filepath.Abs(*project); err == nil && isStaticProject(root) {
		if err := freezeStaticVersion(root, n); err != nil {
			return err
		}
		fmt.Printf("froze v%d\n", n)
		return nil
	}
	p, err := reports.Open(*project)
	if err != nil {
		return err
	}
	if err := p.Freeze(n); err != nil {
		return err
	}
	fmt.Printf("froze v%d\n", n)
	return nil
}

func cmdDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	n, err := parseVersionArg(fs.Arg(0))
	if err != nil {
		return err
	}
	if root, err := filepath.Abs(*project); err == nil && isStaticProject(root) {
		if err := deleteStaticVersion(root, n); err != nil {
			return err
		}
		fmt.Printf("deleted v%d\n", n)
		return nil
	}
	p, err := reports.Open(*project)
	if err != nil {
		return err
	}
	if err := p.Delete(n); err != nil {
		return err
	}
	fmt.Printf("deleted v%d\n", n)
	return nil
}

func cmdActivate(args []string) error {
	fs := flag.NewFlagSet("activate", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	n, err := parseVersionArg(fs.Arg(0))
	if err != nil {
		return err
	}
	if root, err := filepath.Abs(*project); err == nil && isStaticProject(root) {
		if err := setActiveStaticVersion(root, n); err != nil {
			return err
		}
		fmt.Printf("active version is now v%d\n", n)
		return nil
	}
	p, err := reports.Open(*project)
	if err != nil {
		return err
	}
	if err := p.SetActive(n); err != nil {
		return err
	}
	fmt.Printf("active version is now v%d\n", n)
	return nil
}

func cmdRefresh(args []string) error {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if root, err := filepath.Abs(*project); err == nil && isStaticProject(root) {
		fmt.Println("static project has no registry.ts")
		return nil
	}
	p, err := reports.Open(*project)
	if err != nil {
		return err
	}
	if err := p.RefreshRegistry(); err != nil {
		return err
	}
	fmt.Println("rewrote reports/registry.ts")
	return nil
}

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	urlBase := fs.String("url", "http://localhost:8080", "base URL of the running Next.js app")
	project := fs.String("project", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("export: usage: export <N> <out.pdf>")
	}
	n, err := parseVersionArg(fs.Arg(0))
	if err != nil {
		return err
	}
	out := fs.Arg(1)
	// Make sure the version exists locally so we don't render 404s.
	if root, err := filepath.Abs(*project); err == nil && isStaticProject(root) {
		if err := exportStaticVersion(root, n, out); err != nil {
			return err
		}
		abs, _ := filepath.Abs(out)
		fmt.Printf("wrote %s\n", abs)
		return nil
	}
	p, err := reports.Open(*project)
	if err != nil {
		return err
	}
	m, err := p.ReadMeta()
	if err != nil {
		return err
	}
	found := false
	for _, v := range m.Versions {
		if v.N == n {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("v%d not in meta.json", n)
	}
	target := fmt.Sprintf("%s/v%d", *urlBase, n)
	if err := pdf.Render(target, out); err != nil {
		return err
	}
	abs, _ := filepath.Abs(out)
	fmt.Printf("wrote %s\n", abs)
	return nil
}

// reorderFlags moves --flag (and --flag value) tokens ahead of positional
// arguments so callers can write `init <dir> --template X` interchangeably
// with `init --template X <dir>`. Stops at `--`.
func reorderFlags(args []string) []string {
	var flags, positional []string
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			flags = append(flags, a)
			// If it's `--key=value` or a bool, no value follows.
			if !containsByte(a, '=') && i+1 < len(args) && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
				// Heuristic: assume next token is the value unless it's
				// another flag. Bool flags will swallow a positional, so we
				// reserve the `-flag=true` form for those — the schema in
				// this CLI never mixes a bool flag immediately followed by
				// a non-flag positional.
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			positional = append(positional, a)
		}
		i++
	}
	return append(flags, positional...)
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

func parseVersionArg(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("missing version argument (e.g. 1 or v1)")
	}
	if s[0] == 'v' || s[0] == 'V' {
		s = s[1:]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q", s)
	}
	return n, nil
}

// copyTreeSkippingHeavy copies src -> dst but skips generated, local, and
// sensitive-ish artifacts so `init` produces a clean checkout.
func copyTreeSkippingHeavy(src, dst string) error {
	skip := map[string]bool{
		".git":                 true,
		".next":                true,
		".DS_Store":            true,
		"node_modules":         true,
		"tsconfig.tsbuildinfo": true,
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		base := filepath.Base(path)
		if skip[base] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipDataFile(rel, info) {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyOneFile(path, target)
	})
}

func shouldSkipDataFile(rel string, info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	if filepath.Dir(rel) != "data" {
		return false
	}
	switch filepath.Base(rel) {
	case ".gitkeep", "README.md":
		return false
	default:
		return true
	}
}

func copyOneFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
