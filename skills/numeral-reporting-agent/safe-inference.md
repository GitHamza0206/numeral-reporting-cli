---
name: numeral-reporting-safe-inference
description: Use to decide whether a missing financial element can be filled in by inference or must be left as a gap/alert. Defines what is safe to infer (deterministic from known data) versus what is not.
---

# Compléter uniquement si fiable

Use this skill before writing any amount that does not come directly from a source document. Inference is allowed only when the result is deterministic from data already in the report or from a published reference rate.

## Iron rule

> Si tu ne peux pas dire **d'où vient le chiffre** en une phrase et **comment le re-vérifier**, ne l'écris pas.

Chaque inférence doit produire une entrée `evidence.json` :

```json
{
  "path": "structure.charges.salaires_charges",
  "value": 18420.00,
  "source": "inferred",
  "formula": "brut 43 000 × 0,4284 (taux moyen patronal 2025)",
  "inputs": ["evidence#1281 (brut)", "ref#urssaf-2025-q1"]
}
```

Pas de formule → pas d'inférence.

## Safe — peut être inféré

| Cas | Entrée | Formule | Pourquoi sûr |
| --- | --- | --- | --- |
| TVA collectée | CA HT + taux | `HT × taux` | Déterministe |
| TVA déductible sur achat | Montant HT + taux | `HT × taux` | Déterministe |
| Charges patronales | Brut connu | `brut × ~0,4284` (régime général, 2025) | Taux publié URSSAF |
| Charges TNS | Rémunération | `rému × ~0,45` | Taux moyen URSSAF |
| Cotisation retraite cadres (tranche B) | Brut > PMSS | barème AGIRC-ARRCO | Barème publié |
| Amortissement linéaire | Immobilisation en classe 2 | `(HT − résiduel) / durée` | Déterministe, durée du tableau d'amortissement |
| Cut-off prorata simple | Facture chevauchant 2 exercices | `montant × jours_exercice / jours_total` | Déterministe |
| Conversion devise | Montant en devise + date | Taux BCE de la date | Référence publique |
| Charge récurrente identique sur ≥ 6 mois | Historique loyer/abonnement | Reconduire le même montant | Régularité documentée |
| Total à partir des lignes | Lignes du même bloc | Somme arithmétique | Pure arithmétique |
| Variation N vs N-1 | Deux montants déjà écrits | `(N − N-1) / N-1` | Pure arithmétique |

## Not safe — ne pas inférer

| Cas | Pourquoi |
| --- | --- |
| Affecter un libellé bancaire flou à un compte précis | Ambiguïté multi-comptes possibles |
| Estimer un CA d'une période non clôturée par extrapolation | Aucune garantie de linéarité |
| Provision pour risque sans risque identifié et chiffrable | Trois critères requis : obligation, probabilité, montant |
| Inventer une facture fournisseur depuis un virement seul | Pièce manquante |
| Solde stocks sans inventaire physique | Données source manquantes |
| Reconstituer une rémunération nette → brut sans bulletin | Charges variables (mutuelle, prévoyance, ticket resto, abattements) |
| Charges sociales TNS d'une période en cours non encore appelée | Les appels URSSAF arrivent en décalé, montant pas définitif |
| Récurrence < 3 occurrences | Une coïncidence n'est pas une règle |
| Tendance saisonnière non documentée | Hypothèse non vérifiable |
| Tout chiffre dont la formule reposerait sur une « moyenne du marché » non sourcée | Pas de référence reproductible |

## Comportement par défaut quand fiable ≠ disponible

1. **Ne pas écrire le chiffre** dans `report.json` (laisser `null` ou retirer la ligne).
2. **Créer une alerte** dans `report.alerts` :

```json
{
  "label": "Charges sociales mars 2026 non disponibles",
  "kind": "erreur",
  "amount": null,
  "comment": "Appel URSSAF non reçu — chiffre laissé vide, à compléter avant clôture."
}
```

3. **Mentionner dans `notes.md`** pourquoi le poste est vide.

`doctor --strict` doit échouer tant qu'une alerte « erreur » avec montant attendu reste ouverte.

## Marquage dans evidence.json

Trois sources possibles, à choisir explicitement :

| `source` | Sens |
| --- | --- |
| `document` | Pièce reçue (facture, FEC, relevé) |
| `inferred` | Calcul déterministe à partir d'autres `evidence` |
| `external_rate` | Taux ou barème publié (URSSAF, BCE, DGFiP) — préciser la version |

Toute valeur `inferred` doit citer en `inputs` les `evidence` qui la nourrissent. Si une de ces sources change → l'inférence doit être recalculée.

## Garde-fou de cohérence

Si une inférence produit un écart > 5 % par rapport :
- au même poste de la `vN-1`, **et**
- à la même période de l'exercice précédent,

alors **ne pas l'écrire seule** : la traiter comme suspecte, lever une alerte et demander confirmation avant de continuer.
