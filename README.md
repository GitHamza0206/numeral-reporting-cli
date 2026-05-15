# numeral-reporting-cli

Go CLI that scaffolds and manages Numeral reporting projects. The default path
is now a static local app: no Next.js, no npm install, and report versions live
as JSON under `versions/vN/`. The static renderer embeds the same report
HTML/CSS design system as the original Next.js reporting app.

## Install

```bash
go install ./cmd/numeral-reporting
# or build a static binary
go build -o numeral-reporting ./cmd/numeral-reporting
```

`create` does not need the Next.js template unless you explicitly pass
`--mode next`.

## Commands

| Command | What it does |
| --- | --- |
| `init <dir> [--template PATH]` | Legacy Next.js init: copy the template to a fresh directory and create `reports/v0/` + `meta.json` |
| `create <dir> --kind KIND [--mode static\|next]` | Create a fictional factory report. Defaults to the static app layout |
| `doctor [--version vN] [--strict] [--json]` | Run integrity, evidence, and visible-text checks before delivery |
| `render [--version vN] [--out DIR]` | Render a static report to `dist/vN/index.html` |
| `app [--addr 127.0.0.1:8787]` | Serve the local static app with version navigation |
| `list [--json]` | Show tip, active, and the version table |
| `new [--from N] [--name NOTE]` | Clone vN (default: tip) into the next slot and bump tip + active |
| `freeze <N>` | Mark vN immutable |
| `delete <N>` | Remove vN (cannot delete v0 or a frozen version) |
| `activate <N>` | Set `active_version` |
| `refresh` | Legacy Next.js: rewrite `reports/registry.ts` from the directory contents |
| `export <N> <out.pdf> [--url URL]` | Export a version to PDF via headless Chrome |
| `score [--version vN] [--json] [--write] [--score-threshold N]` | Compute the deterministic reliability score (3 blocks weighted by €) and top risks |
| `entities <list\|show\|merge\|split\|rename\|reset>` | Manage the project-wide entity store (`entities.json`) |

All commands accept `--project DIR` to operate on a project other than
the current directory.

## Report factory

Use `create` when you want a ready-to-open demo report instead of a blank
template:

```bash
numeral-reporting create demo-saas-report --kind demo-saas
numeral-reporting create restaurant-report --kind restaurant
numeral-reporting create cabinet-client-report --kind cabinet-client
```

The factory reports are fictional. They are useful for demos, QA, screenshots,
and PDF export checks. Replace `versions/v0/report.json` with source-backed
amounts before using a generated project for a real client.

Static projects look like this:

```text
my-report/
  meta.json
  versions/
    v0/
      report.json
      evidence.json
      notes.md
  exports/
```

## Reporting doctor

Run `doctor` before treating an agent-authored report as done:

```bash
numeral-reporting doctor --project my-report --version v0
numeral-reporting doctor --project my-report --version v0 --strict
numeral-reporting doctor --project my-report --version v0 --strict --json
```

The doctor checks totals, monthly/annual consistency, balance equality,
blocking alert totals, version metadata, evidence coverage for client-mode
reports, and visible text that leaks internal tooling language.

## Local app

Serve the app directly from the Go binary:

```bash
numeral-reporting app --project my-report
```

Open:

```text
http://127.0.0.1:8787
```

The app lists versions, previews reports, creates new versions, freezes
versions, and changes the active version.

## Agent bundle

This CLI ships with the skills Claude Code needs to actually drive it.
The intelligence lives in the markdown — the binary stays a thin set of
primitives.

- `AGENTS.md` — entry point for coding agents.
- `skills/numeral-reporting-agent/SKILL.md` — main skill, indexes the
  five sub-skills below and lays out the 5-step pipeline.
- `skills/numeral-reporting-agent/consolidate.md` — merge historical
  versions and the current-period data into one `report.json`, handle
  re-imports without duplicates, mark provisional periods.
- `skills/numeral-reporting-agent/categorize.md` — PCG account mapping
  and libellé heuristics for FEC, CSV bancaire, Pennylane exports.
- `skills/numeral-reporting-agent/business-rules.md` — TVA, charges
  sociales, amortissement, cut-off (FNP/CCA/FAE/PCA), provisions, IS.
- `skills/numeral-reporting-agent/safe-inference.md` — what can be
  inferred deterministically and what must stay as an alert.
