---
name: numeral-reporting-categorize
description: Use when assigning a Plan Comptable Général account and a CR/SIG bucket to a transaction, écriture FEC, or bank line, including libellé heuristics and disambiguation rules.
---

# Catégoriser les flux

Use this skill any time you face a raw line (FEC, relevé bancaire, export Pennylane) and must decide which PCG account it hits and where it lands in the income statement.

## PCG → poste CR (charges, classe 6)

| Compte | Libellé PCG | Poste CR |
| --- | --- | --- |
| 601 / 602 | Achats matières premières / autres approvisionnements | Achats consommés |
| 6063 | Fournitures non stockables | Achats consommés |
| 607 | Achats de marchandises | Achats marchandises |
| 6037 | Variation de stocks marchandises | Variation stocks |
| 611 | Sous-traitance générale | Services extérieurs |
| 613 | Locations (loyers) | Services extérieurs |
| 615 | Entretien et réparations | Services extérieurs |
| 616 | Primes d'assurances | Services extérieurs |
| 618 | Documentation, abonnements | Services extérieurs |
| 622 | Honoraires (expert-comptable, avocat) | Autres services extérieurs |
| 623 | Publicité, marketing | Autres services extérieurs |
| 624 | Transports | Autres services extérieurs |
| 625 | Déplacements, missions, réceptions | Autres services extérieurs |
| 626 | Télécoms, frais postaux | Autres services extérieurs |
| 627 | Services bancaires (commissions, frais Stripe) | Autres services extérieurs |
| 631 / 633 | Impôts et taxes sur rémunérations | Impôts et taxes |
| 635 | Autres impôts (CFE, CVAE, taxe foncière) | Impôts et taxes |
| 641 | Rémunérations du personnel (brut) | Salaires et traitements |
| 644 | Rémunération du dirigeant TNS | Salaires et traitements |
| 645 | Charges de sécurité sociale et prévoyance | Charges sociales |
| 647 | Autres charges sociales (mutuelle, tickets resto) | Charges sociales |
| 651 | Redevances pour concessions, licences logicielles | Autres charges de gestion |
| 654 | Pertes sur créances irrécouvrables | Autres charges de gestion |
| 661 | Charges d'intérêts (emprunts) | Charges financières |
| 666 | Pertes de change | Charges financières |
| 671 | Charges exceptionnelles sur opérations de gestion | Charges exceptionnelles |
| 675 | Valeur comptable d'immobilisations cédées | Charges exceptionnelles |
| 681 | Dotations aux amortissements et dépréciations | Dotations |
| 686 | Dotations financières | Dotations |
| 6951 | Impôt sur les bénéfices | Impôt sur bénéfices |

## PCG → poste CR (produits, classe 7)

| Compte | Libellé PCG | Poste CR |
| --- | --- | --- |
| 701 | Ventes de produits finis | Production vendue |
| 706 | Prestations de services | Production vendue |
| 707 | Ventes de marchandises | Ventes marchandises |
| 708 | Produits des activités annexes | Production vendue |
| 7085 | Ports et frais accessoires facturés | Production vendue |
| 709 | Rabais, remises, ristournes accordés (—) | Production vendue (—) |
| 713 | Variation de stocks de produits | Production stockée |
| 72 | Production immobilisée | Production immobilisée |
| 74 | Subventions d'exploitation | Subventions |
| 75 | Autres produits de gestion courante | Autres produits |
| 76 | Produits financiers | Produits financiers |
| 77 | Produits exceptionnels | Produits exceptionnels |
| 78 | Reprises sur amortissements et provisions | Reprises |

## Heuristiques libellé bancaire

Quand seul le relevé bancaire est disponible (pas de FEC) :

| Pattern libellé | Compte probable | À vérifier |
| --- | --- | --- |
| `URSSAF`, `URS COTIS` | 431 → 645 | Recoupement avec DSN |
| `DGFIP`, `IMPOT SOC` | 695 ou 444 | IS vs acompte vs régul |
| `DGFIP` + `TVA` | 44551 | Montant CA3 |
| `EDF`, `ENGIE`, `TOTAL ENERGIES` | 6061 | — |
| `ORANGE`, `SFR`, `BOUYGUES`, `FREE PRO` | 626 | — |
| `OVH`, `AWS`, `GOOGLE CLOUD`, `VERCEL` | 651 ou 626 | Hébergement = 626, licence = 651 |
| `STRIPE`, `SUMUP`, `MOLLIE` | 411 (clients) + 627 (commissions) | Séparer le brut versé et la commission |
| `LOYER`, `SCI`, `BAIL` | 613 | — |
| `AMAZON BUSINESS` | 606 ou 607 | Fournitures vs revente |
| `LINKEDIN`, `GOOGLE ADS`, `META ADS` | 623 | — |
| Virement reçu d'un IBAN client connu | 411 (encaissement) → solde 706/707 | Recoupement facture |
| Récurrence mensuelle même montant | Probable abonnement (651 ou 626) | Confirmer sur 3+ mois |

## Règles de désambiguïsation

- **Recurring same amount, same counterparty, 3+ mois** → abonnement, catégoriser identique aux occurrences précédentes.
- **Montant rond, libellé flou** → ne pas catégoriser, mettre en suspens.
- **Multi-comptes possibles** (ex. Amazon = fournitures bureau ou marchandises revendues) → trancher avec l'activité (`meta.json` `business_type`), sinon poser la question dans les alertes.
- **TVA visible** dans le libellé ou le montant → ventiler HT/TVA selon le taux (20/10/5,5/2,1) et le régime du client.
- **Virement intra-comptes** (compte courant → livret pro) → 58 (virements internes), ne jamais en CR.

## Sortie attendue

Pour chaque ligne traitée, écrire dans `report.json` (et `evidence.json` côté client) :

```json
{
  "account": "626",
  "label": "OVH — hébergement infra",
  "amount_ht": 89.00,
  "amount_tva": 17.80,
  "amount_ttc": 106.80,
  "tva_rate": 0.20,
  "period_month": "2026-02",
  "source_ref": "FEC#1842"
}
```

Toute ligne qui ne rentre dans aucune règle → alerte type « écriture en suspens », pas d'invention.
