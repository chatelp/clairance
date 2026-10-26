# 36 — Le contrat entre le lanceur et le serveur

*Écrit le 26/10/2026 par la fiche cloud R (plan 35, étapes B3, C1 et C2 ; Q60, Q61). Ce que le lanceur peut
demander à co-atc et ce que co-atc promet, pour que le lanceur **démarre un seul programme et lise son API**, sans
plus lire aucun fichier du serveur. Côté serveur : fait sur la branche de la fiche R. Côté Swift (C3) : à faire sur
le Mac, d'après la table « Ce que chaque lecture devient ». **Version du contrat : 1** (champ `contract` de
`/api/v1/health/detail`).*

## Les règles

- **On ajoute, on ne casse pas.** Un champ promis ici ne change ni de nom ni de type ; un champ nouveau est
  optionnel pour le lanceur (le décodeur Swift ignore ce qu'il ne connaît pas). Le schéma de
  `/api/v1/health/detail` est figé dans `internal/api/testdata/health_detail.txt` ; un champ retiré, renommé ou
  changé de type fait échouer `go test ./internal/api` (étape 4 de la fiche).
- Heures en RFC 3339 UTC ; **une valeur inconnue est absente, pas à zéro** (règle de `/health/detail` depuis le 24/10).
- Chemins **absolus** : le lanceur n'a plus à connaître le dossier de travail de co-atc.
- **Accès** : inchangé. `/api/v1/health/detail` répond à la machine elle-même sans session (connexion de 127.0.0.1
  ou ::1, Host de boucle locale, aucun en-tête de mandataire) ou à une session propriétaire ; 403 à tout autre.

## 1. Démarrer

```
co-atc -config <chemin absolu du fichier> [-status-fd N]
```

Rien d'autre : ni PATH (co-atc trouve ses outils lui-même, `pkg/tool`), ni script, ni mot de passe. co-atc lance
avec lui, selon sa configuration, ses **compagnons** :

| Compagnon | Réglage | Remplace |
|---|---|---|
| le sidecar de transcription | `[transcription.local] command` (existait) | — |
| la liaison avec la station (`radio-ctl-sync`) | `[station_link]`, désactivée par défaut | son lancement par `runs/production.command`, avec l'adresse de la station en dur et le mot de passe lu dans `~/.config/clairance/` |
| l'empêchement de veille (macOS, `caffeinate -i -w <pid>`) | `[server] keep_awake = true` ; ignoré ailleurs | `caffeinate` du script |

La liaison s'authentifie par une **clé interne**, tirée au hasard à chaque démarrage, passée par son environnement
(`COATC_LINK_TOKEN`), écrite nulle part. Elle n'ouvre que les cinq routes que la liaison appelle (`GET /frequencies`,
`PUT /frequencies/{id}/label`, `PUT` et `DELETE /sources/{id}`, `PUT /receiver`), et seulement depuis la boucle
locale. La liaison n'a plus besoin du compte du propriétaire. `[server] instance` donne le **nom de l'instance**
(« production », « essai »…), que le lanceur lisait jusqu'ici dans le chemin de la configuration.

## 2. Le canal d'état (`-status-fd N`)

