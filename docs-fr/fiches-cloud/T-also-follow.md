# Fiche T — « Also follow » : les aérodromes cochés et les plus proches d'abord, le reste replié

*Écrite le 28/10/2026. Lis d'abord les règles communes de `README.md`. Crédit restant : environ 10 $ ; **la fiche est
courte, reste dans son périmètre**, et écris le rapport même si une étape n'est pas finie.*

## Pourquoi

Dans Réglages › Station › Listening, le champ « Also follow » (suivre d'autres aérodromes que celui de référence,
trois au plus) affiche **tous** les aérodromes à portée de la station en cases à cocher : 21 sur la station du
propriétaire, sur trois ou quatre lignes, pour en cocher trois au plus. La session design propose (28/10), et le
propriétaire l'a accepté : **montrer d'abord ceux qui sont cochés et les plus proches, puis le reste replié.**

## Ce qui existe (vérifie-le dans le code)

- `www/index.html`, page `listening` des réglages (cherche `Also follow`) : le champ, ses cases `cl-check` liées à
  `alsoChoices()`, `alsoFollowed(code)` et `toggleAlso(code)`, son aide « Up to three: each gets a PAR and an arrival
  sequence. » Un compte viewer voit la même valeur en texte (`!admin`) : **ne la change pas**.
- `www/server-settings.js` : `airportChoices()` (code, nom, `distance_nm` au récepteur), `alsoChoices()` (tous sauf
  l'aérodrome de référence), `alsoFollowed`, `toggleAlso` (la limite de trois et l'enregistrement).
- Le composant replié du système de design : `cl-disclosure` (un bouton avec `aria-expanded`), déjà employé dans la
  même page (« Advanced » dans Matching, « Diagnostics » dans Display).
- La fiche du composant : `docs-fr/design/systeme/project/components/SettingsPage/README.md`.

## Les étapes (un commit chacune, sur `cloud/t-also-follow`)

1. **Une fonction pure** dans `www/server-settings.js`, exportée pour les tests comme `settingsNav` : à partir des
   choix (déjà sans l'aérodrome de référence), des codes cochés et d'un nombre `n` (5), elle rend deux listes :
   `shown`, les cochés (toujours, même lointains) puis les plus proches non cochés jusqu'à `n` en tout, triés par
   distance ; `more`, tous les autres, triés par distance. Aucun aérodrome perdu ni en double. Tests Node dans
   `www/server-settings.test.js` : 21 choix, 0, 1, 3 cochés, un coché lointain, moins de `n` choix, des distances
   égales (ordre stable par code).
2. **La page** : les cases de `shown`, puis, s'il y a un reste, un `cl-disclosure` « More aerodromes (N) » qui
   déplie les cases de `more`, dans le même style et avec le même comportement (clavier : espace, rôle `checkbox`,
   `aria-checked`, la limite de trois). Fermé par défaut ; **un aérodrome coché depuis le repli remonte dans
   `shown`** au prochain rendu, le repli reste ouvert. L'aide sous le champ ne change pas.
3. `tools/tailwind/build.sh` si tu ajoutes des classes, puis `node tools/www-check/check.mjs` vert.

Rien côté serveur, rien d'autre dans la page.

## Ce que tu ne peux pas vérifier

L'affichage réel (navigateur, la vraie liste de la station). Dis-le dans le rapport : la session du Mac le vérifiera
sur son instance d'essai, et la session design relira.

Rapport : `docs-fr/fiches-cloud/T-rapport.md`, 40 lignes au plus.
