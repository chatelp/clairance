# Pilotage — stratégie, chantiers, jalons, décisions attendues

*Dernière mise à jour : 26/10/2026, 16 h 05 (fiches O et N fusionnées, pistes en service gardées, couche des procédures, Safari).*

*Le document du chef d'orchestre. Tenu par la session Clairance, mis à jour à chaque livraison
d'une session, à chaque décision du propriétaire et à chaque jalon franchi. Ouvert le 12/10/2026
à la demande du propriétaire. Le journal détaillé reste dans `32-audit-produit.md` (« État ») et
la mémoire des décisions dans `05-decisions.md` ; ici, la vue d'ensemble, courte, à jour.*

## La stratégie, en cinq lignes

1. **Clairance est un produit à part** (D71, 10/10) : plus un fork qu'on rebase, une interface
   à nous, ce qui ne sert pas se retire, ce qui est contribuable reste identifiable.
2. **Ce qu'il fait que les autres ne font pas** : lier ce qui se dit à ce qui vole, en local,
   sans service, avec le français en seconde lecture. Mesuré (95 % de précision d'appariement,
   D73) ; rien d'ouvert, local et apparié n'existe ailleurs (étude communauté, question 2).
3. **Le cap** : un outil installable par d'autres, en France d'abord, pour une communauté de
   radioamateurs, de passionnés et de simmers qui fournisse captations et annotations, vers un
   **modèle de transcription communautaire adapté au français** (étude communauté, 12/10).
4. **Le nom a changé le 13/10** (D74 complétée, Q53 close) : « Scribe » était l'outil de la DSNA,
   contesté par le SNCTA ; le propriétaire a choisi **Clairance** (marques et domaines vérifiés,
   `design/00-nom.md`), avec le positionnement « espace aérien français ».
5. **Règles du chantier** : ne rien casser de ce qui marche (sept garde-fous, doc 32), mesurer
   avant de conclure, interface utilisable par un novice, aides contextuelles partout, rien ne
   tourne en permanence sur le Mac sans décision.

## Les chantiers en cours (sessions parallèles)

