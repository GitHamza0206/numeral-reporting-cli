package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/numeral/numeral-reporting-cli/internal/reports"
)

type factoryLine struct {
	Label string
	N     float64
	N1    float64
}

type factoryAlert struct {
	Label   string
	Level   string
	Account string
	Amount  float64
	Comment string
}

type factoryMonthlyCharge struct {
	Label string
	Cells []float64
}

type factoryBalanceRow struct {
	Label  string
	Amount float64
}

type factoryRatio struct {
	Label string
	Value string
}

type evidenceFile struct {
	Version string        `json:"version"`
	Items   []evidenceRow `json:"items"`
}

type evidenceRow struct {
	Path    string  `json:"path"`
	Value   float64 `json:"value"`
	Source  string  `json:"source"`
	Formula string  `json:"formula,omitempty"`
	Note    string  `json:"note,omitempty"`
}

type factoryProfile struct {
	Kind                string
	PackageName         string
	Client              string
	Title               string
	Period              string
	Source              string
	Subtitle            string
	Footnote            string
	Score               int
	ScoreLevel          string
	ScoreLabel          string
	Traitement          int
	NonTraite           int
	Ajustement          int
	MontantTraite       string
	MontantNonTraite    string
	MontantAjuste       string
	Products            []factoryLine
	Charges             []factoryLine
	Blocking            []factoryAlert
	BlockingTotal       float64
	Points              []factoryAlert
	Sig                 map[string]float64
	MonthlyProducts     []float64
	MonthlyCharges      []factoryMonthlyCharge
	Actif               []factoryBalanceRow
	Passif              []factoryBalanceRow
	Ratios              []factoryRatio
	DSOAlert            string
	ScoreNarrative      string
	PNLNarrative        string
	SIGNarrative        string
	MonthlyNarrative    string
	StructureNarrative  string
	BridgeNarrative     string
	ComparisonNarrative string
	FiscalNarrative     string
	SummaryNarrative    string
	ReviewPoint         string
}

func cmdCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	kind := fs.String("kind", "", "report factory kind: demo-saas, restaurant, or cabinet-client")
	mode := fs.String("mode", "static", "project mode: static or next")
	template := fs.String("template", defaultTemplate(), "path to the numeral-reporting template directory to copy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("create: missing target directory")
	}
	profile, ok := factoryProfiles()[*kind]
	if !ok {
		return fmt.Errorf("unknown --kind %q (expected: %s)", *kind, strings.Join(factoryKindNames(), ", "))
	}

	target, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("target %s already exists", target)
	}

	switch *mode {
	case "static":
		if err := writeStaticFactoryProject(target, profile); err != nil {
			return err
		}
		fmt.Printf("created %s static report project at %s\n", profile.Kind, target)
		fmt.Println("next: numeral-reporting app --project", target)
		return nil
	case "next":
		// Keep the original Next.js template flow available while the static
		// app becomes the default path for agent-generated reports.
	default:
		return fmt.Errorf("unknown --mode %q (expected: static or next)", *mode)
	}

	if _, err := os.Stat(*template); err != nil {
		return fmt.Errorf("template %s not found: %w", *template, err)
	}
	if err := copyTreeSkippingHeavy(*template, target); err != nil {
		return err
	}

	p, _ := reports.Open(target)
	if _, err := p.ReadMeta(); err != nil {
		return err
	}
	if err := writeFactoryProject(target, profile); err != nil {
		return err
	}
	if err := p.RefreshRegistry(); err != nil {
		return err
	}

	fmt.Printf("created %s report project at %s\n", profile.Kind, target)
	fmt.Println("next: cd", target, "&& npm install && npm run dev")
	return nil
}

