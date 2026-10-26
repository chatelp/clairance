# 35 — Le plan des étapes à venir

*Écrit le 26/10/2026 avec le propriétaire, après la revue des chantiers du même jour. Tenu par la session
principale (« Clairance - Main Orchestrator ») ; mis à jour à chaque étape franchie, à chaque décision. Les
questions qu'il cite sont dans `05-decisions.md` ; la vue d'ensemble et les sessions, dans `33-pilotage.md` ;
le journal au jour le jour, dans la section « État » de `32-audit-produit.md`.*

*Son tableau d'avancement, en blocs logiques : `35-plan-tableau.html`, publié en artefact privé
(https://claude.ai/artifact/ApUt47do2VUPU9gEcapDYa). **Règle du propriétaire (26/10), pour toute la suite du
projet** : le tableau reste synchrone avec ce document, `05-decisions.md` et `33-pilotage.md` ; à chaque étape
franchie ou décision prise, les fichiers `.md` et le tableau sont mis à jour dans le même geste, et le tableau est
republié à la même adresse.*

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
| B1. **Mesurer le décodage audio en Go** (MP3 d'Icecast, PCM brut) face à ffmpeg | décision du propriétaire du 26/10 : la mesure d'abord | **fait le 26/10** (détail dans Q60) : décodeur goxiph identique à ffmpeg à l'arrondi près (≈ 100 dB), chaîne complète 72 à 80 dB, 5,6 s de calcul par heure ; `-fflags nobuffer` perd 3,6 s à chaque démarrage de ffmpeg. **Choix à faire** : goxiph intégré au dépôt, ou ffmpeg embarqué ; retirer `nobuffer` ; 24 ou 16 kHz |
| B2. La page dans le binaire (`go:embed`) | la page figée et sa copie (`runs/production/www`) | le binaire sert la page de son propre commit |
| B3. Les compagnons lancés par co-atc lui-même (la liaison avec la station, l'empêchement de veille), leurs réglages dans la configuration | `runs/production.command`, hors du dépôt, et l'adresse de la station écrite dans un script | le lanceur ne démarre plus qu'un seul programme |
| B4. Le sidecar dans son propre environnement verrouillé, créé par une commande d'installation ; les modèles téléchargés au premier lancement dans le dossier de données | la dépendance au venv de `whisper-lab` | un compte utilisateur neuf fait tourner la transcription |
| B5. Un bilan au démarrage : ce qui manque (outil, environnement, modèle, disque) et ce qui en souffre | les pannes découvertes une fois en production | chaque absence nommée au démarrage, testée |

**Sortie** : sur ce Mac, la production tourne avec le PATH minimal de macOS, sans `whisper-lab`, sans
`production.command`, et dit d'elle-même ce qui lui manque.

#### B1 bis — Valider goxiph avant la bascule complète *(demandé par le propriétaire le 26/10)*

Le propriétaire : si les tests valident goxiph, on **bascule complètement** et ffmpeg disparaît ; la production
peut être arrêtée pour les tests. Le décodeur Go est dans le dépôt (`third_party/goxiph`, a62e9c2) et branché
derrière `frequencies.decoder = "go"` (839674c), ffmpeg restant par défaut jusqu'à la décision.

| Niveau | Ce qu'il vérifie | Critère | État |
|---|---|---|---|
| N1. Le décodeur | sur des enregistrements synthétiques, avec la référence de ffmpeg à côté : chaque format courant, dont celui de la station | 90 dB ou plus en mono | **passé** : 105 dB sur chaque fichier mono ; stéréo 52 dB sur la voie gauche, **limite connue** (ffmpeg, LAME et Apple s'accordent entre eux à 79-85 dB) |
| N2. La robustesse | flux pris à n'importe quel octet ; octets inversés, mis à zéro, insérés, retirés ; coupure dans une trame ; ce qui n'est pas du MP3 ; changement de format ; tout découpage réseau ; une heure en mémoire constante ; bombardement aléatoire | aucun plantage, reprise sur la trame suivante, son intact hors de la zone abîmée | **passé** : tous ; une heure décodée 2 000 fois plus vite que le temps réel, mémoire stable ; 146 665 entrées aléatoires sans plantage |
| N3. La chaîne dans Clairance | HTTP, rééchantillonnage au fil de l'eau, coupure de connexion, flux muet, bascule de format, arrêt ; le tout avec un ffmpeg **inexistant** | l'audio arrive, chaque panne est reprise, rien ne fuit | **passé** |
| N4. L'audio réel, transcrit par paires | une demi-heure des deux fréquences enregistrée le 26/10 (11:59 – 12:29, décidé avec le propriétaire) ; décodée par ffmpeg et en Go ; découpée comme la production ; chaque transmission transcrite par un sidecar d'essai (12:30 – 13:06, production arrêtée) | 99 % des transmissions avec les mêmes bornes ; **pas plus de textes différents qu'un bruit inaudible ajouté à l'audio de ffmpeg** (critère révisé, voir ci-dessous) | **passé** : 237 transmissions sur 237 aux mêmes bornes ; textes : goxiph 17 différents sur 237 ; un bruit d'un pas d'arrondi sur l'audio de ffmpeg, 22 ; le même audio deux fois, 0 |

**N4, le détail et le critère révisé.** Le critère écrit d'abord, 97 % de textes identiques, a échoué (92,4 %) ; les
contrôles ont montré qu'il était mal posé. (A) Le même audio de ffmpeg transcrit deux fois : 237 identiques sur 237,
Whisper est déterministe. (B) goxiph pour décoder, ffmpeg pour passer à 24 kHz : 20 différents ; ce n'est donc pas
le rééchantillonneur. (C) L'audio de ffmpeg avec un bruit aléatoire d'un pas d'arrondi en 16 bits (inaudible) : **22
différents**. Whisper bascule une transmission difficile sur douze à la moindre perturbation ; les 17 différences de
goxiph, dont l'audio est à un pas d'arrondi de celui de ffmpeg, sont dans ce bruit. Elles tombent presque toutes sur des
transmissions que Whisper lit mal des deux côtés (du français, des passages incompréhensibles) ; aucune ne montre un
camp meilleur que l'autre. Rapports : `runs/b1-decodage/{heure,controle-a,controle-b,controle-c}/paired-report.json`
(hors du dépôt).
| N5. La production en Go | 24 h de production avec `decoder = "go"` | aucune panne du décodeur ; reprise après chaque bascule de la station ; débit conforme ; transmissions par heure dans la plage des jours précédents ; le gardien sans alerte audio | **en cours depuis le 26/10 à 13:09:51** (accord du propriétaire ; binaire e8f500b, `decoder = "go"` dans la configuration, sauvegarde `runs/config.toml.avant-n5` ; station prévenue). Vérifié au départ : les deux flux décodés en Go, aucun processus ffmpeg, première transcription à 13:10:05. Jugé le 27/10 vers 13 h 10 sur le journal et la base : erreurs du décodeur, reconnexions et coupures pour silence, transmissions par heure face aux mêmes heures des jours précédents, alertes du gardien. Retour arrière : retirer la ligne et relancer |

**Après N5, si goxiph est validé (décidé par le propriétaire le 26/10)** : ffmpeg est abandonné. B1 ter : le décodeur
Go devient le seul ; les sources SRT et carte son sont retirées (configuration, exemple, documentation, tests) ; le PCM
brut par UDP est réécrit en Go (lecture réseau et rééchantillonneur existant), sauf avis contraire du propriétaire ;
`-fflags nobuffer`, `ffmpeg_path` et les réglages propres à ffmpeg disparaissent ; la recherche d'outils (`pkg/tool`)
ne sert plus qu'à pdftotext. Si N5 échoue, rien de cela n'est fait. **Et, décidé le 26/10, la production passe de
24 à 16 kHz** (la fréquence de Whisper), appliqué juste après N5 pour ne pas mêler deux changements dans l'essai.

**Décidé le 26/10** : B3, co-atc lance lui-même ses compagnons (le propriétaire, « oui, Clairance fait tout »).

### C — Le contrat entre le lanceur et le serveur ; le lanceur, produit à part (Q61) *(fondation ; après B3)*

| Étape | Qui |
|---|---|
| C1. Écrire le contrat (en anglais, versionné) : démarrer (`co-atc -config <fichier>`), arrêter (SIGTERM), prêt (`/api/v1/health`) ; la santé (`/api/v1/health/detail`) ; la description du processus (PID et enfants, ports, journal, données, version et commit du binaire) ; l'authentification par jeton et le rôle | principale |
| C2. Le serveur expose ce qui manque, et un test de contrat échoue si une modification casse le lanceur | principale |
| C3. Le lanceur cesse de lire `config.toml`, les journaux, `git`, `lsof` et les noms de processus : il démarre un programme et lit l'API | lanceur |
| C4. Où vit le lanceur | **décidé le 26/10 : dans ce dépôt, rangé à part**, sans dépendance aux fichiers du serveur |
| C5. Les étapes 2 (tableau de bord) et 3 (poste de commande) du lanceur, sur le contrat | lanceur, design |

**Sortie** : le lanceur pilote un co-atc installé ailleurs que dans ce dépôt ; le test de contrat est vert ; le
lanceur ne lit plus aucun fichier du serveur.

### D — L'accès à distance pour un proche (Q62) *(un usage de la période de tests, pas une fonctionnalité)*

Décidé le 26/10 : un ami, nommé, accède à la production de chez lui, **audio compris**, le temps des tests
restreints, **par Internet, derrière le mandataire Traefik** qui tourne déjà sur une autre machine du réseau du
propriétaire (certificats compris). Ce bloc rend cet accès **sûr et fiable** ; il ne vise pas un produit
multi-utilisateurs. **Rien n'est ouvert avant D3.**

| Étape | Contenu | Qui |
|---|---|---|
| D1. Le passage par le mandataire | une route Traefik vers le Mac, en HTTPS ; l'adresse du Mac fixée sur le réseau ; le mandataire déclaré dans `trusted_proxies` de co-atc (bonne adresse du client, HTTPS reconnu, cookie `Secure`) ; l'origine publique acceptée pour les requêtes et le WebSocket | **fait le 26/10, fermé** : route Traefik ajoutée (copie du fichier gardée), certificat obtenu, limite des essais sur la connexion, en-têtes de sécurité, **barrière temporaire** (l'identifiant du tableau de bord Traefik) jusqu'à l'ouverture ; adresse du Mac réservée (propriétaire) ; `trusted_proxies` et `allowed_origins` posés, production relancée à 13:19. Vérifié de l'extérieur : tout reçoit 401 de la barrière. **Le domaine passe par le proxy de Cloudflare** : co-atc et Traefik voient l'adresse de Cloudflare, pas celle du client ; la limite des essais et le journal des connexions devront lire celle que Cloudflare transmet (à traiter à l'intégration de la fiche O). Reste : l'essai du propriétaire dans un navigateur (page, carte, flux, WebSocket) |
| D2. Les flux audio par la même adresse | aujourd'hui la page demande les flux sur les ports 8001 à 8004 ; derrière un mandataire unique, ils doivent passer par l'adresse de la page | principale |
| D3. Le durcissement de ce qui devient public | relecture de chaque route ouverte sans session (`/health`, `/vitals`, `/auth/*`, `/setup/*`) ; la limite des essais de mot de passe, vérifiée derrière le mandataire ; ce que voit et fait un viewer, route par route, WebSocket, flux, archive, réglages, outil AIP, un test par interdit ; `/api/v1/health/detail` refusée depuis l'extérieur (déjà) ; une limite de débit côté Traefik | principale |
| D4. Le compte et la trace | un compte `viewer` à son nom ; l'audio ouvert aux viewers (`audio_replay = "all"`, global, suffit tant qu'il est seul) ; un journal des connexions (qui, quand, d'où), et la marche à suivre pour couper l'accès | principale, propriétaire pour le mot de passe |
| D5. La fiabilité vue de chez lui | les sessions vivent en mémoire, par choix : **chaque relance de la production le déconnecte** ; ce que voit sa page quand la production s'arrête ou redémarre ; la reprise des flux ; le débit montant des deux flux ; la machine du mandataire est sur l'onduleur qui saute (deux redémarrages le 19/10) : une sonde Uptime Kuma sur l'adresse publique | principale, propriétaire |
| D6. Plus tard : le lanceur chez lui, en client distant | jetons par appareil, routes adaptées au viewer, signature Developer ID et notarisation (compte développeur Apple) | lanceur, après C |

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
   lancer tôt) — **fiche N écrite le 26/10**, à lancer ;
2. **C2, les tests de contrat** et la description du processus, une fois C1 écrit ;
3. **D3, la relecture du rôle viewer**, route par route, avec ses tests — **fiche O écrite le 26/10**, élargie à
   tout ce qui devient public derrière le mandataire et à la trace des connexions (D4), à lancer.

B1 se mesure sur le Mac : il demande l'audio réel de la station, qui ne va jamais dans le cloud.

### H — Ce qui revient au propriétaire

- relancer ou autoriser chaque relance de la production ;
- la route Traefik vers le Mac et l'adresse fixe du Mac (D1), le mot de passe de l'ami (D4) ;
- le compte développeur Apple, quand viendra D6 ;
- la purge des anciens objets chez GitHub, et la description courte du dépôt.

## L'enchaînement prévu

| Période | Ce qui avance |
|---|---|
| **Du 26/10 au 1/11** | A jusqu’au bout (A3, A4, A5) ; **en parallèle (décidé le 26/10)** B1, la mesure du décodage, et D1 à D4, pour que l'ami commence ; lancer les fiches cloud G1 puis G3 |
| **Du 2 au 8/11** | B2 à B5 ; C1 et C2 (fiche G2) ; D5 sur une semaine d'usage réel |
| **Ensuite** | C3 à C5 ; E ; puis F |

Les dates suivent le travail, pas l'inverse : une étape qui révèle un problème de production passe devant.

## Les dépendances, en une ligne chacune

- B3 avant C3 : le lanceur ne peut démarrer « un seul programme » que si co-atc lance lui-même ses compagnons.
- C avant D6 et avant les étapes 2 et 3 du lanceur : sinon on les referait.
- B avant E3 : le guide décrit ce que B construit.
- D3 avant toute ouverture : rien n'est public tant que la relecture n'est pas faite.
- D1 à D4 ne dépendent pas de B ni de C : l'ami peut commencer cette semaine.
