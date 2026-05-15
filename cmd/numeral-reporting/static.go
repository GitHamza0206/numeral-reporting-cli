package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/numeral/numeral-reporting-cli/internal/pdf"
)

//go:embed report.css
var embeddedReportCSS string

//go:embed numeral_shell.css
var embeddedShellCSS string

type staticMeta struct {
	Tip           int             `json:"tip"`
	ActiveVersion int             `json:"active_version"`
	Etag          string          `json:"etag"`
	Versions      []staticVersion `json:"versions"`
}

type staticVersion struct {
	N          int     `json:"n"`
	Parent     *int    `json:"parent"`
	Frozen     bool    `json:"frozen"`
	ChangeNote *string `json:"change_note"`
	CreatedAt  *string `json:"created_at"`
}

type staticReport struct {
	Version          string                 `json:"version"`
	Kind             string                 `json:"kind"`
	Mode             string                 `json:"mode"`
	RequiresEvidence bool                   `json:"requiresEvidence"`
	Client           string                 `json:"client"`
	Title            string                 `json:"title"`
	Period           string                 `json:"period"`
	Year             int                    `json:"year"`
	PriorYear        int                    `json:"priorYear"`
	GeneratedAt      string                 `json:"generatedAt"`
	Source           string                 `json:"source"`
	Subtitle         string                 `json:"subtitle"`
	Footnote         string                 `json:"footnote"`
	Score            staticScore            `json:"score"`
	Processing       staticProcessing       `json:"processing"`
	KPIs             []staticKPI            `json:"kpis"`
	PNL              staticPNL              `json:"pnl"`
	Alerts           staticAlerts           `json:"alerts"`
	Sig              map[string]float64     `json:"sig"`
	Monthly          staticMonthly          `json:"monthly"`
	Structure        staticStructure        `json:"structure"`
	Analyse          staticAnalysis         `json:"analyse"`
	Extra            map[string]interface{} `json:"extra,omitempty"`
}

type staticScore struct {
	Global int    `json:"global"`
	Level  string `json:"level"`
	Label  string `json:"label"`
}

type staticProcessing struct {
	Traitement       int    `json:"traitement"`
	NonTraite        int    `json:"nonTraite"`
	Ajustement       int    `json:"ajustement"`
	MontantTraite    string `json:"montantTraite"`
	MontantNonTraite string `json:"montantNonTraite"`
	MontantAjuste    string `json:"montantAjuste"`
}

type staticKPI struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
	Accent  string `json:"accent,omitempty"`
}

type staticLine struct {
	Label string  `json:"label"`
	N     float64 `json:"n"`
	N1    float64 `json:"n1"`
}

type staticPNL struct {
	Produits []staticLine      `json:"produits"`
	Charges  []staticLine      `json:"charges"`
	Totals   staticPNLTotalSet `json:"totals"`
}

type staticPNLTotalSet struct {
	ProduitsN              float64 `json:"produitsN"`
	ProduitsN1             float64 `json:"produitsN1"`
	ChargesN               float64 `json:"chargesN"`
	ChargesN1              float64 `json:"chargesN1"`
	ResultatExploitationN  float64 `json:"resultatExploitationN"`
	ResultatExploitationN1 float64 `json:"resultatExploitationN1"`
	ResultatNetN           float64 `json:"resultatNetN"`
	ResultatNetN1          float64 `json:"resultatNetN1"`
}

type staticAlerts struct {
	Blocking      []staticAlert `json:"blocking"`
	BlockingTotal float64       `json:"blockingTotal"`
	Points        []staticAlert `json:"points"`
}

type staticAlert struct {
	Label    string  `json:"label"`
	Severity string  `json:"severity"`
	Account  string  `json:"account,omitempty"`
	Amount   float64 `json:"amount,omitempty"`
	Comment  string  `json:"comment"`
}

type staticMonthly struct {
	Headers      []string            `json:"headers"`
	Produits     staticMonthlyRow    `json:"produits"`
	Charges      []staticMonthlyLine `json:"charges"`
	ChargesTotal staticMonthlyRow    `json:"chargesTotal"`
	Result       staticMonthlyRow    `json:"result"`
}

type staticMonthlyRow struct {
	Cells []float64 `json:"cells"`
	Total float64   `json:"total"`
}

type staticMonthlyLine struct {
	Label string    `json:"label"`
	Cells []float64 `json:"cells"`
	Total float64   `json:"total"`
}

type staticStructure struct {
	Balance  staticBalance  `json:"balance"`
	Ratios   []factoryRatio `json:"ratios"`
	DSOAlert string         `json:"dsoAlert"`
	Charts   staticCharts   `json:"charts"`
}

type staticBalance struct {
	Actif       []factoryBalanceRow `json:"actif"`
	Passif      []factoryBalanceRow `json:"passif"`
	TotalActif  float64             `json:"totalActif"`
	TotalPassif float64             `json:"totalPassif"`
}

type staticCharts struct {
	Donut staticDonut `json:"donut"`
}

type staticDonut struct {
	Categories []staticDonutCategory `json:"categories"`
}

type staticDonutCategory struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type staticAnalysis struct {
	Narratives  map[string]string `json:"narratives"`
	Penalties   []staticPenalty   `json:"penalties,omitempty"`
	Fiscalite   []staticFiscal    `json:"fiscalite,omitempty"`
	Synthese    []staticSummary   `json:"synthese"`
	FooterExtra []string          `json:"footerExtra"`
}