func factoryProfiles() map[string]factoryProfile {
	return map[string]factoryProfile{
		"demo-saas": {
			Kind:             "demo-saas",
			PackageName:      "numeral-demo-saas-reporting",
			Client:           "Demo SaaS SAS",
			Title:            "SaaS financial review",
			Period:           "FY 2025 - demo data",
			Source:           "Fictional SaaS demo dataset.",
			Subtitle:         "Revenue, operating costs, and profitability for a subscription software company.",
			Footnote:         "Demo-only amounts. Replace with balances, ledgers, or FEC exports for real client work.",
			Score:            86,
			ScoreLevel:       "fiable",
			ScoreLabel:       "Fiable",
			Traitement:       93,
			NonTraite:        4,
			Ajustement:       10,
			MontantTraite:    "93 %",
			MontantNonTraite: "4 %",
			MontantAjuste:    "10 %",
			Products: []factoryLine{
				{"Subscriptions", 420000, 300000},
				{"Implementation services", 90000, 65000},
				{"Other operating income", 15000, 8000},
			},
			Charges: []factoryLine{
				{"Hosting and tools", 76000, 52000},
				{"External services", 110000, 90000},
				{"Personnel costs", 240000, 185000},
				{"Taxes and duties", 18000, 14000},
				{"Depreciation", 9000, 7000},
			},
			Points: []factoryAlert{
				{"Customer receivables follow-up", "alerte", "", 110000, "Receivables equal roughly 76 days of revenue; ageing should be reviewed before closing."},
				{"Demo dataset", "info", "", 0, "All amounts are fictional and only demonstrate the report format."},
			},
			Sig: map[string]float64{
				"margeBruteN": 449000, "margeBruteN1": 321000,
				"valeurAjouteeN": 339000, "valeurAjouteeN1": 231000,
				"ebeN": 81000, "ebeN1": 32000,
				"reN": 72000, "reN1": 25000,
				"rcaiN": 72000, "rcaiN1": 25000,
				"rnN": 72000, "rnN1": 25000,
			},
			MonthlyProducts: []float64{110000, 128000, 140000, 147000},
			MonthlyCharges: []factoryMonthlyCharge{
				{"Hosting and tools", []float64{17000, 19000, 20000, 20000}},
				{"External services", []float64{26000, 27000, 28000, 29000}},
				{"Personnel costs", []float64{55000, 60000, 62000, 63000}},
				{"Taxes and duties", []float64{4000, 4000, 5000, 5000}},
				{"Depreciation", []float64{2000, 2000, 2500, 2500}},
			},
			Actif: []factoryBalanceRow{
				{"Cash", 85000},
				{"Customer receivables", 110000},
				{"Software assets", 65000},
				{"Other assets", 35000},
			},
			Passif: []factoryBalanceRow{
				{"Equity and reserves", 90000},
				{"Current-year profit", 72000},
				{"Supplier payables", 65000},
				{"Tax and social payables", 68000},
			},
			Ratios: []factoryRatio{
				{"Revenue growth", "+40.8 %"},
				{"Net margin", "13.7 %"},
				{"Cash available", "85 k"},
				{"Indicative DSO", "76 days"},
			},
			DSOAlert:            "Receivables are the first review topic in this SaaS demo.",
			ScoreNarrative:      "The demo SaaS file is mostly readable, with receivables kept as the main review point.",
			PNLNarrative:        "Revenue growth is driven by subscriptions, while personnel costs remain the largest cost base.",
			SIGNarrative:        "Value added and EBITDA both improve year on year in this scenario.",
			MonthlyNarrative:    "Profitability becomes steadier over the second half as recurring revenue grows.",
			StructureNarrative:  "The structure is balanced for a demo file; receivables should be aged in a real review.",
			BridgeNarrative:     "The bridge shows the conversion from recurring revenue to operating profit.",
			ComparisonNarrative: "Revenue grows faster than the main cost lines.",
			FiscalNarrative:     "Tax figures are placeholders for demonstration only.",
			SummaryNarrative:    "Demo ready for versioning, review, and PDF export.",
			ReviewPoint:         "Customer receivables",
		},
		"restaurant": {
			Kind:             "restaurant",
			PackageName:      "numeral-restaurant-reporting",
			Client:           "Atelier Midi SARL",
			Title:            "Restaurant financial review",
			Period:           "FY 2025 - demo data",
			Source:           "Fictional restaurant demo dataset.",
			Subtitle:         "Sales, food margin, staffing, and occupancy costs for a restaurant.",
			Footnote:         "Demo-only amounts. Replace with actual sales journals, ledgers, and bank files for client work.",
			Score:            78,
			ScoreLevel:       "acceptable",
			ScoreLabel:       "Acceptable",
			Traitement:       84,
			NonTraite:        9,
			Ajustement:       18,
			MontantTraite:    "84 %",
			MontantNonTraite: "9 %",
			MontantAjuste:    "18 %",
			Products: []factoryLine{
				{"On-site sales", 610000, 520000},
				{"Delivery sales", 145000, 125000},
				{"Private events", 95000, 75000},
			},
			Charges: []factoryLine{
				{"Food and beverage purchases", 285000, 250000},
				{"Personnel costs", 310000, 275000},
				{"Rent and occupancy", 84000, 78000},
				{"External services", 76000, 62000},
				{"Taxes and duties", 21000, 17000},
				{"Depreciation", 14000, 8000},
			},
			Points: []factoryAlert{
				{"Food margin pressure", "alerte", "", 285000, "Purchases represent 33.5 % of sales; supplier invoices and inventory should be reviewed."},
				{"Cash close routine", "info", "", 0, "Cash and card settlement controls would be expected in a real restaurant file."},
			},
			Sig: map[string]float64{
				"margeBruteN": 565000, "margeBruteN1": 470000,
				"valeurAjouteeN": 405000, "valeurAjouteeN1": 330000,
				"ebeN": 74000, "ebeN1": 38000,
				"reN": 60000, "reN1": 30000,
				"rcaiN": 60000, "rcaiN1": 30000,
				"rnN": 60000, "rnN1": 30000,
			},
			MonthlyProducts: []float64{178000, 210000, 226000, 236000},
			MonthlyCharges: []factoryMonthlyCharge{
				{"Food and beverage purchases", []float64{60000, 70000, 76000, 79000}},
				{"Personnel costs", []float64{74000, 76000, 79000, 81000}},
				{"Rent and occupancy", []float64{21000, 21000, 21000, 21000}},
				{"External services", []float64{17000, 22000, 25000, 12000}},
				{"Taxes and duties", []float64{5000, 5000, 5500, 5500}},
				{"Depreciation", []float64{3000, 3000, 3500, 4500}},
			},
			Actif: []factoryBalanceRow{
				{"Cash", 52000},
				{"Card settlement receivables", 33000},
				{"Inventory", 41000},
				{"Kitchen equipment", 189000},
			},
			Passif: []factoryBalanceRow{
				{"Equity and reserves", 105000},
				{"Current-year profit", 60000},
				{"Supplier payables", 86000},
				{"Tax and social payables", 64000},
			},
			Ratios: []factoryRatio{
				{"Food purchase ratio", "33.5 %"},
				{"Net margin", "7.1 %"},
				{"Cash available", "52 k"},
				{"Delivery share", "17.1 %"},
			},
			DSOAlert:            "Delivery platform settlements should be reconciled with bank receipts.",
			ScoreNarrative:      "The restaurant demo is readable, with attention on food margin and payment settlement controls.",
			PNLNarrative:        "Sales progress across on-site, delivery, and private events. Purchases and staffing drive most costs.",
			SIGNarrative:        "The gross margin covers occupancy and staffing, leaving a positive operating result.",
			MonthlyNarrative:    "The second half carries stronger sales while costs remain broadly controlled.",
			StructureNarrative:  "Equipment and supplier balances are the largest balance-sheet topics in this example.",
			BridgeNarrative:     "The margin bridge highlights food costs, payroll, rent, and external services.",
			ComparisonNarrative: "Revenue growth is supported by private events and delivery volume.",
			FiscalNarrative:     "Tax lines are illustrative and require source journals in a real file.",
			SummaryNarrative:    "Restaurant demo ready for review, versioning, and PDF export.",
			ReviewPoint:         "Food margin and settlements",
		},
		"cabinet-client": {
			Kind:             "cabinet-client",
			PackageName:      "numeral-cabinet-client-reporting",
			Client:           "Client Cabinet SAS",
			Title:            "Cabinet client review",
			Period:           "FY 2025 - demo data",
			Source:           "Fictional cabinet-client demo dataset.",
			Subtitle:         "Accounting review with suspense items, tax checks, and closing points.",
			Footnote:         "Demo-only amounts. For a real cabinet file, every amount must be tied to source accounting evidence.",
			Score:            61,
			ScoreLevel:       "acceptable",
			ScoreLabel:       "Acceptable",
			Traitement:       72,
			NonTraite:        18,
			Ajustement:       24,
			MontantTraite:    "72 %",
			MontantNonTraite: "18 %",
			MontantAjuste:    "24 %",
			Products: []factoryLine{
				{"Sales of goods", 720000, 680000},
				{"Services", 220000, 190000},
				{"Grants and other income", 40000, 25000},
			},
			Charges: []factoryLine{
				{"Purchases", 310000, 295000},
				{"External services", 245000, 220000},
				{"Personnel costs", 320000, 300000},
				{"Taxes and duties", 35000, 30000},
				{"Depreciation", 30000, 25000},
			},
			Blocking: []factoryAlert{
				{"Bank suspense pending", "erreur", "Bank suspense", 12500, "Several bank movements still need invoices or counterparty confirmation before closing."},
			},
			BlockingTotal: 12500,
			Points: []factoryAlert{
				{"VAT balance to reconcile", "alerte", "VAT", 8400, "VAT accounts should be reconciled against declarations before final close."},
				{"Shareholder current account", "info", "Current account", 0, "Movements should be supported by agreements or expense evidence where relevant."},
			},
			Sig: map[string]float64{
				"margeBruteN": 670000, "margeBruteN1": 600000,
				"valeurAjouteeN": 425000, "valeurAjouteeN1": 380000,
				"ebeN": 70000, "ebeN1": 50000,
				"reN": 40000, "reN1": 25000,
				"rcaiN": 40000, "rcaiN1": 25000,
				"rnN": 40000, "rnN1": 25000,
			},
			MonthlyProducts: []float64{225000, 240000, 255000, 260000},
			MonthlyCharges: []factoryMonthlyCharge{
				{"Purchases", []float64{76000, 78000, 78000, 78000}},
				{"External services", []float64{59000, 61000, 62000, 63000}},
				{"Personnel costs", []float64{78000, 80000, 81000, 81000}},
				{"Taxes and duties", []float64{8000, 8500, 9000, 9500}},
				{"Depreciation", []float64{7000, 7500, 7500, 8000}},
			},
			Actif: []factoryBalanceRow{
				{"Cash", 120000},
				{"Customer receivables", 180000},
				{"Inventory", 90000},
				{"Fixed assets", 120000},
			},
			Passif: []factoryBalanceRow{
				{"Equity and reserves", 160000},
				{"Current-year profit", 40000},
				{"Supplier payables", 155000},
				{"Tax and social payables", 155000},
			},
			Ratios: []factoryRatio{
				{"Revenue growth", "+9.5 %"},
				{"Net margin", "4.1 %"},
				{"Open suspense", "12.5 k"},
				{"VAT review", "Required"},
			},
			DSOAlert:            "Receivables and bank suspense should be reviewed before client delivery.",
			ScoreNarrative:      "The file is usable for review but still has closing points before it can be treated as final.",
			PNLNarrative:        "The result remains positive, but open suspense and VAT checks keep the file below a high-confidence level.",
			SIGNarrative:        "Operating profit is positive after personnel costs and depreciation.",
			MonthlyNarrative:    "Quarterly results stay positive, with moderate growth through the year.",
			StructureNarrative:  "Receivables, supplier balances, and tax liabilities are the main balance-sheet review areas.",
			BridgeNarrative:     "The bridge separates trading margin, external costs, payroll, and closing adjustments.",
			ComparisonNarrative: "The year improves slightly against the prior period, but not enough to ignore closing controls.",
			FiscalNarrative:     "VAT and income tax positions are placeholders until declarations and ledgers are reconciled.",
			SummaryNarrative:    "Cabinet-client demo ready for review workflows and versioning.",
			ReviewPoint:         "Suspense and VAT",
		},
	}
}

