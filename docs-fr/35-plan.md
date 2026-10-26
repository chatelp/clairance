# 35 — Le plan des étapes à venir

*Écrit le 26/10/2026 avec le propriétaire, après la revue des chantiers du même jour. Tenu par la session
principale (« Clairance - Main Orchestrator ») ; mis à jour à chaque étape franchie, à chaque décision. Les
questions qu'il cite sont dans `05-decisions.md` ; la vue d'ensemble et les sessions, dans `33-pilotage.md` ;
le journal au jour le jour, dans la section « État » de `32-audit-produit.md`.*

## Ce qui a motivé ce plan

La journée du 26/10 a fait voir, en deux heures, où Clairance tient mal :

- la production lancée par le nouveau lanceur **n'avait pas d'audio** : ffmpeg n'était pas trouvé hors du PATH d'un
  terminal (corrigé dans le serveur, 134e4fb) ;
- relancée, elle est **redevenue sourde** : le lanceur a été réinstallé sous son porteur, et macOS a retiré le
  réseau local au porteur (règle posée : aucune installation pendant qu'un porteur tourne) ;
- le lanceur a affiché **« Production did not start »** alors qu'elle tournait : il juge l'état en lisant un journal
  (corrigé, d25fc08) ;
- le binaire en production avait **dix jours de retard**, et rien ne le disait.

Le fond est le même à chaque fois : la façon dont Clairance tourne dépend de choses qui ne sont pas dans le
produit (un PATH, un script hors du dépôt, l'environnement Python d'un autre dépôt), et le lanceur en sait trop sur
le serveur. Le plan commence donc par la fiabilité, puis pose les fondations (un serveur auto-contenu, un contrat
entre le lanceur et le serveur), avant d'ouvrir l'accès à un proche et de rendre Clairance installable ailleurs.

## Les principes

1. **La production tourne et entend, avant tout le reste.** Toute étape qui la touche se fait production arrêtée,
   avec l'accord du propriétaire, la station prévenue.
2. **Un programme, un contrat.** Le serveur se lance seul, dit ce qui lui manque, et répond par son API ; le
   lanceur ne lit aucun de ses fichiers.
3. **Mesurer avant de choisir** (la culture du projet) : chaque bloc a sa mesure de sortie.
4. **Ce qui est contribuable reste un commit autonome** (D71).
5. **Rien ne s'ouvre vers l'extérieur sans relecture** : l'accès d'un proche passe par un chemin chiffré et un compte
   à son nom.

## Les blocs

### A — La production fiable *(maintenant ; en cours)*

| Étape | Qui | État |
|---|---|---|
| A1. Plus de faux « did not start » ; un co-atc trop ancien signalé ; le commit du binaire dans le journal | lanceur | **fait** (d25fc08, poussé le 26/10) |
| A2. Aucune installation du lanceur sous un porteur vivant (`build.sh` refuse) | lanceur | **fait** (règle et garde, 26/10) |
| A3. Un porteur qui tourne depuis sa propre copie de l'application, que l'installation ne remplace jamais | lanceur | à faire (proposé le 26/10) |
| A4. Une journée de production **sans fausse alerte** du gardien | lanceur | en cours depuis la relance du 26/10 à 10 h 58 |
| A5. Une journée de mesures sur la production : justesse réelle de la piste et de la sortie probables (Q57), mémoire du sidecar (14 → 17 Go en 12 h le 15/10), sauts des avions | principale | en cours, passif |
| A6. La ligne « co-atc prêt (commit …) » de `production.command` donne le commit du dépôt, pas celui du binaire | — | **laissée telle quelle** (propriétaire, 26/10) jusqu'à B3 ; la ligne qui fait foi est celle du lanceur, « launcher: co-atc answers, … built from <commit du binaire> » |

**Sortie** : vingt-quatre heures de production sans surdité ni fausse alerte ; une surdité provoquée signalée en
moins de deux minutes.

### B — Un serveur auto-contenu (Q60) *(fondation ; à commencer après A5)*

| Étape | Ce qu'elle retire | Mesure |
|---|---|---|
| B1. **Mesurer le décodage audio en Go** (MP3 d'Icecast, PCM brut) face à ffmpeg | décision du propriétaire du 26/10 : la mesure d'abord | sur les mêmes flux réels, par paires : écart des échantillons, transcriptions identiques ou non (gagnées et perdues), coût processeur ; puis choix : plus de ffmpeg pour le cas courant, ou un ffmpeg embarqué |
| B2. La page dans le binaire (`go:embed`) | la page figée et sa copie (`runs/production/www`) | le binaire sert la page de son propre commit |
| B3. Les compagnons lancés par co-atc lui-même (la liaison avec la station, l'empêchement de veille), leurs réglages dans la configuration | `runs/production.command`, hors du dépôt, et l'adresse de la station écrite dans un script | le lanceur ne démarre plus qu'un seul programme |
| B4. Le sidecar dans son propre environnement verrouillé, créé par une commande d'installation ; les modèles téléchargés au premier lancement dans le dossier de données | la dépendance au venv de `whisper-lab` | un compte utilisateur neuf fait tourner la transcription |
| B5. Un bilan au démarrage : ce qui manque (outil, environnement, modèle, disque) et ce qui en souffre | les pannes découvertes une fois en production | chaque absence nommée au démarrage, testée |

**Sortie** : sur ce Mac, la production tourne avec le PATH minimal de macOS, sans `whisper-lab`, sans
`production.command`, et dit d'elle-même ce qui lui manque.

**Ouvert** : B3, le principe d'un co-atc qui lance ses compagnons, attend l'accord du propriétaire.

### C — Le contrat entre le lanceur et le serveur ; le lanceur, produit à part (Q61) *(fondation ; après B3)*

| Étape | Qui |
|---|---|
| C1. Écrire le contrat (en anglais, versionné) : démarrer (`co-atc -config <fichier>`), arrêter (SIGTERM), prêt (`/api/v1/health`) ; la santé (`/api/v1/health/detail`) ; la description du processus (PID et enfants, ports, journal, données, version et commit du binaire) ; l'authentification par jeton et le rôle | principale |
| C2. Le serveur expose ce qui manque, et un test de contrat échoue si une modification casse le lanceur | principale |
| C3. Le lanceur cesse de lire `config.toml`, les journaux, `git`, `lsof` et les noms de processus : il démarre un programme et lit l'API | lanceur |
| C4. Où vit le lanceur : un dossier `launcher/` de ce dépôt, ou un dépôt séparé | **décision du propriétaire, ouverte** |
| C5. Les étapes 2 (tableau de bord) et 3 (poste de commande) du lanceur, sur le contrat | lanceur, design |

**Sortie** : le lanceur pilote un co-atc installé ailleurs que dans ce dépôt ; le test de contrat est vert ; le
lanceur ne lit plus aucun fichier du serveur.

### D — L'accès à distance pour un proche (Q62) *(un usage de la période de tests, pas une fonctionnalité)*

Décidé le 26/10 : un ami, nommé, accède à la production de chez lui, **audio compris**, le temps des tests
restreints. Ce bloc rend cet accès **sûr et fiable** ; il ne vise pas un produit multi-utilisateurs.

| Étape | Contenu | Dépend de |
|---|---|---|
| D1. Le chemin | partage du Mac à l'ami par Tailscale, rien d'exposé sur Internet ; choix entre HTTP dans le tunnel chiffré et HTTPS par `tailscale serve` (cookie `Secure`, mandataire de confiance à déclarer, et la route de santé à revérifier) | Tailscale, côté propriétaire |
| D2. Le compte | un compte `viewer` à son nom ; l'audio ouvert à ce compte (aujourd'hui `audio_replay` est global : `all` suffit tant qu'il n'y a que lui) | — |
| D3. La relecture du rôle | ce que voit et fait un viewer : chaque route, le WebSocket, les flux, l'archive, les réglages, l'outil AIP ; un test par interdit | — |
| D4. La trace | un journal des connexions (qui, quand, d'où) et la marche à suivre pour couper l'accès (révoquer le compte, retirer le partage) | — |
| D5. La fiabilité vue de chez lui | les sessions vivent en mémoire, par choix : **chaque relance de la production le déconnecte** ; ce que voit sa page quand la production s'arrête ou redémarre ; la reprise des flux ; le débit montant des deux flux MP3 | D1 à D3 |
| D6. Plus tard : le lanceur chez lui, en client distant | jetons par appareil, routes adaptées au viewer, signature Developer ID et notarisation (compte développeur Apple) | C |

**Sortie** : l'ami utilise Clairance une semaine depuis chez lui ; aucun accès inattendu dans le journal ; une
relance de la production ne le laisse pas sans explication.

### E — Installable ailleurs (le P0 de l'étude communauté) *(après B)*

| Étape | Dépend de |
|---|---|
| E1. Le portage du sidecar sur Linux avec NVIDIA (faster-whisper, repli processeur ; Q54) | B4 |
| E2. L'avertissement légal (dans l'application et le README) : ne pas rediffuser, ne pas publier d'audio ni d'immatriculations privées | — |
| E3. Le guide d'installation, en anglais et en français | B, E1 |
| E4. La mesure de sortie : une installation sur une machine vierge, à partir du seul guide, **en moins d'une heure, par quelqu'un qui n'a pas écrit le code** ; la précision d'appariement inchangée après portage (95 %, D73) | E1 à E3 |

### F — Le produit, ensuite

L'interface en anglais et en français ; le rapprochement d'une instruction et de la réponse du pilote (pour la
lecture, jamais un jugement d'erreur) ; une source ADS-B externe en renfort (Q51) ; le corpus simulé avec
consentement (voie A) ; le premier pas public. L'ordre se décidera après E.

### G — Le crédit cloud *(94 $ le 24/10, jusqu'au 5/11)*

Ce qui se vérifie sans la station et sans audio réel, en fiches autonomes (`fiches-cloud/`), relues sur le Mac
avant d'être intégrées :

1. **E1, le portage Linux du sidecar**, testé sur processeur avec un petit modèle (le plus long, le plus utile à
   lancer tôt) ;
2. **C2, les tests de contrat** et la description du processus, une fois C1 écrit ;
3. **D3, la relecture du rôle viewer**, route par route, avec ses tests.

B1 se mesure sur le Mac : il demande l'audio réel de la station, qui ne va jamais dans le cloud.

### H — Ce qui revient au propriétaire

- relancer ou autoriser chaque relance de la production ;
- décider C4 (où vit le lanceur) et le principe de B3 ;
- le partage Tailscale à l'ami (D1) ;
- le compte développeur Apple, quand viendra D6 ;
- la purge des anciens objets chez GitHub, et la description courte du dépôt.

## L'enchaînement prévu

| Période | Ce qui avance |
|---|---|
| **Du 26/10 au 1/11** | A jusqu’au bout (A3, A4, A5) ; B1, la mesure du décodage ; D1 à D4, pour que l'ami commence ; lancer les fiches cloud G1 puis G3 |
| **Du 2 au 8/11** | B2 à B5 ; C1 et C2 (fiche G2) ; D5 sur une semaine d'usage réel |
| **Ensuite** | C3 à C5 ; E ; puis F |

Les dates suivent le travail, pas l'inverse : une étape qui révèle un problème de production passe devant.

## Les dépendances, en une ligne chacune

- B3 avant C3 : le lanceur ne peut démarrer « un seul programme » que si co-atc lance lui-même ses compagnons.
- C avant D6 et avant les étapes 2 et 3 du lanceur : sinon on les referait.
- B avant E3 : le guide décrit ce que B construit.
- D1 à D4 ne dépendent de rien d'autre : l'ami peut commencer cette semaine.