| Session | Mission | État au 20/10 (revue du 20/10) | Prochain livrable | Ce que j'en attends |
|---|---|---|---|---|
| **Clairance** (session principale, chef d'orchestre) | le produit : code, mesures, documents, coordination | paliers 0 à 5 de l'audit faits ; migration du design en production depuis le 15/10 ; Q55 à Q57 en production le 16/10 ; mouvement des avions (D79, D80, troisième famille de sauts corrigée le 16/10) ; revue des documents du 20/10 | l'analyse des mesures de la nuit du 16 au 17/10 (non faite) ; les prérequis « installable ailleurs » | — |
| **Clairance - Design** (branche `design-suite` depuis Q56/Q57 ; index `docs-fr/design/README.md`) | identité, nom, système de design, maquettes, fiches de composants, relecture des livraisons de l'interface | **migration close côté design le 15/10** (étapes 0 à 13, plan `04-migration.md`, relues conformes) ; identité (05), esquisses (07, D77), système de composants (08, artefact https://claude.ai/artifact/Ut6hWDRRUcWC58HHz6UP2K), quatorze écrans finaux (09, D78, galerie https://claude.ai/artifact/2gZSBGbTkEySxThbNRYhMF) ; depuis le 16/10, les fiches de Q56 (`ProcedureRoute`) et de Q57 (étiquette, sorties possibles), relues après construction ; référence figée `design-reference-2026-10-13` ; les captures de la carte réelle restent hors dépôt (D75) | les fiches que demande le propriétaire | relire chaque livraison de l'interface sur l'instance d'essai avant qu'elle parte en production |
| **Plan lanceur de Clairance** (depuis le 24/10 ; branche `lanceur`, arbre `.claude/worktrees/lanceur` ; mission dans `docs-fr/34-lanceur.md`) | le lanceur de la barre des menus, devenu poste de surveillance : gardien, tableau de bord natif, poste de commande ; en Swift, sur `tools/menubar/` seulement ; forme confiée à la session design (« Clairance natif macOS ») | **étape 1a fusionnée dans `local` le 24/10** (14c39e6) : lancement délégué au lanceur, instances repérées par PID, instances d'essai à échéance ; route de santé du serveur livrée (`/api/v1/health/detail`, 3307610) ; Q59 tranchée en (b), fiche design 10 validée | **étape 1b livrée le 25/10** (938f8c3) : gardien, notifications, icône à six états, Dock ; mesuré sur un banc d'essai (coupure ADS-B vue en 49 à 54 s, coupure audio en 64 s, co-atc tué en 4 s, arrêt demandé muet). Reste : une journée normale de production sans fausse alerte, après la relance par le propriétaire. Icône d'application définitive du design intégrée le 25/10 (3b6ed5f) | mesurer le temps entre une coupure provoquée et la notification (cible : moins de 2 min), et zéro fausse alerte sur une journée normale |
| **Whisper-lab** (`~/Dev/Aero/whisper-lab`, privé) | reconnaissance : modèles, corpus, mesures, GPU | précision par niveau d'indices mesurée (borne haute) ; Kyutai écarté (28/09) ; a proposé Q55 (15/10) ; bases du ciel témoin copiées par le labo (D82, 16/10) ; état depuis le 16/10 non relevé ici | tirage au hasard de la précision quand l'annotation aura avancé (jalon du labo) | la mesure de non-régression du modèle converti, le jour du portage |
| **ATC-scribe communauté** (`~/Dev/Aero/atc-scribe-communaute`, privé, dépôt `chatelp/atc-scribe-communaute`) | cadre légal, existant, communautés françaises, prérequis, recommandation | étude livrée le 12/10 (`SYNTHESE.md`, fiches 01 à 05) ; premier pas tranché le 13/10 avec le propriétaire (voir « Faits ») ; en veille | — (reprend sur question de Pierre) | le P0 de la fiche 04 entre dans le jalon « installable ailleurs » |
| **Station radio** (session de la station) | la station : réception, chaîne VHF, captations | en veille au dernier relevé (12/10) ; `radio-ctl-sync` rapporte l'état au bandeau | — | prévenue à chaque bascule d'écoute |
| **Cardputer / M5Stack** (projet `~/Dev/Aero/aero-cardputeradv`) | appareils de poche | une application d'écoute M5Stack, que le propriétaire a autorisée le 16/10 à appeler l'API `/selection` de la station ; état non relevé ici | — | — |

## Les jalons

### Faits

- Paliers 0 à 5 de l'audit produit (10–12/10) : retraits, réglages, journal d'événements,
  curseur d'altitude QNH/FL, réécoute, fiche de vol, séquence d'arrivée, consignes en clair,
  bandeau d'état, barre radio, bibliothèques locales, Tailwind compilé, `app.js` découpé,
  préréglages d'appariement mesurés (D73), preuve d'appariement (D72). Validé à l'usage par le
  propriétaire le 12/10 (« nette amélioration UX »).
- Bulles de dernière transmission sur la carte (12/10).
- **D75** (12/10) : position de la station à deux décimales, adresses du réseau domestique
  génériques, **historique git réécrit** (499 commits, quatre branches, poussé de force) ;
  sauvegarde complète d'avant sur le Mac ; reste la purge côté GitHub, à demander par le
  propriétaire.
- Garde-fou 3 rebouché (12/10, trouvé par la session design) : la production sert une copie
  figée de `www/` prise à son lancement (`server.static_dir`), plus les fichiers en cours
  d'édition ; l'instance d'essai reste la vérification en direct.
- Trois études lancées en parallèle (12/10) ; cadre légal, existant, communautés, inventaire
  visuel livrés.
- **D79** (13/10) : le mouvement des avions ne pulse plus. Deux défauts de l'amont révélés par
  la cadence de la station (une position toutes les ~1,7 s par avion, sondée chaque seconde) :
  prédiction serveur ancrée sur le relevé et non sur la position, moteur de la page qui
  re-datait le point figé. Mesuré avant/après dans l'onglet du propriétaire : 33 % → 3,3 % de
  pas à reculons. Commit autonome contribuable (70c8eff). Reste la liste Trafic qui vole
  ~100 ms par seconde au fil principal (étape 5 ou commit dédié).
- **Fiches H et I intégrées** (13/10 soir) : serveur sans transcription, Docker amont retiré ;
  CORS et WebSocket fermés aux origines étrangères, rôle lecture seule, écoute par le serveur
  réservée au propriétaire par défaut, purges réglables (à 0 chez nous). Le voyant
  « Transcription » connaît l'état Off.
- **Le nom : Clairance** (13/10, D74 complétée, Q53 close) : renommage fait le jour même — dépôt
  `chatelp/clairance`, page, lanceur `Clairance.app`, dossier de configuration `~/.config/clairance`,
  documents, README. Reste au propriétaire, quand toutes les sessions seront fermées : déplacer le
  dossier local `~/Dev/Aero/atc-scribe` et, s'il le souhaite, renommer le dépôt privé de la communauté.
- **La marque appliquée** (13/10, D76, signe = le scope radar, choisi par le propriétaire sur une planche de
  huit) : favicon et manifeste, images de la barre de menus, README ; puis le troisième état « à l'écoute »
  (favicon allumé quand une fréquence est écoutée, lanceur en couleurs quand la production tourne et que
  l'antenne est à l'antenne). La marque en tête de la connexion et de l'en-tête de l'outil relevait de la
  migration.
- **Le premier pas communautaire tranché** (13/10, avec la session communauté) : l'outil strictement local,
  installable ailleurs (le P0 de la fiche 04), puis un corpus simulé avec consentement (voie A seule) ;
  les voies sur du réel sont fermées ; ni avocat ni démarche auprès d'une autorité. Reste à décider
  plus tard : le pilote de corpus simulé.
- **Fiches cloud de la vague 3 toutes intégrées** (G à M, 13 et 14/10) : voir plus bas.
- **Migration du design terminée** (plan `04-migration.md`, 14 étapes, ≈ 17 j + 4 h 30 de serveur) : étapes 0 à 4
  versées dans `local` avant le 13/10 (système, familles de phase et palette F, barre du haut, onglet Traffic) ;
  5 (« Right now ») le 13/10 dans la nuit, après quatre corrections de la relecture design (dont le sens des
  pistes, côté serveur) ; 6 (le fil), 7 (la barre radio quitte le grand écran), 8 (suivre un vol avec « Then »
  et le cadre PAR, b0c4e23), 9 (fenêtres et pages), 10 (états et avis) le 14/10 ; 11 (clavier, focus, mouvement)
  le 15/10 ; 12 (le téléphone) et 13 (le nettoyage, avec la palette de la carte, choix C du propriétaire)
  relues conformes par la session design le 15/10 : côté design, la migration est close. Le propriétaire ayant
  choisi de passer directement en production plutôt que par une soirée sur l'instance d'essai, tout est versé
  dans `local` le 15/10 au soir (efaaa7a) ; **production démarrée le 15/10 à 18 h 33** sur efaaa7a (page figée
  3e6a65f), vrai modèle, première transmission en 4,4 s, après la mesure réseau de la station. Versés avec elle,
  relus conformes le 15/10 : le voyant « Transcription Simulated » d'un faux sidecar (deee2d5), la connexion au
  serveur perdue (son neutre après 10 s, ligne dans le journal, fil comblé, 9a8ac6b), l'échelle des altitudes
  aux couleurs des avions (cf144ea) ; la barre du haut au téléphone corrigée sur `main` (6a816f6).
- **Étiquettes allégées à faible zoom** (demande du 13/10, choisie le 15/10, cad595f, versée avec la migration) :
  l'indicatif seul au-delà de 24 NM sauf les avions qui comptent, le dernier message en bulle sous l'étiquette ;
  boîtes chevauchées de 81 à 20 % à 67 NM, de 67 à 37 % au zoom par défaut. Noté pour après : le détecteur de
  phases (aérodrome attribué loin de l'avion, APP qui retombe en UNK en courte finale).
- **Les avions fantômes** (16/10) : deux causes trouvées et corrigées (04dc5e3, d0b54d5), mesurées sur le trafic
  réel ; **en production depuis le 16/10 à 16 h 02** (ab4d319). La troisième famille (un recul de 5 à 40 s de
  vol) venait du rafraîchissement complet de la carte, déclenché par les transcriptions, qui replaçait chaque
  avion à sa position brute : corrigé, **en production le 16/10 à 22 h 47** (a09abf1) ; avec flux, 76 sauts en
  5 min au lieu de 580 (67 sans flux). **La règle de l'avion silencieux (D80) n'était pas en cause** : laissée
  telle quelle (le diagnostic du 16/10 au soir l'avait accusée, corrigé dans le doc 32). Même versement : les mises
  à jour d'avions ne portent plus l'avion entier (91 Mo en 5 min → 5,4 Mo).
- **Les procédures publiées dessinées sur la carte** (Q56, 16/10) : feu vert le 16/10, construit sur `migration`
  (726b10c, ef10b99), relu et clos par la session design ; **en production le 16/10 à 16 h 02** (ab4d319), le cache
  complété depuis le disque (fiches DATA, ENR 4.1, AD 2 de Beauvais). Reste son clic sur « Update from the SIA »
  pour écrire les routes dans la liste.
- **La piste et la sortie probables sur le cartouche** (Q57, 16/10) : mesurées (2 338 arrivées, 3 111 départs),
  forme du design, **décidées par le propriétaire et en production le 16/10 à 17 h 06** (9fcc281), avec les routes
  des sorties possibles du départ sélectionné ; quatre corrections de la relecture design (un nom par point, la
  route d'une puce l'emporte) versées à 17 h 18 (4ba147b) ; amorçage de la page sur la trace du serveur versé à
  17 h 51 (2c5c6f2) : premières sorties 4 s après l'ouverture, contre environ 70 s. À surveiller à l'usage : les
  sorties d'Orly, peu couvertes par la réception ; après chaque relance, les départs déjà en l'air n'ont pas de
  sortie (la piste de décollage n'est jugée que pour ceux vus décoller).

### En cours

**Le plan détaillé, par blocs, avec étapes, dépendances et mesures de sortie : `35-plan.md`** (26/10).
**Soir du 26/10, décisions du propriétaire** : production relancée à 17:02:06 (pistes gardées, fiches O et N en
service) ; ami le 28/10, invité aux adresses masquées et aux flux limités ; `/vitals` public ; connexion de 30 jours ;
ensuite **le lanceur, puis le serveur autonome** ; 61 $ de crédit cloud à déléguer dès maintenant.
Fiches cloud P (invité), Q (page dans le binaire, bilan au démarrage) et R (contrat du lanceur, côté serveur)
lancées puis intégrées le soir même ; en production à la relance du 27/10. La moitié Swift du contrat (C3)
revient à l'agent du lanceur, après le 28/10.
Préparé pour le 27/10 : la bascule audio sur la branche `b1-ter` ; A3 avec l'agent du lanceur ; la relance en trois
temps (A3, nouveau co-atc, bascule audio), une cause par relance. La page intégrée attendra que la page se stabilise.
**27/10 au matin** : fiche S intégrée et vérifiée sur le Mac : les sessions survivent à une relance ; l'ami ne
retapera plus son mot de passe à chaque mise à jour. Elle part avec la relance 2. Au passage, une panique rare à
l'arrêt corrigée. Puis, à la demande du propriétaire, les flux des ports annexes reçoivent le cookie (ebfd2ce),
vérifié dans Chrome ; même relance. Et « someone is speaking » fréquences coupées (98f675a) : la mesure du
serveur arrivait mais était jetée par la page ; correctif de page seulement, en ligne dès la relance 1 (chaque démarrage recopie `www/`).
**27/10, 13:17 : N5 passé**, goxiph validé (journal, base, lanceur et relevé Icecast de la station) : ffmpeg sera abandonné à la relance 3 avec `b1-ter` et le passage à 16 kHz, comme décidé le 26/10.
**27/10, 13:50 : relance 1 faite**, A3 en production (le porteur sur sa propre copie), même co-atc ; la page du jour (98f675a) part avec. Restent la relance 2 (nouveau co-atc) et la relance 3 (`b1-ter`, 16 kHz).
**27/10, 13:53 : relance 2 faite**, le nouveau co-atc (84db233) en production, essayé d'abord sur 8020 ; les connexions survivent désormais aux relances. Reste la relance 3 (`b1-ter`, 16 kHz, ffmpeg retiré).
**Fin du 26/10** : plus de relance aujourd'hui sauf panne (propriétaire). Faits l'après-midi : la page joue le son
elle-même (Q63), Safari compris depuis 16:45 (reprise au clic, rééchantillonnage dans la page) et l'iPhone
depuis 17:20 ; la couche des
procédures avec le choix « pistes en service / toutes » ; les pistes en service gardées d'une relance à l'autre (A7,
en production à la relance du 27/10 après N5). Le 27/10 vers 13 h 10 : jugement de N5, puis 16 kHz, retrait de
ffmpeg, de SRT et de la carte son. Fiches cloud O (ouverture publique, trace des connexions) et N (sidecar sur
Linux) relues, vérifiées sur le Mac et fusionnées : en production à la même relance. Au propriétaire : six constats
de la fiche O à trancher (05, Q62), puis le compte de l'ami (D4).
**Ordre des chantiers, accepté par le propriétaire le 26/10** :
1. le faux « Production did not start » du lanceur (agent du lanceur), puis une journée de production sans
   fausse alerte ;
2. une journée de mesures sur la production : justesse réelle de Q57, mémoire du sidecar, sauts ;
3. **Q60, le serveur auto-contenu** : d'abord la mesure du décodage audio en Go, puis la page dans le
   binaire, l'environnement propre du sidecar, le script de production absorbé ;
4. **Q61, le contrat entre le lanceur et le serveur** ; les étapes 2 et 3 du lanceur l'attendent ;
5. **Q62, l'accès à distance pour un proche** (ajouté le 26/10) : un usage de la période de tests restreints, pas
   une fonctionnalité ; audio compris ; par Internet derrière le Traefik du propriétaire ; mené **en parallèle** de
   B1 (décisions du 26/10) ;
6. en parallèle d'ici le 5/11, le crédit cloud (94 $) sur ce qui se vérifie sans station ni audio réel ;
7. puis le jalon « installable ailleurs » (avertissement légal, guide), et plus loin l'interface en/fr, le
   rapprochement instruction/réponse, Q51, le corpus simulé, le premier pas public.

0. **Le lanceur en poste de surveillance** (doc 34, 24/10) : agent dédié, étape 1 (le gardien) d'abord.
   Côté serveur, la route de santé est faite (3307610). Étape 1a fusionnée (14c39e6) ; porteur livré (e71e4bd,
   Q59) ; gardien de l'étape 1b livré (938f8c3, 25/10). À faire par le propriétaire : démarrer la vraie production
   par le nouveau lanceur, en prévenant la station ; la journée sans fausse alerte se mesure ensuite. Ce qui doit précéder la prochaine relance de la
   production : rien ; elle portera la route dès qu'elle sera relancée sur un binaire reconstruit.
1. **Les prérequis pour être installable ailleurs** (communauté, question 4) → liste hiérarchisée →
   chantier technique dans cette session (voir « À venir »).
2. **À regarder à l'usage par le propriétaire** : le fond clair IGN/OSM et le téléphone (versés avec la
   migration) ; les étiquettes entre 34 et 48 NM autour de l'approche de Roissy, où les avions qui comptent
   sont serrés (laissé tel quel pour une soirée, choix du propriétaire, 15/10).
3. ~~**L'analyse des mesures de la nuit du 16 au 17/10**~~ — **retirée le 26/10** : la production était sourde, rien à en tirer. (`runs/mesures/nuit-2026-10-16/` : relevé de la
   production toutes les 5 min, instance d'essai avec flux par tranches de 55 min) : **pas encore faite**. Constat de
   la revue du 20/10, de visu sur les fichiers, non analysé : la production était sourde à partir de 23 h 05
   (voir « Les risques suivis »), le total de transmissions du relevé reste figé à 8 109 de 23 h 08 à minuit, et
   seule la première tranche de l'instance (22 h 48) porte des données ; les tranches suivantes sont vides. À refaire sur une nuit où la chaîne entend.

### À venir, dans l'ordre probable (à confirmer par le propriétaire à chaque étape)

*Le renommage, l'application de la marque et la migration du design, qui figuraient ici, sont faits : voir « Faits ».*

| Jalon | Dépend de | Décidé ? |
|---|---|---|
| **Installable ailleurs** — le P0 de la fiche 04 de l'étude, « avant d'en parler à quiconque » : sidecar porté ; ~~mode sans transcription (le serveur démarre sans sidecar)~~ — **fait le 13/10** (fiche H) ; avertissement légal ; ~~rediffusion de l'audio coupée par défaut et rôle lecture seule~~ — **faits le 13/10** (fiche I : `audio_replay = "owner"`, rôle `viewer`) ; guide d'installation bilingue ; ~~Docker de l'amont retiré~~ — **fait le 13/10** (fiche H). **Restent : le sidecar porté, l'avertissement légal, le guide d'installation** (revue du 20/10). Mesure de sortie : une installation sur une machine vierge, à partir du seul guide, **en moins d'une heure, par quelqu'un qui n'a pas écrit le code** ; précision d'appariement inchangée après portage (95 %, D73) | portage (ci-dessous) pour la transcription ; le reste n'en dépend pas | principe : oui (mission communauté) ; ordre et calendrier : non |
| Version en/fr de l'interface | après le design (les textes bougent) | principe : oui (10/10) |
| **Portage du sidecar : Linux x86-64 + NVIDIA** (faster-whisper / CTranslate2, repli processeur ; Pi qui reçoit, transcription déportée) ; **Windows au moins pour l'affinage** (les simmers) ; sous-point pour un mode différé (Pi 5, dix minutes de retard) : apparier contre le ciel de l'instant de la transmission relu dans `adsb_targets`, comme le fait déjà l'outil de mesure, au lieu de la flotte vivante (modification bornée au point d'appariement) | question 4 ; format permettant l'affinage ; non-régression mesurée au labo | **cible : oui (12/10) ; pas dans l'immédiat** (Q54) |
| **Refonte de la structure de l'écran** — **esquisses validées le 13/10 (D77)** : A par défaut (fil unifié au centre, ciel à côté, colonne « Right now » / « Traffic », la carte suivant les filtres de la liste), B colonne repliée, C suivre un vol (son histoire dans la colonne, son approche dans le cadre PAR, la séquence = le tableau « Arrivals »). La session design passe au système de composants. Si le fil est retenu, deux chantiers serveur bornés : (1) rapprocher chaque instruction ATC de la réponse du pilote qui la suit, **pour la lecture seulement, jamais un jugement d'erreur** (rien de type SLASH) ; (2) ~~enregistrer la raison d'un non-appariement~~ **fait le 13/10** (D72 complétée : `unmatched: …` dans `match_reason`, sans score). **Panneau « Right now », faisabilité vérifiée le 13/10** : tout tient avec l'existant (piste en service et depuis quand, antenne, compteurs, première détection, transferts entendus, plan de descente, heure de toucher) sauf trois petits chantiers, validés par le propriétaire le 13/10 : ~~le journal d'événements qui survit au rechargement~~ et ~~les messages ADS-B par seconde dans le bandeau~~ **faits le 13/10** ; ~~la séquence d'arrivée hors de la PAR~~ et ~~les gabarits français des consignes~~ **faits le 13/10** (`store.arrivals` pour tous les aérodromes suivis, `nextTouchdown`, la PAR dessine depuis le même calcul ; `text_fr` sur chaque consigne, `plainFrench(t)` dans la page avec le nom de la fréquence écoutée) ; « transcription dans 2 s » n'est pas possible (le serveur découpe à la fin de la transmission), « quelqu'un parle » l'est par le niveau audio | esquisses validées par le propriétaire | structure **livrée par la migration (15/10)** ; chantiers serveur du fil : (2) fait le 13/10 ; (1) rapprocher instruction et réponse, non revérifié à la revue du 20/10 |
| **Premier pas (recommandation révisée le 13/10)** : rendre Clairance installable comme **outil strictement local** — c'est le jalon « installable ailleurs » ci-dessus, même mesure de sortie ; ses garde-fous (rien ne sort de la machine, rediffusion de l'audio coupée par défaut, avertissement) passent du rang de bonne pratique à celui de **condition** : c'est ce qui rend l'outil défendable sans avocat, comme les guides français qui recommandent déjà une clé SDR pour la bande aviation. **Aucune démarche** auprès de la DGAC, de la DSNA ou de l'ANSSI, ni consultation d'avocat (objection du propriétaire : coût, et attirer l'attention « pour rien ») | — | **oui (13/10)** |
| Corpus simulé avec consentement (**voie A, la seule retenue**) : une parole simulée n'est pas une communication réelle (ni 226-15 ni le terrain de la DGAC ; reste le RGPD) ; consentement écrit d'après des modèles universitaires gratuits, relecture gratuite possible (CNIL, clinique juridique) ; pilote avec IVAO France ou un cercle proche, **après** l'étape « installable ». C'est aussi la source naturelle du **jeu de validation partageable** | installable ailleurs ; décision du propriétaire sur le pilote | principe : oui ; pilote : non |
| Affinage communautaire ou fédéré (modèle adapté au français) sur le corpus simulé | corpus ; portage | non |
| ~~Voies B (poids entraînés sur du réel) et C (transcriptions réelles publiées)~~ : **fermées** ; un avocat ne redevient utile que si on veut les rouvrir | — | fermées (13/10) |
| Premier pas public (quoi, où, quand) | installable ailleurs ; identité | non |

Plus tard, à discuter (doc 32) : strips de vol, clairance contre trajectoire, vue d'approche en
plan, procédures du SIA, fréquences comme un pupitre, programmation d'écoute.
## Les sessions cloud, vague 3 (13/10, validée par le propriétaire) : toutes intégrées

Sept fiches (G à M) dans `docs-fr/fiches-cloud/` (prompts à coller dans son README), du serveur et de la
logique pure, sans la station : **G** la route publique des chiffres avant connexion et les trois
valeurs qui manquent ; **H** le serveur sans transcription et le Docker amont retiré ; **I** les
dettes de sécurité (CORS, WebSocket, rôle lecture seule, purges) ; **J** la compilation
Windows ; **K** « Then » sans verdict ; **L** (ajoutée le 13/10 au soir, après D79) le bloc
position, trajectoire, prédiction remis d'équerre : banc de rejeu synthétique avec métriques de
mouvement, un seul prédicteur, la prédiction en direct hors de `service.go`, l'observé séparé
du déduit dans la page, interface écrite du moteur — à métriques égales ou meilleures ; **M** (écrite par la session
design, relue ici) le mouvement mesuré dans un vrai navigateur, les trois cas restants (première minute, reprise
après un trou, onglet remontré) et la comparaison de stratégies. Lancées par le propriétaire (G, H, I le 13/10 au soir ;
L le 13/10 au soir ; J, K et M le 13/10 dans la nuit) ; **toutes intégrées** : G le 13/10 (route `/api/v1/vitals`, `since` de
la piste, valeur et unité des consignes, version et commit du binaire), H et I le 13/10 (soir tard), L dans la nuit
du 13 au 14/10 (vitesse dessinée qui varie de 8 à 13 % contre 38 à 48 %, plus aucun pas à reculons hors trous de
réception), J, K et M le 14/10 au matin (Windows compile ; « Then » prêt pour l'étape 8 ; banc du mouvement et cinq
corrections, stratégie A confirmée par le propriétaire, D80). Les fiches A à F (vagues 1 et 2) l'avaient été le 10/10 ;
A, B et C relancées par erreur le 13/10 puis arrêtées n'ont laissé aucune branche à intégrer. **Rien n'est en attente**
(revue du 20/10). La migration de l'interface, elle, est restée sur le Mac.

**Le crédit** : 250 $ au départ, **94 $ restants le 24/10** (relevé du propriétaire), jusqu'au 5/11/2026 ; ce qu'on en lance encore est une décision (n° 10 ci-dessous).

## Les décisions attendues du propriétaire

1. ~~La stratégie du mouvement~~ — **décidée le 14/10 : A** (D80), prolonger la trajectoire et corriger en douceur ; B et C restent des options du banc.

2. ~~Le nom~~ — **décidé le 13/10 : Clairance** (D74 complétée, Q53 close), renommage fait le
   même jour : dépôt `chatelp/clairance`, page, lanceur `Clairance.app`, dossier de configuration
   `~/.config/clairance`, documents. Reste à faire par le propriétaire, quand toutes les sessions
   seront fermées : déplacer le dossier local `~/Dev/Aero/atc-scribe` (commande fournie le 13/10)
   et, s'il le souhaite, renommer le dépôt privé `atc-scribe-communaute` et les sessions.
3. ~~**Le système de design**~~ — **tous ses choix tranchés** : (1) police, (2) cinq familles de
   phase, (3) rampe d'altitude (palette F, 13/10), (4) J / K au lieu de Tab (13/10), (5)
   l'interpolation des avions gardée sous « Réduire les animations » (**15/10**), (6) esquisses
   (D77) et écrans finaux (D78). La migration elle-même est suivie dans le tableau des chantiers
   (étapes 0 à 13 versées le 15/10 ; production au lancement suivant).
4. ~~Le premier pas communautaire~~ — **tranché le 13/10** avec la session communauté : pas
   d'avocat ni de démarche auprès d'une autorité ; le premier pas est l'outil strictement local
   installable ailleurs (P0), puis un corpus simulé avec consentement (voie A seule) ; les
   voies sur du réel sont fermées. Reste à décider plus tard : le pilote de corpus simulé.
5. **Demander à GitHub la purge des anciens objets** (*reportée par le propriétaire le 14/10*) (D75, deux réécritures : le 12/10 pour la
   position et les adresses, le 13/10 au soir pour les captures de la carte réelle entrées par
   la fusion du design ; sauvegardes `atc-scribe-sauvegarde-2026-10-12.git` et `-13.git`) :
   les commits d'avant restent servis par GitHub par leur identifiant jusqu'à une demande au
   support, que seul le propriétaire peut faire (texte fourni le 12/10, identifiants à citer :
   `d0b14c3`, `cdbf843`, `a892d77`, `1febe52`, `e786aeb`, `bde4b04`). À vérifier aussi : le réglage de
   précision de la carte de couverture sur FlightAware (« My ADS-B »).
6. Le moment du **portage** (cible fixée, calendrier non).
7. Le **tirage au hasard** de la précision d'appariement (jalon du labo, lié à l'annotation).
8. ~~**Les noms officiels de l'AIP dans l'appariement**~~ — **lancée le 15/10 au soir** : l'outil est fait et se pilote depuis les réglages ; la grammaire, mesurée sur le ciel témoin, **n'est pas adoptée** (35 justes perdus, 0 gagné). L'outil est en production depuis le 16/10 à 10 h 09 (liste faite par le propriétaire). L'indice de direction, mesuré le 16/10, n'est pas adopté (l'avion désigné s'approche du point, mais 39 % des autres aussi) ; l'affichage du nom réel est **en production depuis le 16/10 à 12 h 21** (seuil 0,95, réglable : 18 sur 18 justes sur la séance d'écoute ; liste refaite par le propriétaire à 12 h 23 avec les 100 tables de codage, 25 points de procédure) ; sur le trafic des 15 et 16/10, environ six procédures nommées par jour, toutes justes à la relecture. Énoncé d'origine (Q55, proposée par le labo le 15/10) :
   l'outil générique qui fabrique la liste sur place (rien de l'AIP dans le dépôt), puis la
   grammaire des procédures, l'indice de direction au départ et l'affichage du nom réel ; quoi
   et quand. Recommandation : l'outil et la grammaire après la migration, mesurés par paires.
9. ~~**Les procédures dessinées sur la carte**~~ (Q56, 16/10) — **feu vert le 16/10, construit sur
   `migration` le même jour** (726b10c, ef10b99) d'après la fiche `ProcedureRoute` : la route au clic
   sur la puce, la couche « Procedures (SID, STAR) » éteinte par défaut. Relu et clos par la session
   design ; **versé en production le 16/10 à 16 h 02** (ab4d319) sur la demande du propriétaire, le
   cache complété depuis le disque (fiches DATA, ENR 4.1, AD 2 de Beauvais). Reste son clic sur
   « Update from the SIA » pour écrire les routes dans la liste.
10. **Le crédit cloud restant** (revue du 20/10) : **94 $ restants le 24/10** (relevé du propriétaire), jusqu'au 5/11, soit douze jours ; toutes les fiches A à M sont
   intégrées, rien n'est en attente. Quoi lancer avant l'échéance, ou rien ? Pistes, à trancher par le propriétaire (elles doivent se
   vérifier sans la station et sans audio) : le guide d'installation bilingue et l'avertissement légal du jalon « installable
   ailleurs » ; la version en/fr de l'interface, que l'achèvement de la migration débloque ; des tests de ce qui tourne sans filet.
   Chaque fiche demande d'abord une relecture ici et une soirée d'écoute pour tout ce qui se voit ; lancer tard laisse peu de
   temps à l'intégration.

11. ~~**Qui porte la production quand le lanceur se ferme**~~ (Q59) — **tranchée le 24/10 : (b)**, la production
   portée à part du menu. **Porteur mesuré et livré le 24/10** (e71e4bd) : une instance à part de `Clairance.app`
   (`open -n`) ; l'agent launchd est écarté, faute de réseau local. L'étape 1a est fusionnée (14c39e6) et la fiche
   design 10 est validée.

## Les risques suivis

- **Deux liaisons avec la station** (trouvé par l'agent du lanceur, 25/10) : le script d'un démarrage manqué
  restait vivant à attendre son co-atc, et le démarrage réussi suivant de `production.command` aurait lancé une
  seconde liaison `radio-ctl-sync`. Le lanceur l'arrête désormais ; le risque demeure pour qui lance
  `production.command` à la main deux fois de suite. Raison de plus pour ne démarrer que par le lanceur.

- ~~**Latence de transcription**~~ — **réduite le 14/10 au soir** (D81 et correctif du sidecar).
  Avant, sur l'instance d'essai : 9,2 s de la requête à la première lecture en médiane, 16,4 s au
  troisième quartile, 28,6 s au 90ᵉ centile ; le labo a montré que la file (un seul fil, la lecture
  française de l'une passant devant l'anglaise de l'autre) et la reconstruction du modèle à chaque
  bascule anglais/français en faisaient l'essentiel. Après, 83 transmissions sur dix minutes de
  trafic réel : **1,0 s en médiane, 1,9 s au troisième quartile, 3,3 s au 90ᵉ centile** ; la
  lecture française suit, la file de seconde voie se vide d'elle-même (0 à 4 en attente). Reste à
  surveiller : un maximum de 24 s, probablement la construction des modèles au démarrage.
- **Brevet US12277862B1** (Aerlogics, 2025) : corrélation transcription ATC / ADS-B pour la
  conformité ; aucun dépôt européen trouvé, revendications non lues. À faire lire avant toute
  diffusion hors de France (Q53).
- **Cadre légal** : audio réel non partageable ; corpus simulé seulement avec sessions dédiées et
  accord écrit (les règles de VATSIM et d'IVAO ne valent pas consentement RGPD).
- **Le nom** : choisi le 13/10 ; marques vérifiées dans TMview le 13/10 (aucune marque « CLAIRANCE » seule active, aucune à
  l'EUIPO ni aux États-Unis, `design/00-nom.md`). Le dépôt est public sous ce nom (`chatelp/clairance`).
- **Production lancée hors session (16-18/10)** : du 16/10 à 23 h 04 au 18/10 à 13 h 36, la production n'a rien entendu. Cause :
  l'application Claude de bureau s'est mise à jour et relancée seule, au repos (« stealth update », 23:04:53 dans
  `~/Library/Logs/Claude/main.log`) ; tout processus démarré depuis la session Claude (la production, via la ligne de commande du
  lanceur de la barre des menus, l'instance d'essai, les liens avec la station) a perdu l'accès au réseau local de macOS :
  « no route to host » vers la station chaque seconde, aucune transcription les 17 et 18/10, alors qu'Internet (météo)
  passait. L'instance d'essai a aussi tourné jusqu'au 18/10 à 13 h 35, le script de nuit n'ayant arrêté que les sondes. **Leçon** : la
  production est lancée par le propriétaire (lanceur de la barre des menus) ou par le système (`open`), jamais comme enfant d'une session Claude
  (`CLAUDE.md`, règle 5, 20/10) ; même règle pour tout ce qui doit passer la nuit. Les mesures de la nuit du 16 au 17/10 en sont marquées (voir « En cours », n° 3).
- **Secrets et données** : jamais de clé, de mot de passe, d'audio ni de base dans un dépôt ou
  dans le cloud (règles permanentes).
- **Dettes à solder avant toute installation ailleurs** (vérifiées dans les sources le 12/10 pour
  la fiche 04 de l'étude communauté) : ~~CORS renvoie toute origine avec les identifiants, le
  WebSocket accepte toute origine, aucun rôle (tout compte connecté réécoute l'audio) ; l'archive
  audio et les journaux bruts n'ont pas de purge ; Docker est mort (compile un seul fichier)~~
  — **soldées le 13/10 par les fiches H et I** (CORS et WebSocket limités aux pages du serveur
  et à `allowed_origins`, rôle `viewer`, `audio_replay`, purges réglables, Docker amont retiré,
  serveur sans transcription) ; la compilation Windows est faite (fiche J, 14/10 ; l'exécution sous Windows reste à vérifier). Sans
  gravité tant que D5 tient (réseau local seulement) ; bloquante le jour où quelqu'un expose le
  serveur.

## Comment ce document se tient

À chaque message d'une session qui livre, à chaque décision du propriétaire, à chaque jalon :
la ligne du chantier, le tableau des jalons et la liste des décisions attendues sont mis à jour
dans le même commit que le reste. Le propriétaire peut demander « où en est-on ? » : la réponse
est ce document.