func factoryKindNames() []string {
	return []string{"demo-saas", "restaurant", "cabinet-client"}
}

func writeFactoryProject(root string, profile factoryProfile) error {
	if err := os.WriteFile(filepath.Join(root, "reports", "v0", "model.ts"), []byte(renderFactoryModel(profile)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "reports", "v0", "notes.md"), []byte(renderFactoryNotes(profile)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "reports", "v0", "reporting.json"), []byte(renderFactoryReporting(profile)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "reports", "v0", "evidence.json"), []byte(renderFactoryEvidence(profile)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(renderFactoryReadme(profile)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(renderFactoryMemory(profile)), 0o644); err != nil {
		return err
	}
	if err := ensureDemoDataDir(root); err != nil {
		return err
	}
	if err := updatePackageName(filepath.Join(root, "package.json"), profile.PackageName); err != nil {
		return err
	}
	if err := updatePackageName(filepath.Join(root, "package-lock.json"), profile.PackageName); err != nil {
		return err
	}
	return preferMJSNextConfig(root)
}

func ensureDemoDataDir(root string) error {
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, ".gitkeep"), []byte(""), 0o644)
}

func updatePackageName(path string, name string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	doc["name"] = name
	if packages, ok := doc["packages"].(map[string]any); ok {
		if rootPkg, ok := packages[""].(map[string]any); ok {
			rootPkg["name"] = name
		}
	}
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	return os.WriteFile(path, buf, 0o644)
}

