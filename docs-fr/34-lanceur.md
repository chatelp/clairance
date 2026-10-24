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
  coupe plus le réseau local de la production.
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
