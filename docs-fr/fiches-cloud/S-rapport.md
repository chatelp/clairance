# Rapport de la fiche S — rester connecté quand la production redémarre

*Session cloud du 26/10/2026 au soir. Branche **`claude/bold-edison-nqqtm6`** (la branche imposée à la session, comme
P, Q et R l'ont fait avec `claude/fiche-…` ; pas de `cloud/s-sessions-relance`), partie de `main` à `11a7256`, rien sur
`main`, pas de pull request. Go 1.24.7, en root. Trois commits, +642 / −32 lignes, dont 265 de tests et 133 d'outil.*

## Les commits
1. **`2cf83f4` auth : les sessions survivent à une relance** (+503 / −30, 9 fichiers, dont `session_test.go`, 265).
   `configs/sessions.json`, à côté de `users.json` et `signins.jsonl` : par session, **l'empreinte SHA-256 du jeton**
   (jamais le jeton), le compte, le rôle à l'émission, création et expiration. 0600, fichier temporaire puis renommage,
   ignoré par git (`configs/sessions.json*`). Relu au démarrage : écartée si expirée, si le compte n'existe plus, si son
   rôle a changé ; s'il y en a d'écartées, le fichier est réécrit aussitôt. Fichier illisible, corrompu ou répertoire :
   le serveur démarre sans session, le journal le dit (`Sessions could not be read back…`), la connexion suivante le
   remplace. La table est indexée par l'empreinte ; une session relue apprend son jeton quand le navigateur le présente
   (la déconnexion marche donc après relance). `Store.Create` prend le rôle. Durée, cookie et rôles inchangés.
2. **`2c14ba3` www : le WebSocket attend 5 s au plus entre deux essais, au lieu de 30** (+6 / −2 : `websocket-client.js`,
   et la première ligne de `tailwind.css`, reconstruit pour son empreinte seulement). Voir les mesures.
3. **`fcea23a` tools : `restart-bench`**, le scénario de l'étape 2 (`restart.mjs`, 105 l. ; `mp3loop.py`, 28 l.), à rejouer.

## L'expiration glissante : le choix
Connexion, déconnexion, `RevokeUser`, `RevokeAll` (le choix « local ») **écrivent tout de suite** : une déconnexion
qu'une relance annulerait rendrait la session. Le glissement seul écrit **au plus une fois par minute**, par la requête
qui trouve la minute passée (pas de minuterie à arrêter), et **à l'arrêt** (`Handler.SaveSessions`, après l'arrêt des
serveurs HTTP). Un plantage perd au plus une minute de glissement sur trente jours. Une écriture ratée laisse la table
« sale » : retentée dans la minute et à l'arrêt, qui écrit l'erreur au journal.

## Étape 2 : la page de l'invité pendant une relance (mesuré)
Vrai binaire, faux tar1090 (`tools/startup-bench/fake.py`), quatre sources sur un MP3 en boucle (`station-8k-vbr.mp3`),
comptes `pierre` et `guest` (viewer) dans `users.json`, `audio_replay = "all"`, `trusted_proxies = ["127.0.0.1/32"]` et
`X-Forwarded-For` envoyé par Chromium (Playwright) : le serveur sert alors les flux sur l'origine de la page, **comme à
travers le Traefik de l'ami**. Vrai formulaire, quatre flux joués, SIGINT puis relance (arrêt 1,1–2,1 s, démarrage
1,1–1,3 s) ; temps comptés depuis la première réponse de `/api/v1/health`.

| Coupure | `main` (sessions en mémoire) | branche, WebSocket à 30 s | branche, 5 s (commit 2) |
|---|---|---|---|
| 3 s | **rien ne revient en 60 s** : 47 × 401, son en 401 toutes les 10 s, bandeau figé | WS 1,9 s ; son 2,6 s | WS 1,9 s ; son 3,2 s |
| 15 s | — | WS **16,9 s** ; son 2,7 s | WS 3,0 s ; son 2,7 s |
| 30 s | — | WS **28,6 s** ; son 5,2 s | WS 0,4 s ; son 5,2 s |

