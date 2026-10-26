# 34 — Le lanceur de la barre des menus, en poste de surveillance de Clairance

*Proposition du 24/10/2026, demandée par le propriétaire : « quelque chose de beaucoup plus
puissant, informatif et utile, peut-être avec une interface propre ». Un agent y sera dédié.
Rien n'est codé avant son accord sur ce document.*

## Pourquoi

Le lanceur actuel (`tools/menubar/Clairance.swift`, 628 lignes, depuis le 10/10) dit si la
production tourne, la démarre et l'arrête, et liste les instances de co-atc. La semaine du
16 au 20/10 a montré ce qui lui manque :

- **Une production sourde ne se voit pas.** Du 16 à 23 h 05 au 18 à 13 h 36, co-atc tournait,
  le lanceur affichait « production running », et aucune transmission n'était transcrite :
  « no route to host » vers la station, chaque seconde, un jour et demi. C'est l'agent de la
  station qui l'a vu, par le nombre d'auditeurs d'Icecast.
- **Une production lancée depuis une session Claude meurt avec elle** (règle 5 de
  `CLAUDE.md`) : pour macOS, son application responsable est celle qui l'a lancée. La
  commande `clairance --start` lancée d'un terminal hérite de ce terminal.
- **Une instance d'essai oubliée tourne des jours** (16 au 18/10), avec deux auditeurs de trop
  sur la station.
- **Un motif de recherche de processus se trompe** : une commande qui contient le motif de la
  production fait croire au lanceur qu'elle tourne (16/10, deux fois).
- **Rien ne dit quelle version tourne** face à ce qui est sur `local`, ni ce que coûte la
  production en mémoire (le sidecar est passé de 14 à 17 Go en 12 h le 15/10, point ouvert).

## Ce qu'on en ferait

Trois rôles, du plus utile au plus confortable.

### 1. Le gardien (ce qui aurait évité le 16-18/10)

