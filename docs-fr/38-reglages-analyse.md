# Les réglages : analyse et organisation proposée

> **Analyse du 28/10/2026**, faite en lecture seule par un sous-agent (Sonnet) à la demande du
> propriétaire : « des settings multi-pages, et pas une longue liste dans une colonne étroite, ce serait
> mieux → revoir l'organisation des settings en profondeur ». Rien n'est changé dans la page avant sa
> décision (à consigner dans `05-decisions.md`). Les numéros de ligne renvoient à `www/index.html` au
> commit `2ff9760`.

## Ce qui ne va pas

- **Une colonne étroite** : `.cl-page__inner` fait 900 px au plus, la navigation 200 px, les sections
  640 px ; sur grand écran, un tiers de la surface sert (`components.css`).
- **Une seule longue page** d'environ 650 lignes ; le suivi de la section en vue (`spy()`) contourne le
  fait que les dernières sections sont trop courtes pour atteindre le haut.
- **Des natures mêlées** : Display (préférences du navigateur, localStorage) entre deux sections du
  serveur ; « Station console on the sign-in page » (réglage du serveur) sous Account ; Account mêle
  une préférence (Sign out), de l'administration (Accounts, Usage, Who may open) et un réglage du
  serveur ; System mêle trois réglages modifiables, des rapports et un outil de mise au point du
  navigateur (Live updates, 22 compteurs).
- **Le compte viewer voit tous les contrôles du serveur**, qui échouent en 403 : la page ne lit pas le
  rôle (`/api/v1/auth/status` le donne). L'avertissement d'archive erroné chez un viewer, relevé par
  l'analyse, est corrigé (db5a6c3).
- **Des chargements inutiles** : comptes, usage et état du serveur se chargent à l'ouverture de la
  page, réglages fermés ; l'usage se rafraîchit toutes les 15 s quelle que soit la section ouverte.
- **Des doublons** d'aide (la phrase sous le champ répète la bulle : « Reference airport », « Names
  from the AIP ») et un vocabulaire des rôles qui dérive (« admin » à l'écran, `owner` dans l'API,
  « guest » dans le code).
- **Des termes obscurs** pour un novice : les préréglages Measured / Cautious / Experimental, « Score
  0.9 », AIRAC, SIA, eAIP, « SID and STAR coding tables », « Gain / PPM », « binCraft », « Log level »,
  des pourcentages de mesure dans les aides.
- **Sur téléphone**, la navigation devient une rangée de six liens qui défilent (rendu à 375 px non
  vérifié).
- **La maquette** `docs-fr/design/ecrans/reglages.html` est périmée (ni Regional data ni comptes).

## L'organisation proposée

Une page à la fois, sans défilement entre pages, et une navigation groupée :

| Groupe | Page | Contenu | Qui |
|---|---|---|---|
| Mon navigateur | **Display** | les six réglages d'affichage ; « Live updates » et les compteurs repliés en « Diagnostics » | tous |
| | **Account** | « Signed in as », « Sign out » | tous |
| Station | **Listening** | On air, Reference airport, Also follow, puis les noms officiels (AIP) ; hors de France, une ligne | admin ; viewer en lecture |
| | **Matching** | les préréglages, puis « Advanced » (règles, secteurs) | admin |
| Administration | **Access** | Who may open Clairance, Accounts, Station console | admin |
| | **Usage** | le rapport par compte | admin |
| | **Server** | niveau du journal, taille gardée, archive, puis les rapports en lecture | admin |

- **Viewer** : Display, Account et Listening en lecture ; le rôle lu une fois ; le serveur reste
  l'autorité (403).
- **Ordinateur** : navigation à gauche avec les intitulés de groupe ; `cl-page__inner` élargi
  (≈ 1 080 px) ; les champs gardent leur largeur de lecture ; Usage, comptes et rapports en deux
  colonnes au-delà de 1 024 px (une classe à ajouter au système de design).
- **Téléphone** : liste puis détail (la liste des pages en rangées tactiles, puis une page avec
  « ‹ Settings »), sur le modèle de « ‹ Back to Traffic ».

## Simplifications proposées

- Regional data fondu dans Listening.
- « Accept one digit off » (marqué « experiments only ») et « First two digits » sous « Advanced »,
  voire retirés (D71 : ce qui n'a pas de rôle se retire) — au propriétaire de trancher.
- « Reports » et « Performance and debug » fondus en un seul « Diagnostics », en bas.
- Règle d'aide : la phrase sous le champ dit quoi, la bulle dit pourquoi ou la mesure ; plus de
  doublons.
- Renommages à valider : « Measured » → « Standard » ; « Station console on the sign-in page » →
  « Live figures on the sign-in page » ; « Log level » → « Log detail » ; un seul mot pour le rôle
  admin.

## Plan proposé

1. La décision, dans `05-decisions.md`.
2. Le rôle viewer lu par la page ; contrôles du serveur masqués ou en lecture pour lui.
3. Une page à la fois (`page` à la place de `current`, `spy`, `goTo`) ; les liaisons ne bougent pas.
4. Les blocs déplacés entre pages ; usage, comptes et AIP chargés à la première ouverture de leur page,
   rafraîchissement de l'usage seulement sur sa page.
5. La mise en page (`components.css`), puis `tools/tailwind/build.sh` et `tools/www-check/check.mjs`.
6. Le téléphone, mesuré à 375 px dans Chrome sans fenêtre (`capture.mjs`).
7. Libellés et aides.
8. Les documents et textes qui citent les chemins des réglages : README (« Settings → Account →
   Accounts », « Usage »), `config.toml.example`, `startcheck.go` (« Settings > System »), la maquette.

Risques relevés : aucun test Node ne fige un libellé ni un identifiant de section ; `check.mjs`
signale toute fonction appelée sans définition. Non vérifié par l'analyse : les tests Go qui citent
« Settings > ».
