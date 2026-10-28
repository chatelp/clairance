# Rapport de la fiche T — « Also follow » : les cochés et les plus proches d'abord, le reste replié

*Session cloud du 28/10/2026. Branche **`claude/nifty-cannon-w8yd6x`**, pas `cloud/t-also-follow` : la session
cloud impose sa branche et interdit d'en pousser une autre sans accord (comme la fiche S). Partie de `main` (`64062a0`).*

## Commits

1. **`b74d560` — `alsoSplit(choices, followed, n)`**, fonction pure dans `www/server-settings.js` (+20), exportée
   à côté de `settingsNav`. `shown` : les cochés (même lointains) et les plus proches non cochés, `n` en tout ;
   `more` : le reste ; les deux par distance, à égalité par code. 5 tests dans `www/server-settings.test.js` (+58) :
   21 choix avec 0, 1, 3 cochés, un coché lointain, moins de `n` choix, aucun, distances égales ; aucun aérodrome
   perdu ni en double.
2. **`43fb954` — la page.** `index.html` (+21 −3) : les cases de `shown`, puis, s'il reste quelque chose, un
   `cl-disclosure` « More aerodromes (N) » fermé par défaut, qui déplie `more` dans les mêmes `cl-check` (rôle
   `checkbox`, `aria-checked`, espace, limite de trois). `server-settings.js` (+12) : `alsoMoreOpen`,
   `alsoLists()` (`n` = 5), `keepAlsoFocus(code)`. Aide du champ et texte du viewer inchangés.
3. **Pas de commit propre** : aucune classe ajoutée. `tools/tailwind/build.sh` ne change que l'empreinte des sources
   de `www/vendor/tailwind.css` ; elle couvre aussi les JS, donc chaque commit porte la sienne.

## Vérifié, et comment

- `node tools/www-check/check.mjs` OK **sur chacun des deux commits** (arbre extrait à `b74d560`) ; tests : 8/8.
- **Dans Chromium**, sur un banc : balisage du champ extrait de `index.html`, Alpine du dépôt, feuilles dans l'ordre
  de la page, 21 aérodromes fictifs. 5 visibles, « More aerodromes (16) » fermé ; ouvrir, fermer ; espace sur une
  case du repli : elle remonte cochée, le repli reste ouvert, **le focus la suit** ; une quatrième refusée ; aucune
  erreur. Le banc a trouvé un défaut, corrigé avant le commit : la case cochée depuis le repli est retirée du DOM, et
  `this.$root` lu depuis l'élément détaché est indéfini ; `keepAlsoFocus` lit la racine avant le rendu.
- Go (non touché) : `go build`, `go vet` verts. `go test -race ./...` : trois échecs, **aussi sur `main` intact**
  ici : `TestAChoiceThatCannotBeWrittenChangesNothing` (`internal/auth` ; la session est root et écrit où le test
  attend un refus), `TestAnHourOfStream` (`goxiph/mp3`, durée sous `-race`), `TestASlowClientIsCutAndALongAnswerIsNot`
  (`cmd/server`, 1 passage complet sur 2 ; 3/3 vert seul).

## Pas vérifié

La vraie page, avec le serveur et la vraie liste de la station, le téléphone, le lecteur d'écran : à faire sur
l'instance d'essai du Mac ; relecture par la session design.

## Doutes, à regarder en priorité sur le Mac

- **Ordre de `shown`** lu comme « cochés + plus proches, trié par distance » : une case cochée ne saute pas en tête.
- **Cocher depuis le repli fait descendre le cinquième plus proche** dans le repli (le compte N ne change pas).
- **Décocher un lointain le renvoie au repli** ; fermé, il disparaît et le focus est perdu. Règle de la fiche.
