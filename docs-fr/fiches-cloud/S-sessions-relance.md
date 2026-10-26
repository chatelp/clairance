# Fiche S — Rester connecté quand la production redémarre

*Écrite le 26/10/2026 au soir. Plan : `docs-fr/35-plan.md`, bloc D (étape D5, « la fiabilité vue de chez lui »).
Lis d'abord les règles communes de `README.md`. Crédit restant : environ 17 $ ; **reste dans le périmètre**, et
écris le rapport même si une étape n'est pas finie.*

## Pourquoi

Le 28/10, un ami du propriétaire reçoit un compte invité (`viewer`) sur la production, ouverte sur Internet derrière
un mandataire Traefik. Les sessions de connexion vivent **en mémoire** (`internal/auth/session.go`) : **chaque
relance de la production déconnecte tout le monde**, et l'ami doit retaper son mot de passe. La production redémarre
souvent en ce moment (trois fois le 27/10). Le cookie, lui, dure 30 jours (décision du propriétaire du 26/10 :
inchangé).

## Ce qui existe (vérifie-le dans le code)

- `internal/auth` : `session.go` (la table des sessions, l'expiration glissante, la révocation par compte),
  `service.go`, `store_file.go` (les comptes dans `configs/users.json`, écrit en 0600), `trace.go` (la trace des
  connexions dans `signins.jsonl` à côté des comptes, fiche O : un modèle d'écriture sûre sur disque).
- Le réglage `session_ttl_hours` (30 jours glissants).
- La page : `www/websocket-client.js` (reconnexion du WebSocket), `www/live-player.js` (le son reprend après une
  pause qui s'allonge), `www/login.js`.

## Les étapes (un commit chacune, sur `cloud/s-sessions-relance`)

1. **Les sessions survivent à une relance.** Elles sont gardées sur disque, à côté des comptes, **sans jamais écrire
   le jeton lui-même** : seulement son empreinte (SHA-256), le compte, le rôle, les dates. Fichier en 0600, écrit
   en entier ou pas du tout (fichier temporaire puis renommage), ignoré par git (`.gitignore`, comme `users.json` et
   `signins.jsonl`). Relu au démarrage : une session expirée, ou dont le compte n'existe plus, ou dont le rôle a
   changé, est écartée. La déconnexion et la révocation par compte l'effacent aussi du disque. Écrire à chaque
   requête serait trop : l'expiration glissante peut être persistée par lots (par exemple au plus une fois par
   minute, et à l'arrêt) ; dis ton choix et pourquoi. Tests : connexion, relance de la pile, la même cookie est
   acceptée ; déconnexion, relance, refusée ; compte retiré, relance, refusée ; fichier illisible ou corrompu :
   le serveur démarre, sans session, et le dit au journal ; le fichier ne contient aucun jeton (cherche-le dedans).
2. **Ce que voit la page pendant une relance.** Avec le vrai binaire et un vrai navigateur (Chromium par Playwright,
   comme les fiches M et P) : connecté en invité, la page ouverte, le son en cours (une source de test : un MP3 en
   boucle servi localement, ou la tonalité UDP de `internal/audio/audiotest` si elle existe), **arrêter puis
   relancer le serveur** : la page ne doit pas renvoyer à la connexion ; le WebSocket se reconnecte ; le son reprend
   seul ; mesure combien de temps après le retour du serveur. Si quelque chose ne reprend pas, corrige-le dans la page
   (un commit à part) et dis-le. Si un message « reconnexion… » existe déjà, vérifie qu'il s'affiche et disparaît.
3. **Le rapport**, `docs-fr/fiches-cloud/S-rapport.md`, en français : chaque commit avec ses lignes, le choix de
   persistance de l'expiration glissante, les temps mesurés à l'étape 2, ce qui est vérifié et comment, ce qui ne
   l'est pas.

## Ce que tu ne fais pas

- Ne change pas la durée des sessions (30 jours), ni le format des cookies, ni les rôles.
- Pas de base de données nouvelle : un fichier, comme les comptes et la trace.
- Ne touche pas à `docs/` (l'amont). Aucune adresse réelle : `station.lan`, `192.0.2.x`, `example.com`.

## Vérifier

`go build ./...`, `go vet ./...`, `go test ./...` (et `-race` sur `internal/auth`, `internal/api`) ;
`node tools/www-check/check.mjs` si tu touches `www/`.
