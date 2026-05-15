# numeral-reporting-cli agent guide

This directory owns the Numeral reporting CLI and the bundled agent skill.

When an agent needs to generate, edit, validate, or review a Numeral report, load:

```text
skills/numeral-reporting-agent/SKILL.md
```

## Fast path

From the repository root:

```bash
./numeral-reporting-cli/numeral-reporting create <target-dir> --kind demo-saas
./numeral-reporting-cli/numeral-reporting doctor --project <target-dir> --version v0 --strict
./numeral-reporting-cli/numeral-reporting render --project <target-dir> --version v0
```

From this directory:

```bash
./numeral-reporting create <target-dir> --kind demo-saas
./numeral-reporting doctor --project <target-dir> --version v0 --strict
./numeral-reporting render --project <target-dir> --version v0
```

Available factory kinds:

- `demo-saas`
- `restaurant`
- `cabinet-client`

## Guardrails

- Use the CLI factory before hand-building a report.
- Run `doctor --strict` before saying a report is ready.
- For generated static projects, also run `render`.
- Use `app --project <dir>` to inspect versions locally.
- Preserve the original report template look: static HTML must keep using the embedded `report.css` and `numeral_shell.css` design system.
- Keep demo reports fictional and visibly marked as demo data.
- For real client reports, do not invent values. Every non-null financial amount should be backed by `versions/vN/evidence.json`.
- Do not ship visible text that mentions internal implementation details such as `repo`, `script`, `model.ts`, `JSON`, `doctor`, or `skill`.
- Keep changes small, readable, and easy to review.