func preferMJSNextConfig(root string) error {
	_ = os.Remove(filepath.Join(root, "next.config.ts"))
	const cfg = `/** @type {import("next").NextConfig} */
const nextConfig = {};

export default nextConfig;
`
	return os.WriteFile(filepath.Join(root, "next.config.mjs"), []byte(cfg), 0o644)
}

func renderFactoryModel(p factoryProfile) string {
	productTotalN, productTotalN1 := sumLines(p.Products)
	chargeTotalN, chargeTotalN1 := sumLines(p.Charges)
	resultN := productTotalN - chargeTotalN
	resultN1 := productTotalN1 - chargeTotalN1
	monthlyChargeTotals := sumMonthlyCharges(p.MonthlyCharges)
	monthlyResult := diffCells(p.MonthlyProducts, monthlyChargeTotals)
	actifTotal := sumBalance(p.Actif)
	passifTotal := sumBalance(p.Passif)

	return fmt.Sprintf(`import { defineReportModel } from "@/schemas/report";

/**
 * v0 - %s factory report.
 * Fictional amounts for demo/report scaffolding only.
 */
export const model = defineReportModel({
  meta: {
    title: %s,
    client: %s,
    period: %s,
    year: 2025,
    priorYear: 2024,
    generatedAt: "2026-05-15T12:00:00+02:00",
    currentPeriodLabel: "2025",
    priorPeriodLabel: "2024",
    periodDescription: %s,
    source: %s,
  },
  pnl: {
    subtitle: %s,
    footnote: %s,
    score: {
      global: %d,
      level: %s,
      levelLabel: %s,
      traitement: %d,
      nonTraite: %d,
      ajustement: %d,
      montantTraite: %s,
      montantNonTraite: %s,
      montantAjuste: %s,
    },
    produits: [
%s
    ],
    charges: [
%s
    ],
    totals: {
      produitsN: %s,
      produitsN1: %s,
      chargesN: %s,
      chargesN1: %s,
      resultatExploitationN: %s,
      resultatExploitationN1: %s,
      resultatNetN: %s,
      resultatNetN1: %s,
    },
  },
  alerts: {
    blocking: [
%s
    ],
    blockingTotal: %s,
    points: [
%s
    ],
  },
  sig: {
%s
  },
  monthly: {
    headers: ["Q1", "Q2", "Q3", "Q4"],
    produits: {
      cells: %s,
      total: %s,
    },
    charges: [
%s
    ],
    chargesTotal: {
      cells: %s,
      total: %s,
    },
    result: {
      cells: %s,
      total: %s,
    },
  },
  structure: {
    balance: {
      actif: [
%s
      ],
      passif: [
%s
      ],
      totalActif: %s,
      totalPassif: %s,
    },
    ratios: [
%s
    ],
    dsoAlert: %s,
    charts: {
      comparison: true,
      donut: {
        categories: [
%s
        ],
      },
    },
  },
  analyse: {
    scoring: {
      global: %d,
      identite: %d,
      coherence: %d,
      recurrence: %d,
      montant: %d,
    },
    penalties: [
      {
        label: %s,
        weight: %d,
        reason: %s,
      },
    ],
    fiscalite: [
      {
        label: "Income tax",
        status: "Demo",
        comment: "Illustrative only; source ledgers are required for a real tax position.",
      },
      {
        label: "VAT",
        status: "Demo",
        comment: "VAT controls require declarations and account reconciliation in a real file.",
      },
    ],
    narratives: {
      score: %s,
      pnl: %s,
      sig: %s,
      monthly: %s,
      structure: %s,
      bridge: %s,
      comparison: %s,
      fiscalite: %s,
      synthese: %s,
    },
    synthese: [
      { label: "Revenue", value: %s },
      { label: "Net result", value: %s, accent: "positive" },
      { label: "Review point", comment: %s },
      { label: "Usage", comment: "Factory demo only" },
    ],
    footerExtra: [
      "Factory-generated Numeral report - fictional amounts, no accounting or tax value.",
    ],
  },
});
`,
		p.Kind,
		q(p.Title),
		q(p.Client),
		q(p.Period),
		q("Factory-generated report. Amounts are fictional and should be replaced before client use."),
		q(p.Source),
		q(p.Subtitle),
		q(p.Footnote),
		p.Score,
		q(p.ScoreLevel),
		q(p.ScoreLabel),
		p.Traitement,
		p.NonTraite,
		p.Ajustement,
		q(p.MontantTraite),
		q(p.MontantNonTraite),
		q(p.MontantAjuste),
		renderLines(p.Products),
		renderLines(p.Charges),
		num(productTotalN),
		num(productTotalN1),
		num(chargeTotalN),
		num(chargeTotalN1),
		num(resultN),
		num(resultN1),
		num(resultN),
		num(resultN1),
		renderAlerts(p.Blocking),
		num(p.BlockingTotal),
		renderAlerts(p.Points),
		renderSig(p.Sig),
		numArray(p.MonthlyProducts),
		num(sum(p.MonthlyProducts)),
		renderMonthlyCharges(p.MonthlyCharges),
		numArray(monthlyChargeTotals),
		num(sum(monthlyChargeTotals)),
		numArray(monthlyResult),
		num(sum(monthlyResult)),
		renderBalanceRows(p.Actif),
		renderBalanceRows(p.Passif),
		num(actifTotal),
		num(passifTotal),
		renderRatios(p.Ratios),
		q(p.DSOAlert),
		renderDonut(p.Charges),
		p.Score,
		min(100, p.Score+6),
		p.Score,
		max(0, p.Score-5),
		max(0, p.Score-10),
		q(p.ReviewPoint),
		max(4, 100-p.Score),
		q("Main review point for this factory kind."),
		q(p.ScoreNarrative),
		q(p.PNLNarrative),
		q(p.SIGNarrative),
		q(p.MonthlyNarrative),
		q(p.StructureNarrative),
		q(p.BridgeNarrative),
		q(p.ComparisonNarrative),
		q(p.FiscalNarrative),
		q(p.SummaryNarrative),
		q(fmt.Sprintf("%s k", formatK(productTotalN))),
		q(fmt.Sprintf("%s k", formatK(resultN))),
		q(p.ReviewPoint),
	)
}

