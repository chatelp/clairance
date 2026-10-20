# Design — identité et système de l'outil

Dossier de la mission design (`MISSION.md`), ouvert le 12/10/2026. La branche `design` a été versée
dans `local` le 13/10 ; la session (« Clairance - Design ») travaille depuis Q56/Q57 sur la branche
`design-suite`. **La migration vers `www/` est terminée** : étapes 0 à 13 relues conformes le 15/10 et
versées en production (`04-migration.md`) ; ce dossier reste la référence des jetons, des composants
et des écrans (revue du 20/10).

| Fichier | Contenu | État |
|---|---|---|
| `MISSION.md` | la fiche de mission | — |
| `00-nom.md` | la recherche du nom en trois tours et la place du français (D74, Q53) | **Clairance**, décidé le 13/10 |
| `01-inventaire.md` | l'interface du 12/10, mesurée : tailles, couleurs, contrastes, états, icônes | fait |
| `01-inventaire-brut.md` | la sortie complète de `tools/design/inventory.mjs`, régénérable | fait |
| `02-principes.md` | dix principes, chacun justifié par un usage et une mesure | fait ; principe 8 tranché le 13/10 |
| `03-systeme.md` | le système : couleurs, typographie, espace, composants, mouvement, densité, accessibilité | fait ; quatre choix tranchés le 13/10, le cinquième (mouvement réduit) sans effet sur le dessin |
| `jetons.json` | les 125 jetons (couleur 74, police 15, taille 10, espace 8, z 7, rayon 4, mouvement 4, bordure 2, ombre 1), source unique des valeurs ; compté le 20/10, c'est aussi le total de `tools/design/tokens.mjs` | fait |
| `03-systeme-verif.md` | chaque contraste et chaque distance pour les daltoniens, produit par `tools/design/tokens.mjs` | régénérable |
| `maquettes/` | écrans en HTML statique sur les jetons (`index.html` pour commencer) | fait |
| `captures/` | l'interface « avant », 12/10 ; la PAR reprise le 13/10 (la première capture avait figé ses cadres à 480 px). **Hors dépôt** (D75 : la carte réelle avec ses anneaux situe la station mieux qu'au kilomètre) : sur le Mac, `runs/design/captures/`, non suivi ; les captures « après » de chaque étape y vont aussi | référence, privée |
| `04-migration.md` | le plan de migration vers les écrans validés : 14 étapes vérifiables, leurs écrans de référence et leurs mesures (≈ 17 j) | réécrit et validé le 13/10 ; **terminée le 15/10** |
| `05-identite.md`, `identite/` | la marque Clairance : signe, nom, favicon, barre de menus, connexion, README (`identite/index.html` pour la planche) | fait, nom décidé le 13/10 |
| `06-avions.md`, `avions/` | l'inventaire des avions dessinés dans l'outil, et notre famille de neuf silhouettes (`tools/design/avions.mjs`) | fait |
| `07-esquisses.md` | le fil au centre : A et sa colonne à deux onglets, B replié, suivre un vol ; d'où vient chaque donnée | esquisses validées le 13/10 (A, B replié, C) |
| `08-design-system.md`, `systeme/` | le design system Clairance : livre de marque, 95 jetons, 27 composants et 2 compositions en classes CSS `cl-`, le signe et les avions ; publié comme artefact, source dans `systeme/project/` | fait le 13/10 |
| `09-ecrans.md`, `ecrans/` | les quatorze écrans finaux, faits du seul design system, et où va chaque fonction d'aujourd'hui | faits et validés le 13/10 (six propositions retenues) |
| `connexion/` | trois propositions pour la page de connexion : A le scope, B un échange, C le ciel (`a-scope.html`, `b-echange.html`, `c-ciel.html`), produites par `tools/design/connexion.py` ; **C retenue** par le propriétaire le 13/10 ; décrites dans `09-ecrans.md` | fait |

Outils, dans `tools/design/` :

```bash
node tools/design/inventory.mjs > docs-fr/design/01-inventaire-brut.md   # l'inventaire
node tools/design/tokens.mjs                                              # vérifier les jetons, écrire tokens.css
node tools/design/ds-export.mjs                                           # les jetons du design system (systeme/project/)
python3 tools/design/ecrans.py                                            # les écrans finaux (ecrans/)
python3 tools/design/connexion.py                                         # les propositions de connexion (connexion/)
```

`par-mini.py` n'est pas un script à lancer : `ecrans.py` l'importe pour dessiner le cadre PAR statique des écrans et de la fiche `ParFrame`, avec la géométrie de `www/par/par-view.js`.

`contrast-probe.js` se colle dans la console d'une page (l'instance d'essai ou une maquette)
pour mesurer le texte rendu ; `color.mjs` porte l'arithmétique des couleurs commune aux deux
scripts.

Pour voir les maquettes avec leurs feuilles de style, servir ce dossier :

```bash
python3 -m http.server 8791 --bind 127.0.0.1 --directory docs-fr/design
```

puis ouvrir http://127.0.0.1:8791/maquettes/ (et arrêter le serveur ensuite).
