# Fiche O — Ouvrir la production sur Internet derrière un mandataire : relecture, tests, trace

*Écrite le 26/10/2026. Plan : `docs-fr/35-plan.md`, bloc D (étapes D3 et D4) et G3. Décision : Q62
dans `docs-fr/05-decisions.md`. Lis d'abord les règles communes de `README.md`.*

## Pourquoi

Le propriétaire va donner à un proche l'accès à sa production, **par Internet**, derrière un
mandataire Traefik qui termine le HTTPS et transmet à co-atc en HTTP, avec `X-Forwarded-For` et
`X-Forwarded-Proto`. Ce proche aura un compte **`viewer`**, avec l'audio (`server.audio_replay =
"all"`). C'est un usage d'une période de tests restreints, pas une fonctionnalité multi-utilisateurs.
**Rien ne sera ouvert avant que cette relecture soit faite.** Ton travail : trouver ce qui ne tient
pas une fois public, le prouver par un test, le corriger, et ajouter la trace des connexions.

## Ce qui existe (vérifie-le dans le code)

- Les comptes et les rôles : `internal/auth` (`service.go`, `role.go`, `session.go`, `request.go`) ;
  `owner` et `viewer` ; `ViewerGuard` et `viewerMay` dans `internal/api/auth_handlers.go`.
- Les mandataires de confiance : `server.trusted_proxies` ; `TrustedProxies.Scheme`, `ClientIP`,
  `Forwarded` (`internal/auth/request.go`) ; le cookie `Secure` derrière un mandataire en HTTPS.
- Les routes ouvertes sans session : la liste figée `internal/api/testdata/routes.txt` (colonne
  `open`) : `/health`, `/health/detail` (refusée depuis un mandataire, voir son test), `/vitals`,
  `/auth/*`, `/setup/*`, et les fichiers de la page.
- Les flux audio passent par l'adresse de la page derrière un mandataire (`streamPortFor`,
  `internal/api/frequency_handlers.go`, commit 43a5eed) ; le WebSocket `/api/v1/ws`, l'origine
  vérifiée (`internal/auth/origin.go`, `ws_origin_test.go`) ; la limite des essais de mot de passe
  (`Limiter`).

## Les étapes (un commit chacune, sur `cloud/o-ouverture-publique`)

1. **L'inventaire.** Pour chaque route (`routes.txt`) et pour le WebSocket : ce qu'un inconnu, un
   viewer et le propriétaire peuvent faire, derrière un mandataire déclaré (`proxies` dans
   `stackOptions` des tests). Écris-le en tableau dans le rapport.
2. **Un test par interdit**, dans `internal/api`, avec la pile de test existante (`newStack`,
   `signInAs(viewerUser)`, en-têtes du mandataire) : chaque écriture refusée au viewer ; chaque route
   verrouillée refusée sans session ; l'audio (flux et extraits archivés) refusé au viewer avec
   `audio_replay = "owner"` et accordé avec `"all"` ; le WebSocket refusé à une origine étrangère et
   accepté depuis l'origine publique configurée ; `/setup` fermé une fois un compte créé ;
   `/health/detail` refusée à travers le mandataire.
3. **Ce qui ne tient pas** : chaque défaut trouvé, corrigé dans **son propre commit**, avec son test.
   Regarde au moins : la limite des essais quand l'adresse vient de `X-Forwarded-For` (qui ne peut
   être contournée en variant l'en-tête, et qui ne bloque pas tout le monde derrière l'adresse du
   mandataire) ; l'énumération des comptes par les messages ou le temps de réponse de
   `/auth/login` ; les drapeaux du cookie (`HttpOnly`, `SameSite`, `Secure`) ; la durée des
   sessions (`session_ttl_hours`) ; les délais du serveur contre une connexion lente (`ReadTimeout`
   et l'absence de `ReadHeaderTimeout`, `cmd/server/serve.go`) en gardant les flux audio longs
   possibles ; la taille des corps de requête ; ce que `/vitals` dit à un inconnu ; les méthodes et
   chemins du serveur de fichiers (`*api.StaticFileHandler`, fichiers cachés, `..`).
4. **La trace des connexions** (étape D4) : chaque connexion réussie ou refusée est journalisée avec
   le compte, l'adresse du client (celle que donne le mandataire de confiance), l'agent et l'heure ;
   le propriétaire peut lire les dernières (route verrouillée au propriétaire, à ajouter à
   `routes.txt`). Rien de cela ne doit apparaître pour un viewer.
5. **L'exemple de mandataire** : dans le README (section en anglais), un exemple générique de
   configuration Traefik (routeur HTTPS, en-têtes transmis, limite de débit sur `/api/v1/auth/login`,
   en-têtes de sécurité, flux longs non coupés) et la ligne `trusted_proxies` qui va avec. **Aucun
   nom de domaine ni adresse réels.**

## Ce qui ne doit pas bouger

Les routes et leurs noms (lus par `www/`, `cmd/radio-ctl-sync`, `tools/`), le comportement en accès
direct sur le réseau local, le lanceur (`tools/menubar/`), la page. Toute route ajoutée met à jour
`routes.txt` (`ATC_WRITE_ROUTES=1`).

## Le rapport (`O-rapport.md`)

Le tableau de l'étape 1 ; les commits ; chaque défaut trouvé, sa gravité (exploitable par un
inconnu, par un viewer, ou seulement gênant) et sa correction ; ce que tu n'as pas pu vérifier
(Traefik réel, navigateur réel) ; ce que le propriétaire doit régler lui-même dans Traefik.