func renderFactoryNotes(p factoryProfile) string {
	return fmt.Sprintf(`# %s Factory Notes

This v0 report was generated with:

    numeral-reporting create <dir> --kind %s

The amounts are fictional and should be replaced before any real client use.

Agent checklist:

- Keep amounts fictional in demo mode.
- For a client report, switch reporting.json to client mode.
- Add evidence.json entries for every non-null amount.
- Run: numeral-reporting doctor --project . --version v0 --strict.
`, p.Kind, p.Kind)
}

func renderFactoryReadme(p factoryProfile) string {
	return fmt.Sprintf(`# %s

Factory-generated Numeral reporting project.

Kind: %s

## Run

    npm install
    npm run dev

Then open:

    http://localhost:8080/v0

## Edit

- Report data: reports/v0/model.ts
- Page order: reports/v0/report.tsx
- Version metadata: reports/meta.json

All amounts are fictional demo data.

## Guardrails

Run the reporting doctor before treating the report as done:

    numeral-reporting doctor --project . --version v0 --strict
`, p.Client, p.Kind)
}

func renderFactoryMemory(p factoryProfile) string {
	return fmt.Sprintf(`# MEMORY - %s

## Scope

Factory-generated demo report.

## Rules

- Treat all amounts as fictional.
- Do not copy client files into data/.
- Replace reports/v0/model.ts with source-backed amounts before real delivery.
`, p.Client)
}

