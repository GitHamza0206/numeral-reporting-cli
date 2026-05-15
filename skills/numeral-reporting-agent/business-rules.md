---
name: numeral-reporting-business-rules
description: Use when applying TVA, charges sociales, amortissement, cut-off (FNP/CCA/FAE/PCA), provisions, and IS to a report. Use as the source of computational truth for French accounting rules.
---

# Règles métier

Use this skill any time you must compute a derived amount (TVA, charges patronales, amortissement, IS) or apply a closing rule (cut-off, provisions).

> Les taux et plafonds ci-dessous sont des ordres de grandeur 2025 — vérifier sur la situation réelle du client avant clôture.

## TVA

Comptes :

| Compte | Usage |
| --- | --- |
| 44566 | TVA déductible sur biens et services |
| 44562 | TVA déductible sur immobilisations |
| 44571 | TVA collectée |
| 44551 | TVA à décaisser |
| 44567 | Crédit de TVA |
| 44587 | Taxes sur le CA à régulariser |

Calcul mensuel/trimestriel :

```
TVA à décaisser = TVA collectée (44571) − TVA déductible biens/services (44566) − TVA déductible immo (44562)
```

Si négatif → crédit de TVA (44567).

Taux 2025 :

| Taux | Usage |
| --- | --- |
| 20 % | Normal |
| 10 % | Restauration sur place, transports voyageurs, travaux logements anciens |
| 5,5 % | Produits alimentaires de base, livres, énergies renouvelables, billetterie culturelle |
| 2,1 % | Médicaments remboursés, presse |

Régimes :

| Régime | Seuil CA HT | Déclaration |
| --- | --- | --- |
| Franchise en base | < 37 500 € (services) / 85 000 € (vente) | Aucune TVA facturée |
| Réel simplifié | ≤ 254 000 € (services) / 840 000 € (vente) | CA12 annuelle + 2 acomptes |
| Réel normal | au-delà | CA3 mensuelle (ou trimestrielle si TVA annuelle < 4 000 €) |

Intracommunautaire (B2B) : autoliquidation — pas de TVA facturée, mention « autoliquidation » sur facture, déclaration DEB/DES.

## Charges sociales

### Salarié (régime général)

À partir du **brut mensuel** :

| Bloc | Taux moyen | Compte |
| --- | --- | --- |
| Cotisations salariales (URSSAF + retraite + chômage) | ~22 % du brut (retenu sur fiche) | 421 / 431 / 437 |
| Cotisations patronales (URSSAF + retraite + chômage + AT) | ~42 % du brut | 645 |

Net ≈ brut × 0,78. Coût employeur ≈ brut × 1,42.

Allègement Fillon : forte réduction des cotisations patronales sur les salaires ≤ 1,6 SMIC (dégressive).

### Dirigeant TNS (gérant majoritaire SARL, EI, indépendant)

| Élément | Taux moyen |
| --- | --- |
| Cotisations sociales (URSSAF, retraite, prévoyance) | ~45 % de la rémunération nette |

Compte : 646.

### Comptabilisation des paies (modèle)

```
641 Salaires bruts             D
645 Charges patronales         D
   421 Personnel rémunérations dues   C (net à payer)
   431 URSSAF                          C
   437 Caisses retraite                C
   447 Impôt prélevé à la source       C
```

## Amortissement

Méthode linéaire :

```
dotation annuelle = (valeur HT − valeur résiduelle) / durée en années
```

Durées indicatives :

| Bien | Durée |
| --- | --- |
| Logiciel | 1–3 ans |
| Ordinateur, smartphone | 3 ans |
| Matériel de bureau | 5 ans |
| Mobilier | 10 ans |
| Agencements, installations | 5–10 ans |
| Véhicule | 5 ans (plafond fiscal sur VP : 18 300 € ou 9 900 €) |
| Bâtiment | 25–50 ans |

Écriture annuelle :

```
6811 Dotations aux amortissements         D
   281x Amortissements (par classe)       C
```

Au-delà d'une dotation, vérifier que l'immobilisation existe bien en classe 2 et que sa valeur résiduelle reste cohérente.

## Cut-off (rattachement à l'exercice)

Quatre cas à traiter à la clôture :

| Situation | Compte | Sens |
| --- | --- | --- |
| Charge engagée, facture non reçue | 408 Fournisseurs FNP | Crédit (passif) |
| Charge payée d'avance | 486 CCA | Débit (actif) |
| Produit acquis, facture non émise | 418 Clients FAE | Débit (actif) |
| Produit encaissé d'avance | 487 PCA | Crédit (passif) |

Règle : la charge / le produit doit être inscrit dans l'exercice où le service a été rendu / consommé, pas dans celui du flux de trésorerie.

À l'ouverture de l'exercice suivant : extourner ces écritures (contrepassation).

## Provisions

| Provision | Compte de dotation | Quand |
| --- | --- | --- |
| Dépréciation stocks | 6817 / 39 | Stocks invendables ou obsolètes |
| Créances douteuses | 6817 / 491 | Client en défaut > 6 mois ou procédure |
| Risques et charges | 6815 / 151 | Litige, garantie, restructuration |
| Dépréciation immo | 6816 / 29 | Indice de perte de valeur |

Provision = obligation actuelle + sortie de ressources probable + montant estimable. Si l'un manque → pas de provision (éventuellement engagement hors bilan).

## Impôt sur les sociétés (IS)

Taux 2025 :

| Tranche | Taux | Condition |
| --- | --- | --- |
| 0 – 42 500 € | 15 % | CA HT < 10 M€, capital entièrement libéré, détention 75 % personnes physiques |
| > 42 500 € | 25 % | — |

Calcul : sur le **résultat fiscal** (résultat comptable ± réintégrations / déductions). Ne pas confondre avec le résultat comptable brut.

Réintégrations classiques :
- Amortissement véhicule au-delà des plafonds fiscaux
- TVS (taxe sur véhicules de société)
- Amendes et pénalités
- Cadeaux > 73 € TTC / bénéficiaire / an
- Charges somptuaires (chasse, pêche, yachting, résidence de plaisance)

Acomptes IS : 15 mars, 15 juin, 15 septembre, 15 décembre. Solde au 15 mai N+1 (ou +4 mois après clôture).

Comptabilisation :

```
695 Impôt sur les bénéfices    D
   444 État - IS                C
```

## Garde-fous

Avant d'appliquer une règle, vérifier que les **éléments d'entrée existent vraiment** :
- Pas d'amortissement sans immobilisation source.
- Pas de provision sans risque documenté.
- Pas de cut-off sans pièce ou estimation justifiée.
- Pas d'IS sans résultat fiscal calculé.

Toute application d'une règle doit laisser une trace dans `evidence.json` (`formula` + `source`).
