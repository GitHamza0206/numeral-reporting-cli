---
name: numeral-reporting-consolidate
description: Use when merging historical periods and current-period data into a single report.json, deciding whether to clone from a frozen version, handling re-imports without duplicating, and tracking provisional state.
---

# Consolidate historical + current period

Use this skill when you receive new data (FEC partiel, CSV banque, export Pennylane, écritures manuelles) and the project already has one or more versions.

## Decide the target version

| Situation | Action |
| --- | --- |
| Période clôturée, brouillon en cours | Travailler dans la `vN` non gelée la plus récente |
| Période clôturée, dernière `vN` gelée | `numeral-reporting new --from <N>` puis travailler dans la nouvelle |
| Nouvelle période (ex. nouveau trimestre) | `numeral-reporting new --from <N>` et changer `report.period` |
| Première saisie | `versions/v0/report.json` (déjà créé par `create`) |

Ne jamais éditer une version gelée. Ne jamais écraser `v0` après livraison.

## Merge order

1. **Historique d'abord** — recopier les soldes et lignes annuelles des `vN` antérieures, en lecture seule, pour ancrer les comparatifs (N-1, N-2).
2. **Période en cours ensuite** — appliquer les nouvelles écritures par mois croissant.
3. **Ajustements en dernier** — OD de clôture, cut-off, provisions (voir `business-rules.md`).

## Idempotence (re-import sans doublon)

Si la même source peut être ré-importée (un FEC qui s'étoffe au fil du mois) :

- Tenir un index dans `evidence.json` : chaque opération a un `evidence[i].source_ref` unique (n° de pièce, ligne du FEC, hash `date|montant|libellé`).
- Avant d'ajouter une ligne, vérifier que `source_ref` n'existe pas déjà.
- Si une ligne existe avec le même `source_ref` mais un montant différent → la source a été corrigée : remplacer, pas dupliquer, et noter le delta dans les alertes.

## Marquer provisional

Tant que la période n'est pas clôturée :

```json
{
  "mode": "provisional",
  "requiresEvidence": true,
  "period": { "from": "2026-01-01", "to": "2026-03-31", "status": "open" }
}
```

Ajouter une alerte d'information listant ce qui manque pour clôturer (écritures de paie du dernier mois, factures fournisseurs non reçues, OD de TVA, etc.).

## Quand finaliser

Une fois la période bouclée :

1. `numeral-reporting doctor --version vN --strict` doit passer.
2. Passer `mode: "client"` (ou laisser `provisional` si c'est volontaire) et `period.status: "closed"`.
3. `numeral-reporting freeze N`.

## Garde-fous

- Si le total des produits ou charges varie de plus de 20 % par rapport à la `vN-1` non gelée précédente sans raison documentée → s'arrêter et lever une alerte avant de continuer.
- Si une écriture du FEC n'a pas pu être catégorisée (voir `categorize.md`) → la mettre en alerte « en suspens », ne pas l'inventer.
- Si une période antérieure gelée semble fausse → ne pas la modifier. Créer une `vN` d'ajustement avec une OD de correction documentée.