func renderFactoryReporting(p factoryProfile) string {
	doc := map[string]any{
		"kind":             p.Kind,
		"mode":             "demo",
		"requiresEvidence": false,
	}
	buf, _ := json.MarshalIndent(doc, "", "  ")
	return string(append(buf, '\n'))
}

func renderFactoryEvidence(p factoryProfile) string {
	file := evidenceFile{
		Version: "v0",
		Items:   factoryEvidenceRows(p),
	}
	buf, _ := json.MarshalIndent(file, "", "  ")
	return string(append(buf, '\n'))
}

func factoryEvidenceRows(p factoryProfile) []evidenceRow {
	source := fmt.Sprintf("Factory dataset: %s", p.Kind)
	var rows []evidenceRow
	add := func(path string, value float64, formula string) {
		rows = append(rows, evidenceRow{
			Path:    path,
			Value:   value,
			Source:  source,
			Formula: formula,
			Note:    "Fictional demo amount.",
		})
	}

	productTotalN, productTotalN1 := sumLines(p.Products)
	chargeTotalN, chargeTotalN1 := sumLines(p.Charges)
	resultN := productTotalN - chargeTotalN
	resultN1 := productTotalN1 - chargeTotalN1
	monthlyChargeTotals := sumMonthlyCharges(p.MonthlyCharges)
	monthlyResult := diffCells(p.MonthlyProducts, monthlyChargeTotals)

	for i, line := range p.Products {
		add(fmt.Sprintf("pnl.produits.%d.n", i), line.N, "factory product line N")
		add(fmt.Sprintf("pnl.produits.%d.n1", i), line.N1, "factory product line N-1")
	}
	for i, line := range p.Charges {
		add(fmt.Sprintf("pnl.charges.%d.n", i), line.N, "factory charge line N")
		add(fmt.Sprintf("pnl.charges.%d.n1", i), line.N1, "factory charge line N-1")
	}
	add("pnl.totals.produitsN", productTotalN, "sum(pnl.produits.*.n)")
	add("pnl.totals.produitsN1", productTotalN1, "sum(pnl.produits.*.n1)")
	add("pnl.totals.chargesN", chargeTotalN, "sum(pnl.charges.*.n)")
	add("pnl.totals.chargesN1", chargeTotalN1, "sum(pnl.charges.*.n1)")
	add("pnl.totals.resultatExploitationN", resultN, "produitsN - chargesN")
	add("pnl.totals.resultatExploitationN1", resultN1, "produitsN1 - chargesN1")
	add("pnl.totals.resultatNetN", resultN, "produitsN - chargesN")
	add("pnl.totals.resultatNetN1", resultN1, "produitsN1 - chargesN1")

	for i, alert := range p.Blocking {
		if alert.Amount != 0 {
			add(fmt.Sprintf("alerts.blocking.%d.amount", i), alert.Amount, "factory blocking alert")
		}
	}
	if p.BlockingTotal != 0 {
		add("alerts.blockingTotal", p.BlockingTotal, "sum(alerts.blocking.*.amount)")
	}
	for i, alert := range p.Points {
		if alert.Amount != 0 {
			add(fmt.Sprintf("alerts.points.%d.amount", i), alert.Amount, "factory attention point")
		}
	}

	for _, key := range sigEvidenceKeys() {
		add(fmt.Sprintf("sig.%s", key), p.Sig[key], "factory SIG amount")
	}

	for i, value := range p.MonthlyProducts {
		add(fmt.Sprintf("monthly.produits.cells.%d", i), value, "factory monthly product")
	}
	add("monthly.produits.total", sum(p.MonthlyProducts), "sum(monthly.produits.cells)")
	for i, charge := range p.MonthlyCharges {
		for j, value := range charge.Cells {
			add(fmt.Sprintf("monthly.charges.%d.cells.%d", i, j), value, "factory monthly charge")
		}
		add(fmt.Sprintf("monthly.charges.%d.total", i), sum(charge.Cells), "sum(monthly.charges row cells)")
	}
	for i, value := range monthlyChargeTotals {
		add(fmt.Sprintf("monthly.chargesTotal.cells.%d", i), value, "sum(monthly.charges.*.cells column)")
	}
	add("monthly.chargesTotal.total", sum(monthlyChargeTotals), "sum(monthly.chargesTotal.cells)")
	for i, value := range monthlyResult {
		add(fmt.Sprintf("monthly.result.cells.%d", i), value, "monthly.produits - monthly.chargesTotal")
	}
	add("monthly.result.total", sum(monthlyResult), "sum(monthly.result.cells)")

	for i, row := range p.Actif {
		add(fmt.Sprintf("structure.balance.actif.%d.amount", i), row.Amount, "factory balance asset")
	}
	for i, row := range p.Passif {
		add(fmt.Sprintf("structure.balance.passif.%d.amount", i), row.Amount, "factory balance liability")
	}
	add("structure.balance.totalActif", sumBalance(p.Actif), "sum(structure.balance.actif.*.amount)")
	add("structure.balance.totalPassif", sumBalance(p.Passif), "sum(structure.balance.passif.*.amount)")
	for i, line := range p.Charges {
		add(fmt.Sprintf("structure.charts.donut.categories.%d.value", i), line.N, "factory charge distribution")
	}

	return rows
}