type staticPenalty struct {
	Label  string  `json:"label,omitempty"`
	Weight float64 `json:"weight,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

type staticFiscal struct {
	Label   string `json:"label,omitempty"`
	Status  string `json:"status,omitempty"`
	Comment string `json:"comment,omitempty"`
}

type staticSummary struct {
	Label   string `json:"label"`
	Value   string `json:"value,omitempty"`
	Comment string `json:"comment,omitempty"`
	Accent  string `json:"accent,omitempty"`
}

type staticAmount struct {
	Path  string
	Value float64
}

type staticReportView struct {
	Report         staticReport
	Meta           *staticMeta
	CurrentVersion int
	ReportCSS      template.CSS
	ShellCSS       template.CSS
	Sommaire       []staticSommaireItem
	BalanceRows    []staticBalancePair
	SigRows        []staticSigRow
	ScoringRows    []staticScoringRow
}

type staticSommaireItem struct {
	Num   string
	Label string
	Desc  string
}

type staticBalancePair struct {
	Actif  *factoryBalanceRow
	Passif *factoryBalanceRow
}

type staticSigRow struct {
	Label string
	Class string
	N     float64
	N1    float64
}

type staticScoringRow struct {
	Label string
	Value string
	Class string
}

func writeStaticFactoryProject(root string, profile factoryProfile) error {
	if err := os.MkdirAll(staticVersionDir(root, 0), 0o755); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	meta := staticMeta{
		Tip:           0,
		ActiveVersion: 0,
		Versions: []staticVersion{{
			N:         0,
			Frozen:    false,
			CreatedAt: &now,
		}},
	}
	if err := writeStaticMeta(root, &meta); err != nil {
		return err
	}

	report := staticReportFromProfile(profile)
	report.Version = "v0"
	if err := writeJSONFile(staticReportPath(root, 0), report); err != nil {
		return err
	}
	evidence := evidenceFile{Version: "v0", Items: factoryEvidenceRows(profile)}
	if err := writeJSONFile(staticEvidencePath(root, 0), evidence); err != nil {
		return err
	}
	if err := os.WriteFile(staticNotesPath(root, 0), []byte(renderStaticNotes(profile)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(renderStaticReadme(profile)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(renderStaticAgents(profile)), 0o644); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(root, "exports"), 0o755)
}

func staticReportFromProfile(p factoryProfile) staticReport {
	productTotalN, productTotalN1 := sumLines(p.Products)
	chargeTotalN, chargeTotalN1 := sumLines(p.Charges)
	resultN := productTotalN - chargeTotalN
	resultN1 := productTotalN1 - chargeTotalN1
	monthlyChargeTotals := sumMonthlyCharges(p.MonthlyCharges)
	monthlyResult := diffCells(p.MonthlyProducts, monthlyChargeTotals)

	monthlyCharges := make([]staticMonthlyLine, 0, len(p.MonthlyCharges))
	for _, c := range p.MonthlyCharges {
		monthlyCharges = append(monthlyCharges, staticMonthlyLine{
			Label: c.Label,
			Cells: c.Cells,
			Total: sum(c.Cells),
		})
	}

	donut := make([]staticDonutCategory, 0, len(p.Charges))
	for _, line := range p.Charges {
		donut = append(donut, staticDonutCategory{Label: line.Label, Value: line.N})
	}

	return staticReport{
		Kind:             p.Kind,
		Mode:             "demo",
		RequiresEvidence: false,
		Client:           p.Client,
		Title:            p.Title,
		Period:           p.Period,
		Year:             2025,
		PriorYear:        2024,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Source:           p.Source,
		Subtitle:         p.Subtitle,
		Footnote:         p.Footnote,
		Score: staticScore{
			Global: p.Score,
			Level:  p.ScoreLevel,
			Label:  p.ScoreLabel,
		},
		Processing: staticProcessing{
			Traitement:       p.Traitement,
			NonTraite:        p.NonTraite,
			Ajustement:       p.Ajustement,
			MontantTraite:    p.MontantTraite,
			MontantNonTraite: p.MontantNonTraite,
			MontantAjuste:    p.MontantAjuste,
		},
		KPIs: []staticKPI{
			{Label: "Revenue", Value: formatMoney(productTotalN), Comment: "Current period"},
			{Label: "Net result", Value: formatMoney(resultN), Comment: "After operating costs", Accent: "positive"},
			{Label: "Score", Value: fmt.Sprintf("%d/100", p.Score), Comment: p.ScoreLabel},
			{Label: "Review point", Value: p.ReviewPoint, Comment: p.DSOAlert, Accent: "warning"},
		},
		PNL: staticPNL{
			Produits: staticLines(p.Products),
			Charges:  staticLines(p.Charges),
			Totals: staticPNLTotalSet{
				ProduitsN:              productTotalN,
				ProduitsN1:             productTotalN1,
				ChargesN:               chargeTotalN,
				ChargesN1:              chargeTotalN1,
				ResultatExploitationN:  resultN,
				ResultatExploitationN1: resultN1,
				ResultatNetN:           resultN,
				ResultatNetN1:          resultN1,
			},
		},
		Alerts: staticAlerts{
			Blocking:      staticAlertsFromFactory(p.Blocking),
			BlockingTotal: p.BlockingTotal,
			Points:        staticAlertsFromFactory(p.Points),
		},
		Sig: p.Sig,
		Monthly: staticMonthly{
			Headers:      []string{"Q1", "Q2", "Q3", "Q4"},
			Produits:     staticMonthlyRow{Cells: p.MonthlyProducts, Total: sum(p.MonthlyProducts)},
			Charges:      monthlyCharges,
			ChargesTotal: staticMonthlyRow{Cells: monthlyChargeTotals, Total: sum(monthlyChargeTotals)},
			Result:       staticMonthlyRow{Cells: monthlyResult, Total: sum(monthlyResult)},
		},
		Structure: staticStructure{
			Balance: staticBalance{
				Actif:       p.Actif,
				Passif:      p.Passif,
				TotalActif:  sumBalance(p.Actif),
				TotalPassif: sumBalance(p.Passif),
			},
			Ratios:   p.Ratios,
			DSOAlert: p.DSOAlert,
			Charts:   staticCharts{Donut: staticDonut{Categories: donut}},
		},
		Analyse: staticAnalysis{
			Narratives: map[string]string{
				"score":      p.ScoreNarrative,
				"pnl":        p.PNLNarrative,
				"sig":        p.SIGNarrative,
				"monthly":    p.MonthlyNarrative,
				"structure":  p.StructureNarrative,
				"bridge":     p.BridgeNarrative,
				"comparison": p.ComparisonNarrative,
				"fiscalite":  p.FiscalNarrative,
				"synthese":   p.SummaryNarrative,
			},
			Penalties: []staticPenalty{
				{
					Label:  p.ReviewPoint,
					Weight: float64(max(4, 100-p.Score)),
					Reason: "Main review point for this factory kind.",
				},
			},
			Fiscalite: []staticFiscal{
				{
					Label:   "Income tax",
					Status:  "Demo",
					Comment: "Illustrative only; source ledgers are required for a real tax position.",
				},
				{
					Label:   "VAT",
					Status:  "Demo",
					Comment: "VAT controls require declarations and account reconciliation in a real file.",
				},
			},
			Synthese: []staticSummary{
				{Label: "Revenue", Value: formatMoney(productTotalN)},
				{Label: "Net result", Value: formatMoney(resultN), Accent: "positive"},
				{Label: "Review point", Comment: p.ReviewPoint},
				{Label: "Usage", Comment: "Factory demo only"},
			},
			FooterExtra: []string{
				"Factory-generated Numeral report - fictional amounts, no accounting or tax value.",
			},
		},
	}
}

func staticLines(lines []factoryLine) []staticLine {
	out := make([]staticLine, 0, len(lines))
	for _, line := range lines {
		out = append(out, staticLine{Label: line.Label, N: line.N, N1: line.N1})
	}
	return out
}

func staticAlertsFromFactory(alerts []factoryAlert) []staticAlert {
	out := make([]staticAlert, 0, len(alerts))
	for _, alert := range alerts {
		out = append(out, staticAlert{
			Label:    alert.Label,
			Severity: alert.Level,
			Account:  alert.Account,
			Amount:   alert.Amount,
			Comment:  alert.Comment,
		})
	}
	return out
}

func renderStaticNotes(p factoryProfile) string {
	return fmt.Sprintf(`# %s Static Report Notes

This v0 report was generated with:

    numeral-reporting create <dir> --kind %s

The amounts are fictional and should be replaced before real client use.

Agent checklist:

- Edit only versions/v0/report.json, evidence.json, and notes.md unless the renderer itself needs a product change.
- For client reports, switch report.json to client mode and set requiresEvidence to true.
- Add evidence.json entries for every non-null financial amount.
- Run: numeral-reporting doctor --project . --version v0 --strict.
- Run: numeral-reporting render --project . --version v0.
`, p.Kind, p.Kind)
}

func renderStaticReadme(p factoryProfile) string {
	return fmt.Sprintf(`# %s

Static Numeral report project.

Kind: %s

## Run

    numeral-reporting app --project .

Then open:

    http://127.0.0.1:8787

## Edit

- Report data: versions/v0/report.json
- Evidence: versions/v0/evidence.json
- Notes: versions/v0/notes.md
- Version metadata: meta.json

All factory amounts are fictional demo data.

## Guardrails

    numeral-reporting doctor --project . --version v0 --strict
    numeral-reporting render --project . --version v0
`, p.Client, p.Kind)
}

func renderStaticAgents(p factoryProfile) string {
	return fmt.Sprintf(`# %s agent guide

This is a static Numeral report project. Keep edits focused on version data.

Use this workflow:

1. Edit `+"`versions/vN/report.json`"+`, `+"`versions/vN/evidence.json`"+`, or `+"`versions/vN/notes.md`"+`.
2. Create versions with `+"`numeral-reporting new --project . --from vN`"+`.
3. Validate with `+"`numeral-reporting doctor --project . --version vN --strict`"+`.
4. Render with `+"`numeral-reporting render --project . --version vN`"+`.

Do not invent client financial amounts. If a value has no source, leave it out
or mark it for review.
`, p.Client)
}

func isStaticProject(root string) bool {
	if root == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(root, "meta.json")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(root, "versions"))
	return err == nil && info.IsDir()
}

func listStaticProject(root string, asJSON bool) error {
	m, err := readStaticMeta(root)
	if err != nil {
		return err
	}
	if asJSON {
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

func newStaticVersion(root string, from *int, name string) (int, int, error) {
	m, err := readStaticMeta(root)
	if err != nil {
		return 0, 0, err
	}
	next := 0
	for _, v := range m.Versions {
		if v.N >= next {
			next = v.N + 1
		}
	}
	src := m.Tip
	if from != nil {
		src = *from
	}
	if !staticMetaHasVersion(m, src) {
		return 0, 0, fmt.Errorf("unknown source version v%d", src)
	}
	if err := copyStaticTree(staticVersionDir(root, src), staticVersionDir(root, next)); err != nil {
		return 0, 0, err
	}
	report, err := readStaticReport(root, next)
	if err == nil {
		report.Version = fmt.Sprintf("v%d", next)
		_ = writeJSONFile(staticReportPath(root, next), report)
	}
	evidence, err := readStaticEvidence(root, next)
	if err == nil {
		evidence.Version = fmt.Sprintf("v%d", next)
		_ = writeJSONFile(staticEvidencePath(root, next), evidence)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	parentCopy := src
	var note *string
	if s := strings.TrimSpace(name); s != "" {
		note = &s
	}
	m.Versions = append(m.Versions, staticVersion{
		N:          next,
		Parent:     &parentCopy,
		Frozen:     false,
		ChangeNote: note,
		CreatedAt:  &now,
	})
	m.Tip = next
	m.ActiveVersion = next
	return next, src, writeStaticMeta(root, m)
}

func freezeStaticVersion(root string, n int) error {
	m, err := readStaticMeta(root)
	if err != nil {
		return err
	}
	for i := range m.Versions {
		if m.Versions[i].N == n {
			m.Versions[i].Frozen = true
			return writeStaticMeta(root, m)
		}
	}
	return fmt.Errorf("v%d not found", n)
}

func deleteStaticVersion(root string, n int) error {
	if n == 0 {
		return errors.New("v0 is the immutable baseline")
	}
	m, err := readStaticMeta(root)
	if err != nil {
		return err
	}
	idx := -1
	for i, v := range m.Versions {
		if v.N == n {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("v%d not found", n)
	}
	if m.Versions[idx].Frozen {
		return fmt.Errorf("v%d is frozen", n)
	}
	if err := os.RemoveAll(staticVersionDir(root, n)); err != nil {
		return err
	}
	m.Versions = append(m.Versions[:idx], m.Versions[idx+1:]...)
	if m.Tip == n {
		m.Tip = maxStaticVersion(m)
	}
	if m.ActiveVersion == n {
		m.ActiveVersion = m.Tip
	}
	return writeStaticMeta(root, m)
}

func setActiveStaticVersion(root string, n int) error {
	m, err := readStaticMeta(root)
	if err != nil {
		return err
	}
	if !staticMetaHasVersion(m, n) {
		return fmt.Errorf("v%d not found", n)
	}
	m.ActiveVersion = n
	return writeStaticMeta(root, m)
}

func cmdRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	version := fs.String("version", "", "version to render (default: active)")
	outDir := fs.String("out", "", "output directory (default: project dist)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	if !isStaticProject(root) {
		return fmt.Errorf("render only supports static projects")
	}
	n, err := staticVersionFromFlag(root, *version)
	if err != nil {
		return err
	}
	out := *outDir
	if strings.TrimSpace(out) == "" {
		out = filepath.Join(root, "dist", fmt.Sprintf("v%d", n))
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	htmlPath := filepath.Join(out, "index.html")
	if err := renderStaticVersion(root, n, htmlPath); err != nil {
		return err
	}
	abs, _ := filepath.Abs(htmlPath)
	fmt.Printf("rendered %s\n", abs)
	return nil
}

func exportStaticVersion(root string, n int, out string) error {
	htmlPath := filepath.Join(root, "dist", fmt.Sprintf("v%d", n), "index.html")
	if err := renderStaticVersion(root, n, htmlPath); err != nil {
		return err
	}
	absHTML, err := filepath.Abs(htmlPath)
	if err != nil {
		return err
	}
	fileURL := url.URL{Scheme: "file", Path: absHTML}
	return pdf.Render(fileURL.String(), out)
}

func cmdApp(args []string) error {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	project := fs.String("project", ".", "project directory")
	addr := fs.String("addr", "127.0.0.1:8787", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := filepath.Abs(*project)
	if err != nil {
		return err
	}
	if !isStaticProject(root) {
		return fmt.Errorf("app only supports static projects")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		n, err := staticVersionFromFlag(root, "")
		if err != nil {
			httpError(w, err)
			return
		}
		html, err := renderStaticReportPage(root, n)
		if err != nil {
			httpError(w, err)
			return
		}
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		m, err := readStaticMeta(root)
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"project": root, "meta": m})
	})
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		n, err := staticVersionFromQuery(root, r)
		if err != nil {
			httpError(w, err)
			return
		}
		report, err := readStaticReport(root, n)
		if err != nil {
			httpError(w, err)
			return
		}
		evidence, _ := readStaticEvidence(root, n)
		notes, _ := os.ReadFile(staticNotesPath(root, n))
		writeJSON(w, map[string]interface{}{
			"version":  fmt.Sprintf("v%d", n),
			"report":   report,
			"evidence": evidence,
			"notes":    string(notes),
		})
	})
	mux.HandleFunc("/api/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var from *int
		if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
			n, err := parseVersionArg(raw)
			if err != nil {
				httpError(w, err)
				return
			}
			from = &n
		}
		v, parent, err := newStaticVersion(root, from, r.URL.Query().Get("name"))
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"created": fmt.Sprintf("v%d", v), "parent": fmt.Sprintf("v%d", parent)})
	})
	mux.HandleFunc("/api/freeze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		n, err := staticVersionFromQuery(root, r)
		if err != nil {
			httpError(w, err)
			return
		}
		if err := freezeStaticVersion(root, n); err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"frozen": fmt.Sprintf("v%d", n)})
	})
	mux.HandleFunc("/api/activate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		n, err := staticVersionFromQuery(root, r)
		if err != nil {
			httpError(w, err)
			return
		}
		if err := setActiveStaticVersion(root, n); err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"active": fmt.Sprintf("v%d", n)})
	})
	mux.HandleFunc("/report/", func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.URL.Path, "/report/")
		n, err := parseVersionArg(raw)
		if err != nil {
			httpError(w, err)
			return
		}
		html, err := renderStaticReportPage(root, n)
		if err != nil {
			httpError(w, err)
			return
		}
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})

	fmt.Printf("serving %s at http://%s\n", root, *addr)
	return http.ListenAndServe(*addr, mux)
}

func doctorStaticProject(root string, version string, strict bool, asJSON bool, scoreThreshold int) error {
	n, err := staticVersionFromFlag(root, version)
	if err != nil {
		return err
	}
	report, err := readStaticReport(root, n)
	if err != nil {
		return err
	}
	evidence, err := readStaticEvidence(root, n)
	if err != nil && (report.RequiresEvidence || report.Mode == "client") {
		return err
	}

	errorsFound, warnings := staticDoctorFindings(report, evidence)

	// Reliability score gate: when --score-threshold is set, a global below
	// it counts as a warning (or an error in strict mode).
	if scoreThreshold > 0 {
		if report.Score.Global == 0 {
			warnings = append(warnings,
				"score not computed — run `numeral-reporting score --write` before doctor with --score-threshold")
		} else if report.Score.Global < scoreThreshold {
			msg := fmt.Sprintf("reliability score %d%% < threshold %d%%",
				report.Score.Global, scoreThreshold)
			if strict {
				errorsFound = append(errorsFound, msg)
			} else {
				warnings = append(warnings, msg)
			}
		}
	}

	ok := len(errorsFound) == 0 && (!strict || len(warnings) == 0)
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{
			"ok":       ok,
			"errors":   errorsFound,
			"warnings": warnings,
		})
	}
	for _, item := range errorsFound {
		fmt.Fprintln(os.Stderr, "error:", item)
	}
	for _, item := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", item)
	}
	if !ok {
		return fmt.Errorf("doctor found %d errors and %d warnings", len(errorsFound), len(warnings))
	}
	fmt.Printf("doctor ok: v%d\n", n)
	return nil
}

func staticDoctorFindings(report staticReport, evidence evidenceFile) ([]string, []string) {
	var errorsFound []string
	var warnings []string
	if strings.TrimSpace(report.Client) == "" {
		errorsFound = append(errorsFound, "report.client is empty")
	}
	if strings.TrimSpace(report.Title) == "" {
		errorsFound = append(errorsFound, "report.title is empty")
	}
	if report.Mode != "demo" && report.Mode != "client" {
		warnings = append(warnings, "report.mode should be demo or client")
	}

	productN, productN1 := sumStaticLines(report.PNL.Produits)
	chargeN, chargeN1 := sumStaticLines(report.PNL.Charges)
	checkAmount(&errorsFound, "pnl.totals.produitsN", productN, report.PNL.Totals.ProduitsN)
	checkAmount(&errorsFound, "pnl.totals.produitsN1", productN1, report.PNL.Totals.ProduitsN1)
	checkAmount(&errorsFound, "pnl.totals.chargesN", chargeN, report.PNL.Totals.ChargesN)
	checkAmount(&errorsFound, "pnl.totals.chargesN1", chargeN1, report.PNL.Totals.ChargesN1)
	checkAmount(&errorsFound, "pnl.totals.resultatExploitationN", productN-chargeN, report.PNL.Totals.ResultatExploitationN)
	checkAmount(&errorsFound, "pnl.totals.resultatExploitationN1", productN1-chargeN1, report.PNL.Totals.ResultatExploitationN1)
	checkAmount(&errorsFound, "pnl.totals.resultatNetN", report.PNL.Totals.ResultatExploitationN, report.PNL.Totals.ResultatNetN)
	checkAmount(&errorsFound, "pnl.totals.resultatNetN1", report.PNL.Totals.ResultatExploitationN1, report.PNL.Totals.ResultatNetN1)

	monthlyChargeTotals := sumStaticMonthlyCharges(report.Monthly.Charges)
	monthlyResult := diffCells(report.Monthly.Produits.Cells, monthlyChargeTotals)
	checkAmount(&errorsFound, "monthly.produits.total", sum(report.Monthly.Produits.Cells), report.Monthly.Produits.Total)
	checkAmount(&errorsFound, "monthly.chargesTotal.total", sum(monthlyChargeTotals), report.Monthly.ChargesTotal.Total)
	checkAmount(&errorsFound, "monthly.result.total", sum(monthlyResult), report.Monthly.Result.Total)
	checkCells(&errorsFound, "monthly.chargesTotal.cells", monthlyChargeTotals, report.Monthly.ChargesTotal.Cells)
	checkCells(&errorsFound, "monthly.result.cells", monthlyResult, report.Monthly.Result.Cells)

	checkAmount(&errorsFound, "structure.balance.totalActif", sumBalance(report.Structure.Balance.Actif), report.Structure.Balance.TotalActif)
	checkAmount(&errorsFound, "structure.balance.totalPassif", sumBalance(report.Structure.Balance.Passif), report.Structure.Balance.TotalPassif)
	checkAmount(&errorsFound, "structure.balance equality", report.Structure.Balance.TotalActif, report.Structure.Balance.TotalPassif)

	var blockingTotal float64
	for _, alert := range report.Alerts.Blocking {
		blockingTotal += alert.Amount
	}
	checkAmount(&errorsFound, "alerts.blockingTotal", blockingTotal, report.Alerts.BlockingTotal)

	for _, text := range visibleStaticText(report) {
		for _, banned := range []string{"script", "repo", "model.ts", "json", "skill", "doctor"} {
			if containsInternalTerm(text, banned) {
				warnings = append(warnings, fmt.Sprintf("visible text mentions internal term %q", banned))
				break
			}
		}
	}

	if report.RequiresEvidence || report.Mode == "client" {
		byPath := map[string]evidenceRow{}
		for _, item := range evidence.Items {
			byPath[item.Path] = item
		}
		for _, amount := range staticFinancialAmounts(report) {
			item, ok := byPath[amount.Path]
			if !ok {
				errorsFound = append(errorsFound, fmt.Sprintf("missing evidence for %s", amount.Path))
				continue
			}
			checkAmount(&errorsFound, "evidence "+amount.Path, amount.Value, item.Value)
			if strings.TrimSpace(item.Source) == "" {
				errorsFound = append(errorsFound, fmt.Sprintf("missing evidence source for %s", amount.Path))
			}
		}
	}
	return dedupeStrings(errorsFound), dedupeStrings(warnings)
}

func renderStaticVersion(root string, n int, outPath string) error {
	html, err := renderStaticReportPage(root, n)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, html, 0o644)
}

func renderStaticReportPage(root string, n int) ([]byte, error) {
	report, err := readStaticReport(root, n)
	if err != nil {
		return nil, err
	}
	meta, err := readStaticMeta(root)
	if err != nil {
		return nil, err
	}
	return renderStaticReportHTML(report, meta, n)
}

func renderStaticReportHTML(report staticReport, meta *staticMeta, currentVersion int) ([]byte, error) {
	t, err := template.New("static-report").Funcs(template.FuncMap{
		"euro":           formatEuro,
		"number":         formatNumberFr,
		"generatedAt":    formatGeneratedAtFr,
		"scoreTagClass":  scoreTagClassStatic,
		"scoreCardClass": scoreCardClassStatic,
		"severityClass":  severityClassStatic,
		"severityLabel":  severityLabelStatic,
		"posNeg":         posNegStatic,
		"donutPercent":   donutPercentStatic,
		"versionPath":    versionPathStatic,
		"versionLabel":   versionLabelStatic,
		"activeClass":    activeVersionClassStatic,
	}).Parse(staticReportTemplate)
	if err != nil {
		return nil, err
	}
	view := makeStaticReportView(report, meta, currentVersion)
	var buf bytes.Buffer
	if err := t.Execute(&buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readStaticMeta(root string) (*staticMeta, error) {
	var m staticMeta
	if err := readJSONFile(filepath.Join(root, "meta.json"), &m); err != nil {
		return nil, err
	}
	if m.Versions == nil {
		m.Versions = []staticVersion{}
	}
	sort.Slice(m.Versions, func(i, j int) bool { return m.Versions[i].N < m.Versions[j].N })
	return &m, nil
}

func writeStaticMeta(root string, m *staticMeta) error {
	return writeJSONFile(filepath.Join(root, "meta.json"), m)
}

func readStaticReport(root string, n int) (staticReport, error) {
	var r staticReport
	err := readJSONFile(staticReportPath(root, n), &r)
	return r, err
}

func readStaticEvidence(root string, n int) (evidenceFile, error) {
	var e evidenceFile
	err := readJSONFile(staticEvidencePath(root, n), &e)
	return e, err
}

func writeJSONFile(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	return os.WriteFile(path, buf, 0o644)
}

func readJSONFile(path string, v interface{}) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func staticVersionFromFlag(root string, raw string) (int, error) {
	if strings.TrimSpace(raw) != "" {
		return parseVersionArg(raw)
	}
	m, err := readStaticMeta(root)
	if err != nil {
		return 0, err
	}
	return m.ActiveVersion, nil
}

func staticVersionFromQuery(root string, r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("version"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("v"))
	}
	return staticVersionFromFlag(root, raw)
}

func staticVersionDir(root string, n int) string {
	return filepath.Join(root, "versions", fmt.Sprintf("v%d", n))
}

func staticReportPath(root string, n int) string {
	return filepath.Join(staticVersionDir(root, n), "report.json")
}

func staticEvidencePath(root string, n int) string {
	return filepath.Join(staticVersionDir(root, n), "evidence.json")
}

func staticNotesPath(root string, n int) string {
	return filepath.Join(staticVersionDir(root, n), "notes.md")
}

func staticMetaHasVersion(m *staticMeta, n int) bool {
	for _, v := range m.Versions {
		if v.N == n {
			return true
		}
	}
	return false
}

func maxStaticVersion(m *staticMeta) int {
	maxN := 0
	for _, v := range m.Versions {
		if v.N > maxN {
			maxN = v.N
		}
	}
	return maxN
}

func copyStaticTree(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyOneFile(path, target)
	})
}

func sumStaticLines(lines []staticLine) (float64, float64) {
	var n, n1 float64
	for _, line := range lines {
		n += line.N
		n1 += line.N1
	}
	return n, n1
}

func sumStaticMonthlyCharges(charges []staticMonthlyLine) []float64 {
	var out []float64
	for _, row := range charges {
		for i, value := range row.Cells {
			if len(out) <= i {
				out = append(out, 0)
			}
			out[i] += value
		}
	}
	return out
}

func staticFinancialAmounts(r staticReport) []staticAmount {
	var out []staticAmount
	add := func(path string, value float64) {
		out = append(out, staticAmount{Path: path, Value: value})
	}
	for i, line := range r.PNL.Produits {
		add(fmt.Sprintf("pnl.produits.%d.n", i), line.N)
		add(fmt.Sprintf("pnl.produits.%d.n1", i), line.N1)
	}
	for i, line := range r.PNL.Charges {
		add(fmt.Sprintf("pnl.charges.%d.n", i), line.N)
		add(fmt.Sprintf("pnl.charges.%d.n1", i), line.N1)
	}
	add("pnl.totals.produitsN", r.PNL.Totals.ProduitsN)
	add("pnl.totals.produitsN1", r.PNL.Totals.ProduitsN1)
	add("pnl.totals.chargesN", r.PNL.Totals.ChargesN)
	add("pnl.totals.chargesN1", r.PNL.Totals.ChargesN1)
	add("pnl.totals.resultatExploitationN", r.PNL.Totals.ResultatExploitationN)
	add("pnl.totals.resultatExploitationN1", r.PNL.Totals.ResultatExploitationN1)
	add("pnl.totals.resultatNetN", r.PNL.Totals.ResultatNetN)
	add("pnl.totals.resultatNetN1", r.PNL.Totals.ResultatNetN1)
	for i, alert := range r.Alerts.Blocking {
		if alert.Amount != 0 {
			add(fmt.Sprintf("alerts.blocking.%d.amount", i), alert.Amount)
		}
	}
	if r.Alerts.BlockingTotal != 0 {
		add("alerts.blockingTotal", r.Alerts.BlockingTotal)
	}
	for i, alert := range r.Alerts.Points {
		if alert.Amount != 0 {
			add(fmt.Sprintf("alerts.points.%d.amount", i), alert.Amount)
		}
	}
	for _, key := range sigEvidenceKeys() {
		add(fmt.Sprintf("sig.%s", key), r.Sig[key])
	}
	for i, value := range r.Monthly.Produits.Cells {
		add(fmt.Sprintf("monthly.produits.cells.%d", i), value)
	}
	add("monthly.produits.total", r.Monthly.Produits.Total)
	for i, charge := range r.Monthly.Charges {
		for j, value := range charge.Cells {
			add(fmt.Sprintf("monthly.charges.%d.cells.%d", i, j), value)
		}
		add(fmt.Sprintf("monthly.charges.%d.total", i), charge.Total)
	}
	for i, value := range r.Monthly.ChargesTotal.Cells {
		add(fmt.Sprintf("monthly.chargesTotal.cells.%d", i), value)
	}
	add("monthly.chargesTotal.total", r.Monthly.ChargesTotal.Total)
	for i, value := range r.Monthly.Result.Cells {
		add(fmt.Sprintf("monthly.result.cells.%d", i), value)
	}
	add("monthly.result.total", r.Monthly.Result.Total)
	for i, row := range r.Structure.Balance.Actif {
		add(fmt.Sprintf("structure.balance.actif.%d.amount", i), row.Amount)
	}
	for i, row := range r.Structure.Balance.Passif {
		add(fmt.Sprintf("structure.balance.passif.%d.amount", i), row.Amount)
	}
	add("structure.balance.totalActif", r.Structure.Balance.TotalActif)
	add("structure.balance.totalPassif", r.Structure.Balance.TotalPassif)
	for i, row := range r.Structure.Charts.Donut.Categories {
		add(fmt.Sprintf("structure.charts.donut.categories.%d.value", i), row.Value)
	}
	return out
}

func visibleStaticText(r staticReport) []string {
	out := []string{
		r.Client, r.Title, r.Period, r.Source, r.Subtitle, r.Footnote,
		r.Score.Label, r.Score.Level, r.Structure.DSOAlert,
	}
	for _, kpi := range r.KPIs {
		out = append(out, kpi.Label, kpi.Value, kpi.Comment)
	}
	for _, line := range r.PNL.Produits {
		out = append(out, line.Label)
	}
	for _, line := range r.PNL.Charges {
		out = append(out, line.Label)
	}
	for _, alert := range append(r.Alerts.Blocking, r.Alerts.Points...) {
		out = append(out, alert.Label, alert.Severity, alert.Account, alert.Comment)
	}
	for _, ratio := range r.Structure.Ratios {
		out = append(out, ratio.Label, ratio.Value)
	}
	for _, text := range r.Analyse.Narratives {
		out = append(out, text)
	}
	for _, s := range r.Analyse.Synthese {
		out = append(out, s.Label, s.Value, s.Comment)
	}
	out = append(out, r.Analyse.FooterExtra...)
	return out
}

func checkAmount(errorsFound *[]string, label string, expected float64, actual float64) {
	if !almostEqual(expected, actual) {
		*errorsFound = append(*errorsFound, fmt.Sprintf("%s mismatch: expected %.2f, got %.2f", label, expected, actual))
	}
}

func checkCells(errorsFound *[]string, label string, expected []float64, actual []float64) {
	if len(expected) != len(actual) {
		*errorsFound = append(*errorsFound, fmt.Sprintf("%s length mismatch: expected %d, got %d", label, len(expected), len(actual)))
		return
	}
	for i := range expected {
		checkAmount(errorsFound, fmt.Sprintf("%s.%d", label, i), expected[i], actual[i])
	}
}

func almostEqual(a float64, b float64) bool {
	if a > b {
		return a-b < 0.01
	}
	return b-a < 0.01
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range in {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

func containsInternalTerm(text string, term string) bool {
	lowerText := strings.ToLower(text)
	lowerTerm := strings.ToLower(term)
	if strings.Contains(lowerTerm, ".") {
		return strings.Contains(lowerText, lowerTerm)
	}
	for _, token := range strings.FieldsFunc(lowerText, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if token == lowerTerm {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func httpError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func makeStaticReportView(report staticReport, meta *staticMeta, currentVersion int) staticReportView {
	if report.Version == "" {
		report.Version = fmt.Sprintf("v%d", currentVersion)
	}
	if report.Year == 0 {
		report.Year = 2025
	}
	if report.PriorYear == 0 {
		report.PriorYear = report.Year - 1
	}
	if strings.TrimSpace(report.GeneratedAt) == "" {
		report.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if meta == nil {
		meta = &staticMeta{
			Tip:           currentVersion,
			ActiveVersion: currentVersion,
			Versions:      []staticVersion{{N: currentVersion}},
		}
	}
	return staticReportView{
		Report:         report,
		Meta:           meta,
		CurrentVersion: currentVersion,
		ReportCSS:      template.CSS(embeddedReportCSS),
		ShellCSS:       template.CSS(embeddedShellCSS),
		Sommaire:       staticSommaireItems(report),
		BalanceRows:    staticBalanceRows(report.Structure.Balance),
		SigRows:        staticSigRows(report),
		ScoringRows:    staticScoringRows(report),
	}
}

func staticSommaireItems(report staticReport) []staticSommaireItem {
	base := []struct {
		Label string
		Desc  string
	}{
		{"Fiabilité des données", "Score global, écritures classées, manquantes et ajustées"},
		{"Erreurs et points d'attention", "Encaissements/décaissements non identifiés et alertes de la revue comptable"},
		{"Compte de résultat", "P&L annuel jusqu'au résultat net après IS, détail par compte"},
	}
	if len(report.Sig) > 0 {
		base = append(base, struct {
			Label string
			Desc  string
		}{"Soldes intermédiaires de gestion", "Marge, valeur ajoutée, EBE, RCAI et CAF"})
	}
	if len(report.Monthly.Headers) > 0 {
		base = append(base, struct {
			Label string
			Desc  string
		}{"Compte de résultat mensuel", "Vue synthétique regroupée mois par mois"})
	}
	base = append(base, struct {
		Label string
		Desc  string
	}{"Structure financière", "Bilan synthétique, ratios, pont de marge et comparatif N/N-1"})
	base = append(base, struct {
		Label string
		Desc  string
	}{"Analyse, fiscalité et fiabilité", "Scoring, IS estimé, TVA et synthèse"})

	out := make([]staticSommaireItem, 0, len(base))
	for i, item := range base {
		out = append(out, staticSommaireItem{
			Num:   fmt.Sprintf("%02d", i+1),
			Label: item.Label,
			Desc:  item.Desc,
		})
	}
	return out
}

func staticBalanceRows(balance staticBalance) []staticBalancePair {
	n := len(balance.Actif)
	if len(balance.Passif) > n {
		n = len(balance.Passif)
	}
	rows := make([]staticBalancePair, 0, n)
	for i := 0; i < n; i++ {
		var pair staticBalancePair
		if i < len(balance.Actif) {
			item := balance.Actif[i]
			pair.Actif = &item
		}
		if i < len(balance.Passif) {
			item := balance.Passif[i]
			pair.Passif = &item
		}
		rows = append(rows, pair)
	}
	return rows
}

func staticSigRows(report staticReport) []staticSigRow {
	defs := []staticSigRow{
		{Label: "Marge brute", Class: "line-item", N: report.Sig["margeBruteN"], N1: report.Sig["margeBruteN1"]},
		{Label: "Valeur ajoutée", Class: "line-item", N: report.Sig["valeurAjouteeN"], N1: report.Sig["valeurAjouteeN1"]},
		{Label: "Excédent brut d'exploitation (EBE)", Class: "line-item", N: report.Sig["ebeN"], N1: report.Sig["ebeN1"]},
		{Label: "Résultat d'exploitation", Class: "line-item", N: report.Sig["reN"], N1: report.Sig["reN1"]},
		{Label: "Résultat courant avant impôts (RCAI)", Class: "line-item", N: report.Sig["rcaiN"], N1: report.Sig["rcaiN1"]},
		{Label: "Résultat net", Class: "net-result", N: report.Sig["rnN"], N1: report.Sig["rnN1"]},
	}
	if defs[len(defs)-1].N == 0 {
		defs[len(defs)-1].N = report.Sig["resultatNetN"]
	}
	if defs[len(defs)-1].N1 == 0 {
		defs[len(defs)-1].N1 = report.Sig["resultatNetN1"]
	}
	return defs
}

func staticScoringRows(report staticReport) []staticScoringRow {
	return []staticScoringRow{
		{
			Label: "Fiabilité globale",
			Value: fmt.Sprintf("%d%%", report.Score.Global),
			Class: "scoring-total",
		},
	}
}

func scoreTagClassStatic(globalScore int) string {
	if globalScore >= 85 {
		return "tag-brand"
	}
	if globalScore >= 70 {
		return "tag-warning"
	}
	return "tag-danger"
}

func scoreCardClassStatic(level string) string {
	switch strings.ToLower(level) {
	case "fiable":
		return "score-fiable"
	case "acceptable":
		return "score-acceptable"
	case "douteux":
		return "score-non-fiable"
	default:
		return "score-acceptable"
	}
}

func severityClassStatic(severity string) string {
	switch strings.ToLower(severity) {
	case "erreur", "error":
		return "severity-erreur"
	case "alerte", "warning", "warn":
		return "severity-alerte"
	default:
		return "severity-info"
	}
}

func severityLabelStatic(severity string) string {
	switch strings.ToLower(severity) {
	case "erreur", "error":
		return "ERREUR"
	case "alerte", "warning", "warn":
		return "ALERTE"
	default:
		return "INFO"
	}
}

func posNegStatic(v float64) string {
	if v > 0 {
		return "positive"
	}
	if v < 0 {
		return "negative"
	}
	return ""
}

func donutPercentStatic(categories []staticDonutCategory, cat staticDonutCategory) string {
	var total float64
	for _, item := range categories {
		total += item.Value
	}
	if total == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", int((cat.Value/total)*100+0.5))
}

func versionPathStatic(n int) string {
	return fmt.Sprintf("/report/v%d", n)
}

func versionLabelStatic(n int) string {
	return fmt.Sprintf("v%d", n)
}

func activeVersionClassStatic(n int, current int) string {
	if n == current {
		return "version-tab is-active"
	}
	return "version-tab"
}

func formatEuro(v float64) string {
	return formatNumberFr(v) + " €"
}

func formatNumberFr(v float64) string {
	n := int64(v)
	if v < 0 {
		n = int64(v - 0.5)
	} else {
		n = int64(v + 0.5)
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	raw := strconv.FormatInt(n, 10)
	var parts []string
	for len(raw) > 3 {
		parts = append([]string{raw[len(raw)-3:]}, parts...)
		raw = raw[:len(raw)-3]
	}
	parts = append([]string{raw}, parts...)
	return sign + strings.Join(parts, " ")
}

func formatGeneratedAtFr(iso string) string {
	d, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return d.Format("02/01/2006 15:04")
}

func formatMoney(v float64) string {
	return fmt.Sprintf("%s k", formatK(v))
}

func formatSignedMoney(v float64) string {
	prefix := ""
	if v > 0 {
		prefix = "+"
	}
	return prefix + formatMoney(v)
}

func formatDelta(n float64, n1 float64) string {
	if n1 == 0 {
		return "-"
	}
	return fmt.Sprintf("%+.1f%%", ((n-n1)/n1)*100)
}

const legacyStaticReportTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Client}} - {{.Version}}</title>
  <style>
    :root {
      color-scheme: light;
      --ink: #151515;
      --muted: #666c72;
      --line: #d8dce0;
      --paper: #fbfbf8;
      --panel: #ffffff;
      --accent: #176a5d;
      --accent-2: #295f9f;
      --warn: #9a5d00;
      --bad: #9b2f2f;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--paper);
      color: var(--ink);
      font-family: Charter, "Iowan Old Style", Georgia, serif;
      line-height: 1.45;
    }
    .page {
      width: min(1120px, calc(100% - 40px));
      margin: 0 auto;
      padding: 40px 0 56px;
    }
    header {
      border-bottom: 2px solid var(--ink);
      padding-bottom: 22px;
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 24px;
      align-items: end;
    }
    h1, h2, h3, p { margin: 0; }
    h1 {
      font-size: clamp(32px, 5vw, 64px);
      line-height: 0.98;
      max-width: 820px;
      letter-spacing: 0;
    }
    .eyebrow, .label {
      color: var(--muted);
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 12px;
      text-transform: uppercase;
    }
    .meta {
      text-align: right;
      min-width: 220px;
    }
    .meta strong {
      display: block;
      font-size: 24px;
      color: var(--accent);
    }
    .intro {
      display: grid;
      grid-template-columns: minmax(0, 1.6fr) minmax(280px, 0.9fr);
      gap: 18px;
      padding: 22px 0;
      border-bottom: 1px solid var(--line);
    }
    .intro p { color: #32363a; font-size: 18px; }
    .kpis {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      border: 1px solid var(--line);
      background: var(--panel);
    }
    .kpi {
      padding: 16px;
      border-right: 1px solid var(--line);
      min-width: 0;
    }
    .kpi:last-child { border-right: 0; }
    .kpi strong {
      display: block;
      margin-top: 6px;
      font-size: 24px;
      overflow-wrap: anywhere;
    }
    .kpi p { color: var(--muted); font-size: 13px; }
    section { padding: 26px 0; border-bottom: 1px solid var(--line); }
    section h2 { font-size: 24px; margin-bottom: 14px; }
    .grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      padding: 18px;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-variant-numeric: tabular-nums;
      background: var(--panel);
      border: 1px solid var(--line);
    }
    th, td {
      padding: 10px 12px;
      border-bottom: 1px solid var(--line);
      text-align: right;
      vertical-align: top;
    }
    th:first-child, td:first-child { text-align: left; }
    th {
      color: var(--muted);
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 12px;
      text-transform: uppercase;
      background: #f1f3f1;
    }
    tr:last-child td { border-bottom: 0; }
    tfoot td {
      font-weight: 700;
      border-top: 2px solid var(--ink);
    }
    .alert {
      border-left: 4px solid var(--accent-2);
      padding: 12px 14px;
      background: var(--panel);
      margin-bottom: 10px;
    }
    .alert.alerte { border-left-color: var(--warn); }
    .alert.bloquant, .alert.blocking { border-left-color: var(--bad); }
    .alert strong { display: block; }
    .alert span { color: var(--muted); }
    .narrative {
      font-size: 18px;
      color: #2f3438;
      max-width: 860px;
    }
    footer {
      padding-top: 24px;
      color: var(--muted);
      font-size: 13px;
    }
    @media (max-width: 820px) {
      .page { width: min(100% - 24px, 1120px); padding-top: 24px; }
      header, .intro, .grid { grid-template-columns: 1fr; }
      .meta { text-align: left; }
      .kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .kpi:nth-child(2) { border-right: 0; }
    }
    @media print {
      body { background: #fff; }
      .page { width: 100%; padding: 0; }
      section { break-inside: avoid; }
    }
  </style>
</head>
<body>
  <main class="page">
    <header>
      <div>
        <p class="eyebrow">{{.Period}} / {{.Version}}</p>
        <h1>{{.Title}}</h1>
      </div>
      <div class="meta">
        <p class="label">Client</p>
        <strong>{{.Client}}</strong>
        <p>{{.Score.Label}} / {{.Score.Global}} out of 100</p>
      </div>
    </header>

    <div class="intro">
      <p>{{.Subtitle}}</p>
      <p>{{.Footnote}}</p>
    </div>

    <section class="kpis" aria-label="Key figures">
      {{range .KPIs}}
      <div class="kpi">
        <p class="label">{{.Label}}</p>
        <strong>{{.Value}}</strong>
        <p>{{.Comment}}</p>
      </div>
      {{end}}
    </section>

    <section>
      <h2>Profit and loss</h2>
      <table>
        <thead><tr><th>Line</th><th>N</th><th>N-1</th><th>Change</th></tr></thead>
        <tbody>
          {{range .PNL.Produits}}<tr><td>{{.Label}}</td><td>{{money .N}}</td><td>{{money .N1}}</td><td>{{pct .N .N1}}</td></tr>{{end}}
          {{range .PNL.Charges}}<tr><td>{{.Label}}</td><td>{{money .N}}</td><td>{{money .N1}}</td><td>{{pct .N .N1}}</td></tr>{{end}}
        </tbody>
        <tfoot>
          <tr><td>Net result</td><td>{{money .PNL.Totals.ResultatNetN}}</td><td>{{money .PNL.Totals.ResultatNetN1}}</td><td>{{pct .PNL.Totals.ResultatNetN .PNL.Totals.ResultatNetN1}}</td></tr>
        </tfoot>
      </table>
    </section>

    <section>
      <h2>Review points</h2>
      <div class="grid">
        <div>
          {{range .Alerts.Blocking}}
          <div class="alert {{lower .Severity}}"><strong>{{.Label}}</strong><span>{{money .Amount}}</span><p>{{.Comment}}</p></div>
          {{else}}
          <div class="alert"><strong>No blocking point</strong><p>The demo file has no blocking item.</p></div>
          {{end}}
        </div>
        <div>
          {{range .Alerts.Points}}
          <div class="alert {{lower .Severity}}"><strong>{{.Label}}</strong>{{if .Amount}}<span>{{money .Amount}}</span>{{end}}<p>{{.Comment}}</p></div>
          {{end}}
        </div>
      </div>
    </section>

    <section>
      <h2>Monthly view</h2>
      <table>
        <thead><tr><th>Line</th>{{range .Monthly.Headers}}<th>{{.}}</th>{{end}}<th>Total</th></tr></thead>
        <tbody>
          <tr><td>Revenue</td>{{range .Monthly.Produits.Cells}}<td>{{money .}}</td>{{end}}<td>{{money .Monthly.Produits.Total}}</td></tr>
          {{range .Monthly.Charges}}<tr><td>{{.Label}}</td>{{range .Cells}}<td>{{money .}}</td>{{end}}<td>{{money .Total}}</td></tr>{{end}}
        </tbody>
        <tfoot>
          <tr><td>Result</td>{{range .Monthly.Result.Cells}}<td>{{money .}}</td>{{end}}<td>{{money .Monthly.Result.Total}}</td></tr>
        </tfoot>
      </table>
    </section>

    <section>
      <h2>Structure</h2>
      <div class="grid">
        <table>
          <thead><tr><th>Assets</th><th>Amount</th></tr></thead>
          <tbody>{{range .Structure.Balance.Actif}}<tr><td>{{.Label}}</td><td>{{money .Amount}}</td></tr>{{end}}</tbody>
          <tfoot><tr><td>Total assets</td><td>{{money .Structure.Balance.TotalActif}}</td></tr></tfoot>
        </table>
        <table>
          <thead><tr><th>Liabilities</th><th>Amount</th></tr></thead>
          <tbody>{{range .Structure.Balance.Passif}}<tr><td>{{.Label}}</td><td>{{money .Amount}}</td></tr>{{end}}</tbody>
          <tfoot><tr><td>Total liabilities</td><td>{{money .Structure.Balance.TotalPassif}}</td></tr></tfoot>
        </table>
      </div>
    </section>

    <section>
      <h2>Analysis</h2>
      <p class="narrative">{{index .Analyse.Narratives "synthese"}}</p>
    </section>

    <footer>
      <p>{{.Source}}</p>
      {{range .Analyse.FooterExtra}}<p>{{.}}</p>{{end}}
    </footer>
  </main>
</body>
</html>`

const staticReportTemplate = `<!doctype html>
<html lang="fr">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Report.Client}} - {{.Report.Version}}</title>
  <style>{{.ReportCSS}}</style>
  <style>{{.ShellCSS}}</style>
</head>
<body>
  <nav class="numeral-navbar">
    <div class="numeral-navbar-inner">
      <div class="navbar-tabs">
        {{range .Meta.Versions}}
        <span class="{{activeClass .N $.CurrentVersion}}">
          <a class="version-tab-label" href="{{versionPath .N}}">
            {{versionLabel .N}}
            {{if .Frozen}}<span class="version-tab-frozen">lock</span>{{end}}
          </a>
        </span>
        {{end}}
        <button class="version-add" type="button" title="Nouvelle version" onclick="createVersion()">+</button>
      </div>
      <button class="version-publish" type="button" onclick="window.print()">PDF</button>
    </div>
  </nav>

  <section class="page cover" id="section-cover">
    <div class="cover-client">{{.Report.Client}}</div>
    <div class="cover-title">{{.Report.Title}}</div>
    <div class="cover-period">{{.Report.Period}}</div>
    <div class="cover-brand">
      <div class="cover-brand-rule"></div>
      <div class="cover-brand-name">NUMERAL</div>
    </div>
  </section>

  <section class="page sommaire" id="section-sommaire">
    <div class="sommaire-title">Sommaire</div>
    <ol class="sommaire-list">
      {{range .Sommaire}}
      <li class="sommaire-item">
        <span class="sommaire-num">{{.Num}}</span>
        <div>
          <div class="sommaire-label">{{.Label}}</div>
          <div class="sommaire-desc">{{.Desc}}</div>
        </div>
      </li>
      {{end}}
    </ol>
  </section>

  <section class="page" id="section-scores">
    <div class="page-header">
      <div>
        <h1>Fiabilité des données</h1>
        <p class="subtitle">Score global et détail par bloc — <span>{{.Report.Period}}</span></p>
      </div>
      <span class="tag {{scoreTagClass .Report.Score.Global}}">Score {{.Report.Score.Global}}%</span>
    </div>
    <div class="scores-grid">
      <div class="score-card is-primary {{scoreCardClass .Report.Score.Level}}">
        <div class="score-label">Fiabilité globale</div>
        <div class="score-value">{{.Report.Score.Global}}%</div>
        <div class="score-badge">{{.Report.Score.Label}}</div>
        <div class="score-desc">Confiance dans les chiffres affichés, après revue comptable</div>
      </div>
      <div class="score-card">
        <div class="score-label">Écritures classées</div>
        <div class="score-value" style="color: var(--text)">{{.Report.Processing.Traitement}}%</div>
        <div class="score-desc"><span>{{.Report.Processing.MontantTraite}}</span> identifiés et rangés dans le bon compte</div>
      </div>
      <div class="score-card">
        <div class="score-label">Écritures manquantes</div>
        <div class="score-value" style="color: var(--text)">{{.Report.Processing.NonTraite}}%</div>
        <div class="score-desc"><span>{{.Report.Processing.MontantNonTraite}}</span> payés en banque mais absents du P&amp;L</div>
      </div>
      <div class="score-card">
        <div class="score-label">Ajustements estimés</div>
        <div class="score-value" style="color: var(--text)">{{.Report.Processing.Ajustement}}%</div>
        <div class="score-desc"><span>{{.Report.Processing.MontantAjuste}}</span> reconstitués par estimation</div>
      </div>
    </div>
    {{with index .Report.Analyse.Narratives "score"}}<p class="section-copy">{{.}}</p>{{end}}
  </section>

  <section class="page" id="section-alerts">
    <div class="page-header">
      <div>
        <h1>Erreurs et points d'attention</h1>
        <p class="subtitle">Ce qui doit être corrigé avant clôture — <span>{{.Report.Period}}</span></p>
      </div>
      <span class="tag tag-danger">{{len .Report.Alerts.Blocking}} erreurs · {{len .Report.Alerts.Points}} alertes</span>
    </div>

    <h3>
      Erreurs bloquantes
      <span class="tag tag-danger" style="vertical-align: middle">{{len .Report.Alerts.Blocking}}</span>
    </h3>
    {{if .Report.Alerts.Blocking}}
    <table class="report-table" style="margin-top: 12px; margin-bottom: 0">
      <thead>
        <tr><th>Libellé</th><th>Compte</th><th>Commentaire</th><th class="num">Montant</th></tr>
      </thead>
      <tbody>
        {{range .Report.Alerts.Blocking}}
        <tr>
          <td>{{.Label}}</td>
          <td>{{.Account}}</td>
          <td>{{.Comment}}</td>
          <td class="num">{{euro .Amount}}</td>
        </tr>
        {{end}}
      </tbody>
      <tfoot>
        <tr class="total-row">
          <td colspan="3"><strong>Total</strong></td>
          <td class="num"><strong>{{euro .Report.Alerts.BlockingTotal}}</strong></td>
        </tr>
      </tfoot>
    </table>
    {{else}}
    <p class="section-copy">Aucune écriture en compte d'attente — bon point.</p>
    {{end}}

    <h3 style="margin-top: 32px">
      Points d'attention
      <span class="tag tag-danger" style="vertical-align: middle">{{len .Report.Alerts.Points}} alertes</span>
    </h3>
    {{if .Report.Alerts.Points}}
    <table class="report-table" style="margin-top: 12px">
      <thead>
        <tr><th>Sévérité</th><th>Sujet</th><th class="num">Montant</th><th>Commentaire</th></tr>
      </thead>
      <tbody>
        {{range .Report.Alerts.Points}}
        <tr>
          <td><span class="{{severityClass .Severity}}">{{severityLabel .Severity}}</span></td>
          <td>{{.Label}}</td>
          <td class="num">{{euro .Amount}}</td>
          <td>{{.Comment}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}
    <p class="section-copy">Aucune alerte — la balance traverse la revue sans signal.</p>
    {{end}}
  </section>

  <section class="page" id="section-pnl">
    <div class="page-header">
      <div>
        <h1>Compte de Résultat</h1>
        <p class="subtitle">{{.Report.Subtitle}}</p>
      </div>
    </div>
    <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 12px; justify-content: flex-end; margin-top: 24px">
      <button type="button" class="toggle-detail">
        <span class="toggle-icon">&#9660;</span>
        <span>Détail</span>
      </button>
    </div>
    <div class="table-scroll">
      <table class="report-table">
        <colgroup>
          <col>
          <col style="width: 130px">
          <col style="width: 130px">
        </colgroup>
        <thead>
          <tr><th>Libellé</th><th class="num">{{.Report.Year}}</th><th class="num">{{.Report.PriorYear}}</th></tr>
        </thead>
        <tbody>
          <tr class="section-header"><td colspan="3">Produits d'exploitation</td></tr>
          {{range .Report.PNL.Produits}}
          <tr class="line-item"><td>{{.Label}}</td><td class="num">{{euro .N}}</td><td class="num">{{euro .N1}}</td></tr>
          {{end}}
          <tr class="total-row">
            <td>Total produits d'exploitation</td>
            <td class="num">{{euro .Report.PNL.Totals.ProduitsN}}</td>
            <td class="num">{{euro .Report.PNL.Totals.ProduitsN1}}</td>
          </tr>

          <tr class="section-header"><td colspan="3">Charges d'exploitation</td></tr>
          {{range .Report.PNL.Charges}}
          <tr class="line-item"><td>{{.Label}}</td><td class="num">{{euro .N}}</td><td class="num">{{euro .N1}}</td></tr>
          {{end}}
          <tr class="total-row">
            <td>Total charges d'exploitation</td>
            <td class="num">{{euro .Report.PNL.Totals.ChargesN}}</td>
            <td class="num">{{euro .Report.PNL.Totals.ChargesN1}}</td>
          </tr>

          <tr class="result-separator"><td colspan="3"></td></tr>
          <tr class="result-row">
            <td>Résultat d'exploitation</td>
            <td class="num">{{euro .Report.PNL.Totals.ResultatExploitationN}}</td>
            <td class="num">{{euro .Report.PNL.Totals.ResultatExploitationN1}}</td>
          </tr>
          <tr class="net-result">
            <td>Résultat net</td>
            <td class="num">{{euro .Report.PNL.Totals.ResultatNetN}}</td>
            <td class="num">{{euro .Report.PNL.Totals.ResultatNetN1}}</td>
          </tr>
        </tbody>
      </table>
      {{if .Report.Footnote}}<p class="table-footnote">{{.Report.Footnote}}</p>{{end}}
    </div>
    {{with index .Report.Analyse.Narratives "pnl"}}<p class="section-copy">{{.}}</p>{{end}}
  </section>

  {{if .Report.Sig}}
  <section class="page" id="section-sig">
    <div class="page-header">
      <div>
        <h1>Soldes intermédiaires de gestion</h1>
        <p class="subtitle">Marge, valeur ajoutée, EBE, CAF — <span>{{.Report.Period}}</span></p>
      </div>
      <span class="tag tag-brand">{{.Report.Year}}</span>
    </div>
    <div class="table-scroll">
      <table class="report-table">
        <colgroup><col><col style="width: 130px"><col style="width: 130px"></colgroup>
        <thead><tr><th>Solde</th><th class="num">{{.Report.Year}}</th><th class="num">{{.Report.PriorYear}}</th></tr></thead>
        <tbody>
          {{range .SigRows}}
          <tr class="{{.Class}}"><td>{{.Label}}</td><td class="num">{{euro .N}}</td><td class="num">{{euro .N1}}</td></tr>
          {{end}}
        </tbody>
      </table>
    </div>
    {{with index .Report.Analyse.Narratives "sig"}}<p class="section-copy">{{.}}</p>{{end}}
  </section>
  {{end}}

  {{if .Report.Monthly.Headers}}
  <section class="page" id="section-monthly">
    <div class="page-header">
      <div>
        <h1>Compte de résultat mensuel</h1>
        <p class="subtitle">Vue synthétique regroupée — <span>{{.Report.Period}}</span></p>
      </div>
      <span class="tag tag-brand">{{len .Report.Monthly.Headers}} mois</span>
    </div>
    <div class="table-scroll">
      <table class="report-table compact">
        <thead>
          <tr><th>Poste</th>{{range .Report.Monthly.Headers}}<th class="num">{{.}}</th>{{end}}<th class="num">Total</th></tr>
        </thead>
        <tbody>
          <tr class="total-row">
            <td><strong>Produits</strong></td>
            {{range .Report.Monthly.Produits.Cells}}<td class="num">{{number .}}</td>{{end}}
            <td class="num">{{number .Report.Monthly.Produits.Total}}</td>
          </tr>
          {{range .Report.Monthly.Charges}}
          <tr>
            <td>{{.Label}}</td>
            {{range .Cells}}<td class="num">{{number .}}</td>{{end}}
            <td class="num">{{number .Total}}</td>
          </tr>
          {{end}}
          <tr class="total-row">
            <td><strong>Total charges</strong></td>
            {{range .Report.Monthly.ChargesTotal.Cells}}<td class="num">{{number .}}</td>{{end}}
            <td class="num">{{number .Report.Monthly.ChargesTotal.Total}}</td>
          </tr>
          <tr class="result-separator"><td colspan="6"></td></tr>
          <tr class="result-row">
            <td><strong>Résultat</strong></td>
            {{range .Report.Monthly.Result.Cells}}<td class="num {{posNeg .}}">{{number .}}</td>{{end}}
            <td class="num {{posNeg .Report.Monthly.Result.Total}}">{{number .Report.Monthly.Result.Total}}</td>
          </tr>
        </tbody>
      </table>
    </div>
    {{with index .Report.Analyse.Narratives "monthly"}}<p class="section-copy">{{.}}</p>{{end}}
  </section>
  {{end}}

  <section class="page" id="section-structure">
    <div class="page-header">
      <div>
        <h1>Structure financière</h1>
        <p class="subtitle">Pont de marge, répartition des charges et comparatif N/N-1</p>
      </div>
      <span class="tag tag-brand">{{.Report.Year}}</span>
    </div>

    <div>
      <h3 style="margin-bottom: 12px">Bilan synthétique</h3>
      <div class="table-scroll">
        <table class="report-table compact">
          <colgroup><col><col style="width: 130px"><col><col style="width: 130px"></colgroup>
          <thead><tr><th>Actif</th><th class="num">{{.Report.Year}}</th><th>Passif</th><th class="num">{{.Report.Year}}</th></tr></thead>
          <tbody>
            {{range .BalanceRows}}
            <tr>
              <td>{{if .Actif}}{{.Actif.Label}}{{end}}</td>
              <td class="num">{{if .Actif}}{{euro .Actif.Amount}}{{end}}</td>
              <td>{{if .Passif}}{{.Passif.Label}}{{end}}</td>
              <td class="num">{{if .Passif}}{{euro .Passif.Amount}}{{end}}</td>
            </tr>
            {{end}}
            <tr class="total-row">
              <td><strong>Total actif</strong></td>
              <td class="num"><strong>{{euro .Report.Structure.Balance.TotalActif}}</strong></td>
              <td><strong>Total passif</strong></td>
              <td class="num"><strong>{{euro .Report.Structure.Balance.TotalPassif}}</strong></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    {{if .Report.Structure.Ratios}}
    <div>
      <h3 style="margin-top: 24px; margin-bottom: 12px">Ratios</h3>
      <div class="kv-list">
        {{range .Report.Structure.Ratios}}
        <div class="kv-row"><span>{{.Label}}</span><strong>{{.Value}}</strong></div>
        {{end}}
      </div>
    </div>
    {{end}}

    {{if .Report.Structure.DSOAlert}}
    <p class="section-copy" style="color: var(--danger)"><strong>Alerte DSO :</strong> <span>{{.Report.Structure.DSOAlert}}</span></p>
    {{end}}

    <section class="chart-grid">
      <div>
        <h3 style="margin-bottom: 12px">Pont de marge</h3>
        <div id="chart-bridge" class="chart-container"></div>
        {{with index .Report.Analyse.Narratives "bridge"}}<p class="section-copy">{{.}}</p>{{end}}
      </div>
      <div>
        <h3 style="margin-bottom: 12px">Répartition des charges</h3>
        <div id="chart-charges-donut" class="chart-container"></div>
        {{if .Report.Structure.Charts.Donut.Categories}}
        <div class="kv-list mt-16">
          {{range .Report.Structure.Charts.Donut.Categories}}
          <div class="kv-row"><span>{{.Label}}</span><strong>{{donutPercent $.Report.Structure.Charts.Donut.Categories .}}%</strong></div>
          {{end}}
        </div>
        {{end}}
      </div>
    </section>

    <div class="mt-16">
      <h3 style="margin-bottom: 12px">Comparatif {{.Report.Year}} vs {{.Report.PriorYear}}</h3>
      <div id="chart-comparison" class="chart-container"></div>
      {{with index .Report.Analyse.Narratives "comparison"}}<p class="section-copy">{{.}}</p>{{end}}
    </div>
    {{with index .Report.Analyse.Narratives "structure"}}<p class="section-copy">{{.}}</p>{{end}}
  </section>

  <section class="page" id="section-analyse">
    <div class="page-header">
      <div>
        <h1>Analyse et fiabilité</h1>
        <p class="subtitle">À quel point peut-on faire confiance aux chiffres de ce rapport ?</p>
      </div>
      <span class="tag {{scoreTagClass .Report.Score.Global}}">Fiabilité {{.Report.Score.Global}}%</span>
    </div>

    <div class="scoring-grid">
      <div>
        <h4 class="scoring-heading">Calcul du score</h4>
        <table class="scoring-table">
          <tbody>
            {{range .ScoringRows}}
            <tr class="{{.Class}}"><td>{{.Label}}</td><td class="num">{{.Value}}</td></tr>
            {{end}}
          </tbody>
        </table>
      </div>
      <div>
        <h4 class="scoring-heading">Ce qui fait baisser le score</h4>
        <table class="scoring-table">
          <tbody>
            {{range .Report.Analyse.Penalties}}
            <tr>
              <td><div>{{.Label}}</div>{{if .Reason}}<div class="line-detail-text">{{.Reason}}</div>{{end}}</td>
              <td class="num"><span class="tag tag-danger">{{number .Weight}}%</span></td>
            </tr>
            {{end}}
          </tbody>
        </table>
      </div>
    </div>

    <h3 style="margin-top: 32px">Fiscalité estimée</h3>
    {{if .Report.Analyse.Fiscalite}}
    <table class="report-table" style="margin-top: 12px">
      <colgroup><col><col style="width: 160px"><col></colgroup>
      <thead><tr><th>Poste</th><th>Statut</th><th>Commentaire</th></tr></thead>
      <tbody>
        {{range .Report.Analyse.Fiscalite}}
        <tr><td>{{.Label}}</td><td>{{.Status}}</td><td>{{.Comment}}</td></tr>
        {{end}}
      </tbody>
    </table>
    {{end}}

    <h3 style="margin-top: 32px">Synthèse</h3>
    <div class="kv-list mt-16">
      {{range .Report.Analyse.Synthese}}
      <div class="kv-row"><span>{{.Label}}</span><strong class="{{.Accent}}">{{if .Comment}}{{.Comment}}{{else}}{{.Value}}{{end}}</strong></div>
      {{end}}
    </div>

    {{with index .Report.Analyse.Narratives "fiscalite"}}<p class="mt-16">{{.}}</p>{{end}}
    {{with index .Report.Analyse.Narratives "synthese"}}<p class="mt-16">{{.}}</p>{{end}}

    <div class="report-footer">
      <span>
        Rapport généré le <span>{{generatedAt .Report.GeneratedAt}}</span> — Données source : <span>{{.Report.Source}}</span><br>
      </span>
      {{range .Report.Analyse.FooterExtra}}<span>{{.}}<br></span>{{end}}
      Scoring spec V2 : score_transaction = 0.40 × identité + 0.30 × cohérence + 0.20 × récurrence + 0.10 × montant<br>
      Score global = moyenne pondérée par montant des 3 blocs exclusifs (traité / non traité / ajustements)<br>
      Ce document est à usage interne. Les données provisoires n'ont pas valeur de bilan définitif.
    </div>
  </section>

  <script>
    async function createVersion() {
      const res = await fetch("/api/new?from=v{{.CurrentVersion}}", { method: "POST" });
      if (!res.ok) {
        const message = await res.text();
        alert(message);
        return;
      }
      const payload = await res.json();
      window.location.href = "/report/" + payload.created;
    }

    document.querySelectorAll(".toggle-detail").forEach((button) => {
      button.addEventListener("click", () => {
        const table = button.closest(".page").querySelector(".report-table");
        if (!table) return;
        table.classList.toggle("show-detail");
        button.classList.toggle("is-expanded");
        const label = button.querySelector("span:last-child");
        if (label) label.textContent = table.classList.contains("show-detail") ? "Masquer" : "Détail";
      });
    });
  </script>
</body>
</html>`