- `skills/numeral-reporting-agent/income-statement.md` — assemble a
  coherent CR/SIG and drive `doctor --strict` to green.
- `skills/numeral-reporting-agent/entity-resolution.md` — interact with
  the entity store; merge / split / rename when needed.
- `skills/numeral-reporting-agent/scoring.md` — read the reliability
  score and remediate top risks.
- `skills/numeral-reporting-agent/agents/openai.yaml` — OpenAI skill
  metadata.

When sharing the CLI with an agent, export the whole `numeral-reporting-cli/`
directory so the binary, docs, and skills stay together.

## PDF export

Export shells out to a system Chrome/Chromium with `--headless=new
--print-to-pdf`. Set `CHROME_BIN` to override the binary.

```bash
numeral-reporting render --project my-report --version v1
numeral-reporting export 1 my-report/exports/v1.pdf --project my-report
```

For legacy Next.js projects, keep running the web app first and pass `--url`.

## Reliability score

The CLI ships a deterministic engine that scores how close the generated
P&L is to economic reality. The score breaks the period into three
mutually exclusive blocks weighted by €:

- **traité** — transactions the engine classified, each with sub-scores
  for identity (40 %), accounting coherence (30 %), recurrence (20 %)
  and amount coherence (10 %).
- **non_traité** — transactions left unclassified, weighted by a per-
  category sensitivity coefficient (CA × 1.5, salaires × 1.3, loyer ×
  1.2, achats × 1.0, divers × 0.8).
- **ajusté** — engine-added entries (CCA / FNP / FAE / PCA / social /
  amort), scored on historical pattern, current signal and PCG
  coherence.

```bash
numeral-reporting score --project my-report --version v0
numeral-reporting score --project my-report --version v0 --write
numeral-reporting score --project my-report --version v0 --score-threshold 85
```

`--write` persists block scores, top risks, and a percent global into
`report.json` (consumed by the renderer) and per-transaction sub-scores
into `versions/vN/transactions.json`. Threshold buckets:

| Global | Lecture |
| --- | --- |
| ≥ 90 % | Très fiable — publication immédiate |
| 85–90 % | Envoyable au client (seuil par défaut) |
| 80–85 % | Acceptable avec revue rapide |
| 70–80 % | Fragile |
| < 70 % | Non fiable |

The engine is pure (no I/O, no time inside the math) and entirely
deterministic: same inputs → same score, on any machine. Coefficients
live in `internal/scoring/sensitivity.go` — changing them requires
bumping the schema version.

## Entity store

Identity is 40 % of the per-transaction score, so the CLI ships a
deterministic entity resolver. Bank libellés are normalized (NFD strip
accents → ASCII lowercase → strip dates / refs / SEPA noise), then
matched in priority: manual override > exact normalized > IBAN > SIRET
> Damerau-Levenshtein fuzzy ≥ 0.85 (tie-break: smaller ID wins). The
store sits in `entities.json` at the project root and is shared across
versions.

```bash
numeral-reporting entities list   --project my-report
numeral-reporting entities show   ent_xxxxxxxxxxxx --project my-report
numeral-reporting entities merge  ent_src ent_dst --project my-report
numeral-reporting entities split  ent_xxx "Vendor B" --keys "vendor b sas" --project my-report
numeral-reporting entities rename ent_xxx "OVH Cloud" --project my-report
```

## Layout

```
cmd/numeral-reporting/main.go         flag parsing + subcommand dispatch
cmd/numeral-reporting/factory.go      report factory profiles
cmd/numeral-reporting/static.go       static project, doctor, render, app server
cmd/numeral-reporting/score.go        score + entities CLI glue
cmd/numeral-reporting/report.css      embedded original report design system
cmd/numeral-reporting/numeral_shell.css embedded original version navbar styles
internal/entities/normalize.go        deterministic libellé normalization
internal/entities/entities.go         Entity / Store / Override persistence
internal/entities/resolve.go          Resolve pipeline + Cluster + Damerau-Levenshtein
internal/scoring/sensitivity.go       coefficients, PCG table, materiality
internal/scoring/scoring.go           3-block reliability engine + top risks
internal/reports/reports.go           legacy Next.js version operations
internal/pdf/pdf.go                   headless-Chrome PDF wrapper
```