func sigEvidenceKeys() []string {
	return []string{
		"margeBruteN", "margeBruteN1",
		"valeurAjouteeN", "valeurAjouteeN1",
		"ebeN", "ebeN1",
		"reN", "reN1",
		"rcaiN", "rcaiN1",
		"rnN", "rnN1",
	}
}

func renderLines(lines []factoryLine) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "      { label: %s, n: %s, n1: %s },\n", q(l.Label), num(l.N), num(l.N1))
	}
	return b.String()
}

func renderAlerts(alerts []factoryAlert) string {
	var b strings.Builder
	for _, a := range alerts {
		fmt.Fprintf(&b, "      { label: %s, severity: %s", q(a.Label), q(a.Level))
		if a.Account != "" {
			fmt.Fprintf(&b, ", account: %s", q(a.Account))
		}
		if a.Amount != 0 {
			fmt.Fprintf(&b, ", amount: %s", num(a.Amount))
		}
		fmt.Fprintf(&b, ", comment: %s },\n", q(a.Comment))
	}
	return b.String()
}

func renderSig(sig map[string]float64) string {
	keys := []string{
		"margeBruteN", "margeBruteN1",
		"valeurAjouteeN", "valeurAjouteeN1",
		"ebeN", "ebeN1",
		"reN", "reN1",
		"rcaiN", "rcaiN1",
		"rnN", "rnN1",
	}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "    %s: %s,\n", key, num(sig[key]))
	}
	return b.String()
}

