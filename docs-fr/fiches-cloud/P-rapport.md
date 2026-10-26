# Rapport de la fiche P — le compte invité : rien de la maison, des flux bornés (26/10)
**Branche : `claude/fiche-p-execution-0r9rbm`** (la session cloud impose la sienne, pas `cloud/p-invite`). Partie de `main`
(`f38f189`) : 5 commits et ce rapport, rien sur `main`. Go 1.24.7, en root. +981 / −31 lignes dans 21 fichiers, dont 645 de tests.

## Étape 1 — ce qu'un invité lisait (mesuré, figé dans `internal/api/testdata/guest.txt`)
Fait par le code, pas à la main : une instance de test met une adresse ou un chemin partout où le serveur peut en porter
(récepteur, sidecar, source ajoutée, sur la boucle locale ; fichiers sous son répertoire ; rapport de radio-ctl-sync nommant
`station.lan` et `192.0.2.20`), puis les fait échouer. Chaque route GET lisible par un viewer (la marche du routeur de la
matrice O) et le WebSocket sont lus en viewer et en propriétaire ; toute chaîne portant une IPv4, le répertoire ou
`station.lan` est relevée. Avant l'étape 2, le viewer lisait les treize :

| Route ou message | Champ | Ce qu'il porte | Ce que la page en fait pour l'invité |
|---|---|---|---|
| `/frequencies`, `/frequencies/{id}` | `url` | l'adresse du flux de la station | rien (elle joue `stream_url`) |
| idem, et WS `frequency_status` | `last_error`, `error` | `Get "http://<station>:8000/…": dial tcp <IP>…` (décodeur Go) | « Connection failed: … » sur la tuile |
| `/adsb/source` | `aircraft.last_error` | l'URL du tar1090 de la station | panneau Réglages › Reports |
| `/server` | `storage.dir`, `storage.active_db`, `logging.file` | chemins du répertoire personnel | rien |
| `/server` | `transcription.detail` | adresse du sidecar, ou chemin du modèle | Reports |
| `/config` | `storage.sqlite_base_path` | chemin | rien (la page n'appelle pas `/config`) |
| `/status` | `receiver.detail` | `station not reached: Get "http://station.lan/radio/etat": dial tcp <IP>…` | infobulle « Antenna » |
| `/aip-names` | `last_error` (et `skipped`) | chemin de la liste (`mkdir …: not a directory`) | Reports |

Gardés : la position de la station (`/station`, `receiver.data.lat/lon` : la carte en a besoin, publique via FlightAware),
version et commit (publics par `/vitals`), `transcription.backend` (nom du GPU), `failing_why` (déjà sans adresse), les noms
des fichiers du jour (sans chemin). Aucun `os.Hostname` dans le serveur. Mesuré sur le vrai binaire : l'erreur d'une source
injoignable porte son URL et son IP, par le décodeur Go de production.

## Commits (lignes ajoutées / retirées)
- `8f1972f` **étape 1**, l'inventaire figé (+325 −11) ; la pile de test gagne `adsbEvery`. Stable 5 fois sous `-race`.
- `d884f9a` **étape 2**, le masquage (+410 −20). Une table, `guestHidden` (`internal/api/guest.go`), par motif de route chi
  comme `viewerMay`, et par type de message pour le WebSocket ; un middleware, `GuestView`, après `ViewerGuard`, qui ne retient
  que la réponse d'un invité sur ces sept routes, vide les chemins JSON nommés (chaîne `""`, liste `[]`, objet `{}`) et la
  réécrit ; une réponse non JSON passe telle quelle, une JSON illisible n'est pas envoyée (500). Le `if` des noms de comptes
  (O, `2faa6ba`) entre dans la table. Le hub WebSocket gagne `HandleConnectionFiltered` (une copie par client filtré, le message
  diffusé n'est jamais modifié). `guest.txt` passe à « - owner » sur les treize lignes et le test échoue si un viewer lit quoi
  que ce soit. Tests : la page de l'invité (fréquences, récepteur, ADS-B, chiffres des réglages, échec sur le WebSocket, le son
  lui-même), la table ne nomme que des routes GET montées, `emptied` ne modifie jamais ce qu'on lui donne, un client filtré.
- `dc2dd36` **étape 3**, deux flux par fréquence (+151). **Constat** : la limite du rapport O (10 par fréquence, 100 en tout,
  `frequencies.GetAudioStream`) ne vaut que pour le WAV ; le MP3 et le mu-law, ce que joue la page, n'en avaient aucune.
  `StreamAudio` compte désormais les flux d'un invité par compte et fréquence, avant le choix du format : au troisième, `429`
  « 2 streams of this frequency are open for this account, the most it may hold: close another page or tab that plays it ».
  Deux navigateurs du même invité partagent les deux places ; HEAD n'en prend pas ; le propriétaire n'est pas compté. Aucune
  option. Test rouge avec la limite à 100, vert à 2.
- `18893e8` **étape 4**, le 429 de la connexion (+103 −11) : `Retry-After` = secondes jusqu'à la sortie de la fenêtre du plus
  ancien essai qui bloque (arrondi au-dessus, 1 au moins). Un nom inconnu depuis l'adresse bloquée reçoit la même chose : le
  blocage est celui de l'adresse. `login.js` gérait déjà le 429 ; il lit maintenant `Retry-After` (« Try again in 15
  minutes. »). Trois tests attendaient 401 une fois bloqué : ils attendent 429. `tailwind.css` reconstruit pour son empreinte
  seule (CSS identique à l'octet).
- `8ffd1fd` le README (mandataire) et l'exemple (`[auth]`) disent ce qu'un viewer ne lit plus, et ses deux flux (+5 −2).

## Vérifié, et comment
- `go build ./...`, `go vet ./...` ; `go test ./...` vert sauf `TestAChoiceThatCannotBeWrittenChangesNothing`, **déjà rouge sur
  `main`** en root (rapport O) ; `-race` vert sur `internal/api`, `internal/auth` (hors ce test), `internal/audio`,
  `internal/websocket`. `node tools/www-check/check.mjs` : OK. La matrice `access.txt` est **inchangée** (aucune route touchée).
- **Le vrai binaire**, compilé, avec de faux tar1090 et sidecar (`tools/startup-bench/fake.py`), un flux MP3 en boucle (le
  fichier de test goxiph) et une source injoignable, comptes `pierre` et `guest` (viewer), `audio_replay = "all"` : au `curl`,
  le propriétaire lit l'URL et l'erreur `dial tcp 127.0.0.1:…`, l'invité des chaînes vides ; puis **dans Chromium**
  (Playwright) : la page de l'invité charge, ses fréquences sont là (statut, `stream_url`), le WebSocket s'ouvre, aucune erreur
  après connexion hormis le 404 de la fausse météo ; « 127.0.0.1 » est dans le texte de la page du propriétaire, jamais dans
  celle de l'invité ; trois flux mu-law ouverts depuis la page : 200, 200, 429 et son message ; neuvième essai de connexion :
  429, `Retry-After: 895`, « Too many attempts. Try again in 15 minutes. »
## Pas vérifié
Le son entendu dans un navigateur, un vrai Traefik, Safari et l'iPhone, la station, la production ; la carte n'a pas été
regardée (un avion reçu, c'est tout). Sur un 429, `live-player.js` réessaie à 1, 2, 5 puis 10 s (lu, pas observé).

## Doutes et constats
- Ce que perd l'invité : la raison d'un échec (« Connection failed: unknown error. », « Failed: no stream »), le détail de
  l'antenne, celui du sidecar et de la liste AIP dans Reports. Ce sont des textes d'erreur libres : vidés en entier plutôt
  que nettoyés par motif, sûr mais moins bavard. Le panneau Réglages reste montré à l'invité (la page ne lit pas le rôle).
- `receiver.data` et `stats.data` de `/adsb/source` sont les fichiers de tar1090 relayés tels quels : rien d'une adresse dans
  ceux d'aujourd'hui, mais un tar1090 futur pourrait y ajouter un champ ; l'inventaire ne le verrait qu'avec une valeur de test.
- Pour le propriétaire, le MP3 et le mu-law restent sans borne (hors fiche). `TestTheProcessorDecodesInGoWithoutFFmpeg`
  (`internal/audio`, non touché) a échoué une fois, lancé avec `internal/api` sous `-race` (« 2 connections, want the drops
  taken again ») ; vert six fois seul, avec et sans ces commits : instable sous charge.

## À regarder en priorité sur le Mac
1. L'instance d'essai **en invité, à travers Traefik**, au navigateur et à l'iPhone : le son (D2), une source en échec, rien de
   cassé dans Réglages. 2. Recharger la page de l'invité plusieurs fois de suite pendant l'écoute : la seconde place suffit-elle
   derrière Traefik, ou un ancien flux tarde-t-il à se fermer (429 au rechargement) ? 3. Le diff de `guest.txt` et
   d'`access.txt` à chaque changement de route. 4. Le message de blocage dans la vraie page de connexion.
