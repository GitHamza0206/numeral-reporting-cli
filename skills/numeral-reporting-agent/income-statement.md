---
name: numeral-reporting-income-statement
description: Use to assemble a coherent compte de résultat (P&L) from categorized entries, in the structure expected by the static renderer, and to drive doctor --strict to green.
---

# Générer un compte de résultat cohérent

Use this skill once entries are categorized (`categorize.md`), business rules applied (`business-rules.md`), and inferences validated (`safe-inference.md`). The goal: a CR that survives `doctor --strict`.

## Structure du CR (modèle PCG)

```
Produits d'exploitation
  + Ventes de marchandises                       (707)
  + Production vendue (biens + services)         (701, 706, 708)
  + Production stockée                           (713)
  + Production immobilisée                       (72)
  + Subventions d'exploitation                   (74)
  + Reprises sur amortissements et provisions    (78)
  + Autres produits                              (75)
= Total produits d'exploitation                  [A]

Charges d'exploitation
  + Achats de marchandises ± variation stocks    (607 ± 6037)
  + Achats matières et autres ± variation        (601, 602 ± 6031, 6032)
  + Autres achats et charges externes            (606, 61, 62)
  + Impôts, taxes et versements assimilés        (63)
  + Salaires et traitements                      (641, 644)
  + Charges sociales                             (645, 647)
  + Dotations aux amortissements                 (6811)
  + Dotations aux provisions                     (6815, 6817)
  + Autres charges                               (65)
= Total charges d'exploitation                   [B]

Résultat d'exploitation                          [A − B] = REX

+ Produits financiers                            (76)
− Charges financières                            (66)
= Résultat financier                             [F]

Résultat courant avant impôts                    [REX + F] = RCAI

+ Produits exceptionnels                         (77)
− Charges exceptionnelles                        (67)
= Résultat exceptionnel                          [E]

− Participation des salariés                     (691)
− Impôt sur les bénéfices                        (695)

Résultat net                                     [RCAI + E − participation − IS]
```

## SIG (soldes intermédiaires de gestion)

Si le renderer attend les SIG (regarder `report.json` schema) :

| Solde | Calcul |
| --- | --- |
| Marge commerciale | Ventes marchandises − Coût d'achat marchandises vendues |
| Production de l'exercice | Production vendue + stockée + immobilisée |
| Valeur ajoutée | Marge + Production − (achats consommés + services extérieurs) |
| EBE (Excédent brut d'exploitation) | VA + Subventions − Impôts/taxes − Charges de personnel |
| REX | EBE + Reprises + Autres produits − Dotations − Autres charges |
| RCAI | REX + Résultat financier |
| Résultat net | RCAI + Exceptionnel − Participation − IS |

## Cohérence à garantir (doctor)

Avant de considérer le CR comme prêt, ces invariants doivent tenir :

1. **Totaux par bloc** : `total_produits_exploitation` = Σ lignes ; idem charges, financier, exceptionnel.
2. **Cascade des soldes** : REX = produits expl − charges expl, exactement. Pas d'arrondi qui dérive.
3. **Résultat net** = somme des blocs intermédiaires.
4. **Mensuel = annuel** : Σ(12 mois) = total annuel par poste, à l'unité monétaire près (0 si possible, ≤ 1 € sinon, jamais ≥ 1 €).
5. **Bilan équilibré** (si le rapport inclut un bilan) : Actif = Passif. Résultat net du CR = résultat net du bilan.
6. **Alertes bloquantes** : Σ des montants des alertes `kind: erreur` = champ `blocking_total` affiché.

## Workflow d'exécution

```bash
# 1. Construire / mettre à jour le CR
#    → édition de versions/vN/report.json à partir des entrées catégorisées

# 2. Vérifier la cohérence
./numeral-reporting doctor --project <dir> --version vN --strict --json

# 3. Si échec : lire le JSON d'erreurs, corriger, relancer.
#    Ne JAMAIS bidouiller un total pour faire passer le doctor.
#    Si un total ne tombe pas, c'est une ligne qui est fausse — chercher où.

# 4. Rendre le visuel
./numeral-reporting render --project <dir> --version vN

# 5. Optionnel : revue visuelle
./numeral-reporting app --project <dir>
```

## Pièges fréquents

- **Arrondis** : faire les calculs sur les centimes, arrondir une seule fois au moment de l'affichage. Ne pas arrondir poste par poste puis sommer.
- **Signe des stocks** : la variation de stocks de marchandises est en **charges** (`6037`) avec le signe `+achat − consommation`. Production stockée (`713`) est en **produits** avec signe inverse. Erreur classique : inversion de signe → REX faussé.
- **Dotations vs reprises** : dotations en charges, reprises en produits. Une provision qui « se libère » bascule de 6817 à 7817.
- **IS payé vs IS de l'exercice** : 695 = IS rattaché à l'exercice (charge). Les acomptes payés sont au bilan (444). Confondre les deux casse le résultat net.
- **Cut-off oublié** : un loyer trimestriel payé en janvier qui couvre déc-jan-fév doit avoir 2/3 en CCA à la clôture déc. Sinon les charges du nouvel exercice sont gonflées.

## Sortie : critère de fin

Le CR est prêt quand :

- [ ] `doctor --strict` retourne 0.
- [ ] Aucune alerte `erreur` avec montant attendu n'est ouverte.
- [ ] Le résultat net correspond à la cascade des soldes.
- [ ] Le mensuel cumulé = l'annuel sur chaque poste.
- [ ] `render` produit un HTML sans `null` visible ni texte gabarit (`{{...}}`).
- [ ] `evidence.json` couvre 100 % des montants non nuls en mode `client`.

Tant qu'une case manque → ne pas livrer.
