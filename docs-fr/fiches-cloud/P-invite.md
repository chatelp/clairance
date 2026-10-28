# Fiche P — Le compte invité : ne rien lui montrer de la maison, borner ce qu'il peut ouvrir

*Écrite le 26/10/2026 au soir. Plan : `docs-fr/35-plan.md`, bloc D (étape D4). Décision du propriétaire
du 26/10 au soir, dans `docs-fr/05-decisions.md`, Q62. Suite directe de la fiche O : lis son rapport,
`docs-fr/fiches-cloud/O-rapport.md`, section « Constats sans correction ». Lis d'abord les règles
communes de `README.md`.*

## Pourquoi

Le 28/10, un ami du propriétaire reçoit un compte **`viewer`** (invité) sur la production, ouverte sur
Internet derrière un mandataire Traefik, avec l'audio (`server.audio_replay = "all"`). La fiche O a
relevé deux choses qu'un invité peut faire aujourd'hui et que le propriétaire a décidé de corriger
(« corriger les deux ») :

1. **L'invité lit des adresses internes** : l'`url` de chaque fréquence (le flux de la station, une
   adresse du réseau local), `/api/v1/adsb/source` (la source ADS-B), des chemins de fichiers dans
   `/api/v1/server` et `/api/v1/config` (le répertoire personnel du propriétaire, ses dossiers). Rien
   de cela ne lui sert : la page qu'il voit n'en a pas besoin.
2. **L'invité peut ouvrir beaucoup de flux audio** (le rapport O : dix par fréquence, cent en tout) et
   en priver le propriétaire, sur un Mac dont le décodage et le réseau sont partagés.

Et une troisième, sans décision nécessaire (corrige-la) : **bloqué par la limite des essais, la
connexion répond 401**, alors que `www/login.js` attend 429 pour afficher « Too many attempts » ;
l'utilisateur bloqué lit « mauvais mot de passe » et réessaie.

## Les étapes (un commit chacune, sur `cloud/p-invite`)

1. **L'inventaire de ce qu'un invité lit.** Pour chaque route `GET` accordée au viewer (la matrice figée
   `internal/api/testdata/access.txt`, écrite par la fiche O) et pour chaque message du WebSocket : les
   champs qui portent une adresse réseau (IP, nom d'hôte, URL non publique), un chemin de fichier, ou un
   identifiant de la station. Écris-le en tableau dans le rapport. Vérifie ce que la page en fait
   réellement (`www/`) : un champ que la page n'affiche pas à un invité n'a pas à lui parvenir.
2. **Le masquage** : pour un invité, ces champs sont **retirés ou vidés côté serveur**, pas cachés par la
   page. Le propriétaire les voit toujours, inchangés. Une seule règle, au même endroit, pas un `if` par
   gestionnaire si on peut l'éviter (regarde comment `viewerMay` et le rôle sont déjà lus). Un test par
   route : le viewer ne reçoit aucune adresse, le propriétaire reçoit tout. **La page de l'invité doit
   continuer de marcher** : la carte, les fréquences, le son (qui passe par l'adresse de la page derrière
   le mandataire, `streamPortFor`), les transcriptions. Si un champ masqué sert à la page de l'invité,
   dis-le dans le rapport au lieu de casser la page.
3. **La limite des flux** : trouve où les flux d'une fréquence sont servis et comptés
   (`internal/api/frequency_handlers.go`, `internal/audio/`), et ce qui fixe la limite actuelle. Un
   invité a **au plus deux flux par fréquence** (une page ouverte, plus une seconde le temps d'un
   rechargement), comptés par compte ; au-delà, `429` avec un message clair. Le propriétaire n'est pas
   limité par cette règle. Les flux **mu-law** (`?format=ulaw`, ce que joue la page), MP3 et WAV comptent
   pareil. Tests : le troisième flux d'un invité est refusé, un flux fermé libère sa place, le
   propriétaire n'est pas touché.
4. **Le 429 de la limite des essais** : la connexion bloquée répond `429` avec `Retry-After`, et
   `www/login.js` l'affiche (vérifie qu'il le gère déjà ; sinon, corrige-le). Test.
5. **Le rapport**, `docs-fr/fiches-cloud/P-rapport.md`, en français : le tableau de l'étape 1, chaque
   commit avec ses lignes, ce qui a été vérifié et comment, ce qui ne l'a pas été.

## Ce que tu ne fais pas

- Pas de nouveau rôle, pas de nouvelle option de configuration sauf si la limite de flux en exige une
  (alors une seule, avec sa valeur par défaut, documentée dans `configs/config.toml.example`).
- Ne touche pas à la durée des sessions (décision : 30 jours, inchangée), ni à `/vitals` (décision :
  reste public).
- Ne modifie pas `docs/` (l'amont). Aucune adresse réelle dans les tests ni dans le rapport : utilise
  `station.lan`, `192.0.2.x` (adresses de documentation), `example.com`.

## Vérifier

`go build ./...`, `go vet ./...`, `go test ./...` (et `-race` sur `internal/api`, `internal/auth`,
`internal/audio`) ; `node tools/www-check/check.mjs` si tu touches `www/`. La matrice
`internal/api/testdata/access.txt` est régénérée par son test si une route change : le diff va dans le
rapport.
