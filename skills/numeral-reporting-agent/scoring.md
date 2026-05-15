---
name: numeral-reporting-scoring
description: Use when reading the reliability score, deciding whether a report is shippable, or remediating a low score by acting on transactions, entities, or adjustments.
---

# Scoring de fiabilité

Use this skill once a report has been assembled and you want to know whether
it's safe to ship — or what to fix if it isn't.

## What the score answers

> *"À quel point ce compte de résultat colle à la réalité économique ?"*

C'est **pas** un score de qualité comptable, ni de complétude documentaire.
C'est une mesure de **risque de divergence** par rapport à la réalité.

## Architecture (rappel)

Trois blocs **exclusifs en euros** :

| Bloc | Question | Pondération du global |
| --- | --- | --- |
| traité | Mes classifications sont-elles justes ? | × `Amount_traité` |
| non_traité | Quelle part du P&L est dans le noir ? | × `Amount_non_traité` |
| ajusté | Mes auto-écritures (CCA/FNP/social/amort) tiennent-elles ? | × `Amount_ajusté` |

```
Score_global = (M_t·S_t + M_nt·S_nt + M_aj·S_aj) / (M_t + M_nt + M_aj)
```

Pondération **par euros, jamais par lignes**. 2 000 transactions à 20 € de
total qui restent non traitées pèsent moins qu'une seule à 1 500 €.

## How to run it

```bash
numeral-reporting score --project <dir> --version vN              # affichage humain
numeral-reporting score --project <dir> --version vN --json       # machine-readable
numeral-reporting score --project <dir> --version vN --write      # persiste dans report.json + transactions.json
numeral-reporting score --project <dir> --version vN --score-threshold 85
```

`--write` doit être lancé **au moins une fois** avant la livraison : c'est
ce qui remplit `report.Score` (lu par le renderer et par `doctor`).

## Reading the output

```
score v0 — Total P&L 9670 €  (materiality 100 €)
  traité       8450 €  90.0 %
  non traité   1070 €  91.1 %
  ajusté        150 €  76.0 %
  global              88 %  (envoyable)

top risks
  1.  URSSAF cotisations Q3       2400 € (24.8 %)
  2.  Stripe payouts non rapprochés 800 € (8.3 %)
```

| Bucket global | Action |
| --- | --- |
| ≥ 90 % | Très fiable — publication immédiate |
| 85–90 % | **Envoyable** au client (seuil recommandé) |
| 80–85 % | Acceptable avec revue rapide |
| 70–80 % | Fragile — revue obligatoire |
| < 70 % | Non fiable — ne pas publier |

## Score transaction (bloc traité)

Pondération **fixe** des sous-scores :

```
score_tx = 0.40·identité + 0.30·cohérence + 0.20·récurrence + 0.10·montant
```

| Sous-score | Vient de | Comment l'améliorer |
| --- | --- | --- |
| identité (40 %) | `match_confidence` du resolver | Voir [[entity-resolution]]. Créer ou merger l'entity manquante. |
| cohérence (30 %) | Account PCG vs catégorie | Re-vérifier le `account` et la `category` dans transactions.json. Plafonnée à 0.5 si charge/immo ambigu. |
| récurrence (20 %) | Présence de l'entité dans versions gelées | S'améliore automatiquement à chaque période. Pour boost immédiat : geler la version courante. |
| montant (10 %) | Z-score vs historique de l'entité | Vérifier que le montant n'est pas aberrant ; si oui justifier ou corriger. |

## Score non_traité

```
Score_non_traité = clamp01(1 - Σ(|amt| × coef_sensibilité) / Total_PnL)
```

Coefficients (`internal/scoring/sensitivity.go`) :

| Catégorie | Coefficient |
| --- | --- |
| ca | 1.5 |
| salaires | 1.3 |
| loyer | 1.2 |
| achats | 1.0 |
| divers / vide | 0.8 |

**Pour améliorer** : classifier les transactions `status: non_traite` qui
ont un gros montant ou une catégorie sensible. Une transaction de CA non
catégorisée fait beaucoup plus mal qu'un divers du même montant.

## Score ajusté

```
score_aj = 0.45·pattern_historique + 0.35·signal_actuel + 0.20·cohérence_métier
```

- `pattern_historique` : présence de l'entité dans les versions gelées
  antérieures. 1.0 si vu dans ≥ moitié des versions, interpolation linéaire
  sinon. 0.0 sans historique.
- `signal_actuel` : **fourni par l'agent** dans `transactions.json` sous
  `adjustment.signal_actuel` (0..1). Default 0.5 si absent.
- `cohérence_métier` : match entre `adjustment.reason` et `account` PCG :

| reason | accounts attendus |
| --- | --- |
| fnp | 408, 60, 61, 62 |
| cca | 486 |
| fae | 418 |
| pca | 487 |
| amortissement | 6811, 281, 28 |
| is, impot | 695, 444 |
| tva | 445 |
| provision | 6815, 6817, 151, 491, 29, 39 |
| social, salaires | 641, 644, 645, 647 |
| reclassement, autre | n'importe (→ 0.5) |

**Iron rule** : un ajustement ne doit jamais être créé sur la seule base
de l'historique. Toujours un signal actuel. Si pas assez sûr → ne pas le
créer (voir [[safe-inference]]).

## Top risks

5 entrées max, groupées par `(kind, entity_id)`, triées par `impact_pct`
décroissant. Catégories :

| Kind | Sens |
| --- | --- |
| low_identity | Libellé non reconnu (identité < 0.5) |
| category_mismatch | Compte ou catégorie ambigus (cohérence < 0.5) |
| low_score_treated | Score transaction global < 0.85 |
| unrecognized_amount | Statut `non_traite` |
| adjustment_weak | Score ajustement < 0.5 |

L'agent doit **toujours** lire le top risks avant de déclarer un rapport
prêt. Chaque entrée = un poste à corriger.

## Garde-fous opérationnels

- Le scoring est **déterministe** : mêmes inputs → même score. Si ça
  bouge sans changement de données → bug à signaler.
- Les coefficients (0.40 / 0.30 / 0.20 / 0.10 pour le tx, sensibilités,
  seuils) sont **figés en code** (`internal/scoring/sensitivity.go`).
  Modifier un coefficient = bump `SchemaVersion`.
- Le scoring est **conservateur** : en cas de doute, baisse plutôt que
  hausse. Ne jamais hacker un montant pour faire monter le score.
- Le seuil de matérialité (max(100€, 0.05% PnL)) exclut les peanuts des
  top risks mais **les compte toujours** dans les totaux.

## Workflow de remédiation

```
score (read) → top risks
  ├─ low_identity / unrecognized_amount  → [[entity-resolution]]
  ├─ category_mismatch                   → [[categorize]] + corriger account
  ├─ low_score_treated                   → revoir la classification
  └─ adjustment_weak                     → [[business-rules]] + relever signal_actuel
  ↓
score --write
  ↓
doctor --strict --score-threshold 85
  ↓
si tout passe → render → livraison
```

## Lien avec les autres skills

- [[entity-resolution]] gère l'identité (40 % du score tx).
- [[categorize]] gère le compte PCG (alimente cohérence + sensitivity).
- [[business-rules]] décrit comment construire un ajustement défendable.
- [[safe-inference]] décrit quand un ajustement ou un fill est légitime.
- [[consolidate]] décrit comment garder l'historique propre pour que
  recurrence + amount-coherence remontent.
