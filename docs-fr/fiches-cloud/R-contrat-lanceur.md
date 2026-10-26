# Fiche R — Le contrat entre le lanceur et le serveur, côté serveur

*Écrite le 26/10/2026 au soir. Plan : `docs-fr/35-plan.md`, blocs B (étape B3) et C (étapes C1 et C2),
décisions Q59 à Q61 dans `docs-fr/05-decisions.md`. Le propriétaire a fixé l'ordre le 26/10 : **le
lanceur d'abord**, puis le serveur autonome. Lis d'abord les règles communes de `README.md`, puis
`docs-fr/34-lanceur.md`.*

## Pourquoi

Le lanceur macOS (`tools/menubar/`, Swift, application « Clairance ») démarre la production, la surveille
et prévient le propriétaire quand elle devient sourde. Pour cela, il **lit les fichiers du serveur** : sa
configuration TOML (avec un parseur maison), ses journaux, un script shell hors du dépôt, git, la table
des processus. Chaque changement du serveur peut le casser sans qu'aucun test ne le voie ; un ami qui
installerait le lanceur sur son Mac pour regarder une production distante ne pourrait rien lire de tout
cela. Le but (plan 35, bloc C) : **le lanceur démarre un seul programme et lit son API ; il ne lit plus
aucun fichier du serveur.** Cette fiche fait la moitié serveur. La moitié Swift sera faite sur le Mac,
par l'agent du lanceur (tu ne peux pas compiler du Swift macOS ; ne touche pas à `tools/menubar/`).

## Ce que le lanceur lit aujourd'hui (inventaire du 26/10, à revérifier dans `tools/menubar/`)

- **La configuration de chaque co-atc** (`Clairance.swift`, parseur TOML maison) : `[server] port`,
  `additional_ports`, `static_dir` ; `[transcription.local] server_url`, `audio_archive_dir` ;
  `[adsb] source_type`, `tar1090_base_url` ; `[storage] sqlite_base_path` ; `[logging] file`.
- **Des journaux** : `runs/production.log` (il y cherche « arret demande », « co-atc pret » et la cause
  d'un échec), le journal de co-atc (il y cherche `\tfatal\t`, « ADS-B source validation failed », le
  champ JSON `error`), la sortie standard de co-atc retrouvée par `lsof`.
- **Git** dans le dépôt : commit et branche, pour dire de combien le binaire qui tourne est en retard.
- **Les processus** : `co-atc`, `radio-ctl-sync`, le Python du sidecar (`whisper_server.py`), le script
  de production comme parent ; le PID du sidecar par `lsof` sur son port.
- **L'API** : `/api/v1/health` (seul `last_fetch` sert), `/api/v1/vitals` (`version`, `commit`,
  `seconds_per_transmission`, `aircraft`, `transmissions_today`, `matched_share`),
  `/api/v1/health/detail` (`internal/api/health_detail_handlers.go` ; ses champs sont décodés en bloc
  par le lanceur : **un champ non optionnel qui disparaît casse tout le détail**), le `/health` du sidecar,
  et la station directement (`radio-ctl /radio/etat`).
- **Le démarrage** : le lanceur exécute `runs/production.command` (hors dépôt) dans un « porteur ». Ce
  script attend qu'aucun autre co-atc ne tourne (par `pgrep -f` sur sa ligne de commande — un piège :
  toute commande qui contient ce texte le bloque), recopie `www/` dans `runs/production/www`, lance
  `./bin/co-atc -config configs/config.toml` avec `caffeinate`, attend `/api/v1/health`, se connecte avec le
  mot de passe lu dans `~/.config/clairance/coatc-password`, puis lance `bin/radio-ctl-sync` (la liaison
  avec la station : elle règle les sources de co-atc sur ce que la station diffuse ; options dans
  `cmd/radio-ctl-sync/main.go`, mot de passe dans `COATC_PASSWORD`). À l'arrêt, son `trap` arrête la
  liaison et co-atc.

### Complément de l'agent du lanceur (26/10, relu dans le Swift de la branche `lanceur`)

