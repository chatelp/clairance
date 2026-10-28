# Rapport de la fiche T — « Also follow » : les cochés et les plus proches d'abord, le reste replié

*Session cloud du 28/10/2026. Branche **`claude/nifty-cannon-w8yd6x`**, pas `cloud/t-also-follow` : la session
cloud impose sa branche et interdit d'en pousser une autre sans accord (comme la fiche S). Partie de `main` (`64062a0`).*

## Commits

1. **`b74d560` — `alsoSplit(choices, followed, n)`**, fonction pure dans `www/server-settings.js` (+20 lignes),
   exportée à côté de `settingsNav`. `shown` : les cochés (toujours, même lointains) et les plus proches non cochés,
   `n` en tout ; `more` : le reste. Les deux listes sont triées par distance, à égalité par code.
   5 tests dans `www/server-settings.test.js` (+58) : 21 choix avec 0, 1 et 3 cochés, un coché lointain, moins de
   `n` choix, aucun choix, des distances égales ; chacun vérifie qu'aucun aérodrome n'est perdu ni en double.
2. **`43fb954` — la page.** `index.html` (+21 −3) : les cases de `shown`, puis, s'il reste quelque chose, un
   `cl-disclosure` « More aerodromes (N) », fermé par défaut, qui déplie les cases de `more` (même balisage
   `cl-check` : rôle `checkbox`, `aria-checked`, espace, la limite de trois). `server-settings.js` (+12) :
   `alsoMoreOpen`, `alsoLists()` (`n` = 5), `keepAlsoFocus(code)`. L'aide sous le champ et le texte du compte viewer
   ne changent pas.
3. **L'étape 3 n'a pas de commit propre** : aucune classe ajoutée. `tools/tailwind/build.sh` ne change que l'empreinte
   des sources en tête de `www/vendor/tailwind.css` ; le filet la contrôle et elle couvre aussi les JS, donc chaque
   commit porte la sienne.

## Vérifié, et comment

- `node tools/www-check/check.mjs` : OK **sur chacun des deux commits** (vérifié sur un arbre de travail extrait à
  `b74d560`). `node --test www/server-settings.test.js` : 8/8.
- **Dans Chromium**, sur un banc : le balisage du champ extrait de `index.html`, l'Alpine du dépôt, les feuilles dans
  l'ordre de la page, 21 aérodromes fictifs. 5 visibles et « More aerodromes (16) » fermé ; ouverture et fermeture ;
  espace sur une case du repli : elle remonte cochée, le repli reste ouvert, **le focus la suit** ; une quatrième
  case est refusée (« Three further airports at most. ») ; aucune erreur de page.
- Ce banc a trouvé un défaut, corrigé avant le commit : cochée depuis le repli, la case est retirée du DOM, et
  `this.$root` lu ensuite depuis l'élément détaché est indéfini. `keepAlsoFocus` lit la racine avant le rendu.
- Go : rien de Go n'est touché. `go build` et `go vet` sont verts. `go test -race ./...` échoue sur trois tests,
  **qui échouent aussi sur `main` intact** dans ce conteneur : `TestAChoiceThatCannotBeWrittenChangesNothing`
  (`internal/auth`, la session tourne en root, qui écrit là où le test attend un refus),
  `TestAnHourOfStream` (`third_party/goxiph/mp3`, durée sous `-race`), et `TestASlowClientIsCutAndALongAnswerIsNot`
  (`cmd/server`, une fois sur deux passages de la suite complète, 3/3 vert seul).

## Pas vérifié

La vraie page, avec le vrai serveur et la vraie liste de la station (les 21 aérodromes et leurs distances), le
téléphone et le lecteur d'écran. À faire sur l'instance d'essai du Mac ; relecture par la session design.

## Doutes, à regarder en priorité sur le Mac

- **L'ordre de `shown`** : je l'ai lu comme « l'ensemble cochés + plus proches, trié par distance ». Une case ne
  saute donc pas en tête quand on la coche ; une case cochée depuis le repli s'insère à sa distance.
- **Le cinquième plus proche descend dans le repli** quand on coche depuis le repli (le nombre replié ne change pas).
- **Décocher un aérodrome lointain le renvoie dans le repli** ; si celui-ci est fermé, il disparaît de la vue et le
  focus clavier est perdu. C'est la règle de la fiche ; à juger à l'usage.
