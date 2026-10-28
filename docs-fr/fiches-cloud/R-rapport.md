# Fiche R — rapport (26/10/2026)

**Branche : `claude/fiche-r-contrat-lanceur-js1njx`**, partie de `main` (c35c309), et non `cloud/r-contrat-lanceur` :
c'est la branche que l'environnement de la session cloud imposait. Rien sur `main`, pas de pull request.

## Les commits

| Commit | Ce qu'il fait | Lignes |
|---|---|---|
| 098ef17 | **Le contrat** (C1), `docs-fr/36-contrat-lanceur.md`, en français : démarrer, canal d'état, prêt ou pourquoi pas, arrêt, santé, processus, compagnons, version ; chaque lecture de fichier du lanceur et ce qui la remplace ; le jeton par appareil pour un lanceur distant, proposé seulement | +166 |
| 31de87f | **co-atc lance ses compagnons** (B3) : `internal/companion` (groupe de processus, relance après 1 s puis délai doublé jusqu'à 2 min, SIGTERM puis SIGKILL à 5 s) ; `[station_link]`, désactivée par défaut, commentée dans l'exemple ; une **clé interne** de 256 bits par démarrage, dans l'environnement de la liaison, qui n'ouvre que ses cinq routes et seulement depuis la boucle locale : **plus de mot de passe** ; la liaison s'arrête seule si co-atc disparaît ; `[server] keep_awake` (`caffeinate -i -w <pid>`, macOS seulement) | +1 465 / −4, 18 fichiers |
| 2764944 | **Le serveur expose le contrat** (C2) : `-status-fd N` (`internal/lifecycle`, une ligne JSON par événement, une **cause** par échec de démarrage) ; les ports ouverts avant « prêt », **le port principal pris devient un refus de démarrer** ; dans `/health/detail` : `contract`, `process`, `companions` ; `receiver.channels` (ce qui est à l'antenne), relayé par la liaison, avec repli pour un co-atc plus ancien ; `[server] instance` | +1 399 / −35, 29 fichiers (575 de tests) |
| 5c90bd4 | **Le test qui garde le contrat** : schéma de `/health/detail` lu dans les types Go et figé dans `internal/api/testdata/health_detail.txt` (107 champs) ; les 19 champs exigés par le décodeur Swift et les 13 optionnels qu'il lit ; une vraie réponse de la pile vérifiée contre le schéma (102 champs sur 107) | +522 |

**Choix de l'étape 3 : ajouter à `/api/v1/health/detail`** plutôt qu'une route voisine. Le lanceur l'interroge déjà
toutes les 15 s, sous la même règle d'accès (cette machine, ou le propriétaire) ; son décodeur ignore les champs
inconnus ; la fiche Q y ajoute aussi `page` et `missing`. `routes.txt` et `access.txt` restent inchangés.
**L'échec au démarrage ne peut pas venir de l'API** (le processus sort avant d'écouter) : d'où le canal d'état, que
le porteur lit sur la sortie d'erreur (`-status-fd 2`) ou sur un descripteur à lui.

## Ce que la moitié Swift devra faire (C3)

La table complète est à la fin du doc 36. En bref : lancer `co-atc -config <absolu> -status-fd 2` sans script ;
attendre `ready` (son `url` remplace `[server] port`) ou lire `fatal` (`cause`, `error`) au lieu de chercher
`\tfatal\t` et « ADS-B source validation failed » dans le journal ; ne plus lire `config.toml` (ports, `static_dir`,
`server_url`, `audio_archive_dir`, `source_type`/`tar1090_base_url`, `sqlite_base_path`, `logging.file`) : tout est dans
`process` ; ne plus lancer `git` ni `lsof`, ne plus lire libproc pour le dossier de travail ni les arguments de
`radio-ctl-sync`/`whisper_server.py` : `process.repository`, `companions[]` (`pid`, `kind`, `mode`, `restarts`) ; la
mémoire et la pression mémoire : `process.memory_bytes`, `companions[].memory_bytes`, `process.memory_pressure` ; la
production reconnue à `process.instance` ; les fréquences à l'antenne à `receiver.channels`, plus `/radio/etat` ;
l'arrêt par SIGTERM à co-atc seul, sans chercher d'orphelins. Le porteur garde sa sonde de la station.

## Vérifié, et comment

- À chaque commit : `go build`, `go vet`, `go test -race ./...` ; compilation `GOOS=windows`, `darwin`, (`freebsd` pour l'API).
- **Le vrai binaire, liaison activée, contre un faux radio-ctl** (test `TestTheStationLinkLivesAndStopsWithTheServer`,
  puis les binaires compilés côte à côte avec un faux radio-ctl et un faux tar1090 en Python) : connexion obligatoire et
  aucun mot de passe donné ; prêt en 0,64 s, les deux sources réglées en 0,56 à 0,66 s ; la liaison tuée est relancée
  **1,01 s** après ; co-atc s'arrête **15 à 36 ms** après SIGTERM, statut 0, sans liaison restante ; co-atc tué net, la
  liaison disparaît **0,96 s** après ; la clé n'apparaît ni dans la commande, ni dans le journal.
- La clé refusée sur toute autre route, depuis ailleurs, à travers un en-tête de mandataire, sous un nom rebondi,
  fausse, ou sans liaison lancée. Le canal d'état : ordre des événements, `ready` à une URL qui répond, `stopping` et
  `stopped` ; `fatal` avec statut 1 pour une source ADS-B qui refuse, un port principal pris, un programme absent (et,
  par accident pendant la vérification, une configuration invalide et un port pris par une instance oubliée).
- Le test du contrat, par mutation puis retour : `bytes_last_60s` renommé, `omitempty` sur `sidecar.status`, et le
  fichier figé réécrit par-dessus un renommage : chaque fois en échec.

## Pas vérifié

- **macOS** : `caffeinate`, la mémoire par `ps` (RSS, et non l'empreinte physique que lisait le lanceur), la pression
  mémoire par `sysctl` : compilés, jamais exécutés. Le réseau local d'une liaison lancée par co-atc sous le porteur.
- La station réelle, la production, le sidecar réel, Windows à l'exécution, le Swift.
- `internal/auth` `TestAChoiceThatCannotBeWrittenChangesNothing` échoue ici **avant comme après** la branche : la
  session tourne en root, que `chmod 0500` n'arrête pas.

## Doutes

1. **Deux liaisons** : ne pas activer `[station_link]` tant que `production.command` lance la sienne.
2. Le port principal pris est désormais fatal (cause `listen`) : avant, co-atc tournait sans lui, et l'autre serveur
   répondait à sa place. Changement de comportement voulu, à valider.
3. Le serveur lance `git` (trois commandes, au plus une fois par minute) seulement s'il trouve un `.git`.
4. `sidecar.go` (contribuable) reçoit un accesseur en lecture (`Child`) et trois compteurs ; on peut l'isoler.
5. Le plan 35 voulait le contrat en anglais ; la fiche le voulait en français dans `docs-fr/` : suivi la fiche.
6. **Fusion avec Q et P** : Q touche `health_detail_handlers.go` (`page`, `missing`) : après fusion, réécrire le
   schéma (`ATC_WRITE_CONTRACT=1`) et vérifier que le diff n'ajoute que ses champs. P touche sans doute le verrou des
   routes, que j'ai remplacé par `RequireSignIn` (`routes.go`).

## À regarder en priorité sur le Mac

1. `go build -o bin/radio-ctl-sync ./cmd/radio-ctl-sync` à côté de `bin/co-atc` (les scripts de build ne le font pas).
2. Une instance d'essai avec `[station_link]` et `keep_awake`, station prévenue : `pmset -g assertions`, la liaison
   sous le porteur, `/health/detail` comparé à ce que montre le lanceur (mémoire surtout).
3. Ajouter le doc 36 à `docs-fr/README.md` et l'état de la fiche R au tableau des fiches (non touchés ici).