Lectures oubliées ci-dessus, à couvrir aussi dans le contrat :

1. **Git dans `static_dir`** : la ligne « Page: local @ <commit> » vient de `git rev-parse` dans le dossier
   de la page. Le champ `page` de la fiche Q la remplacera : cite-le dans le contrat.
2. **Le dossier de travail de chaque co-atc** (lu par libproc), pour résoudre ses chemins relatifs
   (`-config`, `static_dir`, `sqlite_base_path`, `logging.file`). Le contrat donne des chemins absolus.
3. **Les arguments exacts des compagnons** : pour la liaison, `-coatc <url>` (à quel co-atc elle rapporte)
   et `-once` (« station link, once (should run continuously) ») ; pour le sidecar, le fichier lancé
   (`whisper_server.py` ou `fake-sidecar.py` : « real model » ou « fake ») et `--save-audio <chemin>`
   (d'où le lanceur tire le volume de l'archive quand `audio_archive_dir` est vide). **Pour chaque
   compagnon : PID, rôle, genre (vrai modèle ou faux), et pour la liaison son mode.**
4. **La production se reconnaît** aujourd'hui à son chemin de configuration (`<dépôt>/configs/config.toml`) :
   le contrat donne un **rôle ou un nom d'instance** (production ou essai), dit par le serveur (réglage de
   configuration).
5. **Le journal quotidien de co-atc**, dont le lanceur reconstruit le nom (`<base>-AAAA-MM-JJ<ext>`, d'après
   `pkg/logger`) : le contrat donne son chemin, et surtout la cause d'un échec (étape 3).
6. **La mémoire** de co-atc et du sidecar (lue par PID) et la pression mémoire du système : le serveur
   peut les donner (un lanceur distant ne le pourra pas).
7. **Les compagnons orphelins** (sidecar ou liaison sans co-atc), trouvés dans la table des processus :
   dis dans le contrat ce que le serveur garantit à l'arrêt pour qu'il n'y en ait plus.