func renderMonthlyCharges(charges []factoryMonthlyCharge) string {
	var b strings.Builder
	for _, c := range charges {
		fmt.Fprintf(&b, "      { label: %s, cells: %s, total: %s },\n", q(c.Label), numArray(c.Cells), num(sum(c.Cells)))
	}
	return b.String()
}

func renderBalanceRows(rows []factoryBalanceRow) string {
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, "        { label: %s, amount: %s },\n", q(row.Label), num(row.Amount))
	}
	return b.String()
}

func renderRatios(ratios []factoryRatio) string {
	var b strings.Builder
	for _, ratio := range ratios {
		fmt.Fprintf(&b, "      { label: %s, value: %s },\n", q(ratio.Label), q(ratio.Value))
	}
	return b.String()
}

func renderDonut(lines []factoryLine) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "          { label: %s, value: %s },\n", q(l.Label), num(l.N))
	}
	return b.String()
}

func q(s string) string {
	return strconv.Quote(s)
}

func num(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func numArray(values []float64) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = num(v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func sumLines(lines []factoryLine) (float64, float64) {
	var n, n1 float64
	for _, l := range lines {
		n += l.N
		n1 += l.N1
	}
	return n, n1
}

func sum(values []float64) float64 {
	var total float64
	for _, v := range values {
		total += v
	}
	return total
}

func sumMonthlyCharges(charges []factoryMonthlyCharge) []float64 {
	if len(charges) == 0 {
		return nil
	}
	out := make([]float64, len(charges[0].Cells))
	for _, charge := range charges {
		for i, cell := range charge.Cells {
			out[i] += cell
		}
	}
	return out
}

func diffCells(left []float64, right []float64) []float64 {
	out := make([]float64, len(left))
	for i := range left {
		var rv float64
		if i < len(right) {
			rv = right[i]
		}
		out[i] = left[i] - rv
	}
	return out
}

func sumBalance(rows []factoryBalanceRow) float64 {
	var total float64
	for _, row := range rows {
		total += row.Amount
	}
	return total
}

func formatK(v float64) string {
	return strconv.FormatFloat(v/1000, 'f', 0, 64)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