Facultatif. co-atc écrit sur le descripteur N (1 ou 2 sous Windows ; n'importe lequel ailleurs) **une ligne JSON
par événement**, qui commence toujours par `{"coatc_status":`. Le lanceur, **ou le porteur qui lance co-atc pour
lui** (c'est le cas sur ce Mac : le menu n'est pas le parent de co-atc), donne `-status-fd 2` et lit la sortie
d'erreur. Le porteur range les lignes `{"coatc_status":` dans un fichier d'état que le lanceur suit, avec le statut
de sortie de co-atc, qu'il est seul à connaître : un lanceur rouvert retrouve ainsi l'histoire : les autres lignes (sortie du sidecar, panique de Go) ne
commencent pas ainsi.

| Événement | Champs | Quand |
|---|---|---|
| `starting` | `pid`, `version`, `commit`, `config` | tout de suite |
| `step` | `step` (`adsb_source`, `sidecar`, `storage`, `services`, `listen`, `station_link`), `timeout_s` éventuel | au début de chaque étape longue (le sidecar peut prendre 5 min) |
| `ready` | `pid`, `url` (l'écoute principale, vue de cette machine), `listeners[]` | tout est démarré, les ports sont ouverts |
| `fatal` | `cause`, `error` | juste avant de sortir en échec |
| `stopping`, `stopped` | — | à l'arrêt demandé, et quand tout est arrêté |

## 3. Prêt, ou pourquoi il ne l'est pas

- **Prêt** : l'événement `ready`, ou `GET /api/v1/health` qui répond 200 à son `url`. Les écoutes s'ouvrent après
  tout le reste : une réponse veut dire que tout a démarré.
- **Échec au démarrage** : co-atc **sort avec un statut non nul** (1), après l'événement `fatal`. Le journal garde ses
  messages d'aujourd'hui, mais le lanceur n'a plus à les y chercher.

| `cause` | Ce qui a manqué |
|---|---|
| `config` | fichier introuvable ou invalide (le message dit la clé) |
| `logger` | le journal ne peut pas être créé |
| `adsb_source` | la source ADS-B ne répond pas (ex-« ADS-B source validation failed » ; `error` dit « no route to host », « connection refused »…) |
| `sidecar` | aucun sidecar ne répond, ou le nôtre est mort au démarrage |
| `station_link` | le programme de la liaison est introuvable ou ne démarre pas |
| `storage` | la base du jour ne s'ouvre pas |
| `adsb_service`, `weather`, `frequencies` | un service ne démarre pas |
| `auth`, `origins` | comptes illisibles, origines invalides |
| `no_account` | aucun compte, et le serveur n'écoute pas que sur la boucle locale |
| `listen` | **le port principal est pris** (avant la fiche R, co-atc continuait sans lui) |

Un port annexe pris n'arrête pas le serveur : il est dit dans `process.listeners[]`.

## 4. S'arrêter

**SIGTERM** (ou SIGINT, SIGHUP). co-atc arrête, dans l'ordre : la liaison, les fréquences, le sidecar, l'ADS-B, les
écoutes HTTP, puis l'empêchement de veille ; chaque compagnon reçoit SIGTERM **dans son groupe de processus**, puis
SIGKILL après 5 s. Durée bornée : 5 s par compagnon et 10 s pour les écoutes ; mesurée sur la fiche R (rapport R).
Le statut de sortie est 0.

**Pas d'orphelin**, même si co-atc est tué (SIGKILL) : le sidecar et la liaison surveillent leur parent
(`COATC_SPAWNED=1`) et s'arrêtent seuls quand il disparaît (en 5 s au plus pour le sidecar, 2 s pour la liaison) ;
`caffeinate -w` part avec le PID qu'il suit. Le lanceur n'a plus à chercher d'orphelins dans la table des processus.

## 5. La santé

`GET /api/v1/health/detail`, tel qu'au 24/10 (doc 34). **Champs que le décodeur Swift exige**, gardés par le test :
`pid`, `uptime_s`, `frequencies`, `audio_archive`, `databases` ; `adsb.ok`, `adsb.aircraft`,
`adsb.messages_per_second` ; `frequencies[].id`, `.name`, `.status`, `.transcribes`, `.bytes_last_60s` ;
`sidecar.reachable`, `sidecar.status` ; `sidecar.queue.first`, `.second` ; `audio_archive.available`,
`audio_archive.cap_bytes`.

Ajouté par la fiche R : `contract` (1) ; `receiver.channels[]` (`id`, `designation`, `mhz`, `stream`), **ce qui est à
l'antenne**, relayé par la liaison depuis `/radio/etat` ; une liaison qui ne rapporte plus se voit à `receiver.stale`.

## 6. La description du processus : `process`

| Champ | Contenu |
|---|---|
| `instance` | `[server] instance` ; absent s'il n'est pas réglé |
| `started_at`, `ready_at` | le processus lancé ; tout démarré, les ports ouverts |
| `executable`, `working_dir`, `config` | chemins absolus |
| `listeners[]` | `addr` (telle que configurée), `url` (vue de cette machine), `primary`, `ok`, `error` |
| `paths` | `static_dir`, `databases_dir`, `database` (celle du jour), `log` (le fichier du jour, rotation comprise ; absent si le journal ne va qu'à la sortie standard), `audio_archive` (absent sans archive) |
| `sidecar_url` | `[transcription.local] server_url` |
| `adsb_source` | `type` ; `url` pour `tar1090` (`…/aircraft.json`) et `readsb-api`, absente pour les sources externes |
| `footprint_bytes` | **macOS seulement** : l'empreinte physique de co-atc (`proc_pid_rusage`, `ri_phys_footprint`, ce que le Moniteur d'activité appelle sa mémoire). **Absente ailleurs** plutôt que fausse : la mémoire résidente ne compte pas ce qu'un modèle tient sur le GPU (26/10, le sidecar de production : 93 Mo résidents pour 8 879 Mo d'empreinte). Corrigé le 26/10 après relecture par l'agent du lanceur ; le champ s'appelait `memory_bytes` (mémoire résidente) avant toute mise en production |
| `memory_pressure` | macOS : `normal`, `warn` ou `critical` (`kern.memorystatus_vm_pressure_level`, la lecture du lanceur) |
| `repository` | quand co-atc tourne dans un dépôt git : `dir`, `branch`, `head`, `commits_behind` (de combien le binaire est en retard ; absent si son commit n'est pas dans la branche) |

## 7. Les compagnons : `companions[]`

Un objet par compagnon : `role` (`sidecar`, `station_link`, `keep_awake`), `managed` (co-atc l'a lancé et le
surveille ; faux pour un sidecar lancé par quelqu'un d'autre, listé seulement quand il répond), `pid` (pour le
sidecar, celui qu'il déclare : le processus Python), `state` (`running`, `restarting`, `stopped`, `failed` : le
sidecar mort cinq fois en dix minutes n'est plus relancé), `started_at`, `restarts`, `last_exit`, `last_output` (sa
dernière ligne, pour la cause), `next_start_at` (pendant le délai avant une relance), `command` (la clé de la
liaison n'y est jamais : elle passe par l'environnement), `kind` (sidecar : `model` ou `fake`, d'après ce qu'il
déclare), `mode` (liaison : `continuous`), `footprint_bytes` (macOS seulement, comme pour `process`). Une liaison qui meurt est relancée après 1 s, puis un
délai qui double tant qu'elle meurt en moins d'une minute, jusqu'à 2 min.

## 8. La version

`version` et `commit` (celui du binaire, depuis le 26/10) ; `process.repository` dit s'il diffère du dépôt.
`page` (d'où vient la page) et `missing` (ce qui manque au démarrage) sont **prévus par la fiche Q**, en parallèle.

## Ce que chaque lecture devient

| Le lanceur lit aujourd'hui | Il lira |
|---|---|
| `[server] port` (et l'URL de tous ses appels) | `url` de l'événement `ready`, puis `process.listeners[]` |
| `[server] additional_ports` | `process.listeners[]` |
| `[server] static_dir` et git dans ce dossier (« Page: ») | `process.paths.static_dir` ; `page` (fiche Q) |
| `[transcription.local] server_url`, lsof sur son port, `/health` du sidecar | `process.sidecar_url` ; `companions[role=sidecar]` (pid, genre) ; le bloc `sidecar` |
| `[transcription.local] audio_archive_dir`, `--save-audio` du sidecar | `process.paths.audio_archive`, `audio_archive.available` |
| `[adsb] source_type`, `tar1090_base_url` | `process.adsb_source` |
| `[storage] sqlite_base_path` | `process.paths.databases_dir`, `database` |
| `[logging] file`, nom du jour reconstruit | `process.paths.log` |
| `runs/production.log` (« arret demande », « co-atc pret ») | les événements `stopping`, `stopped`, `ready` ; le statut de sortie |
| journal de co-atc (`\tfatal\t`, « ADS-B source validation failed », champ `error`) | l'événement `fatal` (`cause`, `error`) |
| sortie standard de co-atc retrouvée par `lsof` | `process.paths.log` |
| git dans le dépôt (commit, branche, retard) | `commit`, `process.repository` |
| le dossier de travail (libproc), pour les chemins relatifs | chemins absolus partout |
| la production reconnue au chemin de sa configuration | `process.instance` |
| les processus `radio-ctl-sync` (`-coatc`, `-once`), `whisper_server.py` / `fake-sidecar.py` | `companions[]` (`role`, `pid`, `kind`, `mode`) |
| le script de production comme parent | plus de script : le porteur lance co-atc |
| la mémoire par PID, la pression mémoire | `process.footprint_bytes`, `companions[].footprint_bytes` (macOS), `process.memory_pressure` |
| les orphelins dans la table des processus | la garantie du § 4 |
| `/radio/etat` de la station (mode, titre, fréquences) | `receiver.mode`, `receiver.title`, `receiver.channels[]` |

## Hors du serveur (le lanceur garde)

**La découverte des instances qu'il n'a pas démarrées** (lancées par un agent ou d'un double-clic) : localement
seulement, par la table des processus, le nom exact du processus et son port d'écoute ; tout le reste passe par
l'API de chaque instance, `process.instance` disant laquelle est la production. Un lanceur distant (D6) ne connaît
que les instances qu'on lui nomme. (Proposé par l'agent du lanceur, accepté le 26/10.)

**Un co-atc sans `contract`** (d'avant la version 1) n'est pas lu par les fichiers en secours : le lanceur dit
« co-atc trop ancien pour le lanceur », comme pour un 404 sur `/health/detail`, avec un suivi réduit (processus,
`/api/v1/health`). La production passe à la version 1 le 27/10 : la transition ne dure qu'une relance.

Le planning GPU du labo ; la sonde de la station par le porteur avant le lancement (le réseau local de macOS refuse
la première connexion d'un binaire neuf) ; les instances d'essai lancées avec le binaire posé à côté de leur
configuration, sortie dans `co-atc.log` au même endroit ; ses propres réglages et `events.jsonl`.

## Pour un lanceur distant (D6) : proposé, pas implémenté

Le contrat ne change pas, seul l'accès : un **jeton par appareil**, créé par le propriétaire
(`co-atc -add-device <nom>`, affiché une fois), dont seule l'empreinte est gardée à côté des comptes ; envoyé en
`Authorization: Bearer` par HTTPS à travers le mandataire ; il n'ouvre que `/health`, `/health/detail`, `/vitals` et,
plus tard, les routes de commande ; révocable seul, nommé dans la trace des connexions. La mémoire et la pression
mémoire viennent alors du serveur, puisque le lanceur ne voit plus ses processus ; un lanceur distant ne démarre ni
n'arrête rien sans une route de commande, à concevoir à part.