8. **La page de la station** (`/radio/etat` : mode et fréquences à l'antenne), lue directement par le
   lanceur : le `receiver` de `/health/detail` en couvre une partie, pas la liste des fréquences à
   l'antenne ; la liaison la connaît, le serveur peut la relayer.

Hors du serveur, à noter dans le contrat sans rien y faire : le lanceur écrit une ligne dans le planning
GPU du labo, et lance les instances d'essai avec le binaire posé à côté de leur configuration, sortie
dans `co-atc.log` au même endroit.

**Les champs que le décodeur Swift exige** dans `/api/v1/health/detail` (tous les autres sont
optionnels ; un seul manquant et tout le détail est perdu, le lanceur affiche « Audio not watched ») :
`pid`, `uptime_s`, `frequencies`, `audio_archive`, `databases` ; `adsb.ok`, `adsb.aircraft`,
`adsb.messages_per_second` ; `frequencies[].id`, `.name`, `.status`, `.transcribes`, `.bytes_last_60s` ;
`sidecar.reachable`, `sidecar.status` ; `audio_archive.available`, `audio_archive.cap_bytes`. Les objets
`adsb` et `sidecar` sont optionnels. **Le test figé du schéma (étape 4) garde au moins ceux-là.**

## Les étapes (un commit chacune, sur `cloud/r-contrat-lanceur`)

1. **Le contrat, écrit** (C1) : `docs-fr/36-contrat-lanceur.md`, en français, court. Ce que le lanceur
   peut demander au serveur et ce que le serveur promet : démarrer (un seul programme, ses arguments),
   s'arrêter (quel signal, en combien de temps, ce qui est arrêté avec lui), être prêt (comment le savoir,
   et pourquoi il ne l'est pas : erreur fatale avec sa cause), la santé (le détail actuel), la
   description du processus (ports, chemins de la configuration, du journal, des bases, de la page,
   processus enfants avec leur PID et leur rôle), la version (commit, et s'il diffère du dépôt quand le
   serveur le sait). Pour chaque point lu aujourd'hui dans un fichier (liste ci-dessus), dis par quoi
   il est remplacé. Les champs existants de `/health/detail` **ne changent pas de nom ni de type** : on
   ajoute, on ne casse pas (le lanceur en production les décode). Accès : comme `/health/detail`
   aujourd'hui (boucle locale sans en-tête de mandataire, ou session propriétaire) ; propose dans le
   contrat ce qu'il faudrait pour un lanceur distant (jeton par appareil), **sans l'implémenter**.
2. **co-atc lance ses compagnons** (B3). Une section de configuration pour la liaison avec la station
   (adresse de radio-ctl, en-tête `Host`, base des flux Icecast, intervalle ; valeurs d'exemple
   génériques : `http://station.lan`), sur le modèle de `[transcription.local].command` et de
   `internal/transcription/sidecar.go` (groupe de processus, arrêt par SIGTERM puis kill, redémarrage si
   elle meurt, avec un délai qui s'allonge). **La liaison n'a plus besoin du mot de passe du propriétaire** :
   co-atc lui donne de quoi s'authentifier sans qu'il soit écrit nulle part (par exemple un jeton à usage
   interne, aléatoire à chaque démarrage, passé par l'environnement et accepté seulement depuis la boucle
   locale pour les routes dont la liaison a besoin : regarde lesquelles elle appelle). Sur macOS,
   l'empêchement de veille (aujourd'hui `caffeinate` dans le script) est fait par co-atc s'il est demandé
   dans la configuration ; sur les autres systèmes, rien. Tout cela est **désactivé par défaut** dans
   `configs/config.toml.example` (un utilisateur sans station radio-ctl ne doit rien voir changer).
   Tests : la liaison démarre, meurt et redémarre, s'arrête avec co-atc ; le jeton ne vaut que pour ses
   routes et depuis la boucle locale ; un faux radio-ctl dans les tests (aucun réseau réel).
3. **Le serveur expose le contrat** (C2) : ce que l'étape 1 décrit et qui manque (la description du
   processus, l'état de démarrage avec la cause d'un échec, les compagnons), en **ajoutant** à
   `/api/v1/health/detail` ou par une route voisine, à ton choix argumenté dans le rapport.
4. **Le test qui garde le contrat** : un test Go qui démarre la pile (`newStack` dans `internal/api`) et
   vérifie chaque champ promis, avec son type, et un fichier figé du schéma (à la manière de
   `internal/api/testdata/routes.txt` et `access.txt`) : un champ retiré ou renommé fait échouer le test.
5. **Le rapport**, `docs-fr/fiches-cloud/R-rapport.md`, en français : chaque commit avec ses lignes, ce que
   la moitié Swift devra faire (la liste des lectures de fichiers à remplacer, champ par champ), ce qui
   est vérifié et comment, ce qui ne l'est pas.

## Ce que tu ne fais pas

- Ne touche pas à `tools/menubar/` (Swift), ni à `docs/` (l'amont).
- Ne supprime rien dont la production actuelle a besoin : le script de production continue de marcher
  tant que le lanceur n'est pas passé au contrat. Les nouveautés sont désactivées par défaut.
- Une autre fiche (Q, en parallèle) incorpore la page dans le binaire et ajoute à `/api/v1/health/detail`
  les champs `page` (d'où vient la page) et `missing` (ce qui manque au démarrage). **Ne les écris pas** ;
  mentionne-les dans le contrat comme prévus. Si sa branche `cloud/q-serveur-autonome` existe déjà sur
  `origin` quand tu commences, pars de `main` quand même : la fusion se fera sur le Mac.
- Aucune adresse réelle : `station.lan`, `audio.lan`, `192.0.2.x`.

## Vérifier

`go build ./...`, `go vet ./...`, `go test ./...` (et `-race` sur les paquets touchés) ; le vrai binaire
démarré avec la liaison activée contre un faux radio-ctl local : elle démarre, règle les sources, meurt
quand on la tue, redémarre, s'arrête avec co-atc.
