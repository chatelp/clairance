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
| A3. Un porteur qui tourne depuis sa propre copie de l'application, que l'installation ne remplace jamais | lanceur | **en production depuis le 27/10 à 13:50** (relance 1, fusion b14d485, accord du propriétaire) : le porteur tourne depuis sa copie `carriers/20261027-134954-b14d485`, aucune ligne « not allowed », station jointe à la 2e tentative, ADS-B frais, les deux flux décodés à 13:50:25. Avant : fait sur la branche `lanceur` (4b35232), à installer après A4, production arrêtée et avec l'accord du propriétaire. « Clairance Carrier » compilé dans le dossier de travail du lanceur ; mesuré sur le banc avec une vraie lecture de la station par le réseau local : remplacer l'application par une autre compilation sous un porteur en marche ne fait manquer aucune lecture en 2 min. Une autorisation « Réseau local » donnée une fois par le propriétaire, qui suit ensuite les compilations ; le lanceur demande désormais les autorisations au premier lancement |
| A4. Une journée de production **sans fausse alerte** du gardien | lanceur | **fait le 27/10** : du 26/10 à 10:58 à la relance du 27/10 à 13:49 (26 h 50), cinq alertes, toutes vraies : un sidecar orphelin de 7,9 Go (11:40 – 12:06), deux démarrages manqués (le sidecar cassé, 13:47 et 13:50), un démarrage refusé à 17:01 (une de nos commandes contenait le motif que le script cherche : l'alerte était juste, la cause était nous ; règle déjà écrite), une pression mémoire de macOS la nuit (10 min, aucun effet sur le service). Aucune fausse alerte ; la coupure réseau de 30 s du 26/10 à 13:24, sous les seuils (30 s + 15 s pour l'ADS-B, 60 s pour l'audio), n'en a pas levé, comme prévu. La surdité provoquée, signalée en moins de 2 min, a été mesurée au banc le 25/10 (doc 34) |
| A5. Une journée de mesures sur la production : justesse réelle de la piste et de la sortie probables (Q57), mémoire du sidecar (14 → 17 Go en 12 h le 15/10), sauts des avions | principale | **fait le 27/10.** **Q57**, rejouée sur les 26-27/10 (tout en configuration ouest ; `runs/mesures/q57/*/2026-10-27/`) : la piste d'une arrivée tient (LFPG 743 arrivées : 93,8 % juste à 15 NM, 99,7 % à 10 NM, comme le 16/10 en ouest seul ; LFPO 352 : 100 %) ; toutes les erreurs sur l'autre piste du doublet, le dernier verdict juste pour toutes. La sortie d'un départ tient à 20 et 30 NM (90 et 98 % juste), mais **à 15 NM, 70 % seulement** : le 85 % du 16/10 venait de la configuration est ; non mesurés : la configuration est, les sorties de LFPO (trop peu de traces). **Sauts** (banc de mouvement, six scénarios, `runs/mesures/motion-bench-2026-10-27/`) : mieux que la référence du 13/10 partout, en une passe chacun : sauts 0,3 à 1,1 % contre 1,4 à 2,7 % (150 avions : 6,1 contre 9,9 %), reculs nuls sauf à 150 avions (1,0 contre 2,2 %), images de plus de 50 ms à 150 avions 4,5 contre 43,6 %. **Mémoire du sidecar** bornée sur la journée : 9,0 à 9,4 Go (9,3 Go après 7 h 44 la nuit), contre 14 → 17 Go en 12 h le 15/10. **Vitesse** : médiane 6,7× sur la nuit et la matinée (7 135 transmissions), 6,2× à 16 kHz sur les 289 premières (ordre de grandeur : le rapport dépend de la longueur des transmissions). Avant : en cours, passif. **Mémoire du sidecar traitée le 26/10** : 16 Go 25 min après une relance, la machine en échange (9,7 Go sur 10,2) ; c'était le cache de MLX, sans limite. Borné à 1 Go (d5860d5, caa8a69) : 9,0 Go dont 8,2 de modèles, échange redescendu à 5,3 Go ; vitesse de transcription inchangée (médiane 6,9× contre 6,3× avant, 79 transmissions, à confirmer sur la journée) |
| A7. Les pistes en service gardées d'une relance à l'autre : elles s'apprenaient sur une heure, et la couche des procédures n'en montrait que la moitié après chaque démarrage | principale | **fait le 26/10** (342b637), vérifié sur une instance d'essai ; en production à la relance du 27/10 après N5. La couche a aussi un choix « toutes les pistes » (572e1e2, en production) |
| A6. La ligne « co-atc prêt (commit …) » de `production.command` donne le commit du dépôt, pas celui du binaire | — | **laissée telle quelle** (propriétaire, 26/10) jusqu'à B3 ; la ligne qui fait foi est celle du lanceur, « launcher: co-atc answers, … built from <commit du binaire> » |

**Sortie** : vingt-quatre heures de production sans surdité ni fausse alerte ; une surdité provoquée signalée en
moins de deux minutes.

### B — Un serveur auto-contenu (Q60) *(fondation ; à commencer après A5)*

| Étape | Ce qu'elle retire | Mesure |
|---|---|---|
| B1. **Mesurer le décodage audio en Go** (MP3 d'Icecast, PCM brut) face à ffmpeg | décision du propriétaire du 26/10 : la mesure d'abord | **fait le 26/10** (détail dans Q60) : décodeur goxiph identique à ffmpeg à l'arrondi près (≈ 100 dB), chaîne complète 72 à 80 dB, 5,6 s de calcul par heure ; `-fflags nobuffer` perd 3,6 s à chaque démarrage de ffmpeg. **Choix à faire** : goxiph intégré au dépôt, ou ffmpeg embarqué ; retirer `nobuffer` ; 24 ou 16 kHz |
| B2. La page dans le binaire (`go:embed`) | la page figée et sa copie (`runs/production/www`) | le binaire sert la page de son propre commit ; **fait le 26/10 par la fiche Q** (4d3a79a) ; la production garde sa copie **jusqu'à ce que la page se stabilise** (décision du propriétaire, 26/10 au soir : chaque correctif de page demanderait sinon une relance) ; le plan de l'aéroport est déjà dans `data/station/` |
| B3. Les compagnons lancés par co-atc lui-même (la liaison avec la station, l'empêchement de veille), leurs réglages dans la configuration | `runs/production.command`, hors du dépôt, et l'adresse de la station écrite dans un script | le lanceur ne démarre plus qu'un seul programme ; **code fait le 26/10 par la fiche R** (e810731), désactivé par défaut ; à activer quand le script disparaît |
| B4. Le sidecar dans son propre environnement verrouillé, créé par une commande d'installation ; les modèles téléchargés au premier lancement dans le dossier de données | la dépendance au venv de `whisper-lab` | un compte utilisateur neuf fait tourner la transcription |
| B5. Un bilan au démarrage : ce qui manque (outil, environnement, modèle, disque) et ce qui en souffre | les pannes découvertes une fois en production | chaque absence nommée au démarrage, testée ; **fait le 26/10 par la fiche Q** |

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
| N5. La production en Go | 24 h de production avec `decoder = "go"` | aucune panne du décodeur ; reprise après chaque bascule de la station ; débit conforme ; transmissions par heure dans la plage des jours précédents ; le gardien sans alerte audio | **passé le 27/10 à 13:17** : aucune panne du décodeur (19 h 25 d'affilée depuis la relance de 17:02, les autres coupures étant nos relances) ; reprise seule 20 s après la coupure réseau du 26/10 à 13:24, aucune bascule de la station pendant l'essai ; débit 100,0 à 100,3 % des 48 000 o/s sur chaque période longue ; transmissions par heure comme avec ffmpeg (468 contre 470 de 11 h à 12 h ; 438 contre 433 l'après-midi, face au 16/10) ; une alerte mémoire du lanceur la nuit (le sidecar, 9,3 Go), aucune alerte audio ; Icecast (relevé de la station) : aucun lecteur retiré pour retard, les deux connexions de la production stables 20 h 15. Les silences simultanés de la nuit sont de vrais creux : la nuit, les deux fréquences portent les mêmes transmissions. Lancé le 26/10 à 13:09:51 (accord du propriétaire ; binaire e8f500b, `decoder = "go"` dans la configuration, sauvegarde `runs/config.toml.avant-n5` ; station prévenue). Vérifié au départ : les deux flux décodés en Go, aucun processus ffmpeg, première transcription à 13:10:05. Jugé le 27/10 vers 13 h 10 sur le journal et la base : erreurs du décodeur, reconnexions et coupures pour silence, transmissions par heure face aux mêmes heures des jours précédents, alertes du gardien. Retour arrière : retirer la ligne et relancer |

**Fait le 27/10 à 14:01 (relance 3)** : N5 passé, `b1-ter` fusionnée (749d489), production à 16 kHz sans ffmpeg ; essayée d'abord sur 8020 (une transmission de 30,0 s envoyée au faux sidecar, durée exacte à 16 kHz). **Après N5, si goxiph est validé (décidé par le propriétaire le 26/10)** : ffmpeg est abandonné. B1 ter : le décodeur
Go devient le seul ; les sources SRT et carte son sont retirées (configuration, exemple, documentation, tests) ; le PCM
brut par UDP est réécrit en Go (lecture réseau et rééchantillonneur existant), sauf avis contraire du propriétaire ;
`-fflags nobuffer`, `ffmpeg_path` et les réglages propres à ffmpeg disparaissent ; la recherche d'outils (`pkg/tool`)
ne sert plus qu'à pdftotext. Si N5 échoue, rien de cela n'est fait. **Et, décidé le 26/10, la production passe de
24 à 16 kHz** (la fréquence de Whisper), appliqué juste après N5 pour ne pas mêler deux changements dans l'essai.

**Préparé le 26/10 au soir** sur la branche `b1-ter` (6 commits, non fusionnée) : l'UDP lu en Go, le décodeur Go
seul, SRT et carte son retirés, le bilan au démarrage sans ffmpeg, 16 kHz par défaut ; les anciennes clés ffmpeg
ignorées avec un avertissement, jamais refusées. Essai à part sans ffmpeg dans le `PATH` : démarrage en 0,4 s, flux
UDP à 31 873 o/s pour 32 000 attendus. **La relance du 27/10 se fait en trois temps** (avis de l'agent du lanceur,
une cause par relance) : A3 seul, puis le nouveau co-atc (P, Q, R), puis `b1-ter` si N5 est validé.

**Décidé le 26/10** : B3, co-atc lance lui-même ses compagnons (le propriétaire, « oui, Clairance fait tout »).

### C — Le contrat entre le lanceur et le serveur ; le lanceur, produit à part (Q61) *(fondation ; après B3)*

| Étape | Qui |
|---|---|
| C1. Écrire le contrat (en anglais, versionné) : démarrer (`co-atc -config <fichier>`), arrêter (SIGTERM), prêt (`/api/v1/health`) ; la santé (`/api/v1/health/detail`) ; la description du processus (PID et enfants, ports, journal, données, version et commit du binaire) ; l'authentification par jeton et le rôle | principale ; **écrit le 26/10 par la fiche R** : `36-contrat-lanceur.md` (en français, comme le voulait la fiche) |
| C2. Le serveur expose ce qui manque, et un test de contrat échoue si une modification casse le lanceur | principale ; **fait le 26/10 par la fiche R**, schéma figé |
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
| D1. Le passage par le mandataire | une route Traefik vers le Mac, en HTTPS ; l'adresse du Mac fixée sur le réseau ; le mandataire déclaré dans `trusted_proxies` de co-atc (bonne adresse du client, HTTPS reconnu, cookie `Secure`) ; l'origine publique acceptée pour les requêtes et le WebSocket | **fait le 26/10, fermé** : route Traefik ajoutée (copie du fichier gardée), certificat obtenu, limite des essais sur la connexion, en-têtes de sécurité, **barrière temporaire** (l'identifiant du tableau de bord Traefik), **retirée le 26/10 à 14 h sur décision du propriétaire** : le service worker et les icônes de la page partent sans l'identifiant, le navigateur redemandait le mot de passe et la page restait à moitié vide ; c'est la connexion de Clairance qui protège (le WebSocket et les flux exigent une session, `/.env` répond 404), avec la limite des essais de Traefik ; l'ami n'a pas encore de compte ; adresse du Mac réservée (propriétaire) ; `trusted_proxies` et `allowed_origins` posés, production relancée à 13:19. Vérifié de l'extérieur : tout reçoit 401 de la barrière. **Le domaine passe par le proxy de Cloudflare** : co-atc et Traefik voient l'adresse de Cloudflare, pas celle du client ; la limite des essais et le journal des connexions devront lire celle que Cloudflare transmet (à traiter à l'intégration de la fiche O). Reste : l'essai du propriétaire dans un navigateur (page, carte, flux, WebSocket) |
| D2. Les flux audio par la même adresse | aujourd'hui la page demande les flux sur les ports 8001 à 8004 ; derrière un mandataire unique, ils doivent passer par l'adresse de la page | principale |
| D2 bis. Le son sur tous les navigateurs | la page jouait du WAV sans fin, que Safari (Mac et iPhone) refuse (vérifié par le propriétaire le 26/10 : son dans Dia, rien dans Safari) ; désormais le **MP3 d'origine de la station**, relayé trame par trame par le décodeur Go (`?format=mp3`, `audio/mpeg`), environ 15 kb/s au lieu de 384 ; repli sur le WAV pour une source sans MP3 ; une page ouverte juste après un démarrage attend jusqu'à 8 s la première trame (5,5 s mesurées) | **fait le 26/10** (c45ddd3, b081228), vérifié sur une instance d'essai branchée sur la station, en production depuis 14:12. **Le propriétaire : son dans Safari sur Mac, mais pas sur le téléphone ; dans Safari, le bouton son ne coupait rien et « someone is speaking » n'apparaissait pas** (Safari joue un flux en direct sans passer par Web Audio). Corrigé (32d05e8, en production à 14:24) : l'activité est mesurée par le serveur et envoyée par le WebSocket (`frequency_activity`), le bouton coupe aussi l'élément `<audio>`. Reste : le téléphone |
| D2 ter. La page joue le son elle-même (Q63) | le serveur envoie le son décodé en μ-law à 8 kHz (64 kb/s), la page le décode et le joue par Web Audio : démarrage immédiat, « someone is speaking » mesuré sur ce qui sort, le bouton son partout, l'iPhone | **fait le 26/10** (4d15c1e), en production à 15:15:43. Safari sur Mac : d'abord sans son, puis avec un cliquetis ; causes trouvées et corrigées le jour même (reprise du son à chaque geste, rééchantillonnage dans la page : 623d132, c50787c), **Safari joue par ce lecteur** ; l'iPhone aussi (cbf7ecf, son déclaré comme lecture). Reste, à juger par le propriétaire à une heure creuse (trop de trafic le 26/10 en fin d'après-midi) : l'écran verrouillé, et « someone is speaking » en phase avec la voix sur iPhone |
| D3. Le durcissement de ce qui devient public | relecture de chaque route ouverte sans session (`/health`, `/vitals`, `/auth/*`, `/setup/*`) ; la limite des essais de mot de passe, vérifiée derrière le mandataire ; ce que voit et fait un viewer, route par route, WebSocket, flux, archive, réglages, outil AIP, un test par interdit ; `/api/v1/health/detail` refusée depuis l'extérieur (déjà) ; une limite de débit côté Traefik | principale ; **fait le 26/10 par la fiche O** (6fa8dd5), en production à la relance du 27/10 ; six constats à trancher par le propriétaire (05, Q62) |
| D4. Le compte et la trace | un compte `viewer` à son nom ; l'audio ouvert aux viewers (`audio_replay = "all"`, global, suffit tant qu'il est seul) ; un journal des connexions (qui, quand, d'où), et la marche à suivre pour couper l'accès | principale, propriétaire pour le mot de passe ; **la trace faite** (fiche O : `configs/signins.jsonl`, `GET /api/v1/signins`) ; **les comptes se gèrent depuis la page depuis le 27/10** (D83 : panneau « Accounts » du propriétaire, création, nouveau mot de passe, couper l'accès sans relance, WebSocket et flux fermés compris, retrait ; cookie renouvelé, 90 jours au plus ; 1ea56f6 à 079ea4e, vérifié dans Chrome sur l'instance d'essai ; **en production depuis le 27/10 à 15:37**, be6221b) ; **le compte de l'invité créé par le propriétaire depuis le panneau et l'audio ouvert aux viewers le 27/10 à 16:59** (`audio_replay = "all"`, relance 5) — **fait** ; **ouverture le 28/10** ; avant : adresses internes masquées et flux limités pour un invité (décision du 26/10) |
| D5. La fiabilité vue de chez lui | les sessions vivaient en mémoire, par choix : **chaque relance de la production le déconnectait** (gardées sur disque depuis la fiche S) ; ce que voit sa page quand la production s'arrête ou redémarre ; la reprise des flux ; le débit montant des deux flux ; la machine du mandataire est sur l'onduleur qui saute (deux redémarrages le 19/10) : une sonde Uptime Kuma sur l'adresse publique | principale, propriétaire ; **fiche cloud S intégrée le 27/10** (d84435e) : sessions gardées sur disque (empreintes seulement), relance vérifiée sur le Mac ; WebSocket à 5 s au plus ; **en production depuis la relance 2 du 27/10 à 13:53** (84db233). **Faits le 27/10 à 17:10** : débit montant 383 à 423 Mbit/s (`networkQuality`), un invité des deux fréquences en prend 256 kbit/s ; sonde Kuma n°26, mot-clé `"status":true` sur `/api/v1/health` à l'adresse publique. Limite : Kuma est sur la machine du mandataire, sa chute n'alerte pas — une sonde extérieure reste à décider |
| D6. Plus tard : le lanceur chez lui, en client distant | jetons par appareil, routes adaptées au viewer, signature Developer ID et notarisation (compte développeur Apple) | lanceur, après C |

**Sortie** : l'ami utilise Clairance une semaine depuis chez lui ; aucun accès inattendu dans le journal ; une
relance de la production ne le laisse pas sans explication.

### E — Installable ailleurs (le P0 de l'étude communauté) *(après B)*

| Étape | Dépend de |
|---|---|
| E1. Le portage du sidecar sur Linux avec NVIDIA (faster-whisper, repli processeur ; Q54) — **le code fait le 26/10 par la fiche N** (e375fa0), vérifié sur processeur (cloud) et sans régression sous MLX (Mac) ; reste un GPU et les vrais modèles convertis | B4 |
| E2. L'avertissement légal (dans l'application et le README) : ne pas rediffuser, ne pas publier d'audio ni d'immatriculations privées | — |
| E3. Le guide d'installation, en anglais et en français | B, E1 |
| E4. La mesure de sortie : une installation sur une machine vierge, à partir du seul guide, **en moins d'une heure, par quelqu'un qui n'a pas écrit le code** ; la précision d'appariement inchangée après portage (95 %, D73) | E1 à E3 |

### F — Le produit, ensuite

L'interface en anglais et en français ; le rapprochement d'une instruction et de la réponse du pilote (pour la
lecture, jamais un jugement d'erreur) ; une source ADS-B externe en renfort (Q51) ; le corpus simulé avec
consentement (voie A) ; le premier pas public. L'ordre se décidera après E.

### G — Le crédit cloud *(94 $ le 24/10, 61 $ le 26/10 au soir, jusqu'au 5/11)*

Ce qui se vérifie sans la station et sans audio réel, en fiches autonomes (`fiches-cloud/`), relues sur le Mac
avant d'être intégrées :

1. **E1, le portage Linux du sidecar**, testé sur processeur avec un petit modèle (le plus long, le plus utile à
   lancer tôt) — **fiche N faite le 26/10**, fusionnée (e375fa0) ;
2. **C2, les tests de contrat** et la description du processus, une fois C1 écrit ;
3. **D3, la relecture du rôle viewer**, route par route, avec ses tests — **fiche O écrite le 26/10**, élargie à
   tout ce qui devient public derrière le mandataire et à la trace des connexions (D4) — **faite le 26/10**,
   fusionnée (6fa8dd5).

4. **Vague 5, écrite le 26/10 au soir** (61 $ restants) : **P**, le compte invité (adresses de la maison masquées,
   flux bornés ; pour l'ami du 28/10) ; **Q**, la page dans le binaire et le bilan au démarrage (B2, B5) ; **R**, le
   contrat du lanceur côté serveur (B3, C1, C2 ; remplace l'ancienne G2). En parallèle ; lancées par le propriétaire le 26/10 au soir, **revenues, relues et intégrées le soir même** (202a566, 4d3a79a, e810731). Le contrat est écrit : `36-contrat-lanceur.md`.

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
| **Du 2 au 8/11** | **le lanceur d'abord (décidé le 26/10 au soir)** : C1 à C5 ; puis B2 à B5 ; D5 sur une semaine d'usage réel |
| **Ensuite** | E ; puis F |

Les dates suivent le travail, pas l'inverse : une étape qui révèle un problème de production passe devant.

**Décidé le 26/10 au soir** : la production est relancée le soir même (pistes gardées, fiches O et N, 17:02:06) ;
l'ami entre le 28/10 ; après la bascule audio et l'ami, **le lanceur passe avant le serveur autonome** ; le crédit
cloud restant (**61 $**) part dès maintenant en fiches, choisies par la session principale.

## Les dépendances, en une ligne chacune

- B3 avant C3 : le lanceur ne peut démarrer « un seul programme » que si co-atc lance lui-même ses compagnons.
- C avant D6 et avant les étapes 2 et 3 du lanceur : sinon on les referait.
- B avant E3 : le guide décrit ce que B construit.
- D3 avant toute ouverture : rien n'est public tant que la relecture n'est pas faite.
- D1 à D4 ne dépendent pas de B ni de C : l'ami peut commencer cette semaine.