- **Lancer la production au nom du lanceur, toujours.** `clairance --start` lancé d'un
  terminal ou par un agent ne démarre plus rien lui-même : il demande au lanceur en marche de
  le faire (communication locale entre processus), et c'est le lanceur, application à part
  entière, qui devient responsable de la production. S'il ne tourne pas, la commande l'ouvre
  par le système (`open`) avant. Ainsi aucune relance de Claude ni fermeture de terminal ne
  coupe plus le réseau local de la production — à condition que le lanceur, lui, reste ouvert
  (mesuré le 24/10, voir « L'étape 1 »).
- **Des instances connues par leur PID, pas par un motif.** Le lanceur note ce qu'il démarre
  (production, instance d'essai) ; il retrouve aussi ce qu'il n'a pas démarré, mais par le port
  et la configuration, jamais par un motif dans la ligne de commande.
- **Une vraie santé, pas seulement « le processus existe »**, vérifiée toutes les 10 à 30 s :
  - la station joignable (ADS-B et `/radio/etat`) depuis la production elle-même : la dernière
    lecture ADS-B réussie, et la dernière erreur ;
  - de l'audio qui arrive sur chaque fréquence à l'antenne (octets reçus par ffmpeg) ;
  - des transcriptions récentes quand une fréquence parle (aucune depuis N minutes alors que
    l'audio arrive : alerte) ;
  - le sidecar vivant, sa file, sa mémoire ;
  - le disque (place libre, archive face à son plafond de 60 Go, bases).
- **Des notifications macOS** quand quelque chose se dégrade, avec la cause en une phrase :
  « Production sourde depuis 3 min : la station ne répond pas (no route to host) » ; « Sidecar
  relancé » ; « Archive à 57 Go sur 60 » ; « Nouveau cycle AIRAC le 29/10 ».
- **Une instance d'essai avec une échéance** : lancée pour une durée, arrêtée à l'heure dite,
  rappelée dans le menu tant qu'elle tourne.
- **Option : démarrer la production à l'ouverture de session**, et la relancer d'elle-même
  après une panne franche (avec un plafond, et une notification à chaque fois). Éteinte par
  défaut : « rien ne démarre tout seul » reste la règle tant que le propriétaire n'en décide
  pas autrement.

### 2. Le tableau de bord (une fenêtre, en plus du menu)

Une fenêtre native (SwiftUI), ouverte depuis le menu, qui dit d'un coup d'œil où en est
Clairance :

| Bloc | Ce qu'il montre |
|---|---|
| **Production** | en marche depuis, version et commit qui tournent, commit de la page figée, ports ; face à `local` : « à jour » ou « 4 commits de retard » |
| **Station** | ce qui est à l'antenne, fréquences et flux, auditeurs Icecast, liaison `radio-ctl-sync`, dernier contact |
| **ADS-B** | avions, messages par seconde, portée, dernière lecture, erreurs |
| **Transcription** | modèles chargés, file, latence médiane, transmissions de la dernière heure, part rattachée à un avion |
| **Ressources** | mémoire de co-atc et du sidecar sur 24 h (courbe), disque libre, archive, bases |
| **Machine** | le planning du GPU (`PLANNING-MACHINE.md`), ce qui l'occupe en ce moment, les autres instances |
| **Journal** | les dernières lignes utiles du journal de co-atc, filtrées (erreurs, avertissements), sans les 86 400 lignes par jour d'une erreur répétée |
| **Événements** | les incidents de la journée : coupures, relances, alertes envoyées |

La fenêtre est **du Clairance en natif macOS** (demande du propriétaire, 24/10) : l'identité de
Clairance — le signe et ses états, la palette, le ton des textes, les nombres de la marque — portée
dans les conventions d'une application Mac (SwiftUI, typographie et contrôles du système, barre de
menus, notifications, mode clair et sombre, Human Interface Guidelines), pas une page web dans une
fenêtre. Sa forme est confiée à la session design, comme celle du menu et des notifications.

### 3. Le poste de commande

- **Démarrer, arrêter, relancer** la production ; ouvrir la page, le journal, le dossier des
  mesures.
- **Mettre à jour** : quand `local` est en avance sur ce qui tourne, « Construire et relancer »,
  avec confirmation, le rappel de prévenir la station, et le compte rendu (tests passés, binaire
  reconstruit, coupure de N secondes).
- **Le téléphone** : l'adresse Tailscale et son code QR, pour ouvrir Clairance d'un geste.
- **Les tâches régionales** : « Mettre à jour depuis le SIA » sans ouvrir la page.

## Ce que ça demande côté serveur

Le lanceur lit aujourd'hui des journaux et des processus. Pour le gardien, un **résumé de santé**
tenu par co-atc lui-même serait plus juste et servirait aussi les voyants de la page : par
fréquence, les octets reçus et la dernière transmission ; la dernière lecture ADS-B réussie et
la dernière erreur ; l'état du sidecar. Une route en lecture seule, réservée à l'accès local ou
au propriétaire (`GET /api/v1/health`, à préciser). C'est la seule modification du serveur ; elle
se fait dans la session principale, en commit à part.

**Faite le 24/10 (3307610) : `GET /api/v1/health/detail`**, format arrêté avec l'agent du lanceur ;
`/api/v1/health` est inchangé. Accès : sans session depuis la machine elle-même (connexion depuis
127.0.0.1 ou ::1, en-tête Host de boucle locale avec ou sans port, aucun en-tête `X-Forwarded-For`,
`Forwarded`, `X-Real-IP` ni `X-Forwarded-Host`), ou par la session du propriétaire ; 403 sinon. Une
réponse sert 5 s ; le sidecar est relu au plus toutes les 15 s (même lecture que `/status`) ; aucune
requête en base. Heures en RFC 3339 UTC ; une valeur inconnue est absente, pas à zéro.

| Clé | Contenu |
|---|---|
| `pid`, `uptime_s`, `version`, `commit` | le processus co-atc |
| `adsb` | `ok`, `last_fetch`, `failing_since`, `failing_why`, `aircraft`, `messages_per_second` |
| `frequencies[]` | `id`, `name`, `status`, `error`, `transcribes`, **`bytes_last_60s`**, `last_byte_at` (sortie de ffmpeg), `last_segment_at` (dernière transmission envoyée au sidecar), `last_transcribed_at` (dernier texte rendu) |
| `sidecar` | `reachable`, `status` (`off` si rien ne transcrit), `degraded`, `pid`, `queue {first, second}`, `second_pending`, `archiving`, `archiving_stopped_low_disk`, `read_at` |
| `receiver` | le dernier rapport de `radio-ctl-sync`, comme `/status`, ou `null` |
| `audio_archive` | `available`, `bytes` et `measured_at` (relevés par la purge horaire, absents sans plafond), `cap_bytes`, `disk_free_bytes` |
| `databases` | `dir_bytes` (bases quotidiennes et leurs journaux), `disk_free_bytes` |

La mémoire du sidecar ne figure pas dans la réponse : le serveur ne la connaît pas, et le lanceur
la lit lui-même par le PID.

## Comment on le mène

- **Un agent dédié**, en Swift et SwiftUI, sur `tools/menubar/` ; la session principale tient le
  résumé de santé du serveur ; **la session design** dessine tout ce qui se voit — l'icône et ses
  états, le menu, les notifications, la fenêtre — en Clairance natif macOS, et relit chaque
  livraison. L'agent s'y coordonne **dès l'étape 1**, pas seulement pour la fenêtre.
- **Par étapes, chacune utile seule** :
  1. le gardien : lancement au nom du lanceur, instances par PID, santé et notifications. C'est
     la priorité : il répond à des incidents vécus.
  2. le tableau de bord : la fenêtre et ses blocs.
  3. le poste de commande : mise à jour, téléphone, tâches régionales.
- **Mesurer avant de conclure**, comme le reste du projet : pour l'étape 1, le temps entre une
  coupure provoquée et la notification (cible : moins de 2 minutes), et l'absence de fausse
  alerte sur une journée normale.

## L'étape 1 : le plan retenu, ce qui est fait, ce qui est mesuré

*Tenu par l'agent du lanceur (session « Plan lanceur de Clairance »), sur la branche `lanceur`.*

### Le plan, accepté par le propriétaire le 24/10

- **Le canal** entre la ligne de commande et le lanceur : une prise Unix dans
  `~/Library/Application Support/Clairance/` (dossier en 700, identité de l'appelant vérifiée).
  Ni port TCP ni schéma d'URL, qu'une page web pourrait atteindre ; ni XPC, qui demanderait un
  agent launchd.
- **`--start` ne lance plus rien lui-même** : il demande au lanceur, l'ouvre par `open` s'il ne
  tourne pas, et échoue en le disant s'il ne s'ouvre pas.
- **Les processus par PID** : nom exact, arguments exacts, heure de départ (contre la réutilisation
  d'un PID) ; plus aucun `pgrep -f`.
- **Les contrôles de santé (étape 1b), toutes les 15 s, une alerte par incident** et une autre au
  retour à la normale ; leurs textes sont ceux de la session design (`design/10-lanceur.md`).

| Contrôle | Lu où | Alerte quand | Détection |
|---|---|---|---|
| co-atc vivant | PID | disparu sans arrêt demandé | ≤ 15 s |
| Le serveur répond | `/api/v1/health` | 3 échecs de suite | ≤ 45 s |
| La production voit la station | `last_fetch` | plus vieux que 30 s, deux relevés de suite ; le lanceur interroge lui-même la station pour départager « la station répond au lanceur, pas à co-atc » et « la station ne répond pas » | ≤ 60 s |
| L'audio arrive | `bytes_last_60s` de `/api/v1/health/detail` | à définir sur mesure, la route étant livrée | ≤ 60 s |
| Liaison avec la station | PID de `radio-ctl-sync` | disparu | ≤ 15 s |
| Transcription | sidecar | 3 échecs ; dégradé (langue) ; relancé (PID changé) | ≤ 45 s |
| Silence | transmissions du jour, station en mode « chaîne » | inchangé 10 min de 8 h à 23 h, 45 min la nuit | 10 / 45 min |
| Disques | externe ; interne | externe absent ; interne sous 20 Go puis 5 Go | ≤ 15 s |
| Mémoire | pression mémoire de macOS | avertissement ou critique pendant 2 min | ≤ 2 min |
| Lancée hors du lanceur | processus responsable | autre que le lanceur | au démarrage |

Seuils du silence relevés sur les bases du 03/10 et du 16/10, recoupés avec les arrêts de
`runs/production.log` : de 8 h à 23 h, le plus long silence d'une heure est de 83 s ; la nuit, il va
jusqu'à 21 min (03/10 vers 5 h). Deux journées, sur les approches de Roissy : à revoir si la
station écoute des fréquences plus calmes.

Constat du même jour, dans le code : `channels_on_air` de `/vitals` ne prouve pas que l'audio
arrive. Pour le serveur, « connected » voulait seulement dire « ffmpeg a démarré », et ffmpeg est
relancé toutes les 5 s. C'est ce qui a motivé `bytes_last_60s`.

### 1a, livrée le 24/10 (4c63292) : le lancement par le lanceur, les processus par PID

- `clairance --start`, `--stop`, `--start-test <config> --for 2h`, `--stop-test <config>` passent par
  le lanceur ; `--status` reste autonome. Une instance d'essai s'arrête seule à son échéance, même si
  le lanceur a été fermé et rouvert entre-temps (`started.json`, à côté de la prise).
- La production est le co-atc dont la configuration résolue est `configs/config.toml` du dépôt ; son
  script est son parent, reconnu à ses arguments ; un PID n'est signalé que s'il est encore le
  processus lu.
- Chaque instance dit de quelle application elle dépend (« Depends on: Clairance »).
- Un lanceur tapé dans un terminal se fait rouvrir par le système et s'efface ; un second lanceur
  s'efface devant le premier.

### Mesures du 24/10

Sur une instance d'essai dédiée (`runs/lanceur/`, port 8013, ADS-B seul, ni audio ni transcription,
donc aucun auditeur de plus sur la station). La production, arrêtée depuis le 18/10, n'a pas été
touchée.

| Mesure | Résultat |
|---|---|
| Qui est responsable d'une instance lancée par le lanceur | le lanceur : pour macOS, le lanceur (ouvert par `open`, parent launchd) répond de lui-même et de l'instance, lu par `responsibility_get_pid_responsible_for_pid` |
| Une instance lancée directement depuis une session Claude | « Depends on: claude » : elle dépend de l'application Claude, comme la production du 16/10 |
| **Le lanceur se ferme** | **l'instance continue de tourner, mais perd le réseau local aussitôt et pour de bon** : dernière lecture ADS-B à 14:01:25,76 UTC, lanceur fermé à 14:01:26, plus aucune lecture en 2 min de suivi ; macOS la rend responsable d'elle-même, et un programme nu n'a pas accès au réseau local. Rouvrir le lanceur ne lui rend rien |
| Échéance d'une instance d'essai | lancée pour 1 min, arrêtée seule à +62 s (vérification toutes les 5 s) |
| `--status` | 0,3 s |
| La version reconstruite (signée ad hoc) | a lu la station dès son premier lancement : rien n'a bloqué l'accès au réseau local |
| Défaut trouvé pendant la mesure | une connexion vide sur la prise faisait tomber le lanceur (SIGPIPE, statut 141, malgré `SO_NOSIGPIPE`) ; corrigé en ignorant SIGPIPE, puis revérifié |

**Ce que change la fermeture mesurée.** Le 16-18/10 n'est pas une affaire de mise à jour : il
suffit que l'application dont dépend la production se ferme. Le lanceur ne règle donc le problème
que s'il reste ouvert. Le menu disait « Quit launcher (production keeps running) » : c'était vrai,
mais la production restait sourde. Depuis 1a, quitter le lanceur demande confirmation tant que
quelque chose dépend de lui. Restent deux risques : un plantage du lanceur, et sa mise à jour, qui
rendent la production sourde sans que personne ne puisse le signaler. Décision à prendre par le
propriétaire, avant 1b :

- **(a) en rester là** : le lanceur doit rester ouvert ; le mettre à jour, c'est relancer la
  production ;
- **(b) un porteur** : chaque production démarrée dans une instance à part de `Clairance.app`, ouverte
  par le système, qui ne fait que la porter et se ferme avec elle. Le menu pourrait alors se
  fermer, planter ou être mis à jour sans toucher à la production. À mesurer avant : le porteur
  garde-t-il le réseau local quand l'application est remplacée sur le disque par une mise à jour ?

Recommandation de l'agent : (b), parce que le gardien de 1b ne peut pas signaler sa propre chute.

**Décision du propriétaire, le 24/10 au soir : (b), « séparer les deux »** (Q59), avec l'accord
d'essayer aussi la piste de l'orchestrateur, un agent launchd rattaché à Clairance, le temps de
la mesure.

### Le porteur, mesuré puis livré le 24/10

Deux porteurs essayés, sur la même instance d'essai (port 8013, ADS-B seul) :

| Mesure | Résultat |
|---|---|
| **Agent launchd** rattaché à Clairance (`AssociatedBundleIdentifiers`), chargé depuis `runs/lanceur/` | **écarté** : trois démarrages sur trois sans réseau local (« no route to host »). Retiré de launchd aussitôt après (`bootout`) |
| **Porteur** : une autre instance de `Clairance.app`, ouverte par le système (`open -n`), qui lance le programme et part avec lui | le programme dépend du porteur, pas du menu (lu par `responsibility_get_pid_responsible_for_pid`) |
| Le menu se ferme pendant que le porteur tourne | l'instance continue de lire la station : lecture ADS-B fraîche à chaque relevé, 2 min de suivi (19:31:51 → 19:34:00 UTC), puis 1 min dans la version intégrée |
| L'application est remplacée sur le disque (mise à jour) pendant que le porteur tourne | même chose : aucune lecture manquée sur 2 min |
| **Correction du 26/10** | **cette mesure était fausse** : la compilation étant déterministe, l'application avait été remplacée par un binaire identique, donc sans changer d'identité. Le 26/10 à 10:49, l'installation d'un binaire différent sous le porteur de la production lui a retiré le réseau local aussitôt (« no route to host » dès 10:49:39), alors qu'il continuait de tourner. D'où la garde de `build.sh --install`, qui refuse d'installer tant qu'un porteur tourne depuis l'application installée |
| Arrêt | un SIGTERM au porteur arrête proprement ce qu'il porte ; quand ce qu'il porte s'arrête, le porteur part |
| **Première connexion d'une compilation que macOS ne connaît pas encore** | **elle échoue** (« no route to host »), puis tout passe : vu trois fois (deux porteurs dont co-atc s'est arrêté au démarrage, puis la sonde : station jointe au 2ᵉ essai, 0,5 s après). La compilation est déterministe (même empreinte pour le même code), ce qui a d'abord masqué la règle. Le porteur interroge donc la station lui-même, jusqu'à 20 s, avant de lancer quoi que ce soit, et note dans le journal à quel essai elle a répondu |
| Rouvrir Clairance pendant qu'un porteur tourne | macOS la croit déjà ouverte et ne fait que réveiller le porteur : le menu ne revenait pas. Corrigé : le porteur ouvre alors le menu s'il n'en trouve aucun, et la ligne de commande ouvre toujours une nouvelle instance. Revérifié : le menu revient |

Livré : la production et les instances d'essai démarrées par le lanceur tournent chacune dans son
porteur. Le menu peut être fermé ou mis à jour sans couper l'écoute ; l'alerte de fermeture ne
concerne plus que ce qui dépendrait encore du menu lui-même. Non vérifié sur la production, que
l'agent ne lance pas (règle 5) : son premier démarrage par le nouveau lanceur le dira.

Corrigé au passage : chaque démarrage de la production vidait `runs/launcher.log` au lieu d'y
ajouter (il ne restait que les lignes du 16/10 à 22 h 47).

### Le 26/10 : la production démarrée par le lanceur, trois défauts et un incident

- **Pas d'audio** : ouvert par le système, le porteur a le PATH de launchd (`/usr/bin:/bin:/usr/sbin:/sbin`), où co-atc ne trouvait pas ffmpeg. Corrigé dans le serveur par l'orchestrateur (134e4fb, `pkg/tool`), sur décision du propriétaire : la production ne dépend pas de ce qui la lance. Le banc ne l'avait pas vu, parce que j'ouvrais son lanceur depuis mon shell, qui lui passait mon PATH ; il s'ouvre désormais dans un environnement vierge, comme celui du propriétaire.
- **Un faux « Production did not start »** sur une production qui transcrivait. Un passage du lanceur tombé pendant l'ouverture du porteur ne trouvait pas encore de script et concluait à l'échec, 0,2 s après le départ ; rien n'effaçait ensuite cet échec. Corrigé (d25fc08), puis revérifié sur le banc : démarrage, arrêt, redémarrage dans les 5 min, aucun faux échec.
- **Le silence sur un co-atc trop ancien** : le binaire du 16/10 répondait 404 sur `/api/v1/health/detail`, et l'audio n'était pas surveillé, sans que rien ne le dise. C'est désormais un défaut (« Audio not watched »). Le commit lu est aussi celui du binaire, et non plus celui du dépôt.
- **L'incident** : la réinstallation du lanceur pendant que la production tournait l'a rendue sourde de 10:49:38 jusqu'à sa relance (voir la correction ci-dessus).

### 1b, livrée le 25/10 (460bb8a) : le gardien, ses notifications, l'icône à six états

Codée sur la fiche du design, validée par le propriétaire (`design/10-lanceur.md`) :
- **les contrôles du plan** : panne (arrêtée d'elle-même, démarrage manqué, serveur muet), surdité,
  production lancée hors du lanceur, audio par fréquence (`bytes_last_60s`), liaison avec la
  station, transcription, silence, disques, mémoire, instances oubliées. La table est dans
  `tools/menubar/README.md`. Chaque incident est noté dans `events.jsonl`, à côté de la prise ;
- **les notifications de macOS** : une par incident, remplacée à son retour à la normale, avec les
  boutons de la fiche ;
- **le menu de la fiche** : l'état et sa cause, la section Health, les actions permises par
  l'état, les instances ; et l'icône à six états ;
- **à la demande du propriétaire, le 24/10** : l'icône aussi dans le Dock, présente par défaut,
  avec une fenêtre de réglages (⌘,) pour l'en retirer.

**Le banc d'essai** (`runs/lanceur/`, hors dépôt) : un second lanceur, avec ses propres réglages et
son propre dossier (`CLAIRANCE_LAUNCHER_CONFIG`, `CLAIRANCE_SUPPORT_DIR`), traite une instance
d'essai comme sa production. Il lui fournit trois fausses sources locales, que l'on peut couper :
une source ADS-B (un instantané de la station servi sur 127.0.0.1), un flux audio (une tonalité
ffmpeg) et le faux sidecar. La station n'est jamais touchée, et la production est restée arrêtée.

| Coupure provoquée (25/10) | Incident ouvert après | Refermé après le retour |
|---|---|---|
| Source ADS-B, 5 fois | 49,3 / 49,4 / 50,2 / 54,2 / 54,3 s | 15 à 20 s |
| Audio, 3 fois | 63,9 / 64,0 / 64,4 s | 14 à 24 s |
| Silence, seuil du banc ramené à 2 min | 129 s | 59 s (le temps de couper un segment) |
| co-atc tué (SIGKILL) | 4,3 s | 11,7 s après le redémarrage |
| Sidecar arrêté | co-atc le relance aussitôt : un événement « Transcription restarted », sans incident | — |
| Arrêt demandé | aucun événement | — |

La cible du plan, moins de 2 min, est tenue partout. **Reste à mesurer : l'absence de fausse
alerte sur une journée normale de production**, qui demande que la production tourne.

Défauts trouvés par le banc, et corrigés :
- **un démarrage manqué accusait la station à tort** : la vérification ADS-B de co-atc avait
  échoué sur un `receiver.json` absent, alors que la source répondait. La cause se lit
  maintenant dans l'erreur ; « la station ne répond pas » seulement sur une erreur de connexion ;
- **le script d'un démarrage manqué restait vivant** et attendait son co-atc. Un démarrage
  suivant réussi, il l'aurait pris pour le sien, et `production.command` aurait lancé **une
  seconde liaison avec la station**. Le lanceur l'arrête désormais, par son PID vérifié ;
- l'échec se constate dès la ligne « fatal » du journal de co-atc : 2 s au lieu de 60 sur le banc,
  et d'au plus 3 min avec `production.command`.

**Notifications.** macOS les refusait (« Notifications are not allowed for this application »),
sans rien demander : elles étaient coupées pour Clairance dans Réglages Système > Notifications.
Le propriétaire les a activées le 25/10, et elles passent depuis. Le lanceur est désormais signé
avec l'identité de développeur du propriétaire, nommée hors du dépôt
(`~/.config/clairance/codesign-identity`), ce qui lui garde la même identité d'une version à
l'autre. Ce n'était pas la cause du refus, contrairement à ma première hypothèse.

**L'icône de l'application.** Depuis macOS 26, le système dessine lui-même la forme, le verre et
l'ombre d'une icône à partir d'un document d'Icon Composer. Une icône `.icns` dessinée sur
l'ancienne grille, et même une icône pleine, est rangée dans un cadre et paraît plus petite
(capture du propriétaire, macOS 27). D'où `tools/menubar/AppIcon.icon`, provisoire : le fond
sombre et le signe en un calque, compilés par `actool` de Xcode. Rendue par macOS, l'icône a
désormais la forme et la taille des Réglages système. Le signe y reste petit (environ la moitié
de la tuile, contre les trois quarts pour les icônes voisines) : sa forme définitive est demandée
à la session design, le 25/10.

**Pas encore fait** : les captures de chaque état en clair et en sombre, que le propriétaire n'a
pas voulu me laisser prendre en pilotant l'écran ; le niveau « sensible au temps » des
notifications de panne, non vérifié, qui demande peut-être un droit que la signature de
développeur ne donne pas ; le démarrage de la vraie production par ce lanceur.

## Les garde-fous

- **Jamais toucher la station** : le lanceur la lit, comme aujourd'hui ; il ne bascule pas
  l'écoute et ne modifie aucune configuration.
- **Rien ne démarre seul** sans décision du propriétaire (option de démarrage éteinte par défaut).
- **Pas de secret à l'écran ni dans le dépôt** : le mot de passe de co-atc reste dans
  `~/.config/clairance/`, jamais affiché ; les adresses de la station restent hors du dépôt (D75).
- **Ne rien casser de ce qui marche** : la ligne de commande actuelle (`--status`, `--start`,
  `--stop`) reste, avec le même sens.
- **macOS 13 et plus**, un seul binaire, pas de dépendance hors des outils de Xcode.

## La mission de l'agent dédié

> Tu prends en charge le lanceur de Clairance, `tools/menubar/` du dépôt
> `/Users/pierre/Dev/Aero/atc-scribe` (branche de travail à créer, par exemple `lanceur`, depuis
> `local`). Lis `CLAUDE.md`, `docs-fr/34-lanceur.md` (ce document), `tools/menubar/README.md`,
> puis `docs-fr/33-pilotage.md`. Tu parles au propriétaire en français ; code, commentaires et
> commits en anglais, comme le reste du dépôt.
>
> Commence par l'étape 1, le gardien. Avant de coder, propose au propriétaire, en une page, la
> façon dont `clairance --start` confiera le lancement au lanceur en marche, et la liste exacte des
> contrôles de santé avec leurs seuils. Le résumé de santé côté serveur est demandé à la session
> principale (« Clairance - Main Orchestrator ») ; en attendant, lis ce qui existe
> (`/api/v1/vitals`, `/api/v1/status`, le sidecar `/health`, la station `/radio/etat`).
>
> Règles : ne jamais lancer la production toi-même depuis ta session (règle 5 de `CLAUDE.md`) ;
> prévenir l'agent de la station avant toute relance de la production ; ne jamais arrêter un
> processus par motif ; mesurer chaque promesse (temps de détection, fausses alertes) et la
> consigner ici.
>
> **Le design est celui de la session design (« Clairance - Design ») : Clairance, mais en
> application macOS native** — l'identité du produit (le signe et ses états, la palette, le ton,
> les nombres de la marque ; `docs-fr/design/`) dans les conventions du Mac (SwiftUI, typographie
> et contrôles du système, menus, notifications, clair et sombre, Human Interface Guidelines). Ne
> dessine rien seul : dès l'étape 1, demande-lui la forme de l'icône et de ses états, du menu et des
> notifications ; avant l'étape 2, celle de la fenêtre ; fais-lui relire chaque livraison, captures à
> l'appui (hors du dépôt, sous `runs/design/captures/`).
