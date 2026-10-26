# Fiches pour les sessions cloud

*Ouvert le 10/10. Le propriétaire a un crédit réservé aux sessions cloud, qui expire le
5 novembre 2026 ; ces fiches lui envoient le travail du chantier de l'audit (doc 32) qui se
vérifie sans la station. Chaque fiche est autonome : une session cloud la lit et l'exécute
seule.*

## Pour le propriétaire : lancer une fiche

1. Ouvrir une **session cloud** (claude.ai/code, ou l'application en choisissant
   l'environnement cloud) sur le dépôt **`chatelp/clairance`**, branche `main`, modèle Opus.
2. Coller le prompt de la fiche, tel qu'il est écrit dans le tableau ci-dessous.
3. Laisser travailler. La session pousse une branche `cloud/…` et écrit son rapport.
   **Elle ne touche jamais `main`.**
4. Le dire à la session Clairance du Mac, qui relit la branche, la vérifie (instance
   d'essai, navigateur, garde-fous) et l'intègre.

| Fiche | Vague | Prompt à coller | Branche | Parallèle avec | État (revue du 20/10) |
|---|---|---|---|---|---|
| [A — retraits côté serveur](A-retraits-serveur.md) | 1 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/A-retraits-serveur.md, et exécute la fiche A en entier.` | `cloud/a-retraits-serveur` | B, C | intégrée le 10/10 (vague 1) |
| [B — retraits côté interface, et la carte réparée](B-retraits-interface.md) | 1 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/B-retraits-interface.md, et exécute la fiche B en entier.` | `cloud/b-retraits-interface` | A, C | intégrée le 10/10 (vague 1) |
| [C — tests de ce qui tourne sans filet](C-tests.md) | 1 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/C-tests.md, et exécute la fiche C en entier.` | `cloud/c-tests` | A, B | intégrée le 10/10 (vague 1) |
| [D — configuration réduite](D-config-reduite.md) | 2 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/D-config-reduite.md, et exécute la fiche D en entier.` | `cloud/d-config-reduite` | F | intégrée le 10/10 |
| [F — tests de l'API](F-tests-api.md) | 2 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/F-tests-api.md, et exécute la fiche F en entier.` | `cloud/f-tests-api` | D | intégrée le 10/10 |
| [E — découpage de main() et handlers.go](E-decoupage-serveur.md) | 2, **après D** | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/E-decoupage-serveur.md, et exécute la fiche E en entier.` | `cloud/e-decoupage-serveur` | — | intégrée le 10/10 (`bc311b7`) ; son étape 4 n'est pas faite |
| [G — la route publique des chiffres avant connexion](G-chiffres-avant-connexion.md) | 3 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/G-chiffres-avant-connexion.md, et exécute la fiche G en entier.` | `cloud/g-chiffres-avant-connexion` | H, I, J, K | intégrée le 13/10 |
| [H — le serveur sans transcription, le Docker amont retiré](H-sans-transcription.md) | 3 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/H-sans-transcription.md, et exécute la fiche H en entier.` | `cloud/h-sans-transcription` | G, I, J, K | intégrée le 13/10 (soir tard) |
| [I — les dettes de sécurité (CORS, WebSocket, rôle lecture seule, purges)](I-dettes-securite.md) | 3 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/I-dettes-securite.md, et exécute la fiche I en entier.` | `cloud/i-dettes-securite` | G, H, J, K | intégrée le 13/10 (soir tard) |
| [J — la compilation Windows](J-compilation-windows.md) | 3 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/J-compilation-windows.md, et exécute la fiche J en entier.` | `cloud/j-compilation-windows` | G, H, I, K | intégrée le 14/10 (matin) |
| [K — « Then », ce que l'avion a fait, sans verdict](K-then-sans-verdict.md) | 3 | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/K-then-sans-verdict.md, et exécute la fiche K en entier.` | `cloud/k-then-sans-verdict` | G, H, I, J | intégrée le 14/10 (matin) |
| [L — le bloc position, trajectoire, prédiction : un seul modèle, l'observé séparé du déduit](L-bloc-position.md) | 3 (ajoutée le 13/10 au soir) | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/L-bloc-position.md, et exécute la fiche L en entier.` | `cloud/l-bloc-position` | J, K (H et I sont en intégration) | intégrée dans la nuit du 13 au 14/10 |
| [M — le mouvement à l'écran : le mesurer dans un vrai navigateur, le rendre le plus fluide possible](M-mouvement-ecran.md) | 3 (ajoutée le 13/10 dans la nuit) | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/M-mouvement-ecran.md, et exécute la fiche M en entier.` | `cloud/m-mouvement-ecran` | J, K | intégrée le 14/10 (matin) |
| [N — le sidecar hors du Mac : Linux et NVIDIA, repli processeur](N-sidecar-linux.md) | 4 (26/10, plan 35, G1) | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/N-sidecar-linux.md, et exécute la fiche N en entier.` | `cloud/n-sidecar-linux` | O | à lancer |
| [O — ouvrir la production sur Internet derrière un mandataire : relecture, tests, trace](O-ouverture-publique.md) | 4 (26/10, plan 35, G3) | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/O-ouverture-publique.md, et exécute la fiche O en entier.` | `cloud/o-ouverture-publique` | N | lancée le 26/10 |
| [P — le compte invité : rien de la maison, des flux bornés](P-invite.md) | 5 (26/10 au soir, ami le 28/10) | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/P-invite.md, et exécute la fiche P en entier.` | `cloud/p-invite` | Q, R | à lancer |
| [Q — un serveur qui porte sa page et dit ce qui lui manque](Q-serveur-autonome.md) | 5 (26/10 au soir, plan 35, B2 et B5) | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/Q-serveur-autonome.md, et exécute la fiche Q en entier.` | `cloud/q-serveur-autonome` | P, R | à lancer |
| [R — le contrat entre le lanceur et le serveur, côté serveur](R-contrat-lanceur.md) | 5 (26/10 au soir, plan 35, B3, C1, C2) | `Lis docs-fr/fiches-cloud/README.md puis docs-fr/fiches-cloud/R-contrat-lanceur.md, et exécute la fiche R en entier.` | `cloud/r-contrat-lanceur` | P, Q | à lancer |

**Vague 1 intégrée le 10/10** (doc 32, « Retour de la vague 1 »). **Vague 2 écrite le 10/10** : D et F intégrées le 10/10 ; **E intégrée le 10/10** (doc 32, « Retour de la fiche E ») ; son étape 4 n'est pas faite. **Vague 3 écrite le 13/10**, validée par le propriétaire : du serveur et de la logique pure, en parallèle de la migration de l'interface qui, elle, reste sur le Mac (instance d'essai, relecture de la session design, soirées d'écoute). Les sept fiches (G à M) sont indépendantes et pouvaient tourner en même temps ; G et K touchent toutes deux `explain.go` (G ajoute `value`/`unit`, K les lit ou les déduit) : K marche sans G.

**Vague 4, écrite le 26/10** (plan `35-plan.md`, bloc G) : N et O, indépendantes, à lancer en parallèle ;
G2 (les tests du contrat du lanceur) attend que le contrat soit écrit.

**Vague 5, écrite le 26/10 au soir** (61 $ de crédit restant, décision du propriétaire : « prépare dès maintenant ce
que tu juges le plus utile ») : **P** (le compte invité, pour l'ami du 28/10), **Q** (la page dans le binaire, le
bilan au démarrage), **R** (le contrat du lanceur, côté serveur, qui remplace G2). Les trois en parallèle ; P d'abord
si une seule peut partir.

**État au 20/10 (revue du 20/10) : les treize fiches, A à M, sont intégrées** (dates dans la dernière colonne ; détail dans la section « État » de `32-audit-produit.md`) ; **rien n'est en attente**. Le crédit de 250 $ expire le 5/11 : ce qu'on y lance encore est une décision du propriétaire (`33-pilotage.md`, décisions attendues, n° 10).

## Pour la session cloud : règles communes, à respecter sans exception

**Lis d'abord** `CLAUDE.md` (racine), puis dans `docs-fr/` : `32-audit-produit.md` (sections
« Ce que l'audit doit respecter », « Alignement » et la liste classée) et D71 dans
`05-decisions.md`. Les rapports d'audit bruts sont dans `docs-fr/audit-2026-10-09/`. **Ce
sont des cartes, pas des vérités** : vérifie chaque affirmation dans le code avant d'agir.

**Ce que tu n'as pas, et ne dois pas chercher à avoir** : la station de réception (pas de
`ssh`, aucune adresse `192.168.*`, aucun `*.lan`), la production, le GPU, les bases de
co-atc, l'audio. **Rien de tout cela ne doit jamais entrer dans le cloud.** Le fichier
`configs/config.toml` du propriétaire n'est pas dans le dépôt : tu travailles avec
`configs/config.toml.example`. Tu peux installer des outils dans ta machine (Go ≥ 1.25,
Node ≥ 22, ffmpeg, sqlite3).

**Langue** : code, commentaires, messages de commit en anglais ; ton rapport en français.

**Git** :
- Une branche `cloud/<nom>` partie de `main`, poussée sur `origin`. **Jamais de push sur
  `main`, jamais de fusion, jamais de rebase de `main`, pas de pull request.**
- **Un commit par étape**, qui compile et dont les tests passent ; un retrait = un commit,
  pour qu'on puisse en défaire un seul. Signature en dernière ligne :
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Le message de commit dit ce qui est retiré ou ajouté, **combien de lignes**, et comment tu
  as vérifié que rien ne s'en servait.

**Ne rien casser** (règle du propriétaire) :
- À chaque commit : `go build ./...`, `go vet ./...`, `go test -race ./...` verts ; pour
  `www/` : `node --check` sur chaque fichier JS touché et `node --test www/par/`.
- **Ne renomme rien** de ce qui est enregistré ou appelé ailleurs : les clés `localStorage`
  de `www/`, les noms du store Alpine lus par `index.html` et les autres JS, les routes
  `/api/v1/…` utilisées par `cmd/radio-ctl-sync`, `sidecar/`, `tools/` et `www/`.
- Un doute, une dépendance inattendue, un test qui casse sans raison claire : **arrête
  cette étape**, garde ce qui est fait, et écris-le dans le rapport. Ne devine pas.
- Ne touche pas à `docs/` (à l'amont), ni aux documents de `docs-fr/`, sauf ton rapport.

**Rapport** : à la fin, un fichier `docs-fr/fiches-cloud/<lettre>-rapport.md`, commité sur
ta branche, en français, 80 lignes au plus : la liste des commits avec ce que chacun fait
et ses chiffres ; ce que tu as vérifié et comment ; ce que tu n'as **pas** pu vérifier
(tout ce qui demande un navigateur avec de vrais flux, la station, la production) ; les
doutes ; ce qu'il faut regarder en priorité sur le Mac.
