---
name: numeral-reporting-entity-resolution
description: Use when interacting with the project entity table (entities.json), interpreting Resolve match kinds, or deciding when to merge / split / rename / force-match entities to improve identity sub-score.
---

# Résolution d'entité

Use this skill any time you need to look at how a libellé was clustered into
an entity, fix a clustering mistake, or stabilize identity for upcoming
periods.

## Why this matters

L'identité est **40 % du score transaction**. Si l'entity resolution dérive,
le score chute même quand la catégorisation est juste. Un seul faux merge ou
un libellé non reconnu sur un gros montant suffit à faire tomber le global
sous 85 %.

## Where data lives

```text
<project>/entities.json          # store partagé, racine du projet
<project>/versions/vN/transactions.json
    .transactions[].entity_id     # rempli par le CLI à chaque `score`
    .transactions[].match_kind    # exact|iban|siret|fuzzy|manual|none
    .transactions[].match_confidence
```

L'agent ne touche **jamais** `entity_id` / `match_confidence` à la main. Le
CLI les recalcule à chaque appel à `numeral-reporting score`.

## Resolve priority pipeline (rappel)

| Priorité | Source | Confidence | Quand |
| --- | --- | --- | --- |
| 1 | Manual override | 1.00 | Force-match déclaré dans entities.json |
| 2 | Exact normalized | 1.00 | Libellé normalisé = une `normalized_key` exacte |
| 3 | IBAN | 0.98 | IBAN détecté dans le libellé brut + présent dans une entity |
| 4 | SIRET | 0.97 | 14 chiffres + match SIRET |
| 5 | Fuzzy | 0.85+ | Damerau-Levenshtein ≥ 0.85 (tie-break ID le plus petit) |
| — | None | 0.00 | Aucun match → ligne suspect, contribue à `low_identity` |

## Diagnose a low score

```bash
numeral-reporting score --project <dir> --version vN --json | jq '.top_risks'
```

Si un risk `low_identity` ou `unrecognized_amount` apparaît avec un montant
significatif :

1. Lister les transactions qui en sont à l'origine (`tx_ids`).
2. Pour chaque tx : récupérer `libelle_raw` et `libelle_norm` depuis
   `versions/vN/transactions.json`.
3. Décider :
   - Le libellé est nouveau → créer une entité (via les opérations CLI).
   - Le libellé correspond à une entité existante mal nommée → `merge`.
   - Deux libellés ont été collés ensemble à tort → `split`.

## Commands

```bash
numeral-reporting entities list   [--kind KIND] [--json]
numeral-reporting entities show   <id>
numeral-reporting entities merge  <src_id> <dst_id>
numeral-reporting entities split  <id> <new_canonical_name> --keys k1,k2,...
numeral-reporting entities rename <id> <new_canonical_name>
numeral-reporting entities reset  --yes
```

`merge` et `split` laissent un `manual_overrides` daté dans l'entity touchée
— ne pas l'éditer à la main, le CLI s'en sert pour figer les décisions
manuelles entre runs.

## Quand merger

- Deux entités ont des canonical_names variantes du même fournisseur
  (« OVH » vs « OVH SAS »).
- L'une accumule des keys clairement liées à l'autre.
- Les IBAN/SIRET sont identiques.

```bash
numeral-reporting entities merge ent_xxx ent_yyy
# yyy absorbe xxx ; les keys, IBANs, aliases sont unionés et triés.
```

## Quand splitter

- Une entité a accumulé des keys appartenant en réalité à deux fournisseurs.
- Une normalization a écrasé une distinction utile (libellés trop similaires
  mais entreprises différentes).

```bash
numeral-reporting entities split ent_xxx "Vendor B" --keys "vendor b services,vendor b sas"
```

## Quand renommer

- Le `canonical_name` est faux ou peu lisible (a été dérivé d'un libellé
  bancaire bruité). L'ID ne change **jamais** (sinon l'historique des
  transactions casse).

## Création d'entité

Aujourd'hui : pas de commande `entities create`. Pour créer :

1. Lancer `score --write` une première fois → toutes les lignes nouvelles
   tombent en `match_kind: "none"`.
2. Identifier les libellés `none` aux montants significatifs (top risks).
3. Éditer `entities.json` à la main pour ajouter l'entity (champs requis :
   `id`, `canonical_name`, `kind`, `normalized_keys`). L'`id` doit être
   stable : `entities.NewEntityID(canonical_name)` produit la forme
   recommandée `ent_<sha256[:12]>`.
4. Re-lancer `score` → les transactions concernées passent en
   `match_kind: "exact"` avec confidence 1.0.

> Une commande `entities create` est sur la roadmap. En attendant, l'édition
> manuelle convient pour un MVP.

## Force-match / force-unmatch (overrides)

Cas : un libellé que la normalisation ne parvient pas à rattacher (caractères
exotiques, format atypique). Ajouter dans `entities.json` :

```json
"manual_overrides": [
  { "kind": "force_match",
    "source": "libelle_normalise_exact",
    "target": "ent_xxx",
    "note": "Variant non détecté par fuzzy",
    "date": "2026-05-15T00:00:00Z" }
]
```

`source` est la forme **normalisée** (telle qu'elle apparaît dans
`libelle_norm` du transactions.json), pas le brut.

`force_unmatch` fait l'inverse : empêche un libellé donné d'être matché à
une entité existante (utile quand un fuzzy attrape un faux-positif).

## Garde-fous

- Ne pas merger des entités de `kind` différents (un client et un fournisseur
  ne devraient jamais fusionner).
- Ne pas splitter sur < 2 keys (perte d'info, pas de gain).
- Après chaque édition manuelle, re-lancer `score` pour vérifier que l'effet
  attendu sur le top risks est bien obtenu.
- Toujours commiter `entities.json` dans le repo du projet — c'est le
  contrat de stabilité avec les versions futures.

## Lien avec les autres skills

- [[categorize]] décide du compte PCG et de la catégorie — l'entity ne dit
  rien sur le compte, juste sur l'identité du flux.
- [[scoring]] explique comment le score `identité` est calculé à partir du
  `match_confidence` produit ici.
- [[consolidate]] s'appuie sur l'entity_id pour ne pas dupliquer des
  écritures d'une période à l'autre.
