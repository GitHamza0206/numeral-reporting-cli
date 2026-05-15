---
name: numeral-reporting-agent
description: Use when generating, editing, validating, or reviewing Numeral static reporting projects with numeral-reporting-cli, including versioned report.json files, evidence.json, doctor checks, local app preview, and agent-safe financial report workflows.
---

# Numeral Reporting Agent

Use this skill to create high-quality Numeral reports without guessing, leaking internal tooling, or shipping inconsistent numbers.

## Sub-skills (load when relevant)

These files extend this skill. Each is self-contained — load whichever matches the current step:

| Sub-skill | Use when |
| --- | --- |
| [`consolidate.md`](./consolidate.md) | Merging historical periods + current-period data into one `report.json`; deciding whether to clone a frozen version; handling re-imports; marking provisional state. |
| [`categorize.md`](./categorize.md) | Assigning a PCG account and a CR bucket to a raw FEC line, bank libellé, or Pennylane export entry. |
| [`business-rules.md`](./business-rules.md) | Computing TVA, charges sociales, amortissement, cut-off (FNP/CCA/FAE/PCA), provisions, and IS. |
| [`safe-inference.md`](./safe-inference.md) | Deciding whether a missing element can be inferred deterministically, or must be left as an alert. |
| [`income-statement.md`](./income-statement.md) | Assembling a coherent CR/SIG and driving `doctor --strict` to green. |
| [`entity-resolution.md`](./entity-resolution.md) | Interacting with the `entities.json` store; interpreting Resolve match kinds; merging / splitting / renaming entities to keep the identity sub-score stable. |
| [`scoring.md`](./scoring.md) | Reading the reliability score; deciding ship vs. revise; remediating low scores using the top-risks list. |

## End-to-end pipeline

When the user gives raw data and asks for a report:

1. **Consolidate** historical + new data → `consolidate.md`
2. **Categorize** each flow into a PCG account → `categorize.md`
3. **Apply business rules** (TVA, charges, amortissement, cut-off, IS) → `business-rules.md`
4. **Fill gaps only if safely inferable**, otherwise raise alerts → `safe-inference.md`
5. **Assemble the CR**, pass `doctor --strict`, render → `income-statement.md`
6. **Compute reliability score**, fix top risks until ≥ 85 % → `scoring.md` (uses `entity-resolution.md` and steps 1–5)

The first five steps are skill-driven (markdown, Claude's judgment). Step 6 is deterministic — the CLI does the math via `numeral-reporting score`. Use the skill to interpret the result and decide what to remediate.

Never skip a step. Never write a number you can't trace back to a source or a documented inference. Never ship without a reliability score ≥ 85 %.

## Default Workflow

1. Start from the CLI factory whenever possible.

```bash
./numeral-reporting-cli/numeral-reporting create <target-dir> --kind demo-saas
./numeral-reporting-cli/numeral-reporting create <target-dir> --kind restaurant
./numeral-reporting-cli/numeral-reporting create <target-dir> --kind cabinet-client
```

From inside `numeral-reporting-cli`, use:

```bash
./numeral-reporting create <target-dir> --kind demo-saas
```

2. Edit only version data unless the renderer itself needs a product change.

Expected files live under:

```text
versions/v0/report.json
versions/v0/evidence.json
versions/v0/notes.md
meta.json
```

For a new iteration, create a version instead of overwriting the baseline:

```bash
./numeral-reporting-cli/numeral-reporting new --project <target-dir> --from v0 --name margin-review
```

3. Run the doctor before delivery.

```bash
./numeral-reporting-cli/numeral-reporting doctor --project <target-dir> --version v0 --strict
```

4. Render the report.

```bash
./numeral-reporting-cli/numeral-reporting render --project <target-dir> --version v0
```

5. Use the local app when visual review or version navigation matters.

```bash
./numeral-reporting-cli/numeral-reporting app --project <target-dir>
```

Treat a strict doctor failure as blocking. Fix the report before presenting it as done.

## Template Rule

The static output must look like the original Numeral reporting app. Preserve the embedded `report.css` and `numeral_shell.css` design system, the top version navbar, and the page structure: cover, sommaire, scores, alerts, P&L, SIG, monthly, structure, and analyse.

## Report Modes

Demo reports must be clearly fictional:

- `report.json` should use `mode: "demo"` and `requiresEvidence: false`.
- Visible labels, footnotes, or notes should make it clear the data is demo-only.
- Do not mix real client names or real source claims into demo reports.

Client reports must be source-backed:

- `report.json` should use `mode: "client"` and `requiresEvidence: true`.
- Every non-null financial amount in `report.json` needs a matching `evidence.json` item.
- Evidence should include the model path, value, source, and formula or note when helpful.
- If a number cannot be sourced, leave it out or mark it as a review point. Do not invent it.

## Quality Rules

Check these before delivery:

- P&L totals must equal their line items.
- Monthly totals must equal their visible monthly lines.
- Balance sheet assets and liabilities must balance.
- Blocking alert totals must match the blocking rows.
- Visible report text must not mention internal implementation terms like `script`, `repo`, `model.ts`, `JSON`, `skill`, or `doctor`.
- The report should read like a client-facing financial review, not a technical artifact.

## Legacy Next.js

The CLI still supports the older Next.js template through:

```bash
./numeral-reporting-cli/numeral-reporting create <target-dir> --kind demo-saas --mode next --template numeral-reporting
```

Use the static path by default unless the user explicitly asks for the old Next.js app.

## Style

Keep the implementation boring and readable. Prefer the smallest useful change, concrete file paths, and a short delivery note with the commands that passed.