Branche, tous les essais : **aucun 401, pas de renvoi à la connexion** (URL `/`, pas de formulaire) ; le bandeau
« Connection to the server lost… reconnecting » paraît 1,1–2,1 s après l'arrêt, part dans les 60 ms qui suivent le
WebSocket rouvert ; les quatre flux reprennent seuls (`playing` : premier échantillon en file). `main` : il faut recharger.
Le son dépend des essais du lecteur (1, 2, 5 puis 10 s), non touchés ; le WebSocket, lui, doublait jusqu'à 30 s : la
carte restait figée et le bandeau affiché jusqu'à 29 s après le retour du serveur, le son déjà revenu. Plafonné à 5 s
(3,75 à 6,25 s avec l'aléa), le retard est borné par ce plafond.

## Vérifié, et comment
- `go build ./...`, `go vet ./...` ; `go test ./...` vert sauf `TestAChoiceThatCannotBeWrittenChangesNothing`, **déjà
  rouge sur `main`** en root (rapports O, P ; `chmod` n'arrête pas root ; l'essai sous `nobody` a échoué faute d'accès au
  cache de modules) ; `-race` vert sur `internal/auth` (hors ce test) et `internal/api`. `node --check`,
  `node tools/www-check/check.mjs` : OK.
- Tests nouveaux : session gardée après relance (avec son rôle) ; vraie connexion `SignIn` sur disque aussitôt, deux
  relances ; déconnexion et `RevokeUser`, relance, refusées ; compte retiré et rôle changé, écartés et effacés du
  fichier ; fichier corrompu et répertoire : démarrage sans session, erreur dite, remplacé à la connexion suivante ;
  **aucun jeton dans le fichier** (cherché), seulement les empreintes, 0600, pas de `.tmp` laissé ; glissement écrit à
  la minute et à `Save` (horloge simulée) ; session expirée pendant l'arrêt écartée.
- Au `curl`, vrai binaire : invité 200, relance 200, jeton absent du fichier ; déconnexion, relance, 401. 17 relances.

## Pas vérifié
Un vrai Traefik en HTTPS (cookie `Secure`), Safari, l'iPhone, le son **entendu** (premier échantillon reçu, seulement),
la durée d'une relance de la production (ici 1 s), un disque lent (écriture sous le verrou, ~2 Ko, 1/min au plus).

## Doutes et constats hors périmètre
- **Le commentaire d'origine disait « en mémoire, exprès »** : une relance était le moyen sûr de déconnecter tout le
  monde. Ce n'est plus vrai ; le moyen devient : arrêter le serveur et effacer `configs/sessions.json`, ou retirer le
  compte (ses sessions tombent au démarrage). Écrit dans `session.go`, **à reporter dans `35-plan.md` (D5, l. 139) et
  `05-decisions.md` (l. 2041)**, qui disent encore « les sessions vivent en mémoire ».
- **Flux sur les ports annexes sans cookie**, avant toute relance : sans mandataire, les flux servis sur 18801–18804 sont
  une autre origine, et `fetch(…, {credentials: 'same-origin'})` n'y envoie pas le cookie : **401, trois fréquences sur
  quatre muettes** pour un compte connecté. Sans effet derrière Traefik (le port est retiré) ; à regarder si le
  propriétaire se connecte sur le réseau local avec la connexion activée.
- **Une panique à l'arrêt, 1 fois sur 17**, après « Server fully stopped » : `sqlite.(*TranscriptionStorage)
  .VoiceSummaries` interroge une base déjà fermée (`*sql.DB` nul). Antérieure à cette fiche, sans lien avec les sessions
  (écrites avant ou perdues d'une minute de glissement au pire), mais le processus sort en erreur.

## À regarder en priorité sur le Mac
1. L'instance d'essai **en invité à travers Traefik**, relance réelle (`tools/restart-bench/restart.mjs`, son en-tête) ;
   l'iPhone reste-t-il connecté, le son revient-il.
2. `configs/sessions.json` en 0600 et ignoré par git, « Sessions read back » à chaque démarrage ; compte retiré : refusé.
3. La panique de `VoiceSummaries` à l'arrêt, et les flux des ports annexes sans cookie (ci-dessus).
