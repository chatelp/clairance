# Audit produit : Clairance n'est plus co-atc — 09/10

*Ouvert le 9 octobre 2026 à la demande du propriétaire (Q52 dans `05-decisions.md`). L'étude du
09/10 ouvre le document ; D71 a été validée le 10/10 et le document est depuis **le journal du
chantier produit** : la consolidation et ses paliers, puis, dans la dernière section, « État » (du plus récent
au plus ancien), chaque livraison datée (revue du 20/10).*

## Pourquoi maintenant

Mesuré le 09/10 contre `upstream/main` (dernier commit amont : 3 mai 2026) :

| | |
|---|---|
| Commits du fork | 263 |
| Fichiers touchés | 169, dont **130 nouveaux** et 38 fichiers amont modifiés |
| Lignes | **+33 941 / −1 312** |
| Serveur Go aujourd'hui (hors tests) | 35 077 lignes |
| Interface (JS + HTML) | 13 036 lignes |

Ce que le fork a ajouté n'est pas une adaptation : transcription locale par sidecar, lecture
française à deux modèles, liaison avec la station (`radio-ctl-sync`), un flux par fréquence,
appariement avion-transmission validé contre l'ADS-B, vue PAR, juge des pistes validé contre
FlightAware, authentification, accès réseau. Ce que l'amont visait (Toronto, API OpenAI, chat
ATC, simulation) n'est pas ce qui tourne ici. L'interface, elle, est restée celle de l'amont,
avec nos ajouts par-dessus : c'est là que ça se voit le plus (D70 et le son, 02/10 ; la coupure
à −40 dB, 04/10 — deux défauts amont découverts parce qu'on s'en sert tous les jours).

## Périmètre et méthode

Trois études parallèles, factuelles, chiffrées, limitées à 150 lignes chacune, rapports bruts
dans `docs-fr/audit-2026-10-09/` (copiés dans le dépôt le 10/10 pour les sessions cloud) :

1. **`ux.md`** : inventaire de chaque panneau, bouton, modale et réglage, classé essentiel /
   utile / inutilisé chez nous / douteux ; frictions (écran d'accueil, clics pour le son,
   téléphone à 390 px) ; ce qui manque pour notre usage (antenne, station, sidecar) ; dix
   changements classés par gain et effort.
2. **`backend.md`** : paquets Go, lignes, utilisé ou non dans notre configuration, couverture
   de tests ; ce qui est à nous contre l'amont ; code mort, fonctions de plus de 150 lignes ;
   `config.toml` réel contre `config.toml` lu ; dix simplifications avec risque pour la
   production.
3. **`frontend-code.md`** : carte de `www/`, dépendances, code mort (chat, simulation,
   météo, OpenAI temps réel), grosses fonctions, et un plan de découpage de `app.js`
   par étapes qui laissent l'interface fonctionnelle.

Les trois rapports sont ensuite consolidés **ici**, en une liste unique classée par
gain / effort / risque, et le propriétaire tranche ce qui devient chantier.

## Ce que l'audit doit respecter

- **Ne pas casser ce qui marche** (consigne du propriétaire, 10/10). Ce qui tourne aujourd'hui
  a été mesuré et validé : juge des pistes 368/368 contre FlightAware, appariement mesuré
  contre l'ADS-B, PAR, lecture française, liaison station, archive audio. Optimiser le code
  ne vaut rien si l'un d'eux recule. Les garde-fous, à chaque étape :
  1. **Un pas à la fois, un commit par pas**, défaisable par `git revert`, et le binaire
     précédent gardé (`runs/co-atc.avant-<date>`) pour revenir en une minute.
  2. **Le filet avant de retirer quoi que ce soit** : côté interface, le script qui vérifie que
     chaque `$store.atc.X` d'`index.html` et chaque `store.X` des autres fichiers existe
     encore, `node --check` et les tests de la PAR ; côté serveur, `go build`, `go vet`,
     `go test -race ./...`. Le compilateur et ce script tranchent, pas l'intuition.
  3. **L'instance d'essai d'abord** (`runs/son-test/`, compte, vrais flux de la station) : tout
     changement se regarde dans un navigateur et s'écoute avant de toucher la production.
  4. **Les mesures de référence se rejouent** après tout changement d'un chemin qui compte :
     l'outil d'appariement (`cmd/phraseology`), le juge des pistes sur une journée, les compteurs
     d'animation avant/après avec 80 à 110 avions pour le flux d'avions.
  5. **Le propriétaire valide à l'usage**, une journée d'écoute, avant le pas suivant sur
     le même terrain.
  6. **On ne renomme rien de ce qui est enregistré chez lui** : les 44 clés `localStorage`
     (ses préférences), les noms du store lus par `index.html`, les routes de l'API utilisées
     par `radio-ctl-sync`, le sidecar et les outils.
  7. **Retirer avant de déplacer** : supprimer du code mort ne change pas l'ordre
     d'initialisation ; déplacer, si. Et on ne déplace pas ce qui va être réécrit.
- **Le serveur tourne tous les jours.** Chaque étape doit laisser la production utilisable ;
  rien de « grand soir ».
- **Garder identifiable ce qui est contribuable** (CLAUDE.md) : sidecar, correctif de
  rotation (Q26), authentification, état serveur — même si on ne rebase plus.
- **L'UX se juge sur l'usage réel** : une personne, tous les jours, Mac et téléphone par
  Tailscale, 1 à 7 fréquences. Pas sur ce que l'interface amont permet.
- **Mesurer avant de conclure** : chaque suppression proposée cite le fichier et le nombre
  de lignes, chaque friction cite le geste et le nombre de clics.

## Alignement avec les objectifs de départ (10/10, à valider avant tout code)

Demande du propriétaire : avant de coder, vérifier que le plan ne contredit pas les objectifs
de départ, qui visaient un fork réintégrable dans co-atc. Relu : `00-mission.md`, le
« Why a fork » du README public, Q11 (15/09), D19 (16/09), D3 (15/09), Q7.

### Ce que les textes fondateurs disent

| Texte | Ce qu'il promet | Tenu ? |
|---|---|---|
| `00-mission.md` | « Adapter Co-ATC pour qu'il tourne entièrement en local … et en faire un dépôt public réutilisable » ; trois apports : zéro cloud, bilingue, multicanal réel | oui ; les trois apports existent et sont mesurés |
| `00-mission.md`, hors périmètre | l'assistant vocal et l'extraction de clairances par IA : « on décidera plus tard s'il passe en local, devient optionnel ou disparaît » | la question était ouverte ; tranchée le 10/10 : il disparaît |
| `00-mission.md`, périmètre | météo française (SIA) à la place de Windy (Q7) | **non fait** ; Windy marche (144 relevés/jour) et l'audit la garde |
| Q11 (15/09) | « vrai fork rebasable », parce que notre part est profonde mais étroite : « nous ne touchons pas une ligne de l'interface » | **plus vrai** : 17,9 % d'`index.html`, 8,8 % d'`app.js`, et le plan UX va bien au-delà |
| D19 (16/09) | le dépôt reste un fork git (historique, remote, MIT, crédit) ; **la rebasabilité n'est plus une règle** ; « ce qui est contribuable reste identifiable » (sidecar et `local.go`, Q26, `auth`, état serveur) | tenu ; c'est la règle en vigueur |
| D3 (15/09) | « transcription entièrement locale, l'API OpenAI est retirée » | décidé, **pas fait** : 1 990 lignes du chemin OpenAI sont toujours là |
| README public | « Upstream's map, ADS-B ingestion, flight-phase detection and web interface are used **as they are** … nothing here improves on them » ; assistant vocal « untouched, and off unless you supply a key » | **faux depuis la PAR et le juge des pistes** ; faux dès que le chat part |

### Les contradictions, et comment les lever

1. **« Fork réintégrable » (Q11) contre « produit à part » (Q52).** Q11 a été révisée par D19
   dès le 16/09 : on ne rebase plus, on ne code plus pour rester rebasable. L'audit ne crée pas
   la rupture, il la constate et l'étend à l'interface. Ce qui reste de l'intention d'origine,
   et qui a un sens : **contribuer en amont ce qui est utile à d'autres**, par des correctifs
   autonomes qu'on peut proposer tels quels, sans que le dépôt entier ait à se rapprocher.
   Lever : une décision explicite (D71 ci-dessous) qui remplace l'objectif de Q11.
2. **« Interface utilisée telle quelle » (README, Q11) contre la refonte UX (paliers 1, 2, 4).**
   Une fois la page Réglages, les alertes et le téléphone refaits, l'interface est la nôtre.
   L'amont n'a rien publié depuis le 3 mai ; rien à absorber. Lever : le dire dans D71 et
   réécrire la section « What is different » du README.
3. **« Dépôt public réutilisable » contre « retirer ce qui ne sert pas ici ».** Pas de
   contradiction si « réutilisable » est défini : *une autre station, avec son propre
   récepteur et ses propres flux, sur Apple Silicon ou avec un service compatible
   `LOCAL-STT`*. Ce qui sert cette définition se garde ou s'étend (sources audio par ffmpeg,
   options d'entrée, sources ADS-B autres que tar1090) ; ce qui ne la sert pas se retire
   (chat OpenAI, simulation, couches de cartes américaines, chemin OpenAI déjà retiré par D3).
4. **D3 non exécutée.** Retirer le chemin OpenAI (1.2) n'est pas une décision nouvelle :
   c'est l'exécution de D3, en retard de trois semaines.
5. **Q7 abandonnée en silence.** La météo française n'a jamais été faite et Windy convient ;
   à fermer proprement (Q7 : « Windy gardée, Q36 »), pour que la mission ne promette plus ce
   que le plan ne fait pas.

### Décision : D71 — Clairance est un produit à part *(validée le 10/10)*

1. **Le dépôt reste un fork git** : historique amont, remote `upstream`, licence MIT de Yegor S,
   crédit dans le README — D19.1, D6, D7 inchangés ; `docs/` reste à eux.
2. **On ne rebase plus, jamais** ; si l'amont reprenait, on y prendrait des cerises
   (`cherry-pick`) commit par commit, pas l'inverse.
3. **Ce qui n'a pas de rôle ici se retire**, il ne dort pas : chat vocal OpenAI, simulation,
   couches américaines, chemin de transcription OpenAI (D3). Tout reste dans l'historique git.
4. **L'interface est la nôtre** : réglages, alertes, barre radio, téléphone, et ce qu'Clairance
   sait et ne montre pas. La disposition générale reste celle de l'amont, par choix du
   propriétaire.
5. **Le contribuable reste identifiable, et s'allonge** : en plus de la liste de D19 (sidecar
   et `local.go`, rotation Q26, `auth`, état serveur), le correctif WebSocket (0.1), les deux
   index SQLite (0.4), la fuite de contexte signalée par `go vet`, et les options d'entrée
   ffmpeg avec les options HTTP réservées aux sources HTTP (code écrit le 10/10, en réserve).
   Règle : **un correctif contribuable = un commit autonome**, qui ne dépend d'aucun de nos
   retraits, pour qu'on puisse l'offrir en l'état.
6. **« Réutilisable » veut dire** : une station avec ses propres flux (tout ce que ffmpeg lit),
   tar1090/readsb ou l'une des autres sources ADS-B, Apple Silicon pour le sidecar ou tout
   service qui honore `docs/LOCAL-STT.md`. Le README le dit en ces termes.

**Si D71 est validée, les documents changent ainsi** : `00-mission.md` reçoit un paragraphe
« Révision du 10/10 » (la phrase d'une ligne devient « … et en faire un produit public pour
une station de réception, né d'un fork de Co-ATC ») ; README : « What is different » réécrit,
« used as they are » retiré, assistant vocal « removed », une ligne sur les sources audio ;
Q11 reçoit une note « remplacée par D71 » ; Q7 est fermée ; CLAUDE.md cite D71.
**Validée le 10/10 ; les documents ont été mis à jour le même jour** (mission, README, Q11,
Q7, CLAUDE.md, D71 dans `05-decisions.md`).

### Où en est le code

Le retrait du lecteur SRT et les options d'entrée ffmpeg ont été écrits le 10/10 avant cette
mise au point, rangés en réserve git le temps de l'alignement, puis **commités après la
validation, en deux commits autonomes** comme D71.5 le demande : d'abord les options d'entrée
et les options HTTP réservées aux sources HTTP (offrable à l'amont tel quel : c'est un défaut
amont, mesuré — ffmpeg 8.0.1 s'arrête sur « Option reconnect not found » pour toute source non
HTTP), puis le retrait du lecteur SRT (le nôtre). Compilé, `go vet`, tests `-race` verts, deux
tests nouveaux. La production n'est pas relancée dessus : son binaire date du 2/10 ; le
prochain lancement prendra le nouveau, et l'instance d'essai passe avant (garde-fou 3).

## Le jugement du propriétaire (09/10)

Il est le seul utilisateur ; sa liste prime sur l'inventaire. Dans ses mots :

1. **Interface lourde et pas très moderne.**
2. **Affichage lent des déplacements d'avion** — « peut-être incontournable avec un
   affichage navigateur ». *À mesurer avant de l'admettre* : le serveur relève l'ADS-B
   toutes les secondes (`fetch_interval_seconds = 1`) et diffuse une prédiction par seconde
   (`livePredictionBroadcastInterval`), et `app.js` a un moteur d'animation avec ses propres
   compteurs (images/s, marqueurs mis à jour/s, lignes 339-343). Un navigateur sait animer
   à 60 images/s ; si c'est saccadé, c'est la chaîne relevé → diffusion → interpolation qui
   le fait, pas le navigateur. Premier relevé à faire : les compteurs du moteur sur une
   séance réelle, avec 80 à 110 avions.
3. **Les réglages sont « totalement mal foutus »** : ils méritent une **page à part,
   organisée, lisible**, plutôt qu'une modale.
4. **Les alertes en modale en haut de l'écran** ne sont « ni très intéressantes ni pratiques
   en l'état ».
5. **La disposition générale lui convient** : on ne refait pas l'agencement, on l'allège.

Ce que l'audit UX doit donc hiérarchiser : d'abord les réglages et les alertes (3, 4), puis
l'allègement (1), et une mesure pour trancher la lenteur (2) — sans toucher à la disposition
(5).

## Propositions (09/10, avant les rapports)

Le fil conducteur : **le serveur calcule déjà ce que l'interface ne montre pas.** Chaque
transmission a une preuve d'appariement (`callsign_evidence`), les clairances sont extraites
dans une table (`clearances`), les phases et les verdicts de piste sont datés, et le sidecar
archive **l'audio de chaque transmission** sur le disque externe. co-atc, pensé pour afficher
une carte et un fil de texte, n'a rien prévu pour ça. Efforts en jours de travail, estimés,
à affiner après les rapports ; « risque » = pour la production quotidienne.

### A. Montrer ce qu'on sait déjà (gain fort, effort faible)

| | Proposition | Ce qu'il faut | Effort | Risque |
|---|---|---|---|---|
| A1 | **Écouter la transmission transcrite** : un clic sur une ligne joue son audio. Pour un novice, entendre en lisant est le meilleur apprentissage ; pour nous, c'est le contrôle de qualité le plus direct. | le sidecar renvoie le nom du fichier archivé, co-atc le garde en base et le sert depuis le disque externe (désactivé si le disque manque) | 1 j | faible |
| A2 | **La fiche de vol** : l'histoire d'un avion, toutes fréquences confondues — première écoute, secteur, descente, transfert (« contact De Gaulle 126,425 » entendu), établi 27R, posé 27R à 14:32, confirmé FlightAware. Un clic sur un avion ouvre cette chronologie. | transcriptions par hex/indicatif + `phase_changes` + juge des pistes, et la détection des transferts dans le texte (fréquence citée → notre catalogue) | 1,5 j | faible |
| A3 | **La preuve de l'appariement** au survol de l'indicatif : pourquoi on croit que c'est cet avion (indicatif lu, altitude citée, position), avec un badge de confiance. Aujourd'hui l'indicatif s'affiche comme une certitude. | `callsign_source`, `callsign_evidence` déjà en base | 0,5 j | nul |
| A4 | **Les clairances en clair** sous la transmission : « Autorisation : descendre au niveau 100 » ; « Approche ILS 27R ». Le novice comprend ; l'expert vérifie l'extraction. | table `clearances`, un gabarit de phrase par type | 1 j | nul |
| A5 | **L'aide au survol partout**, comme dans la vue PAR (objet `HELP`) : chaque terme (squawk, QNH, établi, remise des gaz) expliqué en une phrase. | généraliser l'infobulle de `par-view.js` | 1 j | nul |

### B. Des outils de contrôleur, à hauteur de novice (comme la vue PAR)

| | Proposition | Ce qu'il faut | Effort | Risque |
|---|---|---|---|---|
| B1 | **La séquence d'arrivée par piste** (un AMAN simplifié) : qui atterrit ensuite, distance au seuil, heure estimée, espacement avec le précédent en NM et en secondes. À côté de la vue PAR. | juge des pistes + géométrie PAR + vitesse sol ; calcul pur | 1,5 j | nul |
| B2 | **Les strips de vol** par fréquence écoutée : une bande par avion en contact — indicatif, type, phase, dernier message, niveau autorisé (clairance), prochaine fréquence (transfert entendu). La bande passe d'une fréquence à l'autre quand le transfert est entendu ; si la fréquence est à l'antenne, l'avion y est attendu. | A2 + A4 ; le transfert est la pièce nouvelle | 2 à 3 j | faible |
| B3 | **Clairance contre trajectoire** : niveau autorisé contre altitude ADS-B, cap, vitesse — « il fait ce qu'on lui a dit ». Présenté comme information, pas comme alarme : une transcription fausse donnerait des faux écarts, et c'est justement ce que ça révèle. | A4 + ADS-B 1 Hz | 2 j | faible |
| B4 | **Vue d'approche en plan** par aéroport : axes de piste prolongés, portes à 5/10/15 NM, traînées — le complément de la PAR, qui est de profil. | géométrie PAR existante | 2 j | nul |
| B5 | **Procédures** (STAR, SID, points de report, attentes) en fond de carte, depuis l'eAIP France du SIA : rend le trafic lisible (« il suit MOPAR »). | une source de données à qualifier d'abord | inconnu | nul |

### C. L'interface, dans l'ordre du propriétaire

| | Proposition | Effort | Risque |
|---|---|---|---|
| C1 | **Page Réglages à part**, par usage : Compte et accès · Écoute (fréquences, volume, archive) · Transcription (modèles, français, seconde lecture) · Avions et pistes (station, aéroports) · Affichage · Données (rétention) · Station (liaison). Les réglages amont sans objet disparaissent ; ceux qui restent dans `config.toml` sont listés en lecture avec leur valeur. | 2 j | faible |
| C2 | **Les alertes** : plus de modale en haut ; un journal discret (dernier en tête) des événements qui comptent — remise des gaz, changement de piste en service, squawk d'urgence ou MAYDAY, type d'avion d'intérêt (A380, militaire, hélicoptère), bascule de la station, sidecar ou flux tombé — avec un son au choix par événement. Le juge des pistes fournit déjà les deux premiers. | 1 j | nul |
| C3 | **Alléger** : retirer ce que l'inventaire UX classera « inutilisé chez nous » (chat ATC, simulation, météo si elle ne sert pas, OpenAI temps réel) ; remplacer l'écran d'accueil par un seul bouton « activer le son », là où le navigateur l'impose. | 1 à 2 j | faible |
| C4 | **Téléphone** : à 390 px, transcriptions et tuiles d'abord, carte à la demande. | 1,5 j | nul |
| C5 | **Les fréquences comme un pupitre** : la tuile montre le secteur, l'aéroport, et la **qualité mesurée** (battements/min, bruit — `/radio/mesures`) ; et on choisit l'écoute depuis co-atc (`POST /radio/selection`, via la liaison), ce qu'on a fait à la main pendant deux heures le 02/10. | 2 j | moyen : ça bascule l'antenne, à cadrer (D-à-venir) |

### D. Technique

| | Proposition | Effort | Risque |
|---|---|---|---|
| D1 | **La fluidité, mesurée puis corrigée** : lire les compteurs du moteur d'animation sur une séance réelle ; puis, selon le résultat, rendu Leaflet en canvas, estime entre deux relevés (vitesse et cap, comme tar1090), réactivité Alpine bridée pour 100 avions, diffusion des différences (`change_detector`) plutôt que du lot complet chaque seconde. | 0,5 j de mesure, puis 1 à 2 j | faible |
| D2 | **Découper `app.js`** en modules, sans changement fonctionnel, par étapes qui laissent la page utilisable (plan du rapport `frontend-code.md`). | 3 j, étalés | faible |
| D3 | **Nettoyer le serveur** : paquets et sections de configuration sans objet (liste du rapport `backend.md`), `config.toml` réduit à ce qui est lu. | 1 à 2 j | faible |

### E. Fonctionnement

| | Proposition | Effort | Risque |
|---|---|---|---|
| E1 | **Relance automatique** de la production après un redémarrage du Mac (`launchd`, `caffeinate`) : la crainte de chaque absence. | 0,5 j | faible |
| E2 | **Programmation d'écoute** : un groupe par plage horaire (la nuit → `en-route`), avec retour — ce que `nuit.sh` a fait une fois à la main. Plus tard, « l'antenne sur les fréquences les plus claires » d'après les mesures : à discuter, ça déplace l'antenne seul. | 1 j | moyen |
| E3 | **L'état en un coup d'œil** dans co-atc : antenne (groupe, version, depuis quand), flux présents, sidecar (modèles, GPU), archive (fichiers, espace), bases (taille, rétention). | 1 j | nul |

### Ce que je recommande en premier

**A1, A2, B1, C1 + C2, A4** — dans cet ordre. Les trois premiers changent ce qu'on *fait* avec
Clairance (écouter en lisant, suivre un vol, voir qui atterrit ensuite) pour cinq jours de
travail, sans toucher à la disposition ; C1 et C2 sont les deux demandes du propriétaire ;
A4 ouvre B2 et B3. D1 se mesure dès la prochaine séance, avant de choisir. Le reste attend
les rapports.

## Consolidation des trois rapports (09/10)

Rapports bruts : `docs-fr/audit-2026-10-09/{ux,backend,frontend-code}.md` (131, 123 et 142
lignes). Tout ce qui suit est mesuré, sauf mention. Aucun serveur ne tournait pendant l'audit :
pas de mesure de charge réelle ni de fluidité sur trafic vivant.

### Ce que les trois rapports établissent

**Interface.**
- **Téléphone à 390 px : inutilisable.** Carte de 0 px de large (colonne droite fixe de
  480 px), page de 514 px coupée de 124 px, barre radio de 2 494 px sans défilement. Aucune
  règle `@media` dans `style.css`, aucun attribut `aria-`, 97 textes de 7 à 10 px.
- **Mac à 1 440 px : la barre radio déborde dès 5 fréquences** ; à 7, les pistes en service,
  la météo et l'heure sortent de l'écran. Chaque tuile fait 260 à 296 px et répète la fréquence.
- **Réglages** : un volet de 320 px, 700 lignes, 1 392 px de défilement ; 7 sections dont 5
  amont sans objet (Display, General, Debug — 39 % du défilement —, Station, Simulated) ; les
  22 réglages utiles sont en dernier. Pas de bouton de déconnexion.
- **Alertes** : 418 lignes ; 13 306 changements de phase le 04/10, dont 72 % de bruit (UNK,
  CRZ, NEW) ; l'alerte de clairance appelle `this.addAlert`, défini nulle part (à confirmer en
  console) ; le clic droit n'a pas d'équivalent tactile.
- **L'avion identifié est invisible dans 80 % des cas** : le badge `@indicatif` n'apparaît que
  sur les lignes pilote ; 345 des 1 696 transmissions appariées du 04/10 l'affichent.
- **Fond de carte par défaut** : la carte VFR américaine de la FAA (`vfr-sectional`), tuiles en
  404 sur Paris, 200 sur New York : un navigateur neuf voit une carte vide.
- **Chargement** : 5 ressources en CDN (Alpine et Tailwind sans version épinglée), 890 Ko sans
  compression ni cache (`static.go:107` : `no-store`) ; gzip ramènerait `app.js` de 278 à 56 Ko.
  **Sans internet, la page ne se charge pas.**
- **`app.js`** : 6 040 lignes, dont 5 586 dans un seul objet Alpine de 360 propriétés ; 21
  fonctions jamais appelées (346 lignes) ; 126 `console.log`, dont 21 dans le flux d'avions.
- **Retirable sans toucher à la station** : chat IA 1 225, simulation 360, fonctions mortes 330,
  couches américaines 210, clones 230 : **2 355 lignes (13,6 % du JS et du HTML)**.
- **Déplacements** : tar1090 publie à 1 Hz une position déjà âgée d'1,2 à 1,8 s ; co-atc diffuse
  un delta par avion plus une extrapolation serveur par seconde ; le navigateur extrapole encore
  (plafond 30 ips, gain 0,72, recalage lissé). Deux extrapolations empilées sur une donnée à
  1 Hz. **Le navigateur n'est pas seul en cause ; la latence de bout en bout n'est pas mesurée.**

**Serveur.**
- 35 077 lignes de Go, **32 % à nous** ; ≈ 6 500 lignes amont (18,6 %) **inactives dans notre
  configuration** (chat ATC, gabarits, simulation, chemin OpenAI, sources ADS-B hors tar1090,
  SRT) ; 1 485 lignes **mortes quoi qu'on configure** (81 fonctions, 80 amont).
- **Un gel démontré en test** : un client WebSocket qui cesse de lire (téléphone en veille,
  Tailscale qui bascule) fige `Broadcast` après ≈ 0,5 Mo, et la boucle ADS-B avec lui, jusqu'à
  la fermeture TCP par le noyau. *Jamais vu dans les journaux* : le seul trou diurne (02/10,
  13:42-13:51) est l'arrêt par le propriétaire et la relance, documentés plus haut.
- `config.toml` : 559 lignes, 151 clés, dont **46 sans effet** et 20 égales au défaut ; une
  version réduite tient en ≈ 110 lignes et 85 clés. `atc_chat.enabled = true` sans clé faisait
  afficher un bouton de chat « Disconnected » en permanence — **passé à `false` le 09/10.**
- Gains bon marché mesurés : **−14,8 % de base** en retirant deux index SQLite redondants
  (136 → 116 Mo sur une copie), **−51 % de lignes de journal** en rétrogradant deux messages.
  L'écriture SQLite (12,3 ms par cycle d'une seconde) et le JSON (2,1 µs) ne sont pas des goulots.
- Couverture de tests 26,8 % ; `websocket` 0 %, `api` 8 %, `storage` 15 %, `audio` 17 %.
- **D19 est périmé** : notre empreinte sur `app.js` est passée de 1,7 % à 8,8 %, sur
  `index.html` de 3,1 % à 17,9 %, sur `adsb/service.go` de 0,2 % à 9 %. La règle tient (modifier
  l'amont quand c'est le plus simple) ; l'argument « empreinte faible » ne tient plus.

### La liste unique, classée

Classement par gain pour l'usage quotidien, puis effort, puis risque pour la production.
Efforts en heures (h) ou jours (j), estimés par les rapports ou par moi. **État au 10/10**
indiqué en fin de ligne quand il a bougé ; chaque ligne se fait sous les sept garde-fous.

**0. Corrections — avant tout chantier (≈ 1 j)**

| # | Quoi | Effort | Risque |
|---|---|---|---|
| 0.1 | **WebSocket** : échéance d'écriture, ping/pong, verrou relâché pendant l'écriture ; le test du gel devient test de non-régression — ***fait le 10/10*** (`24b8cd0`) : le test échoue sur l'ancien code (Broadcast bloqué 10 s), passe en 0,45 s sur le nouveau ; vérifié dans un navigateur sur l'instance d'essai, 55 s, même connexion, 8 286 messages, aucune reconnexion ; la fermeture 1005 d'une page rechargée n'est plus une erreur (`c82df04`) | 4 h | moyen-faible |
| 0.2 | **Fonds de carte — urgent** (étude du 09-10/10, `docs-fr/audit-2026-10-09/fonds-de-carte.md`, recoupée sur le Mac) : **les fonds Carto `dark` et `light` sont cassés** (tuile « API KEY REQUIRED », même fichier pour toutes les tuiles : clé gratuite désormais exigée) et les fonds FAA sont vides sur Paris ; **seul `osm` marche**. Le cache du service worker masque la panne sur les zones déjà vues et y fige le filigrane. Aucun fond sombre testé ne rend les pistes lisibles (Carto vectoriel : contraste 1,02:1). Proposition à arbitrer : fond **Plan IGN v2** (Licence Ouverte, sans clé, pistes et taxiways nets dès z13) ou OSM, **assombri par filtre CSS** sur sa seule couche ; **les pistes, taxiways et aires dessinés par nous** (OurAirports, domaine public ; OSM Overpass, ODbL) en couche vive, lisible sur tout fond ; **la VAC du SIA en lien** calculé sur le cycle AIRAC (réutilisation permise si non altérée, source et date citées) ; espaces aériens plus tard (Open Flightmaps sans clé jusqu'à z12, ou openAIP avec clé, CC BY-NC). Écartés : SCAN OACI de l'IGN (interdit aux particuliers), proxy ADS-B Exchange (aucune autorisation). **Validée par le propriétaire le 10/10 et confiée à la fiche B** (étapes 6 à 8 : fonds IGN et OSM assombris, défaut selon le pays de la station, Carto retiré, cache du service worker vidé ; plan des aéroports généré par station depuis Overpass, non commité ; lien VAC dans la PAR) ; le réglage à l'œil se fera sur le Mac | 2 h + 1 j | nul |
| 0.3 | Deux messages de journal en `debug` ; fuite de contexte signalée par `go vet` ; `go mod tidy` — ***fait le 10/10*** (`4978ed1`, `0bf7f6b`, `e9c336b`) : 48 % des lignes du 04/10 ; `go vet` propre sur tout le dépôt | 1 h | nul |
| 0.4 | Deux index SQLite redondants, après `EXPLAIN QUERY PLAN` (−14,8 % de base) — ***fait le 10/10*** (`7a17b4c`) : plans identiques avant/après sur les sept requêtes ; base du 01/10 compactée 129 → 110 Mo ; supprimés aussi à l'ouverture d'une base qui les a ; vérifié sur la base de l'instance d'essai | 1 h | faible |
| 0.5 | Bouton **Se déconnecter** (aucun aujourd'hui) — ***fait le 10/10*** (`3f183cb`) : en tête du volet de réglages ; vérifié à 1 440 × 900, cliquable, session fermée côté serveur | 0,5 h | nul |
| 0.6 | `atc_chat.enabled = false` | fait | — |

**1. Retirer ce qui ne sert pas ici (≈ 2 à 3 j)**

| # | Quoi | Effort | Risque |
|---|---|---|---|
| 1.1 | Interface : chat IA, simulation, fonctions mortes, couches américaines, clones (étapes 1 à 5 du plan `frontend-code.md`, −2 355 lignes) ; volets Debug, Station, Simulated | 9 h | faible |
| 1.2 | Serveur : 81 fonctions mortes, chemin OpenAI (D3 le dit déjà retiré, −1 990 lignes) | 10 h | nul-faible |
| 1.3 | Serveur : chat ATC et gabarits (−3 250 lignes) — *tranché le 10/10 : on retire* | 8 h | faible-moyen |
| 1.4 | Serveur : simulation (−375 lignes) — *tranché le 10/10 (D71) : les sources ADS-B hors tar1090 sont **gardées** (réutilisable) ; le lecteur SRT est **retiré** (fait, `e9c336b`, −208 lignes, −9 modules), remplacé par les options d'entrée ffmpeg (`1858fc6`)* | 2 h | faible |
| 1.5 | Accueil : plus d'écran bloquant ; un bandeau « reprendre l'écoute » au premier clic (le seul geste que le navigateur exige) — ***fait le 10/10*** : l'écran (84 lignes, 29 de CSS, son d'accueil) est retiré ; « Click anywhere to resume listening » s'affiche tant qu'une écoute mémorisée attend, et le premier clic ou la première touche la relance. Vérifié sur l'instance d'essai, après une connexion : bandeau visible, puis au clic sur la carte la fréquence mémorisée joue et l'autre reste coupée. La frappe du mot de passe ne compte pas (la page s'initialise après la connexion) : il faut un clic | 2 h | nul |
| 1.6 | ~~Relance automatique (`launchd`)~~ — *refusée le 10/10 : la production ne doit pas tourner en permanence sur le Mac.* **Remplacée par un lanceur dans la barre de menus** : une icône qui dit si la production tourne (et l'instance d'essai) ; Démarrer, Arrêter ; ouvrir co-atc (local, Tailscale) ; antenne et sidecar en une ligne. Application SwiftUI native (`MenuBarExtra`), sans dépendance tierce : Xcode et Swift 6.4 sont sur le Mac (vérifié). Elle reprend la logique de `runs/production.command` (mot de passe rangé, liaison station, arrêt propre) et inscrit/retire la ligne du planning du GPU. — ***fait le 10/10*** (`7a65ab1`, `tools/menubar/`) : installé dans `~/Applications/Clairance.app`, réglages dans `~/.config/clairance/launcher.json` ; il lance le script de production existant et l'arrête par SIGTERM ; état (depuis quand, sidecar, antenne), ouvrir la page, copier l'adresse Tailscale, journal ; tient la ligne du planning du GPU. Vérifié par son mode `--start`/`--stop` (même code que les boutons) : démarrage en 11 s, arrêt propre en 9 s, ligne du planning écrite puis retirée. Le menu lui-même est à essayer par le propriétaire (l'outil de contrôle de l'écran ne voit pas les applications de barre de menus) | 1 j | faible |

**2. Les deux demandes du propriétaire (≈ 2 j)**

| # | Quoi | Effort | Risque |
|---|---|---|---|
| 2.1 | **Page Réglages** à part, 5 sections (Écoute · Appariement avec préréglage Mesuré / Prudent / Expérimental · Affichage « ce navigateur » · Compte · Système en lecture) ; supprime le volet de 700 lignes — ***fait le 10/10*** : page plein écran (roue dentée ; Échap ou croix pour fermer ; la barre radio reste utilisable), cinq rubriques Listening · Callsign matching · Display · Account · System, le diagnostic de performance replié en bas de System ; contrôles déplacés tels quels, liaisons inchangées ; la surcharge de position de la station par GPS est retirée de la page (son code JS reste, à retirer avec le découpage d'`app.js`). Les préréglages Mesuré / Prudent / Expérimental : ***faits le 12/10*** (`b4cfab6`, **D73**), valeurs mesurées sur les journées du 03 et 04/10 et validées par le propriétaire ; le défaut reste le réglage en vigueur | 12 h | faible |
| 2.2 | **Alertes** : supprimer la barre (418 lignes, 72 % de bruit, clairance cassée), puis un **journal discret des événements qui comptent** (remise des gaz, changement de piste, MAYDAY ou squawk d'urgence, type d'intérêt, bascule de la station, sidecar ou flux tombé), son au choix par événement — ***fait le 10/10*** (`7690d41`, `www/events.js`) : la barre et ses fonctions retirées (−360 lignes, avec l'appel cassé de la clairance) ; une cloche à côté de la roue dentée ouvre le journal (100 événements, badge des non-lus). Événements : squawk 7500/7600/7700, remise des gaz (le juge de piste la rapporte désormais, `cc47f6a`), changement de piste en service tenu 2 min, type rare ou très gros (A388, A124, B748…), bascule de canaux de la station, flux audio perdu ou revenu ; un clic montre l'avion ; son par type, l'urgence seule par défaut. Au passage, **le squawk n'arrivait pas dans les mises à jour en direct** (`a322f8f`, correctif contribuable) : un 7700 déclaré en vol ne serait jamais apparu. Vérifié sur l'instance d'essai (urgence simulée sur un avion réel, bascule de piste simulée, PAR sous les boutons). **Pas fait** : « sidecar tombé » (la page ne suit pas encore son état) et MAYDAY entendu à la radio (relève du palier 3, avec la transcription) | 1,5 h + 1 j | nul |
| 2.3 | **Filtre d'altitude** (demande du propriétaire, 10/10 : « un curseur intelligent, marge haute et basse, gestion des flight levels ») — ***fait le 10/10*** (`633637d`, `www/altitude/`) : plancher et plafond sur un seul curseur, la liste et la carte suivent pendant le glissé ; échelle non linéaire (pas de 100 ft jusqu'à l'altitude de transition, soit la moitié de la course ; 500 ft jusqu'au FL195 ; FL par dix jusqu'au FL450 ; puis « sans plafond ») ; pieds jusqu'à la TA, niveaux de vol au-dessus ; histogramme du trafic du moment sur le curseur, compteur « n of m airborne », préréglages (≤ 3000 ft, ≤ TA, ≤ FL100, FL100–195, ≥ FL195). Les avions au sol restent sous le bouton Ground. **Altitudes lues comme un contrôleur les dit** (11/10, `075a089`, à la demande du propriétaire) : pieds sur le QNH jusqu'à la TA, niveau de vol au-dessus, GND au sol, dans la liste, la fiche, la trajectoire, les étiquettes de la carte et les deux filtres ; QNH = médiane des calages ADS-B (`nav_qnh`) des avions à moins de 40 NM sous la TA, l'estimation de la vue PAR, recalculée toutes les 30 s (mesuré le 11/10 : 1015,2 hPa sur 11 à 12 avions) ; standard tant qu'aucun n'en envoie. **Aides contextuelles** (même demande) : `www/help-tips.js` étend la bulle de la vue PAR à toute la page (attribut `data-help`) ; posées sur l'altitude de chaque avion, le QNH, la TA, l'échelle, les préréglages et compteurs du curseur, et le « ALT » à côté de la piste en service (piste alternative, pas une altitude). À étendre à chaque palier. Côté serveur, `min_altitude`/`max_altitude` de `/aircraft` filtrent enfin (`56c3404`, défaut 1 de la fiche F, correctif contribuable) | 1 j | faible |

**3. Montrer ce que le serveur sait déjà (≈ 6 à 7 j)**

| # | Quoi | Effort | Risque |
|---|---|---|---|
| 3.1 | **Avion identifié sur 100 % des lignes appariées** (20 % aujourd'hui) ; corps de texte ≥ 11 px — ***fait le 11/10*** (`e2d0629`) : dans le fil par fréquence, toute ligne appariée nomme son avion (« ATC → AFR123 » quand le contrôleur lui parle), cliquable tant qu'il est à portée, grisé sinon ; bulles d'aide sur le locuteur, l'avion et la lecture FR ; en-tête de ligne 9 → 11 px (le corps était déjà à 12 px). Mesuré sur la base du 04/10 : 1 696 lignes appariées, 345 affichaient l'avion (20 %), 1 696 maintenant. Vérifié sur l'instance d'essai avec des transmissions injectées dans la page | 2 h | nul |
| 3.2 | **Écouter la transmission transcrite** (A1) : le sidecar renvoie le fichier archivé, co-atc le sert — ***fait le 11/10*** (`7d333be`, `6aa6d9c`) **sans toucher au sidecar** : la clé (fréquence, `created_at` comparés en instants) existait déjà dans les manifestes quotidiens de l'archive ; `internal/audioarchive` les indexe et les relit par incréments. `GET /api/v1/transcriptions/{id}/audio` (WAV, plages, derrière la connexion, jamais hors de l'archive, 404 sinon) et `GET /api/v1/audio-archive` (disque branché ou non) ; dossier = `--save-audio` du sidecar, ou `audio_archive_dir`. Bouton lecture sur chaque ligne du fil et de la section Radio, un extrait à la fois, le direct baissé à 20 % pendant la réécoute. **Mesuré** : 8 322 transmissions sur 8 322 du 03/10 et 5 512 sur 5 512 du 04/10 retrouvent leur audio. Vérifié sur l'instance d'essai avec les 25 transmissions du 10/10 copiées dans sa base (lecture, arrêt, durée 6,26 s égale au manifeste, cas sans audio, atténuation) | 1 j | faible |
| 3.3 | **Fiche de vol** (A2) : chronologie d'un avion toutes fréquences, transferts entendus, verdicts de piste — ***fait le 11/10*** (`8e7b8c4`, `www/timeline/`) : onglet TIMELINE du panneau avion (DETAILS reste l'onglet par défaut), du plus ancien au plus récent : première détection, phases (NEW et UNK omises), pistes du juge, remise des gaz, chaque transmission appariée sur toutes les fréquences avec sa réécoute et ses valeurs, transferts (fréquence donnée par le contrôleur, extraite par la grammaire : 947 le 03/10, nommée si la station l'écoute), clairances, signal perdu ; heures d'un autre jour datées. Sans code serveur. **Non fait** : la confirmation FlightAware (quota AeroAPI). Vérifié sur l'instance d'essai avec les transmissions réelles d'un vol du 10/10 | 1,5 j | faible |
| 3.4 | **Séquence d'arrivée par piste** (B1), à côté de la PAR — ***fait le 11/10*** (`90351b6`) : sous chaque cadre de la vue PAR, les avions entrant dans le cône de l'extrémité d'arrivée, du plus proche du seuil au plus loin : distance au seuil (décalé compris), temps à la vitesse sol du moment et heure estimée, écart avec le précédent en NM et en secondes, en ambre sous 3 NM ; l'autre extrémité aussi quand des avions y arrivent (piste en service inconnue, face-à-face). Fonction pure `arrivalSequence` dans `par-geometry.js`, deux tests. Vérifié sur l'instance d'essai (LFPG et LFPO ajoutés à ses aérodromes suivis) : AFR1061 à 7,3 NM de la 26L, 2:32 ; DAL264 à 9,0 NM de la 27R | 1,5 j | nul |
| 3.5 | **Clairances en clair** (A4) sous la transmission — ***fait le 11/10*** (`7ac7930`, `2a8eb72`) : plus large que les clairances, toute consigne reconnue par la grammaire est dite en clair (« Descend to FL100 », « Turn left heading 270 », « Contact 119.255 », « QNH 1015 », « Cleared to land, runway 27R ») sous chaque transmission du fil, de la section Radio et de la chronologie ; vert pour une clairance, ambre pour le reste, bulle d'aide. `Result.Plain()` dans la grammaire (verbe dans l'expression ou les trois mots avant, montée/descente avant maintien, verbes français, « left » après un nombre = côté de piste) ; champ `plain` sur chaque transmission servie, relu à la lecture (**17 µs par transmission**, mesuré sur les 5 512 du 04/10, dont **36 %** disent quelque chose), et dans la mise à jour en direct. Défauts trouvés sur données réelles et corrigés avant commit : « remontez » non reconnu, côté de piste pris pour un virage, cap doublé, zéro en tête | 1 j | nul |
| 3.6 | **Bandeau d'état** (E3) : antenne (sélection, depuis quand, mode), sidecar (statut et `degraded`), ADS-B, « récepteur prêté ou éteint » au lieu d'une barre vide — ***fait le 11/10*** (`28be23b`, `3bc36f2`, `1a1822d`) : `radio-ctl-sync` rapporte à chaque lecture l'état de la station à co-atc (`PUT /api/v1/receiver` : chaine → on_air, exploration → lent, transition → switching, arret → off, avec le libellé, la sélection et « depuis » de la station) ; co-atc le garde en mémoire, périmé après une minute sans rapport. `GET /api/v1/status` réunit antenne, sidecar (relu au plus toutes les 15 s), ADS-B (avions actifs) et archive audio ; il remplace `/audio-archive`. Trois voyants avant les horloges (Antenna, Transcription, ADS-B) avec bulles d'aide ; message à la place de la barre vide quand le récepteur est prêté, éteint ou en bascule. Générique : sans rapporteur, le voyant antenne dit « ? » et pourquoi. Vérifié sur l'instance d'essai avec la vraie station (lecture seule) : « On air · A l antenne · Selection libre - 124,350 + 126,425 · since 09/10 », 71 avions ; récepteur prêté simulé. **Binaire `bin/radio-ctl-sync` reconstruit** (ancien : `runs/radio-ctl-sync.avant-etat`) | 7 h | faible |
| 3.7 | Preuve de l'appariement au survol (A3) ; aide au survol partout (A5) — ***fait le 11/10*** (`266e324`, `aee93d9`, puis les bulles) : **D72** (validée par le propriétaire) : score et raisons du matcher gardés avec la transmission (`match_score`, `match_reason`, écrits dans la même requête que l'indicatif ; « in sector » ajouté quand la recherche est restreinte au secteur). À côté de l'avion apparié, trois barres disent la **force des indices** (forte : indicatif exact + un autre indice concordant ; moyenne : un indice, ou un indicatif partiel soutenu ; faible : indice partiel ou contradiction) et la bulle dit pourquoi en clair, avec le score ; c'est un compte d'indices, **pas une probabilité mesurée**, la bulle le dit. Dans le fil, la section Radio et la chronologie ; rien pour les transmissions d'avant. Bulles d'aide ajoutées sur les filtres de phase, cap (m/t/trk), TAS, GS, distance, Air/Ground, « Heard on the radio », Active/Signal Lost, préréglages, piste en service : **408 éléments** de la page portent une aide (compté sur l'instance d'essai). **Suite** : mesurer la précision par niveau d'indices sur le jeu annoté du labo — fiche écrite le 12/10 (`docs-fr/fiches-labo/precision-par-indices.md`), lancée et **faite le 12/10 par l'agent du labo** (`whisper-lab/resultats/labo/2026-10-12-precision-par-indices.txt`, script relançable `scripts/labo/precision-indices.py`) : sur les 59 transmissions validées, fort 34/34 justes (Wilson 90–100 %), moyen 12/12 (il manque 4 cas), faible 9/9 (il en manque 7), aucun cas faux. **Borne haute** : le jeu vient d'un tri (attachées, valeur ADS-B confirmée) et la phrase montrée portait l'indicatif réécrit. Pierre garde le tirage au hasard (attachées et non, sans réécriture) comme jalon ultérieur du labo ; d'ici là la bulle de la page reste « compte d'indices, pas une probabilité » | 1,5 j | nul |

**4. Lisibilité et accès (≈ 2,5 j)**

| # | Quoi | Effort | Risque |
|---|---|---|---|
| 4.1 | **Barre radio** : 7 tuiles visibles à 1 440 px (≈ 150 px la tuile, sans fréquence répétée, défilable) ; pistes en service et heure fixées à droite ; « tout écouter / tout couper » ; volume par fréquence — ***fait le 11/10*** (`47da917`) : tuiles de 160 px (nom seul quand il commence par la fréquence), bande défilable à la molette, reste de la barre à largeur fixe ; bouton « all » (écouter tout / couper tout) ; curseur de volume par tuile, mémorisé par le navigateur ; le fil de transcriptions garde 320 px. **Mesuré à 1 440 px, 7 fréquences, 3 aérodromes suivis : barre de 1 440 px (plus de débordement), 5 tuiles visibles, les 2 autres à un coup de molette** — l'objectif de 7 visibles n'est pas atteint : les voyants d'état (3.6) et les pistes de trois aérodromes prennent ≈ 590 px à droite ; avec deux fréquences et un aérodrome, tout tient sans défiler | 5 h | nul |
| 4.2 | **Téléphone** (sous 768 px) : colonne pleine largeur, carte / PAR / liste en onglets, barre radio défilable, fiche avec bouton fermer, aide au toucher — ***fait le 11/10*** (`ebd74d0`) : trois onglets Map / PAR / List au-dessus de la barre radio, un volet à la fois ; la sélection d'un avion sur la carte ouvre sa fiche dans le volet liste, bouton « Back to the list » ; cap et TAS cachés sur téléphone ; barre radio défilable ; appui long = bulle d'aide. **Mesuré à 375 px** (émulation) : document 375 px (avant : la colonne de 480 px forçait 563), lignes 349 px sans chevauchement, carte 375 px. Non essayé sur un vrai téléphone | 10 h | nul |
| 4.3 | **Chargement** : dépendances servies par co-atc et épinglées (plus d'hôte externe), Tailwind compilé, gzip, scripts en `defer` ; **mesure de latence de bout en bout** (instant d'observation contre instant de dessin) avant toute décision sur le moteur d'animation — ***en partie fait le 11/10*** (`d52592a`). **Mesuré avant** : 1,76 Mo par chargement, dont 1,2 Mo non compressés servis par co-atc (plan d'aérodrome 871 Ko en `text/plain`, `app.js` 251 Ko), et tout rechargé à chaque F5 (`no-store`). **Fait** : gzip sur le texte (app.js 56 Ko, plan 187 Ko en `application/geo+json` ; audio et WebSocket hors des types, `Flush`/`Hijack` transmis) ; `no-cache` au lieu de `no-store` (304 si inchangé : **un rechargement transfère 8 Ko**) ; tous les scripts en `defer`, Alpine en dernier et **épinglé 3.14.9** (était `3.x.x`). **Latence mesurée** (30 s, 709 positions) : observation → réception p50 325 ms, p90 480, p99 592 (sondage tar1090 à 1 s) ; réception → premier dessin p50 54 ms, p90 97. **Décision** : le moteur d'animation n'est pas le poste de latence, on le garde. **Terminé le 11/10** (`7e6991a`, accord du propriétaire pour les téléchargements) : OpenLayers, Font Awesome (CSS + polices solid et regular) et Alpine dans `www/vendor/`, servis par co-atc, **plus aucun hôte externe** ; Tailwind compilé par `tools/tailwind/build.sh` (31 Ko au lieu du compilateur CDN de 400 Ko), placé après `style.css` comme le CDN l'injectait. **Vérifié** : sur les 280 combinaisons balise + classes de la page (réglages ouverts), tous les styles calculés sont identiques au CDN ; l'unique écart trouvé (l'ombre d'un bouton) venait de l'ordre des feuilles et a été reproduit. Le filet `www-check` recalcule l'empreinte des sources inscrite en tête du CSS compilé : une classe ajoutée sans recompilation est signalée. Au second chargement : 11 Ko transférés. **Réactivité, mesurée le 12/10** (fil principal, boucle `MessageChannel`, 30 s, 130 à 157 avions, même `app.js`) : **CDN Tailwind : 92 % occupé (916 ms/s), 122 blocages > 50 ms, le plus long 573 ms ; compilé : 24 à 26 % (241 à 257 ms/s sur deux passes), 17 à 24 blocages, le plus long 91 ms.** Le compilateur du CDN re-scannait le DOM à chaque mise à jour d'avion : c'est lui que le propriétaire a senti partir (« plus réactive, même avec beaucoup d'avions ») | 6 h | faible |

**5. Structure, sans changement fonctionnel (≈ 4 j, étalés)**

| # | Quoi | Effort | Risque |
|---|---|---|---|
| 5.1 | `app.js` : étapes 6 à 10 du plan (audio, station, fiche avion, flux) ; l'étape 11 (réglages, alertes) se fait avec 2.1 et 2.2 — ***fait le 12/10*** (`961a156`) : **5 139 → 910 lignes**. Six domaines dans `www/store/` (audio 402, transcripts 553, station 526, aircraft-details 1 120, feed 755, sidebar 645), chacun une fabrique poussée dans `window.AtcStore` qu'`app.js` appelle dans `alpine:init` (même instant d'évaluation que le littéral unique : les valeurs initiales lisent `generateUUID`, `API_BASE_URL`…, ce que le bac à sable a attrapé au premier essai) ; assemblage par descripteurs (les 8 accesseurs restent des accesseurs). Cache de tuiles → `tile-cache.js`, `serverSettings()` → `server-settings.js`. Déplacé à l'identique par script (reconstruction du littéral vérifiée à l'octet). **Preuve** : `tools/www-check/store-dump.mjs` avant/après, 343 entrées, 339 identiques, les 4 autres sont l'heure et l'identifiant de client ; filet vert (1 109 `this.X`) ; parcours complet sur l'instance d'essai ; **chemin chaud** : fil principal occupé 17 % avec 106 avions (24 à 26 % le matin même avec 130 à 157, avant). En passant : l'étape 4 de E fusionnée sous test (`743c995`), la surcharge GPS retirée partout (`12d502a`) | 19 h | faible, chemin chaud à mesurer |
| 5.2 | `config.toml` réduit (559 → ≈ 110 lignes) avec défauts en code et détection des clés inconnues | 3 h | moyen-faible |
| 5.3 | Découper `main()` (463 lignes) et `handlers.go` (2 299) — ***fait le 10/10*** par la fiche E (`bc311b7`) : `main.go` 112 lignes, `handlers.go` 166 plus six fichiers par domaine ; la fusion des branches décollage/atterrissage d'`adsb/service.go` (étape 4) n'est pas faite, faute de test | 6 h | moyen, gain nul à l'exécution |
| 5.4 | Tests sur ce qui porte la production sans filet : websocket, api, storage, audio | continu | — |

**Plus tard, à discuter** : strips de vol (B2), clairance contre trajectoire (B3), vue d'approche
en plan (B4), procédures du SIA (B5), les fréquences comme un pupitre (C5, ça bascule l'antenne),
programmation d'écoute par plage horaire (E2).

**Plus tard, tranché (12/10)** : le **portage du sidecar vers Linux x86-64 + NVIDIA**
(faster-whisper / CTranslate2, repli processeur dans la même pile ; le Pi reçoit, la
transcription se déporte), cible fixée par le propriétaire sur l'étude de la session communauté
(Q54), **jalon à venir, pas dans l'immédiat** : après les trois études en cours et ce qu'elles
imposent à l'outil ; le format devra permettre l'affinage, et la non-régression se mesurera au
labo sur le modèle converti.

### Ce que le propriétaire doit trancher

| # | Arbitrage | État |
|---|---|---|
| 1 | Retirer le chat ATC (« AI Advisory ») et la simulation | **tranché le 10/10 : oui** — c'est l'assistant vocal OpenAI, pas les transcriptions par fréquence |
| 2 | Sources ADS-B hors tar1090, et SRT | **tranché le 10/10 (D71)** : sources ADS-B gardées ; lecteur SRT retiré, ffmpeg lit SRT ; options d'entrée ajoutées (UDP brut, carte son) |
| 3 | Langue de l'interface | **tranché le 10/10 : anglais** ; l'interprétation d'une communication suit sa langue ; version en/fr plus tard |
| 4 | Fond de carte par défaut | **en cours** : préférence pour le sombre, et des fonds aéronautiques avec pistes et plans d'aéroport ; étude des sources et des licences en cours (VAC du SIA comprises) |
| 5 | Ordre des paliers | **tranché le 10/10 : 0, 1, 2 puis 3** ; 4 et 5 ensuite, 5.4 en continu. Avec une exception : pas de relance automatique (1.6), un lanceur dans la barre de menus à la place |

Le détail de chaque arbitrage, tel qu'il a été instruit, est dans les sections précédentes et
dans D71 (`05-decisions.md`).

### Retour de la vague 1 (10/10)

Intégrée dans `local` le 10/10 après relecture et vérification sur le Mac (branche d'essai
`integration/vague1`). Ensemble : **+3 983 / −10 342 lignes** ; Go hors tests 35 077 → 27 498,
interface (JS + HTML) 13 036 → 10 881. Rapports : `docs-fr/fiches-cloud/{A,B,C}-rapport.md`.

- **Conflits de sens** entre C (écrit avant A) et A : trois tests de C appelaient des fonctions
  retirées par A ; adaptés. Un test de C utilisait `t.Chdir` (Go 1.24, le module déclare 1.23) :
  réécrit avec `os.Chdir`, il a révélé **un défaut propre au Mac** — la rétention ne reconnaissait
  la base du jour que par son nom ; à travers un lien symbolique (`/var` → `/private/var`), elle
  la prenait pour ancienne et la supprimait au-delà du plafond. Corrigé : reconnue aussi par
  identité de fichier (`os.SameFile`). En production les chemins sont relatifs : pas d'incident.
- **Vérifié sur le Mac** : ta configuration réelle se charge et se valide ; l'instance d'essai
  (faux sidecar `runs/son-test/fake-sidecar.py`, sans GPU) démarre sans aucune ligne chat,
  simulation ni prompt ; dans le navigateur à 1 440 × 900 : fond `ign-dark` choisi seul (et une
  préférence pointant un fond retiré y retombe), plan des aéroports (891 Ko, 35 s d'Overpass ;
  88 pistes, 1 818 taxiways, 289 aires, 71 terminaux, 117 hélistations) lisible à z13 et z15,
  lien VAC de la PAR au cycle AIRAC du 01/10 (PDF du SIA en 200), son, coupure, réglages,
  déconnexion.
- **Tuiles IGN** : le serveur refuse environ une tuile sur vingt (400 « layer unknown »), par
  périodes qui peuvent tenir une même tuile plusieurs secondes ; sous le filtre sombre, un trou
  noir. Réessai ajouté (7 essais espacés de 0,5 à 16 s, ~30 s). Pas une limite de requêtes
  simultanées (25 requêtes parallèles toutes servies).
- **Défauts de C** : n°3 (l'auditeur qui revient repartait jusqu'à 87 s derrière le direct) et
  n°4 (course sur le compteur de ports) **corrigés** ; vérifiés par leurs tests (échec sur
  l'ancien code) et, pour le n°3, sur l'instance d'essai (retour après 5 s et 40 s : débit du
  direct). **N°1 et n°2 à arbitrer** (changent ce qui est stocké) :
  - n°1 : une colonne vide (`tas`, position…) empêche la contrainte d'unicité de reconnaître une
    position répétée ; **mesuré sur la base du 04/10 : 923 816 doublons exacts sur 2 328 654
    positions (40 %)**. Corriger réduirait les bases d'environ 40 %, mais change ce que lisent la
    PAR, le juge des pistes, `cmd/phraseology` et le laboratoire (un état répété n'est plus
    réécrit chaque seconde) ;
  - n°2 : les transcriptions sont datées en heure locale avec décalage, les positions en UTC ;
    les comparaisons de texte se trompent d'une à deux heures et la nuit du changement d'heure
    (25/10) trie à l'envers. Corriger change le format lu par les scripts du laboratoire :
    à coordonner avec Whisper-lab.
- **Les deux défauts de stockage, corrigés le 10/10** (`7db6f42`, `91a3553`), chacun avec son
  test qui échoue avant et passe après : n°1, une position déjà stockée n'est plus réécrite
  même quand une colonne est vide (recherche par `IS`, servie par l'index de la contrainte :
  plan vérifié) — effet attendu : environ −40 % de lignes de positions, à mesurer sur la
  prochaine journée ; n°2, les transcriptions, clairances et valeurs de phraséologie sont
  datées en UTC comme les positions, les anciennes lignes converties une fois à l'ouverture
  du fichier (vérifié sur une copie de la base du 10/10 : 25 lignes `+02:00` → `Z`, ordre
  inchangé), l'en-tête `X-Created-At` du sidecar et les bornes de `cmd/phraseology` suivent.
  **Whisper-lab n'est pas encore prévenu** (session fermée) : `created_at` se termine désormais
  par `Z`, et `phraseology_values.created_at` passe du format Go par défaut à RFC3339.
- **Fond de carte** : « None » ajouté (plan des aéroports et avions sur noir) et **choisi par
  défaut** le 10/10, jugement du propriétaire ; IGN et OSM, clairs ou sombres, restent à un clic.
- **À juger à l'œil par le propriétaire** : le filtre d'assombrissement
  (`style.css`, `.basemap-dark .atc-basemap`) et les couleurs du plan
  (`AIRPORT_LAYOUT_COLORS`, en tête de `openlayers-map-manager.js`).
- Binaire de production reconstruit ; l'ancien est gardé (`runs/co-atc.avant-vague1`).

### Retour de la vague 2, fiches D et F (10/10)

Intégrées dans `local` le 10/10 sans conflit ; `go vet` propre, 15 paquets verts avec `-race`.
- **D, configuration réduite** : l'exemple passe de 538 à 156 lignes et de 150 à 57 clés ;
  défauts en code pour le reste ; une clé inconnue est nommée au démarrage, jamais refusée.
  **Vérifié sur la configuration réelle du propriétaire** : chargée par le code d'avant et par
  celui d'après, elle ne diffère que par trois réglages d'authentification qui passent de 0 à
  720 h, 8 essais et 15 min, exactement les valeurs que l'ancien code appliquait quand ils
  valaient 0 (`orDefault`, `max <= 0`) : aucun changement de comportement. Elle produisait 23
  avertissements (chat, clés OpenAI, deux doublons) ; **nettoyée le 10/10** (58 lignes retirées,
  copie `runs/config.toml.avant-vague2`) : même configuration chargée, aucun avertissement.
- **F, tests de l'API** : couverture 10,9 % → 69,9 % ; 29 routes testées en 401 sans session.
  **Défaut** : `/aircraft` reçoit `min_altitude` et `max_altitude` et ne s'en sert pas ;
  aucun client du dépôt ne les envoie. *Tranché le 10/10 : corrigé plutôt que retiré, l'API les
  documente (`docs/api_spec.md`) ; fait (`56c3404`).* Les filtres de dates (`took_off_after`,
  `took_off_before`, `landed_after`, `landed_before`), même fonction, TODO de l'amont : **faits le
  11/10** (`45001d9`) ; un avion compte par n'importe lequel de ses décollages ou atterrissages dans
  la fenêtre (tours de piste), bornes dans tout fuseau.
- **Observations de F traitées le 11/10** : une référence de proximité introuvable
  (`distance_nm` + `ref_hex`/`ref_flight` inconnus) répond 404 au lieu de tout le ciel, et 400 sans
  référence (`2344a7b`). Laissées : `Access-Control-Allow-Methods` sans `PUT`/`HEAD` (sans effet tant
  qu'aucun client d'une autre origine n'appelle l'API), `transcriptions: null` (sans effet), hex
  français sans indicatif (amont). Mesuré sur le Mac : `internal/api` 71,7 % de couverture, 11 s.
- Instance d'essai démarrée sur cet état : santé et page en 200. Binaire de production
  reconstruit, ancien gardé (`runs/co-atc.avant-vague2`).
- **La fiche E** (découpage de `main()` et de `handlers.go`) peut partir : D est intégrée. Elle
  doit garder la boucle des avertissements de configuration dans `main.go`.

### Retour de la fiche E (10/10)

Intégrée le 10/10 (`bc311b7`), sans conflit avec le palier 2 fait entre-temps. `main()` tient
en une page (`main.go` 695 → 112 lignes, le reste dans `startup.go`, `serve.go`,
`retention.go`, `add_user.go`) ; `handlers.go` 1 636 → 166 lignes, six fichiers par domaine,
`GetAllAircraft` en dix fonctions privées. **Vérifié sur le Mac** :
- la liste des routes figée (`internal/api/testdata/routes.txt`, 45 lignes) passe ;
- le banc de démarrage (`tools/startup-bench/`, désormais dans le dépôt) rejoué sur `local`
  puis sur la fusion, deux passes : mêmes lignes de `main()` (34), même ordre de construction
  (28 étapes), mêmes messages (87) ;
- un démarrage sur la configuration de l'instance d'essai, avant puis après : mêmes 50 lignes de
  `main()`, même ordre, mêmes 88 messages ; le seul écart vu (un message de décollage écarté)
  dépend du trafic et apparaît aussi entre deux démarrages du même binaire ;
- `go test -race ./...` vert ; la boucle des avertissements de configuration est restée dans
  `main()`, avant toute construction.

**Laissé** : l'étape 4, la duplication décollage/atterrissage de
`sendImmediateGroundTransitionAlerts` (≈ 30 lignes), qu'aucun test ne couvre. À reprendre avec
un test d'abord, ou à laisser : son seul effet visible est l'effet de décollage/atterrissage
sur la carte.

## État

- 27/10, 14:10 : **« someone is speaking » fréquences coupées confirmé par le propriétaire** sur sa session, en production (page 98f675a, en ligne depuis la relance 1) : le constat du matin est clos.
- 27/10, 14:01 : **relance 3, ffmpeg retiré, production à 16 kHz** (B1 ter, plan 35). `b1-ter` fusionnée (749d489 ; seul conflit : l'empreinte de la feuille Tailwind, reconstruite), build, vet, `go test ./...` et le filet de la page verts. Configuration : `ffmpeg_sample_rate = 24000` devient `sample_rate = 16000`, les six clés de ffmpeg mises en commentaire (sauvegarde `runs/config.toml.avant-relance-27-10`) ; le nouveau code la charge sans avertissement. Essayé d'abord sur 8020 : décodage à 16 kHz, une transmission de 30,0 s envoyée au faux sidecar avec la bonne fréquence, sessions relues. Station prévenue ; production arrêtée à 14:01:02, reprise à 14:01:05 (ancien binaire dans `runs/co-atc.avant-relance-3`). Vérifié : « Sessions read back: 3 kept » (la connexion du propriétaire a survécu, première relance avec la fiche S), décodage Go à 16 kHz des deux flux à 14:01:15, transcriptions dès 14:01:20, contrat 1, `missing` vide, sidecar ok, ADS-B frais, aucun ffmpeg.
- 27/10, 13:53 : **relance 2, le nouveau co-atc en production** (84db233 : contrat v1, fiches P, Q, R, empreinte mémoire, fiche S, 991152c, ebfd2ce). Démarré d'abord à part sur 8020 (`runs/son-test-s`) : connexion, session gardée après redémarrage (« Sessions read back: 11 kept »), contrat 1, arrêt sans panique. Configuration de production chargée sans avertissement par le nouveau code. Station prévenue ; production arrêtée à 13:52:49, binaire remplacé (`rm` puis `mv`, l'ancien gardé dans `runs/co-atc.avant-relance-2`), reprise à 13:52:52 par le lanceur. Vérifié : `/api/v1/health/detail` contrat 1, `missing` vide, sidecar géré et joignable, pistes en service reprises, décodage Go des deux flux à 13:53:04, liaison avec la station resynchronisée, ADS-B frais, aucun ffmpeg, `/api/v1/vitals` répond, `configs/sessions.json` créé en 0600. Les sessions de l'ancien binaire sont perdues une dernière fois : le propriétaire se reconnecte.
- 27/10, 13:50 : **relance 1, A3 en production** (plan 35). Station prévenue ; production arrêtée à 13:49:43, ancien lanceur fermé par PID, branche `lanceur` fusionnée (b14d485), lanceur installé, production reprise à 13:50:22 par le nouveau lanceur, même co-atc (457718d). Vérifié : le porteur est la copie `carriers/20261027-134954-b14d485`, « Depends on: Clairance », aucune ligne « not allowed », ADS-B frais, pistes en service reprises, décodage Go des deux flux à 13:50:25, transcriptions dès 13:50:3x ; une pression mémoire passagère au chargement du sidecar, retombée sans alerte. La page copiée au démarrage porte 98f675a : « someone is speaking » fréquences coupées est en ligne. Dernier relevé de N5 à l'arrêt : 3 593 192 640 octets en 20 h 47 min 37 s, soit 48 000,6 o/s (100,0 %).
- 27/10, 13:17 : **N5 passé, goxiph validé** (B1 bis, plan 35). 24 h de production décodée en Go (26/10 13:09 → 27/10 13:17) : aucune erreur du décodeur hors nos relances, 19 h 25 d'affilée depuis 17:02 ; reprise seule après la coupure réseau de 13:24 (20 s) ; débit 100,0 à 100,3 % ; transmissions par heure au niveau de ffmpeg (468 contre 470 à la même heure la veille) ; Icecast, relevé de la station : aucun « fallen too far behind », connexions stables. Les deux connexions brèves sur aero-124350 à 18:17 et 18:21 que la station ne s'expliquait pas étaient nos instances d'essai (`runs/son-test`, essais 6 et 7), non annoncées. Suite : les trois relances, dont `b1-ter` et 16 kHz à la troisième.
- 27/10, 12:00 : **« someone is speaking » s'affiche fréquences coupées** (98f675a), constat du propriétaire : rien
  au chargement avec tout coupé, alors que le texte défilait ; affiché après avoir écouté puis coupé. Cause, mesurée
  dans Chrome sans fenêtre (instance d'essai, trois MP3 en boucle) : la page recevait bien `frequency_activity` du
  serveur (93 messages en quelques secondes) mais le client WebSocket ne transmettait que les types d'une liste à lui,
  où celui-ci manquait depuis le 26/10 (Q63). Écouter puis couper laissait le lecteur jouer à volume nul : la mesure
  locale de la page prenait le relais, d'où l'illusion. Le client transmet désormais tout type qu'un écouteur peut
  demander (`clearance_issued`, autre oublié, ne fait qu'une ligne de console). Vérifié : sans clic, tout coupé, les
  trois lignes « now · … » affichées ; un test fait passer chaque type du serveur (échoue sans le correctif sur
  ces deux types). En production à la relance 2.
- 27/10, 11:20 : **les flux des ports annexes reçoivent le cookie** (ebfd2ce, demandé par le propriétaire après le
  constat de la fiche S). Connexion exigée et sans mandataire, les flux répartis sur les ports annexes sont une autre
  origine : le lecteur de la page (Q63, 26/10) les demandait avec `credentials: 'same-origin'`, sans cookie, d'où des
  401. Désormais `include` ; la réponse CORS autorisait déjà la page avec cookie. Second défaut du même genre : une
  autre origine ne lit pas `X-Sample-Rate` sans `Access-Control-Expose-Headers`, et le lecteur retombait sur 8 kHz,
  faux une fois les flux à 16 kHz (relance 3) ; l'en-tête est maintenant exposé, et le test du flux le vérifie.
  **Vérifié dans Chrome sans fenêtre** contre le vrai binaire (instance d'essai, trois sources MP3 en boucle, décodeur
  Go, `trusted_proxies` déclaré donc connexion exigée) : `same-origin` → 401 sur 8023 et 8024, `include` → 200 ;
  la vraie page, « Listen to all » : trois flux à 200 ; `X-Sample-Rate` lu sur les ports annexes avec le correctif,
  `null` sans. Derrière Traefik rien ne change (un seul port). En production à la relance 2 (la page est recopiée au
  démarrage).
- 27/10, 09:55 : **fiche S relue et fusionnée** (d84435e ; en production à la relance 2 du jour). Les sessions
  survivent à une relance : `configs/sessions.json` à côté des comptes, l'empreinte SHA-256 du jeton seulement, 0600,
  ignoré par git ; connexion, déconnexion et révocation écrites aussitôt, l'expiration glissante au plus une fois par
  minute et à l'arrêt. Le WebSocket de la page réessaie toutes les 5 s au plus au lieu de 30 (mesuré au cloud : après
  une coupure de 30 s, la carte revenait 28,6 s après le serveur, 0,4 s désormais). Tests Go, `-race` sur auth et api,
  filet de la page : verts. **Vérifié sur le Mac**, vrai binaire, copie de l'instance d'essai avec
  `trusted_proxies` (une requête venue « d'Internet » sans cookie : 401) : connexion, fichier en 0600 avec une
  empreinte de 64 caractères et aucun jeton ; arrêt, relance, même cookie : 200 (« Sessions read back: 1 kept ») ;
  déconnexion : fichier vidé ; relance : 401. Pas rejoué sur le Mac : la page dans Chromium pendant la relance
  (mesurée au cloud). **Constat de la fiche corrigé** (991152c) : une panique à l'arrêt, une fois sur 17 —
  l'index vocal tournait sur un contexte jamais annulé et interrogeait la base déjà fermée ; il s'arrête avec le
  serveur, et la base fermée répond une erreur (test qui reproduisait la panique). Autre constat, laissé : sans
  mandataire, les flux des ports annexes n'ont pas le cookie (autre origine) ; sans effet derrière Traefik.
- 26/10, 18:20 : **fiches P, Q et R relues et fusionnées** (202a566, 4d3a79a, e810731 ; en production à la relance
  du 27/10). **P**, le compte invité : treize champs portant une adresse ou un chemin vidés pour un viewer (figés
  dans `testdata/guest.txt`), deux flux par fréquence au plus quel que soit le format (la limite de dix ne valait
  que pour le WAV), `429` avec `Retry-After` pour une connexion bloquée. **Q**, le serveur autonome : la page
  incorporée au binaire (`static_dir` la remplace s'il est donné ; la production garde sa copie pour l'instant), les
  fichiers propres à la station servis depuis `<sqlite_base_path>/station/`, un bilan au démarrage (`missing` dans
  `/health/detail`). **R**, le contrat du lanceur côté serveur : `docs-fr/36-contrat-lanceur.md` ; co-atc lance la
  liaison avec la station (clé interne au lieu du mot de passe) et l'empêchement de veille, **désactivés par
  défaut** ; canal d'état `-status-fd` ; `contract`, `process`, `companions` dans `/health/detail` ; schéma figé.
  Fusions : deux conflits Q/P (champs voisins), cinq R/Q (les deux côtés gardés ; `runServer` séparé en arguments et
  environnement) ; schéma figé réécrit, le diff n'ajoute que `page` et `missing` (12 lignes) ; tous les tests Go et
  le filet de la page verts. Démarré à part sur l'instance d'essai : canal d'état (`starting`, étapes, `ready` avec
  ses trois ports), « Startup check: nothing missing », page et feuille servies, mémoire mesurée sur macOS (272 Mo
  contre 217 Mo de RSS par `ps`, l'empreinte comptant plus large), pression « normal ». Pas encore fait : la liaison
  lancée par co-atc et `keep_awake` sur le Mac (tant que `production.command` lance sa propre liaison, ne pas
  activer `[station_link]` : il y en aurait deux).
- 26/10, 17:02 : **production relancée avec les pistes gardées et les fiches O et N** (457718d), à la demande du
  propriétaire. Binaire démarré d'abord à part (page, santé, `TRACE` refusé, pistes reprises, flux μ-law). Arrêt à
  17:01:19 ; un premier départ refusé à 17:01:53 (« co-atc tourne déjà ») : ma commande contenait le motif que
  cherche le script de production (piège connu, mémoire du 16/10) ; reprise à 17:02:06, station prévenue. Première
  transcription à 17:02:27 ; 16 en deux minutes, 5 secondes lectures ouvertes par la détection de langue, 2 boucles
  coupées, archivage en marche ; MLX 8,8 Go actifs, cache 0,7 Go sous sa borne. Le fichier des pistes s'écrit
  (`data/runways-in-use.json`) ; rien à reprendre à ce démarrage-ci, le premier avec la fonction. Débit décodé :
  2,82 à 2,86 Mo par minute par fréquence, soit le PCM à 24 kHz en continu (la station avait demandé ; ce n'est pas
  le débit Icecast).
- 26/10, 17:20 : **le son sur l'iPhone** (cbf7ecf, page seule). Le propriétaire : pas de son dans Safari sur iPhone,
  mode silencieux désactivé, pas de bandeau. La page déclare désormais son son comme lecture
  (`navigator.audioSession.type = 'playback'`) : sur iPhone, Web Audio est sinon un son d'ambiance que le mode
  silencieux coupe. Après réouverture de l'onglet, le son passe. Comme sur Mac une heure plus tôt, le premier essai
  après le dépôt était muet, le suivant non : cause non mesurée. À voir : l'écran verrouillé.
- 26/10, 16:45 : **Safari joue par le lecteur de la page, sans cliquetis** (623d132, c50787c ; page seule, en
  production sans relance). Vu dans le Safari du propriétaire : le bandeau « Click anywhere to resume listening »
  restait après un clic. Safari ne laisse pas reprendre le son sur `pointerdown`, seulement sur click, touchend ou
  keydown, et la page n'essayait qu'au premier geste : elle réessaie maintenant à chaque geste. Le son est alors
  revenu, avec un cliquetis : Safari rééchantillonnait chaque morceau de 0,1 s à 8 kHz séparément. Mesuré hors
  ligne dans WebKit sur un son pur : un écart de 0,02 (pour 0,5) à chaque jointure, rien ailleurs. La page
  rééchantillonne maintenant elle-même, par un sinus cardinal fenêtré dont l'historique passe d'un morceau à
  l'autre : 3·10⁻⁸ entre morceaux et son entier, le son pur exact à 10⁻⁴ près, 6 à 9 ms de calcul par seconde de
  son ; un test (`www/live-player.test.js`). Safari reprend le lecteur de la page par défaut ; `?player=element`
  force encore l'élément `<audio>`. Une fois, après le rechargement, ni son ni activité, puis le son à la
  réouverture de la console : cause non trouvée (les scripts sont toujours revalidés).
- 26/10, 16:00 : **fiches O et N relues et fusionnées** (en production à la relance du 27/10). **O** (6fa8dd5) :
  l'ouverture derrière le mandataire, dix défauts corrigés avec un test rouge avant chacun, qui peut quoi figé route
  par route, la trace des connexions (`configs/signins.jsonl`, `GET /api/v1/signins`) ; tous les tests Go verts après
  fusion. **N** (e375fa0) : le sidecar à deux moteurs, MLX et faster-whisper. Un conflit avec la borne du cache MLX du
  matin, résolu (borne appliquée sous MLX seulement). Vérifié sur le Mac : la commande de production démarre sur le
  port 8179 (moteur mlx, Apple M4 Pro, cache borné à 1 024 Mo) ; les sept tests de contrat passent sur le vrai MLX
  avec le modèle turbo, client Go compris (la voix synthétique passe par `say` de macOS, l'ffmpeg de Homebrew n'a
  pas flite).
- 26/10, 15:45 : **les pistes en service survivent à une relance** (342b637, en production à la prochaine relance
  prévue, le 27/10 après N5). Le propriétaire : « l'overlay procédure n'est plus complet ». Cause : les pistes en
  service s'apprennent des mouvements, sur une heure, et repartent de zéro à chaque démarrage (26L seule à LFPG
  depuis la relance de 15:18, 26L et 27R à 12:29). La base ne garde pas la piste d'un mouvement ; chaque suivi écrit
  donc ses preuves dans `runways-in-use.json`, à côté des bases, toutes les 30 s et à l'arrêt, et les reprend au
  démarrage tant qu'elles sont dans la fenêtre. Vérifié sur l'instance d'essai : arrêt à 15:44:36, relance,
  « Runway evidence restored » (26L 67 %, 27R 33 %), la page montre LFPG 26L et 27R vingt secondes après le démarrage.
- 26/10, 15:35 : **la couche des procédures : pistes en service ou toutes** (572e1e2, page seule, déposée en production
  sans relance). Demandé par le propriétaire : un second choix sous Arrivées/Départs, « Runways in use » ou « All »,
  gardé dans le navigateur, avec ses bulles d'aide. Mesuré sur l'instance d'essai : 13 routes d'arrivée (LFPO 25)
  en service, 103 avec « All ». Le propriétaire, 16:55 : « le toggle ne change pas les procédures affichées ». Mesuré :
  les arrivées des autres pistes suivent les mêmes trajets sous d'autres noms (52 routes et 81 segments distincts en
  service, 103 routes et 84 segments avec « All ») ; les départs changent (136 → 257 segments). La ligne d'état dit
  désormais le nombre de routes tracées, et la bulle d'aide le pourquoi.
- 26/10, 15:25 : **Safari reprend l'élément `<audio>` et le MP3** (d80e2c4). Le propriétaire : plus de son dans Safari
  sur Mac avec le lecteur de la page. Le même lecteur joue dans WebKit 18.2 (Playwright) : l'analyseur monte à 0,29
  pendant la parole. Cause non trouvée. `?player=live` ou `?player=element` force l'un ou l'autre pour essayer un
  navigateur (ea2b2f6). Avant (98a3ced) : un son suspendu par le navigateur le dit (« Click anywhere to resume
  listening ») et reprend au premier geste ; l'icône de l'onglet suit ce qui joue.
- 26/10, 15:15 : **la page joue le son elle-même** (Q63, décidée par le propriétaire : C2). Mesuré avant : Chromium
  attendait 17,4 s pour jouer le MP3 de la station, Safari le jouait sans Web Audio, les temps du navigateur sont faux
  pour un MP3 à débit variable. Le serveur relaie le son décodé en μ-law (`?format=ulaw`) ; `www/live-player.js` le
  joue par Web Audio avec 0,3 s de réserve, à la place de l'élément `<audio>`. Essai sur la station : son dans la
  première seconde, analyseur en phase avec la voix. En production à 15:15:43. En route : un défaut du relais (une
  avance plus grande que la file d'un auditeur bloquait le décodage), corrigé (16e5ed9).
- 26/10, 14:24 : **« someone is speaking » mesuré par le serveur** (32d05e8). Dans Safari, le son passait mais le bouton
  son ne coupait rien et l'indicateur restait muet : Safari joue un flux en direct sans le faire passer par la chaîne
  Web Audio de la page. Le serveur mesure maintenant l'activité sur l'audio qu'il décode (ouverte sur 20 ms au-dessus
  du seuil de la transcription, close après 600 ms de silence), et envoie le niveau quatre fois par seconde par le
  WebSocket ; la page en tire l'indicateur et la barre, dans tous les navigateurs, y compris pour une fréquence qu'on
  n'écoute pas. Le bouton coupe aussi l'élément `<audio>`. Vérifié sur la station par une instance d'essai : 61 niveaux
  et 2 fins en 45 s. Le son sur le téléphone du propriétaire reste à comprendre.
- 26/10, 14:12 : **le MP3 de la station pour les navigateurs** (D2 bis). Safari, sur Mac comme sur iPhone, ne lit pas le
  WAV sans fin de la page (le propriétaire : son dans Dia, rien dans Safari, par la même adresse publique). Le décodeur
  Go remet chaque trame entière à un relais (`internal/audio/mp3relay.go`) : deux secondes d'avance au nouvel auditeur,
  puis les trames au fil de l'eau ; un auditeur lent perd des trames sans retenir les autres ; un changement de format
  ou l'arrêt du flux libère les auditeurs. `?format=mp3` sert de l'`audio/mpeg`, le WAV sinon ; la page le demande
  partout. Environ 15 kb/s par fréquence au lieu de 384. Les tests ont trouvé qu'un auditeur libéré pouvait perdre ses
  dernières trames (corrigé) ; l'essai sur la station, qu'une page ouverte juste après un démarrage recevait du WAV
  (attente portée à 8 s). En production à 14:12:37.
- 26/10, après-midi : **la page derrière HTTPS**. Le WebSocket était ouvert en `ws://` en dur : sur une page HTTPS le
  navigateur le refuse sans bruit, plus d'avions ni de fil (corrigé, 5976a05 ; la copie de la page de production
  rafraîchie sans relance). La barrière Traefik, incompatible avec le service worker de la page, retirée sur décision
  du propriétaire ; le nom passé en « DNS only » par le propriétaire. **Mémoire du sidecar bornée** (cache de MLX à
  1 Go, d5860d5) : de 16 à 9,0 Go, vitesse de transcription inchangée ; le premier correctif plantait au démarrage du
  sidecar (caa8a69), production coupée de 13:47:51 à 13:50:58. Leçon : démarrer à part un composant modifié avant de
  relancer la production. Le son reste muet sur le téléphone : la page lui envoie du WAV sans fin, que les navigateurs
  mobiles refusent souvent ; piste : relayer le MP3 d'origine.
- 26/10, 13:19 : **D1, l'accès distant, en place et fermé** (Q62, plan 35). Route ajoutée au Traefik du propriétaire
  (copie du fichier gardée sur sa machine), certificat obtenu, limite des essais sur `/api/v1/auth/login`, en-têtes de
  sécurité, et une barrière temporaire du mandataire jusqu'à l'ouverture. Côté Clairance, `trusted_proxies` et
  `allowed_origins` dans la configuration (sauvegarde `runs/config.toml.avant-d1`), production relancée à 13:19:09, le
  décodage Go repris à 13:19:13. De l'extérieur, tout reçoit 401 de la barrière. Le domaine passe par le proxy de
  Cloudflare : l'adresse du client est celle de Cloudflare, à traiter avec la fiche O, lancée dans le cloud.
- 26/10, 13:09:51 : **N5 lancé, la production décode ses flux en Go** (goxiph, sans ffmpeg) pour 24 h, avec l'accord du
  propriétaire : binaire reconstruit (e8f500b), `decoder = "go"` dans `configs/config.toml` (sauvegarde
  `runs/config.toml.avant-n5`), relance par le lanceur (coupure de 13:09:42 à 13:09:51), station prévenue. Vérifié :
  « Decoding the stream in Go » sur les deux fréquences, aucun processus ffmpeg, ADS-B à 108 avions, première
  transcription à 13:10:05, aucun « no route to host ». La station rappelle que les flux arrivent en morceaux par
  Caddy et qu'Icecast retire un lecteur en retard de 512 Ko sans fermer la connexion : couvert par la coupure après
  15 s sans données.
- 26/10 (12:30 – 13:06) : **N4, l'audio réel transcrit par paires, passé** (B1 bis, plan 35). Une demi-heure des deux
  fréquences (11:59 – 12:29) décodée par ffmpeg et par goxiph, découpée comme la production (237 transmissions sur
  237 aux mêmes bornes), transcrite par un sidecar d'essai sans archive, production arrêtée 36 minutes (station
  prévenue, créneau au planning de la machine). 17 textes différents sur 237 ; contrôles : le même audio deux fois, 0 ;
  goxiph avec la conversion de fréquence de ffmpeg, 20 ; l'audio de ffmpeg avec un bruit inaudible d'un pas
  d'arrondi, 22. Le critère « 97 % identiques » était mal posé : Whisper bascule une transmission difficile sur
  douze à la moindre perturbation, et goxiph reste dans ce bruit. Production relancée à 13:06 (ffmpeg), vérifiée.
- 26/10 (midi) : **goxiph intégré et testé** (B1 bis, plan 35). Copie figée dans `third_party/goxiph` avec un lecteur
  pour les flux en direct (a62e9c2) ; voie de décodage en Go dans Clairance derrière `frequencies.decoder = "go"`
  (839674c). Niveaux 1 à 3 passés : 105 dB de ffmpeg en mono, robustesse (flux pris en cours, octets abîmés, coupure,
  changement de format, une heure, 146 665 entrées aléatoires), chaîne complète sans ffmpeg. Limite connue : le
  stéréo (voie gauche à 52 dB, quand ffmpeg, LAME et Apple s'accordent à 79-85 dB). Niveau 4 en cours : une heure des
  deux fréquences enregistrée ; l'outil de comparaison par paires, essayé sur 5 min, retrouve les 34 transmissions
  aux mêmes bornes.
- 26/10 : **B1, le décodage audio en Go, mesuré** (Q60, plan 35). Les flux de la station sont du MP3 mono à 8 kHz
  (MPEG-2.5) ; `go-mp3` ne le lit pas ; goxiph (Go pur, BSD-3) le décode à l'identique de ffmpeg à l'arrondi près
  (≈ 100 dB par fenêtre de 5 s), la chaîne jusqu'à 24 kHz à 72 à 80 dB, pour 5,6 s de calcul par heure d'audio.
  Trouvé en route : `-fflags nobuffer`, dans les arguments de ffmpeg de la production, perd 3,6 s d'audio à chaque
  démarrage de ffmpeg. Choix laissés au propriétaire (goxiph ou ffmpeg embarqué, `nobuffer`, 24 ou 16 kHz).
- 26/10, 10 h 49 : **la production relancée par le porteur redevient sourde** (« no route to host » à partir de
  10:49:39, 349 fois à 10:55 ; dernière transmission transcrite à 10:50:09). Cause trouvée par l'agent du lanceur :
  il a réinstallé `~/Applications/Clairance.app` à 10:49 puis 10:53, avec un nouveau binaire, donc une nouvelle
  identité, pendant que le porteur tournait depuis ce bundle ; macOS a retiré le réseau local au porteur et à
  co-atc. Sa mesure du 24/10 (« remplacer l'application ne coupe pas le porteur ») était fausse : la compilation
  était identique, et l'identité n'avait donc pas changé. Corrections : le doc 34 et une garde dans `build.sh`
  (jamais réinstaller pendant qu'un porteur tourne). Hypothèse à mesurer : avec une signature Developer ID au
  lieu d'une signature ad hoc, l'identité resterait stable d'une mise à jour à l'autre (Q61, Q62).
- 26/10 : **première production par le porteur du lanceur, sans audio** : « off air » sur les deux fréquences (vu par
  le propriétaire). Cause mesurée : `"ffmpeg": executable file not found in $PATH`, six fois dans le journal ; le
  porteur, ouvert par `open -n`, donne à co-atc le PATH minimal de launchd (`/usr/bin:/bin:/usr/sbin:/sbin`), sans
  `/opt/homebrew/bin`. Le gardien n'a rien dit : le binaire en production datait du 16/10 et n'avait pas
  `/api/v1/health/detail`. **Jugement du propriétaire : la production ne doit pas dépendre de ce qui la lance.**
  Corrigé dans le serveur (134e4fb, commit autonome) : `pkg/tool` trouve ffmpeg, pdftotext et l'interpréteur du
  sidecar sur le PATH puis dans les dossiers de Homebrew et de MacPorts, et le démarrage dit où est ffmpeg ou qu'il
  manque. Mesuré : le test de la tonalité sur un vrai ffmpeg, lancé avec le PATH du porteur, reçoit ses 102 400
  octets en 3 s. `bin/co-atc` reconstruit (l'ancien dans `runs/co-atc.avant-2026-10-26`). Signalés à l'agent du
  lanceur : un 404 sur la route de santé doit alerter, et le commit écrit au démarrage est celui du dépôt, pas
  celui du binaire.
- 24 → 25/10 : **le lanceur, étapes 1a, porteur et 1b** (agent « Clairance - macOS launcher », fusions 14c39e6,
  e71e4bd, 938f8c3 ; détail et mesures dans le doc 34). Lancement délégué au lanceur, processus suivis par PID ;
  production dans un porteur à part du menu (Q59 b ; l'agent launchd écarté, faute de réseau local) ; gardien et
  notifications, mesurés sur un banc d'essai sans la station : coupure ADS-B vue en 49 à 54 s (5 essais), coupure
  audio en 64 s (3 essais, par `bytes_last_60s`), co-atc tué en 4 s, arrêt demandé sans événement. Défauts trouvés
  en route : un démarrage manqué accusait la station quand la vérification ADS-B échouait pour une autre raison
  (`receiver.json` absent) ; un script de démarrage manqué restait vivant et aurait lancé une seconde liaison avec
  la station. Les notifications de Clairance étaient coupées dans les Réglages de macOS, rétablies par le
  propriétaire. Non mesuré : une journée normale de production sans fausse alerte. Le 25/10, l'icône d'application
  définitive de la session design (document Icon Composer), les réglages groupés et « – Audio · nothing on air »
  quand la station n'a rien à l'antenne (3b6ed5f). Puis (1ef156c) : station à l'antenne mais aucune fréquence chez
  co-atc, la ligne audio attend la liaison 60 s puis passe en défaut avec notification (vu sur le banc à 65 s) ;
  `--status` affiche les lignes de santé du lanceur.
- 24/10 : **le lanceur de la barre des menus devient le poste de surveillance de Clairance** (proposition
  `34-lanceur.md`, 1eb83a3, demandée par le propriétaire ; agent dédié « Plan lanceur de Clairance », branche
  `lanceur`, coordonné avec la session design). Côté serveur, la seule modification demandée : **`GET
  /api/v1/health/detail`** (3307610, commit autonome), format arrêté avec l'agent. Réservée à la machine elle-même
  sans session (connexion depuis la boucle locale, en-tête Host de boucle locale avec ou sans port, aucun en-tête de
  mandataire) ou au propriétaire connecté ; 403 sinon. Une réponse sert 5 s, sans requête en base. Ce qu'elle ajoute
  à `/status` : **les octets sortis de ffmpeg par fréquence sur la dernière minute** (« connected » ne disait que
  « ffmpeg a démarré » : c'est ce qui aurait vu la production sourde du 16 au 18/10), l'heure du dernier segment
  envoyé au sidecar et celle du dernier texte rendu (fréquence silencieuse contre transcription sourde), la file du
  sidecar par voie, la taille de l'archive relevée par la purge horaire face à son plafond, la place libre, le PID
  de co-atc. Mesuré : ffmpeg réel sur une tonalité à 16 kHz, 102 400 octets en 3 s environ (32 ko/s, attendu). Pas
  encore vu sur la production : elle est arrêtée depuis le 18/10.
- 20/10 : **revue de la documentation** (revue du 20/10). Les documents de pilotage et d'index ont été remis
  à jour sur l'état réel : sessions, jalons faits (nom, migration du design, Q55 à Q57, fiches cloud G à M toutes
  intégrées), décision sur le crédit cloud restant (250 $ jusqu'au 5/11), correction du diagnostic de la troisième
  famille de sauts (voir l'entrée du 16/10, après-midi), leçon de la production sourde (entrée ci-dessous). Les
  mesures de la nuit du 16 au 17/10 (`runs/mesures/nuit-2026-10-16/`) n'ont pas encore été analysées ; un coup d'œil
  de la revue montre que la chaîne était sourde dès 23 h 05 (total de transmissions du relevé figé à 8 109 de
  23 h 08 à minuit ; une seule tranche de l'instance d'essai porte des données) : à refaire sur une nuit où la
  chaîne entend.
- 16 → 18/10 : **la production est restée sourde du 16/10 à 23 h 04 au 18/10 à 13 h 36** (revue du 20/10). Cause :
  l'application Claude de bureau s'est mise à jour et relancée toute seule, au repos (« stealth update », 23:04:53
  dans `~/Library/Logs/Claude/main.log`). Tout processus démarré depuis la session Claude — la production, lancée
  par la ligne de commande du lanceur de la barre des menus, l'instance d'essai, les liens avec la station — a
  perdu l'accès au réseau local de macOS : « no route to host » vers la station chaque seconde, aucune
  transcription les 17 et 18/10, alors qu'Internet (la météo) passait. Aucun autre signe. L'instance d'essai a
  aussi tourné jusqu'au 18/10 à 13 h 35 : le script de nuit n'arrêtait que les sondes. **Leçon** : la production est
  lancée par le propriétaire (lanceur de la barre des menus) ou par le système (`open`), jamais comme enfant d'une
  session Claude (`CLAUDE.md`, règle 5, 20/10) ; même règle pour ce qui doit passer la nuit, et un script de nuit
  arrête tout ce qu'il a lancé.
- 16/10 (après-midi, 17 h) : **la piste et la sortie probables sur le cartouche** (Q57, décidée par le
  propriétaire). Mesures d'abord : la piste d'arrivée sur 2 338 arrivées, la sortie de départ sur 3 111 départs, ses
  probabilités calibrées. Forme de la session design (a0b8201), construite et **en production à 17 h 06** (9fcc281) :
  le champ en fin de 2ᵉ ligne de l'étiquette, l'histoire de l'avion, et, pour le départ sélectionné, ses sorties
  possibles dessinées sur la carte. Quatre corrections de la relecture design (un nom par point, silence de la couche
  sous la route d'une puce, la route d'une puce l'emporte) versées à 17 h 18 (4ba147b). Amorçage de la page sur la
  trace du serveur versé à 17 h 51 (2c5c6f2) : les premières sorties s'affichent 4 s après l'ouverture, contre
  environ 70 s. À surveiller : les sorties d'Orly (peu couvertes par la réception) ; après chaque relance, les
  départs déjà en l'air n'ont pas de sortie.
- 16/10 (après-midi) : **les avions fantômes, mesurés et corrigés** (le propriétaire : « des avions
  fantômes qui se déplacent à toute vitesse en diagonale »). La station et la base sont saines (aucun
  saut de plus de 15 NM dans la journée) ; une sonde dans Chrome sans fenêtre, sur l'instance d'essai
  et le trafic réel, note tout avion dessiné à plus de 1 000 kt avec l'état du moteur d'animation.
  Deux causes : (1) deux positions sans cap datées à quelques millisecondes d'écart donnaient au
  moteur une vitesse de l'écart sur 1 ms, jusqu'à 1,6 million de kt, lissée à 35 % par relevé, donc
  une vingtaine de secondes de vol en ligne droite à travers la carte (04dc5e3 : pas de vitesse tirée
  de deux positions à moins d'une seconde, aucune au-delà de 1 000 kt) ; (2) le serveur prédisait
  plein nord un avion sans cap connu, son 0 n'étant pas le nord (d0b54d5 : plus de prédiction sans
  cap). Mesuré sur 10 min de trafic réel : avant, 172 sauts visibles en moins de 1,5 s, 21 avions,
  jusqu'à 4 388 NM ; après le correctif de la page, 10 sauts, 8 avions, au plus 7,3 NM, plus aucune
  vitesse au-delà de 1 500 kt. Sur `migration`, à verser. **Reste une autre famille**, vue en
  remesurant : quand l'instance porte aussi les flux audio et leurs transcriptions, l'avion recule
  par moments d'environ 4 s de vol (0,4 à 0,5 NM) puis repart : 380 à 600 sauts par 5 min avec les
  flux, 12 sans (mesure A/B, même serveur, 15 h 52). Ni la correction du cap ni la couche des
  procédures n'y sont pour rien (essais A/B). **Diagnostic du soir (22 h)** : ni coupure de
  WebSocket, ni retard de livraison (les prédictions arrivent en 1 ms, avec ou sans flux), ni volume
  (identique) ; la prédiction du serveur est juste (écart médian 0,01 s de vol sur ~3 500, aucune
  au-delà de 2 s). Le saut est la règle de l'avion silencieux de la page (D80, 14/10) : un avion dont
  la station ne reçoit plus la position depuis 5 à 33 s, mais encore prédit par le serveur, est
  ramené à sa dernière position reçue, puis renvoyé en avant quand une position revient — et elle
  tombe sur la prédiction (8 cas sur 8). Non expliqué : pourquoi les flux audio multiplient ces cas.
  Au passage : chaque `aircraft_update` porte l'avion entier en plus du delta, ~5 Ko × 57/s, soit
  ~300 Ko/s de JSON pour la page.
  **Corrigé le 16/10 au soir (22 h 47, a09abf1) : le diagnostic de 22 h ci-dessus était faux.** La règle de
  l'avion silencieux (D80) n'était pas en cause et n'a pas été modifiée. La cause était le rafraîchissement
  complet de la carte, déclenché par les transcriptions : il replaçait chaque avion à sa position brute, ce
  qui explique que les flux audio multiplient les sauts. Corrigé (la synchronisation complète garde la pose
  animée) ; avec flux, 76 sauts en 5 min au lieu de 580 (67 sans flux). Même versement : une mise à jour
  d'avion porte son delta et sa datation, plus l'avion entier (91 Mo → 5,4 Mo par 5 min).
- 16/10 (après-midi) : **les routes publiées sur la carte** (Q56, feu vert du propriétaire ; forme de la
  session design, fiche `ProcedureRoute`). Le serveur lit, dans les tables de codage, chaque SID et
  STAR de LFPG et LFPO avec ses points dans l'ordre et ses contraintes par colonne : contraintes lues
  sur les 3 781 segments, en accord avec la lecture mot à mot sur les 2 907 lignes où elle est sûre ;
  428 routes entières sur 445. Points placés par l'ENR 4.4 entière, l'ENR 4.1, les balises d'un
  aérodrome cité (BOV, AD 2 LFOB), les fiches DATA, les seuils de l'AD 2.12 ; `GET
  /api/v1/procedure-routes`. La page : la puce d'une procédure trace sa route jusqu'à fermeture,
  note du ciel et ✕ ; une couche « Procedures (SID, STAR) », éteinte par défaut, pour les pistes en
  service. Vérifié sur l'instance d'essai, sans téléchargement (pages et tables du disque, la page AD 2
  de Beauvais ajoutée). Défaut trouvé au passage et corrigé (8659af3) : un point déplacé ce cycle prenait
  sa nouvelle latitude pour longitude (DEROL, DOPAP, LOUPY hors de la liste). Relecture design en cours.

- 16/10 : **les procédures entendues, nommées dans le fil** (Q55, usage 3 ; choix du
  propriétaire : ne comparer qu'aux points de procédure). La mise à jour depuis le SIA lit aussi les
  tables de codage SID/STAR de l'eAIP des aérodromes suivis (avec `pdftotext` de poppler s'il est
  installé, sinon les seules procédures nommées dans le texte) : 25 points pour LFPG et LFPO, tirés de
  100 tables. Une transmission est servie avec la procédure entendue (« ELCOB 7G » pour « elcope seven
  golf ») quand le mot ressemble au son à l'un de ces points ; la page souligne les mots en pointillé et
  ajoute une puce en pointillé, un clic montre le point dix secondes sur la carte. Réglage dans une
  nouvelle section « Regional data » : interrupteur allumé par défaut, trois positions mesurées sur
  l'écoute du propriétaire (Surer 1,10 : 9 justes sur 9 ; Measured 0,95 : 18 sur 18 ; More 0,90 : 18
  sur 19). Les valeurs mesurées ou décidées pour Paris disent d'où elles viennent ; une source qui ne
  couvre pas la station le dit, sans bouton. Vérifié sur l'instance d'essai, sans téléchargement (pages
  et tables du disque) : la puce, le soulignement, le point sur la carte, et la puce qui disparaît en
  « Surer ».
- 15/10 (soir) : **Q55, l'outil des noms officiels de l'AIP, piloté depuis les réglages** (le
  propriétaire : « il faut que ce script soit pilotable depuis les settings web »). Le script du
  labo devient le paquet Go `internal/aip` : cycle AIRAC calculé (tous les 28 jours depuis le
  01/10/2026), cinq pages de l'eAIP du SIA (ENR 4.4, ENR 2.1, AD 2 de chaque aérodrome suivi)
  téléchargées une fois par cycle dans un cache à côté des bases, liste écrite à côté des bases avec
  sa source et sa date, une page AD 2 manquante sautée et nommée. Routes `GET /api/v1/aip-names` et
  `POST /api/v1/aip-names/refresh`, derrière la session ; réglage « Update at each new AIRAC
  cycle », éteint par défaut (requête vers un site extérieur). Section dans « Callsign matching »
  selon la fiche SettingsPage de la session design. Vérifié : sur les pages du labo, les mêmes
  points (un au bord du rayon près) et procédures que son script, et 41 positions de contrôle de plus
  qu'il manquait (PARIS Contrôle, REIMS Contrôle…) ; sur l'instance d'essai, de « Not made yet »
  à « Up to date · AIRAC 2026-10-01 » en un clic, sans rien télécharger (pages du labo en cache,
  instance sans flux audio). La grammaire, mesurée par paires le même soir sur le ciel témoin du labo (02-04/10), **n'est pas adoptée** : 35 appariements perdus, presque tous justes (indicatifs au nom de compagnie mal transcrit, « Eziad Three Foxtrot Four »), 0 gagné ; le code est retiré, la mesure gardée hors dépôt (D71 ; détail en Q55).
- 15/10 (soir) : **migration du design versée dans `local` et prête pour la production**
  (efaaa7a) : étapes 12 et 13 et tout ce qui a suivi le même jour, relu conforme par la session
  design. Le propriétaire a choisi de la regarder en production plutôt que sur l'instance d'essai
  (l'iPhone y accède déjà, avec connexion, et le vrai modèle y tourne). Avant le versement : `go
  vet`, tous les tests Go, les tests du sidecar et le filet de la page, verts. Binaire de
  production reconstruit ; `runs/production.command` fige la page au lancement.
- 15/10 : **un avion perdu pointait au nord**, à 90° de sa trajectoire sur l'axe est-ouest de
  Roissy (vu par le propriétaire). Mesuré sur l'instance : sur dix avions perdus récents, sept
  n'avaient plus ni `track` ni cap dans leur dernier relevé (le récepteur les fait expirer) ; le
  rendu retombait alors sur 0° et écrasait le dernier cap à chaque mise à jour. Le glyphe garde
  désormais l'orientation qu'il avait (vérifié par une perte simulée : 62° avant, 62° après).
  Reste : une page ouverte après la perte n'a aucun cap à garder et dessine encore le nord ; le
  remède est côté serveur (conserver le dernier cap valide quand un relevé n'en porte plus), sur
  le chemin d'écriture des avions, à faire à part.
- 15/10 : **« Show only » depuis un avion** (le propriétaire pensait au Shift+clic ; écarté :
  invisible, sans équivalent au doigt, et il n'y a pas de sélection multiple). Sous la grille du
  récit, une rangée « Show only » et deux bascules : « FL 290 ± 500 ft » (±500 ft, choix du
  propriétaire) règle la réglette d'altitude, « CRZ » ne garde que ce code de phase exact ; elles
  se cumulent, et un second clic remet le filtre d'avant (vérifié : « up to FL 100 » revient). La
  note du ciel les nomme (« Showing 1 of 87 · FL 290 ± 500 ft · CRZ only ») et finit par un ✕ qui
  efface tout ce qu'elle liste. L'avion suivi reste dans la liste quels que soient les filtres,
  comme il restait déjà sur la carte. Forme de la session design (5530c9e).
- 15/10 : **échelle des altitudes aux couleurs des avions** (demande du propriétaire, forme A
  choisie sur une planche de la session design, 7e63e14 ; fiche AltitudeLegend, Slider mise à
  jour). En bas à gauche du ciel, une légende des cinq bandes de couleur, chacune avec
  l'histogramme de ses niveaux (hauteurs rapportées à la barre la plus haute de toute la légende),
  son nombre d'avions et les seuils dessous (1 000 ft, 3 000 ft, FL 125, niveau de croisière de la
  station) ; version claire sur IGN et OSM ; au-dessus du cadre PAR pendant un suivi (vérifié :
  8 px) ; interrupteur « Altitude legend » dans le menu des calques. La réglette de Traffic prend
  les mêmes couleurs, barre par barre ; son repère TA devient un pointillé tertiaire, « FL 300 »,
  qui chevauchait « FL 200 », est retiré. Les bandes viennent d'un seul module
  (`www/altitude/altitude-bands.js`, testé), que lisent aussi le moteur des avions, la légende et
  la réglette : elles ne peuvent plus diverger.
- 15/10 : **connexion au serveur perdue : un son neutre, une trace, le fil comblé** (le
  propriétaire entendait « retard retard » par moment). C'était le son d'Airbus de l'amont, joué à
  chaque coupure de la WebSocket, même d'une seconde, et qu'aucun réglage ne coupait ; une partie
  venait des redémarrages de l'instance d'essai, le reste de coupures que le serveur ne
  journalisait pas. Le serveur écrit désormais « WebSocket client left » avec la raison (page
  fermée, pas de réponse aux pings en 60 s, écriture échouée, **file de 256 messages pleine** : le
  serveur retire alors le client entier, pas le message, ce que son commentaire disait à tort), la
  durée de connexion et le navigateur. La page : le son ne part que si la coupure atteint 10 s,
  deux notes douces synthétisées (660 puis 495 Hz), réglables sous « Server connection » dans les
  sons de la cloche ; à la reconnexion, une ligne ▲ dans le journal (« Server connection lost from
  14:13Z to 14:22Z (8 min 43 s); 6 exchanges filled back into the thread. ») et les échanges gardés
  par le serveur pendant la coupure remis dans le fil. Forme décidée par la session design
  (7dec77e). Vérifié par une coupure simulée : 6 échanges retirés, 6 remis. Le son « retard » et son
  fichier sont retirés.
- 15/10 : **le voyant de transcription distingue un faux sidecar** (demande du propriétaire :
  même voyant vert avec le vrai modèle et le faux sidecar de l'instance d'essai). `/api/v1/status`
  publie ce que le sidecar déclare (`backend` : `mlx`, le nom d'un GPU, ou `fake`) et la famille du
  modèle ; la page affiche « Simulated » en ambre quand c'est `fake`, avec une bulle qui dit que les
  textes sont inventés et que les chiffres d'appariement n'ont pas de sens ; avec le vrai modèle,
  la bulle nomme le modèle et ce qui le fait tourner. Vérifié sur l'instance d'essai redémarrée.
- 15/10 : **étiquettes de la carte allégées** (demande du propriétaire du 13/10, choisie le 15/10
  sur deux planches de la session design ; fiche MapLabel, 597c142). Mesure de la session sur
  environ 110 avions : au zoom par défaut, 62 % des étiquettes complètes en chevauchaient une
  autre, 87 % à 67 NM. Trois paliers selon la largeur du ciel en NM au sol (33,7 NM au zoom par
  défaut, recoupé ici) : jusqu'à 24 NM tout est complet ; de 24 à 67 NM l'indicatif seul, sauf
  les avions qui comptent (entendus depuis 10 min, dans une séquence d'arrivée, suivis, squawk
  d'urgence) ; au-delà, l'indicatif de ceux-là seulement ; le survol montre tout. Le dernier
  message n'est plus une troisième ligne mais une bulle sous l'étiquette, dessinée dans un canevas
  (barre info, texte de lecture, âge), pleine 2 min, à 55 % jusqu'à 10 min, qui suit le fondu du
  moteur de mouvement. Écart à la fiche, soumis : la bulle est opaque, et l'étiquette à deux
  lignes mordait de 7 px sur le glyphe ; avec une bulle, le bloc s'arrête 2 px au-dessus du glyphe.
  **Relu conforme, écart accepté.** Recompte de la session design, au même instant sur le même
  ciel (part des boîtes chevauchées, « avant » estimé à 140 px par étiquette) : 24 NM 67 → 21 % ;
  34 NM (défaut) 67 → 37 % ; 48 NM 68 → 55 % ; 67 NM 81 → 20 % ; 135 NM 93 → 45 % ; 270 NM
  97 → 58 %. Avions sous une étiquette à 67 NM : 28 → 5. Il reste des chevauchements entre 34 et
  48 NM autour de l'approche de Roissy, où les avions qui comptent sont serrés : laissé tel quel
  pour une soirée, choix du propriétaire.
- 15/10 : **palette de la carte, choix C du propriétaire** (seulement les conflits ; jetons
  `color-map-*` et `color-map-light-*` de la session design, 34ecf70). Un module commun
  `www/map/core/map-palette.js` les lit, avec la variante des fonds clairs (IGN, OSM). Anneau de
  l'avion choisi en accent (2 px), survol en texte primaire (1,2 px) ; traces à la couleur
  d'altitude de l'avion à 50 %, au sol aussi ; vol suivi en accent pointillé ; estimations (futur,
  rétro-prévision) en accent pointillé fin ; trace ancienne ou perdue en gris ; taxiways et
  anneaux gris ; balises, pistes et grands aéroports sur leurs jetons ; l'onde de décollage ou
  d'atterrissage prend la couleur de la famille de phase. Le jaune #FDE047 de l'avion choisi
  était dans une fonction morte (`_getAircraftStyle`, retirée) : le vrai halo était blanc. Les sept
  restes, tranchés par la session design le même jour (eedecb2, mesurés en ΔE76) : axes de piste
  en `centerline` gris (le vert était à ΔE 1,2 de l'accent) ; aérodromes moyens en `airport`,
  petits et hydrobases en `airport-small` ; héliports en `heliport` violet (l'orange était à
  ΔE 3,3 de la phase départ) ; DME seuls en `navaid-dme` ; cercle de proximité en `proximity`,
  l'accent du vol suivi ; jeton `airspace` retiré, les espaces aériens étant des tuiles openAIP ;
  neutres gardés. Dans le code de la carte, il ne reste en dur que ces neutres. Rendu sur fond
  clair (IGN, OSM) non mesuré : à regarder pendant la soirée du propriétaire.
- 15/10 : **relecture design de l'étape 13**. Le « 3° » de la PAR passe en texte tertiaire
  (#949494, environ 6,8:1 ; il était à 2,7:1) ; le « 10 NM » de la portée de la connexion reste,
  décoratif et `aria-hidden` (WCAG 1.4.3, noté dans la fiche SignIn). La légende de la mini-carte
  disait « Past » en rouge d'urgence et « Future » en jaune des avions très bas, alors que la trace
  était grise : la mini-carte et sa légende prennent les mêmes jetons (position en accent, ou en
  texte secondaire si le signal est perdu ; trace reçue en tertiaire pointillé ; estimations en
  accent pointillé). Les positions estimées avant le premier contact, en cyan et absentes de la
  légende, rejoignent les estimations, d'où « Estimated » plutôt que « Future ». La colonne des
  secondes du tableau est retirée, l'heure suffit. La palette de la carte attend le choix du
  propriétaire.
- 15/10 : **étape 13 (nettoyage) livrée sur `migration`**. Mesuré par `tools/design/inventory.mjs`,
  avant la migration → avant le nettoyage → après : utilisations de classes Tailwind 3 879 → 834
  → 433 ; couleurs distinctes 215 → 122 → 82 ; valeurs en dur 284 → 241 → 177 ; `style.css`
  311 → 161 lignes, **plus aucune couleur en dur** (toutes en jetons, transparences par
  `color-mix`). Retirés : les règles mortes (famille `flight-strip`, filtres, visualiseur, barre
  de défilement personnalisée, bande de tuiles, surlignage de transcription), le code des fenêtres
  de transcription de l'ancienne barre radio (`toggleTranscriptionViewer`, `layoutTranscriptionViewers`,
  `openTranscriptionAt`, la recherche par fréquence, les compteurs non lus) et la barre de niveau
  des tuiles. Restylés sur le système, au contenu inchangé : l'onglet Track (mini-carte, positions
  en `cl-table`, tendances en flèches), Nearby (curseur et tableau), la liste radio de Details (en
  échanges du fil), les deux menus de la liste. Les 177 valeurs en dur restantes sont la palette
  propre de la carte (aéroports, balises, héliports, espaces aériens, traces), celle de la PAR et
  les dessins de la connexion : soumises à la session design (jetons de carte). Sonde de
  contraste sur les quatorze écrans : 0 caractère sous le seuil sur onze, quelques caractères
  sur trois (« 3° » de la PAR, « 10 NM » de la portée décorative de la connexion), signalés.
- 15/10 : **étape 11 versée dans `local`** après trois recomptes design et le coup d'œil du
  propriétaire. Tab rendu au navigateur ; J/K (et ↓/↑ sur la carte, qui ne se déplace plus aux
  flèches) d'un avion à l'autre ; A, G et 0-9 sans Cmd/Ctrl/Alt ; rôles et tabulation pour ce qui
  joue un bouton ; liens d'évitement (carte, colonne, fil) ; la page des réglages en dialogue
  modal (reste de la page inerte, focus dedans, rendu à la roue) ; les popovers prennent le focus
  et le rendent ; le focus suit un vol et le repli de la colonne, au clavier seulement ;
  « Réduire les animations » : durées à zéro, défilements instantanés, les avions gardent leur
  glissement (décidé par le propriétaire le 15/10). L'outil de capture envoie de vraies frappes
  (`--keys`, `--trace`) ; sa première version oubliait le « \r » d'Entrée et masquait le défaut
  des dialogues, trouvé par la relecture.
- 15/10 (matin) : **nuit de mesure du 14 au 15/10** (instance d'essai, vrai modèle, D81 et modèles
  gardés en mémoire, 20 h 56 → 9 h). Première lecture, 3 850 transmissions : **1,0 s en médiane,
  2,4 s au 90ᵉ centile**, stable d'heure en heure, pic du matin compris (7 h : 464 transmissions,
  1,0 s, 1,8 s au 90ᵉ). Lecture française, 2 136 : 1,7 s après l'anglaise en médiane, 4,0 s au
  90ᵉ ; 2 030 d'entre elles ont servi à la reconnaissance. Part reconnue 21 h – 9 h : **34,5 %**,
  contre 27,4 et 25,1 % les nuits du 2-3 et du 3-4/10 sur les mêmes fréquences ; écart non
  attribuable à une cause unique (règles, détecteur, D81 et trafic ont changé entre-temps), et la
  précision n'est pas mesurée. ADS-B : 14 coupures d'une à deux minutes (« i/o timeout » vers la
  station), rétablies seules. Disque : +1,5 Go d'archive audio, +0,3 Go de base, 82 Go libres.
  Mémoire du sidecar : 14 → 17 Go en douze heures (trois modèles), à surveiller. **Le relevé toutes
  les 5 minutes, lui, n'a tourné que dix minutes** : le redémarrage de 20 h 56 l'a arrêté (motif
  `pgrep` trop large) ; corrigé dans l'outil (ce59219) ; files d'attente et mémoire heure par
  heure ne sont donc pas connues pour cette nuit.
- 14/10 (soir) : **étape 10 versée dans `local`** après relecture design et coup d'œil du
  propriétaire. L'écran plein « Connection Lost » devient un `cl-banner` en haut du ciel, vérifié
  en coupant le serveur sous une page ouverte (« Connection to the server lost since 18:31Z:
  reconnecting (attempt 4), next in 9 s. The map shows the last positions. ») ; le fil vide dit
  pourquoi (récepteur prêté ou éteint depuis quand, aucune fréquence, rien de transcrit encore) et
  son en-tête n'offre plus de canal quand rien n'est à l'antenne ; l'avis de reprise prend la forme
  du système. Le même soir, hors migration : **D81** (la première lecture d'abord, la seconde
  ensuite) et le correctif du sidecar qui garde les deux modèles en mémoire (mesure du labo :
  4,58 → 0,90 s pour la lecture anglaise, 4,66 → 0,99 s pour la française) ; la mesure en direct
  suit.
- 14/10 (soir) : **étape 9 versée dans `local`** après le coup d'œil du propriétaire (« ok
  validé sauf la roue crantée qui ressemble à un soleil » : remplacée par une roue dentée). Ses
  décisions du soir : la vue PAR **s'adapte à la hauteur de l'écran** (bande d'élévation de 80 à
  230 px selon la place que laissent têtes et tableaux ; une piste sur un grand écran garde la
  pleine hauteur, deux sur 900 px tiennent sans défiler) ; la latence de transcription transmise au
  labo, qui l'a décomposée : de la requête au texte 7,8 s en médiane, 31,7 s au 90ᵉ centile ; calcul
  seul 3,1 s (0,40 s par seconde d'audio) ; **le retard vient de la file à un seul fil** (attente
  médiane 0,5 s, 26,5 s au 90ᵉ centile, quand les deux fréquences parlent ensemble ; la seconde
  lecture française passe devant la première lecture de l'autre fréquence).
- 14/10 (nuit) : **étapes 9b, 9d et 9e livrées sur `migration`, relues conformes pour 9b et 9d**
  (9e en relecture ; 9c, les réglages, en cours). 9b : les phrases d'événements alignées sur la fiche
  EventList, écrites en un seul endroit (`events.js`) pour la colonne et la cloche (« 7700 EZY306H,
  emergency squawk, FL 390, 36 NM north. »), les fiches au nom court. 9d : la connexion et le
  premier lancement sur le ciel (`SignIn` variante ciel), la console de la station lue sur
  `/api/v1/vitals` (route publique, cache 10 s, déjà côté serveur) ; `login.js` perd sa palette ;
  au téléphone une colonne dans le flux (règle du système corrigée, acceptée par la relecture) ;
  aperçu `?signin=1` / `?signin=setup` sur une instance de cette machine. 9e : la vue PAR sur le
  système (ParHead, ParFrame, ArrivalSequence), la piste hors service repliée par défaut.
- 14/10 (soir) : **étape 9b livrée sur `migration`**. Les fiches d'aérodrome, d'héliport et de
  balise deviennent des `cl-card` (tête : indicatif en mono, nom, × ; `KeyValues` ; fréquences en
  `cl-table`, MHz à trois décimales), fermées par ×, Échap ou un clic à côté ; les valeurs venues
  de la base sont échappées ; les règles `ref-popup` de `style.css` (couleurs en dur) sont
  retirées. Le journal devient un `cl-menu` sous la cloche (a-journal) : « Sounds » et « Clear »
  dans sa tête, « Ring a bell for » en cinq cases (la station et ses flux sous une), les lignes
  marquées comme dans Right now (■ ▲ ●), à l'horloge de la page (UTC par défaut ; il affichait
  l'heure locale avec les secondes). Au passage, la trace d'un avion perdu passe au gris tertiaire
  du système (relecture design).
- 14/10 (soir) : **trois retours du propriétaire pendant 9a.** (1) **Perte de signal** : un
  avion trop bas avant l'atterrissage restait figé à côté de sa trace. Cause mesurée : pendant la
  minute qui précède « signal lost », le serveur envoie des positions prédites en ligne droite et
  le moteur extrapole ; l'avion déclaré perdu n'est plus animé là où la prédiction l'avait mené.
  Désormais un avion silencieux revient en glissant (1,2 s) à sa dernière position observée et y
  attend, sans estimation : sous 5 000 ft après 12 s sans position, plus haut une fois déclaré
  perdu. Le banc de rejeu a tranché le seuil : un arrêt à 12 s partout dégradait le trou de 30 s
  en croisière (erreur moyenne 97 → 414 m, images lentes 1,36 → 2,30 %) ; avec la règle « bas
  seulement », les métriques du banc sont inchangées. Une fois perdu, le symbole est gris (texte
  tertiaire) au lieu de la rampe d'altitude, l'étiquette dit « lost 3 min · last 1620ft » ; il
  reste sur la carte le temps du réglage « last seen » (10 min), comme avant. (2) **L'indicatif
  reconnu dans le fil** passe en vert d'accent (règle du système, `.cl-exchange__text b`,
  contraste 8,1:1) ; les valeurs reconnues ne se marquent pas dans le texte, les puces les disent.
  (3) Relecture de 9a : conforme ; la carte repliée se suspend sous la rangée (système). Ajouté à
  l'étape 9 par la relecture : la vue PAR elle-même (tête, tableaux, piste hors service repliée),
  prévue en 9e.
- 14/10 (soir) : **étape 9a livrée sur `migration`** : la rangée du haut du ciel (`cl-sky__top` :
  la bascule Map / PAR · LFPG… en `SegmentedControl`, les notes de carte, la colonne repliée à son
  bout), le bouton des calques dans le coin (`cl-sky__corner`) et son menu `cl-menu--map` (fond de
  carte en `SegmentedControl`, Aircraft, Layers avec interrupteurs et opacités ; mêmes réglages,
  même enregistrement dans le navigateur, domaine `store/sky.js`). L'ancien bouton « Radar / PAR »
  en haut à droite et le panneau « Map Controls » sont retirés. Les notes se raccourcissent avec
  des points de suspension plutôt que de pousser la carte repliée hors de l'écran.
- 14/10 : **étape 8 versée dans `local`** (b0c4e23) après le coup d'œil du propriétaire (« ok pour
  l'étape 8 »). Dernier détail de la relecture : le voyant d'antenne dit les jours au-delà de 48 h
  (« 5 days ago » au lieu de « 126 h 27 ago »).
- 14/10 (fin d'après-midi) : **trois retours du propriétaire sur l'étape 8.** (1) « Le cadre de
  piste n'apparaît pas » pour un avion [ARR] : il ne venait que pour un avion déjà dans le cône
  d'une piste, 3 à 5 des 21 avions ARR/APP mesurés. Il vient maintenant pour tout avion ARR ou APP
  dont l'aérodrome est suivi et plausible : le cadre de la piste en service, sa tête disant
  « AFR1855 off the axis » tant qu'il n'y est pas (relecture design : « yet » jugeait), la piste en
  service retenue étant celle dont l'axe prolongé passe le plus près de l'avion (le score ne
  départage qu'à égalité ; un doublet de De Gaulle ne range plus un avion du nord sous 26L) ; 14 sur 21 mesurés (les 7 autres, un
  aérodrome non plausible). (2) « Antenna: no report since 14:20:38Z » : vrai, l'instance d'essai
  n'avait reçu qu'un rapport (`radio-ctl-sync -once`) ; il tourne désormais en continu à côté
  d'elle. (3) « Showing 8 of 70 sans filtre » : le filtre « Heard » de l'onglet Traffic, gardé dans
  le navigateur d'une visite précédente ; la note de carte nomme maintenant chaque filtre actif
  (« Showing 9 of 72 · heard on the radio only »).
- 14/10 : **étape 8, corrections de la relecture design** (sept points). La colonne garde ses
  380 px pendant un suivi (elle passait à 480 et décalait la carte). **Le cadre PAR sous la carte**
  : `par-view.js` gagne un mode « un seul cadre » (`create(…, { single: true })`), qui dessine le
  cadre de piste de l'avion suivi avec les mêmes fonctions, en plus bas (bandes 140 et 36 px au
  lieu de 230 et 64), sous une tête « 25 → 07 · IN USE · 2 aircraft · Open the PAR view » ;
  affiché quand l'avion est dans une séquence d'arrivée et que la carte est à l'écran. Dans la
  barre du haut, « Following » et la puce de l'indicatif avec ✕ après la marque. La note de carte
  dit ce qu'elle montre (« VLG9EJ · its track since 14:29Z », sur une ligne). **Le vol en cours
  seulement** : il commence à la dernière réapparition (phase NEW, ou trace muette plus de 10 min),
  les vols précédents de la journée se replient (« Earlier today: 2 flights ») dans Story et dans
  l'historique des phases ; un T/O ou un T/D jugé dans la minute d'une réapparition n'est pas
  raconté (daté de la réapparition, pas observé). **Une ligne par événement radio** (« Controller
  on 124.350: FL 100 », « Handed to Approche De Gaulle ») ; un clic amène l'échange au milieu du
  fil, filtres levés. « Seen by the receiver, FL 110, 15 NM south », du premier point de chaque vol.
  **L'aérodrome d'une phase n'est nommé que s'il est plausible** (point de la phase sous 10 000 ft
  et à moins de 20 NM, ou avion dans sa séquence d'arrivée) ; même règle pour le badge de
  l'en-tête. Constaté : un A321 à FL 110, 15 NM au sud, en « Arrival at LFPZ » pour le détecteur ;
  l'attribution elle-même, côté serveur, reste à revoir (le détecteur nomme un aérodrome suivi
  quel que soit l'éloignement). Petits points : secondes retirées de l'historique des phases,
  « Radio · none matched », types en casse normale (« Airbus A-321 », un mot à chiffres reste
  tel quel), âges en heures et minutes. « Map controls » se pose 8 px au-dessus du cadre PAR
  tant qu'il s'affiche (hauteur mesurée ; bas du bouton 619 px, haut du cadre 627 px, à
  1 440 × 900), à la demande du recompte design ; le reste de `MapControls` attend l'étape 9.
  Recompte design : conforme. Noté pour le détecteur de phases, avec l'aérodrome des phases : un
  avion aligné sur 27R à 2 300 ft retombe de APP à UNK en courte finale.
- 14/10 : **étape 8 livrée sur `migration`** (suivre un vol, `AircraftStory`). Choisir un avion
  remplace le contenu de la colonne : « ‹ Back to Traffic » (ou « Back to Right now », l'onglet
  d'où l'on vient), l'en-tête (phase, indicatif, aérodrome, exploitant · type · immatriculation ;
  altitude, distance à la piste et temps restant quand l'avion est dans une séquence d'arrivée,
  sinon distance à la station ; vitesse sol ; cap), les onglets Story, Details, Track et Nearby
  (anciens Timeline, Details, Tracks, Proximity, contenus et rafraîchissements inchangés, Story
  par défaut) ; ✕ « Stop following » dans la barre du haut ; une note sur la carte, « Following X ·
  its track since 14:23Z ». Details en sections repliables (`cl-disclosure`) : historique des
  phases, **consignes entendues avec « Then »**, radio (repliée), puis les faits ADS-B regroupés
  comme c-details. **Écart au plan** : le tableau des consignes ne lit pas `clearances` : le flux
  WebSocket ne les porte pas, la page ne les recevait presque jamais (seule la ligne Nearby, par
  REST). Il lit les valeurs `plain` (déjà munies de `value` et `unit`) de ce que le contrôleur a dit
  à l'avion : les 4 h 30 serveur prévues ne sont pas nécessaires. Sur l'instance d'essai, vrai
  modèle : « climbing through FL 148, 2 min later », « turned to 309° », « level at 3 900 ft, 1 min
  later ». La vitesse se dit « ground speed » (demande de la relecture de K). Le mot « deviation »
  n'apparaît plus. Au passage : un appel à une fonction disparue (`selectAircraft`, à chaque
  clairance de l'avion choisi) retiré, et le filet le retire de ses défauts connus ; la règle du
  système `.cl-story__events i` visait toutes les `<i>` du récit (barres de force, icônes) et ne
  vise plus que la pastille (source `bundle.css`, exportée). Vérifié sans fenêtre : depuis la
  liste, Échap, depuis le fil, depuis la PAR et le journal (`selectAircraftByHex`), un autre avion,
  les quatre onglets, ‹ et ✕, aucun débordement à 390 et 1 440 px.
- 14/10 : **étape 7 versée dans `local`** (3ef997e). **Alarme ADS-B** (choix du propriétaire,
  sans test de coupure Wi-Fi) : le voyant dit « Unreachable · N min » quand chaque lecture de la
  station échoue, avec la raison en mots et le conseil de relancer Clairance si la station
  répond ailleurs ; `/api/v1/status` porte `failing_since` et `failing_why`. Vérifié sur une
  instance jetable et une source factice coupée puis rétablie. La cause du blocage du 13/10
  reste non prouvée. L'instance d'essai tourne avec le **vrai modèle** (choix du propriétaire,
  créneau inscrit au planning du labo). Purge GitHub reportée par le propriétaire.
- 14/10 : **correctif, la barre du haut tient au téléphone** (6a816f6, sur `main`), défaut trouvé
  par la relecture design de l'étape 7 et présent depuis l'étape 3 : à 375 px la barre courait
  jusqu'à 780 px, la cloche et les réglages hors de l'écran (journal et réglages inaccessibles au
  téléphone). Sous 768 px : le signe, le voyant d'antenne coupé d'une ellipse, l'UTC, la cloche et
  les réglages à 44 px ; le nom, l'heure locale et les voyants « ok » s'effacent. Mesuré à
  375 × 812 (Chrome sans fenêtre) : page de 375 px, cloche à x 275, réglages à x 327. Ma mesure de
  l'étape 7 (« aucun débordement ») regardait la barre du bas, pas la largeur de la page : au
  téléphone, mesurer `document.documentElement.scrollWidth`. Étape 7 relue conforme au grand écran.
- 14/10 : **étape 6 versée dans `local`** (bb56782) après le coup d'œil du propriétaire (« ça me
  va »). **Étape 7 livrée sur `migration`** (944c8c4) : la barre radio quitte le grand écran,
  le fil y fait tout ce qu'elle faisait (table de `09-ecrans.md`, lignes de la barre : écouter /
  couper, volume, tout couper, un seul fil avec filtre, recherche et « ↓ N new », niveau, échec
  et connexion) ; la page des réglages ne lui garde plus ses 80 px. **Au téléphone la barre
  reste** jusqu'à l'étape 12, où le fil y est encore caché : ses fenêtres et leur recalcul de
  disposition partent alors. Mesuré (Chrome sans fenêtre) : à 1 440 × 900, carte, fil et colonne
  jusqu'en bas ; à 375 × 812, la barre est là, le fil caché, aucun débordement. Écart à la fiche
  ChannelMenu : pas de bouton « Retry » pour une fréquence en échec ; la barre n'en avait pas
  non plus (elle réessaie seule), parité tenue, l'ajout reste possible. L'instance d'essai a
  désormais deux configurations : `config-migration.toml` avec le vrai sidecar (créneau GPU à
  inscrire), `config-migration-fake.toml` avec le faux, sans GPU.
- 14/10 (matin) : **étape 6, corrections de la relecture design** (migration 3caa41f, 54069f8 ;
  serveur 24f060f). Le fil **ne décroche plus du bas tout seul** : un échange grandit après son
  arrivée et le plafond en retire en haut, ce qui déplaçait la lecture sans le lecteur (290 px en
  12 s) ; seule une molette, un toucher, une touche ou la barre lâchent le bas. Les consignes
  s'écrivent « FL 050 » et « Contact 128.750 » à la source (`explain.go`, valeur comprise), et
  « no aircraft in range was named ». Une rangée de boutons de fréquence, nom coupé d'une
  ellipse ; chaque menu sous son bouton ; `<b>` et `<mark>` du système. Vérifié dans un Chrome
  sans fenêtre (`tools/design/capture.mjs`) : rangée de 32 px, menu à 1 px de son bouton. Leçon
  de test : le volet du navigateur intégré, masqué, ne produit aucune image ; Alpine y laisse
  masqué ce qu'il affiche à l'image suivante, ce qui imite un défaut qui n'existe pas.
- 14/10 (matin) : **fiches J, K et M intégrées** (vague 3 cloud, relues sur le Mac ; aucune ne
  touche aux fichiers de la migration, aucune valeur privée). **J** : le serveur compile pour
  Windows (groupe de processus du sidecar et espace disque séparés par système, saisie du mot
  de passe par `golang.org/x/term`) ; vérifié ici : compilation Windows et Linux, l'espace disque
  de `/api/v1/server` identique à `df` ; pas exécuté sous Windows ; limites dites dans
  `J-rapport.md` (pas de Job Object, arrêt sans SIGTERM). **K** : `www/story/then.js`, ce qu'un
  avion a fait après une consigne, sans verdict, 43 cas ; pas encore branché (étape 8). **M** :
  le banc du mouvement dans un vrai navigateur (`tools/motion-bench`, générateur `simtraffic`),
  cinq corrections (un avion apparu page ouverte n'était jamais animé ; première minute après
  rechargement ; retour d'onglet sans balayage ; reprise après un long trou en fondu au lieu
  d'une glissade ; moteur à 30 i/s au lieu de 20 ; `sent_at` dans la réponse groupée), et les
  stratégies B (dessin dans le passé) et C (hybride) en option, **A gardée par défaut** : B ne
  gagne que 2 à 6 points de régularité pour 1,1 km de retard. **Le choix A/B/C revient au
  propriétaire.** Après intégration : `go test -race` sur les 17 paquets, filet, compilations
  croisées, verts. **Observation** : l'instance d'essai, lancée par la session Claude, a perdu
  l'ADS-B à la coupure réseau de 20 h 42 et ne l'a jamais retrouvé (« no route to host »,
  43 962 échecs en douze heures) alors qu'un processus neuf se connectait ; relancée, elle a
  retrouvé 74 avions. Hypothèse non prouvée : le filtrage « réseau local » de macOS par
  processus. À surveiller en production, qui est lancée depuis le Terminal. Le banc de M, rejoué
  sur le Mac (scénario b, Chrome sans fenêtre) : 0 % de reculs, 0,13 % d'images lentes, 11,6 %
  d'écart-type de vitesse dans le moteur, 26 m d'erreur moyenne, une pose toutes les 33 ms ;
  les chiffres du cloud (11,5 %, 25 m) tiennent sur la vraie machine.
- 13/10 (nuit) : **étape 6 de la migration livrée sur `migration`** (a15da0e), en relecture
  design. La colonne de gauche devient **le fil** : tous les échanges de toutes les fréquences
  en une liste qui grandit vers le bas, l'en-tête des fréquences (écouter / couper, menu avec
  volume et « seulement cette fréquence », filtre « Show », recherche, « Mute all »), chaque
  échange avec locuteur → destinataire, puce d'indicatif et sa phase, force des indices et sa
  preuve, réécoute, transcription (`lang="fr"` pour une émission française), consignes en puces
  et, pour une émission française, en français ; « quelqu'un parle » par le niveau audio, que
  `audio-client.js` dessine désormais aussi dans le fil (il survivra au retrait de la barre) ;
  « ↓ N new » quand on a remonté. La barre du bas reste jusqu'à l'étape 7 pour comparer.
  **Inventaire préalable** : `openTranscriptionAt` n'avait plus d'appelant (les bulles de la
  carte retirées le 13/10, rien d'autre ne l'appelait) : rien à rebrancher ; l'API ne garde
  aucun lien entre une consigne et la réponse du pilote : la ligne « Pilot replied, 3 s later »
  des maquettes n'a pas de source. **Banc de l'instance d'essai** : le faux sidecar répond
  désormais à chaque vraie transmission par une phrase inventée qui nomme un avion présent, en
  anglais ou en français, et le rapprochement local est activé comme en production : on voit des
  échanges appariés, faibles et non appariés arriver en direct (jamais pour juger la
  transcription). **Mesuré** (120 échanges, 150 avions, 10 s) : 0 blocage du fil principal,
  6 % d'occupation ; la table des puces rafraîchie toutes les 5 s a ramené les réécritures du fil
  de 794 à 10 et les recherches d'avion de 1 179 à 62 ; sonde du fil : 853 textes, 0 sous 11 px,
  0 sous AA ; les puces d'indicatif font 20 px (géométrie du système), sous les 24 px.
  **Non vérifié** : « quelqu'un parle » en direct (l'onglet visible disponible était en
  arrière-plan, où le navigateur suspend l'animation ; même valeur que la barre de la tuile, dans
  la même image). Au passage, une coupure de trois minutes entre le Mac et le port ADS-B de la
  station (20 h 42-20 h 45, la station et ses conteneurs sains) a vidé un moment les flux.
- 13/10 (nuit) : **étape 5 versée dans `local`** (0f846b1) après le coup d'œil du propriétaire
  (« ok pour l'étape 5 ») : « Right now » par défaut, colonne repliable, météo et piste en service
  sorties de la barre du bas, avec les corrections de la relecture design et la fiche L. En
  production au prochain lancement (copie de `www/` prise par `runs/production.command`).
- 13/10 (nuit) : **fiche L intégrée** (vague 3 cloud, branche `claude/blissful-babbage-oiwjxm`,
  quinze commits) : le bloc position, trajectoire, prédiction remis d'équerre. Banc de rejeu
  synthétique (générateur Go à 0,6 position/s, récepteur fictif à 45,0 / 4,67, flux enregistré,
  rejeu node à 30 i/s) avec l'erreur contre la vraie position ; un seul prédicteur côté serveur
  (`PredictFuturePositions` retiré) ; la prédiction en direct dans `live_prediction.go` ; formes
  des messages figées clé par clé ; `aircraft.adsb` ne porte plus que ce que la station a dit,
  les estimations vont au moteur (`observe`, `estimate`, échantillons datés par leur source,
  décalage d'horloge serveur/page estimé) ; trois corrections du serveur de production,
  contribuables : la route prédite part de la route du relevé, l'état prédit suit un virage,
  `seen_pos` compté depuis le `now` d'`aircraft.json`. Banc : erreur moyenne 181 → 38 m, pas à
  reculons 1,3 → 0 %. **Relu et mesuré sur le Mac** : build, vet, `go test -race` (seize paquets),
  filet vert ; aucun fichier de la migration touché, aucune valeur privée, l'enregistrement est
  synthétique ; horloges vérifiées (station à 0,1 ms du temps de référence, Mac à 6 ms, âge du
  fichier 0 à 1 s, médiane 0,52 s, l'hypothèse du banc). **En direct, onglet visible, même trafic,
  deux paires de 20 s** :

  | | avant (D79) | après (fiche L) |
  |---|---|---|
  | pas à reculons | 1,1 % / 0,7 % | 0 % / 0 % |
  | images lentes (avions rapides) | 3,2 % / 3,3 % | 0,7 % / 0,1 % |
  | sauts | 2,6 % / 1,2 % | 0,6 % / 0 % |
  | variation de la vitesse dessinée | 38 % / 48 % | 13 % / 11 % |

  Version intégrée (sur `local` et `migration`) : 0 % d'images lentes, 0 % de sauts, 8 % de
  variation, **une fois passée la première minute après un redémarrage du serveur** (la passe
  faite aussitôt montrait 76 % : le moteur estime le décalage d'horloge sur 30 s et le serveur
  n'a pas encore d'historique). Cas limite restant : un avion qui retrouve une position après
  un long trou de réception (AIC143, 18 s sans position, 45 NM) recule pour la rejoindre. Notés
  par la session cloud et non traités : chaque `aircraft_update` transporte l'avion entier,
  prévision comprise (≈ 5,8 Ko par avion et par seconde, à mesurer sur le réseau) ;
  `aircraft-animation.js` reste à 898 lignes, loin des 300 visées côté page.
- 13/10 (soir, tard) : **étape 5 de la migration livrée sur `migration`** (a1533b8), en relecture
  design avant le coup d'œil du propriétaire. « Right now » devient l'onglet par défaut de la
  colonne : la **phrase de synthèse** (gabarits de `www/now/right-now.js`, douze tests node : piste
  de chaque aérodrome suivi et depuis quand, « depuis au moins » l'heure de démarrage du serveur
  tant qu'il n'a vu aucun changement, seconde piste probable, nombre d'avions en finale et prochain
  toucher), les **tableaux d'arrivée** (ceux de la PAR, hors de la PAR), les **événements** de la
  dernière demi-heure, la **météo** de l'aérodrome de référence avec le **METAR lu en clair**
  (`www/now/metar-plain.js`, sept tests ; la source ne donnait qu'une lecture technique), TAF,
  NOTAM et METAR précédents derrière des boutons, et les **compteurs**. Le bouton » **replie la
  colonne** (disposition B, nouvelle clé `columnFolded`) : la carte prend la largeur, « Right now »
  devient une carte sur le ciel. Retirés de la barre du bas : METAR, TAF, NOTAM, piste en service,
  leurs bulles et trois aides du store sans lecteur. Vérifié sur l'instance d'essai : la phrase a
  suivi un vrai changement de piste (De Gaulle passée sur 26L, « since 16:04Z ») ; le repli gardé
  au rechargement ; le téléphone inchangé, sans débordement. **Mesuré** (149 avions, 10 s) : avec
  « Right now » ouvert, **aucun blocage du fil principal** contre 25 blocages et 105 ms perdues par
  seconde avec la liste Trafic ouverte (le chantier de la liste se règle donc d'abord en ne la
  dessinant plus par défaut) ; colonne : 0 texte sous 11 px, 0 sous AA ; page : tailles en dur
  79 → 74, opacités de texte 107 → 95, classes du système 81 → 128. Écarts nommés pour la
  relecture : carte repliée sous la bascule carte/PAR (qui garde le coin jusqu'à son étape) ; TAF
  et NOTAM en carte dépliée sous les boutons plutôt qu'en bulle ; bouton « Earlier » pour les
  METAR précédents (fonction de l'ancienne bulle gardée) ; « uses runway » plutôt que « lands on »
  (`runway_in_use` ne distingue pas arrivées et départs) ; lignes d'événements cliquables à 16 px
  (règle du système à porter à 24) ; pas de classe de texte « urgent » hors de la liste. L'outil
  d'inventaire de la session design ne tourne plus depuis l'étape 2 (il cherche `PHASE_COLORS`
  dans `par-view.js`) : signalé. L'instance d'essai suit désormais Orly comme référence, comme la
  production (Saint-Cyr ne publie pas de METAR). **Relecture design** (même soir, capture à
  1 440 × 900 par `tools/design/capture.mjs`) : disposition et repli bons, écarts tranchés, quatre
  corrections faites (7016379) : (1) **le sens d'une piste** — De Gaulle atterrit sur son doublet
  ouest (26L à 0,57, 27R à 0,43) et la tête passait de l'une à l'autre toutes les quelques minutes :
  le « depuis » se remettait à zéro et trois lignes sur cinq de « Worth knowing » étaient fausses.
  Deux pistes dont les numéros diffèrent d'au plus 1 sont de même sens : nommées ensemble, et un
  changement de piste est un changement de sens, côté serveur (b9e9675, commit autonome,
  contribuable, testé), dans le journal (« LFPG switched to landing west ») et dans la phrase ;
  (2) la carte repliée parle d'un seul aérodrome, celui du prochain atterrissage, et tous les
  comptes lisent les séquences des tableaux ; (3) le present perfect avec « since » ;
  (4) un journal vide dit depuis quand la page regarde. Système recopié depuis design-suite
  db61427 (lignes d'événements à 24 px, `cl-urgent`, boutons dépliés dessinés sélectionnés,
  outil d'inventaire réparé).
- 13/10 (soir, tard) : **fiches H et I intégrées** (vague 3 cloud). H (`claude/quirky-ritchie-mfjscr`,
  six commits) : le serveur démarre **sans transcription** quand aucune source ne transcrit et
  qu'aucun sidecar n'est configuré (`Sidecar.Idle/Off/Ensure`, une source qui passe à
  `transcribe_audio = true` sonde alors le sidecar et est refusée avec un message net s'il ne
  répond pas, 503 sur `PUT /sources`) ; `/api/v1/status` et `/api/v1/server` disent `off` ;
  le Docker de l'amont, qui ne compilait plus, est retiré. I (`cloud/i-dettes-securite`, six
  commits) : CORS répond aux pages du serveur lui-même et à `[server] allowed_origins`, jamais
  à toute origine ; le WebSocket applique la même règle ; **rôle `viewer`** (lecture seule,
  `role` dans un compte, absent = propriétaire) ; `[server] audio_replay = "owner" | "all"`
  (défaut : seul le propriétaire écoute par le serveur) ; purge de l'archive audio
  (`audio_archive_max_gb`, défaut 2) et des journaux bruts (`raw_log_max_days`, défaut 30) —
  **les deux à 0 dans les configurations de production et d'essai avant l'intégration**, rien
  n'est purgé chez nous. Un conflit de fusion (deux tests ajoutés à la fin de
  `sidecar_test.go`, gardés tous deux). Ici : le voyant « Transcription » dit **Off** (neutre)
  quand le serveur tourne sans, au lieu de « Down » en rouge ; binaires reconstruits avec les
  `-ldflags`. Vérifié : `go build/vet`, `go test -race` sur les seize paquets, filet `www-check`,
  instance d'essai relancée sur le code fusionné. Reste des dettes de l'étude communauté : la
  compilation Windows (fiche J) ; `allowed_origins` à renseigner seulement si un proxy réécrit
  `Host` (VPN : en principe rien à déclarer, voir `I-rapport.md`).
- 13/10 (fin d'après-midi) : **le mouvement des avions ne pulse plus (D79)**. Signalé par le
  propriétaire sur l'instance d'essai (pulsation, recul, grands sauts, glyphes mal orientés).
  Mesuré dans son onglet Chrome : 30–35 % des pas à reculons, 40 km de recul en 15 s. Bisect
  sur quatre pages et deux serveurs (jusqu'au 11/10) : identique partout ; la station confirme
  une cadence inchangée depuis le 25/09 (0,6 position/s par avion, soit la même position à la
  moitié des relevés). Correctif contribuable en un commit (70c8eff) : prédiction serveur
  ancrée sur `seen_pos`, moteur qui ne re-date plus un point répété, delta prédit sans cap.
  Après : 3,3 % de recul, 3,2 km en 15 s. Garde-fou 4 tenu (mesure avant/après sur le même
  trafic). Second chantier mesuré au passage, antérieur à la migration : la liste Trafic
  reconstruit ses lignes chaque seconde (14 000 mutations DOM en 10 s, ~100 ms de blocage par
  seconde, moteur à 21 images/s) — à traiter à l'étape 5. Dans la foulée, deux leçons de
  l'instance d'essai : (1) le propriétaire veut **toute instance d'essai sans page de
  connexion** (accès local dans `runs/son-test/users.json`, serveur lié à 127.0.0.1) ; (2) le
  serveur répondait « non modifié » d'après la **date** du fichier, et mes bascules de `www/`
  entre branches avaient laissé Chrome avec un mélange de versions (« Connection lost ») :
  l'ETag est désormais l'empreinte du contenu, sans Last-Modified (commit autonome,
  contribuable, test). Et ne jamais recopier un binaire par-dessus un fichier existant : sur
  Apple Silicon le noyau le tue au lancement (statut 137) ; supprimer puis reconstruire.
- 13/10 (soir) : **fiche G intégrée** (vague 3 cloud, branche
  `cloud/g-chiffres-avant-connexion`, six commits, 1 229 lignes dont 540 de tests) : la route
  publique `GET /api/v1/vitals` (agrégats seulement, cache de 10 s sous mutex, coupable par
  `vitals.enabled` dans les réglages du panneau, 404 coupée ; tests qui énumèrent les clés et
  refusent toute position, tout indicatif, toute fréquence nommée), le sidecar qui déclare son
  appareil (`backend` dans `/health`), la moyenne glissante du temps par transmission, la version
  et le commit du binaire par `-ldflags` (commande dans le README), le « depuis quand » de la
  piste en service (`since` dans `runway_in_use`, `null` avant le premier changement), la valeur
  et l'unité de chaque consigne en clair. Relue sur le Mac : tout vert, aucune valeur privée, lue
  en direct sur l'instance d'essai contre la vraie station (78 avions, 555 messages/s, le plus
  loin à 173 NM, deux canaux à l'antenne une fois les flux remis). Deux écarts justifiés par la
  session cloud et acceptés : `COUNT(NULLIF(callsign, ''))` (le `!= ''` comptait faux), le test
  d'indicatif à frontière de mot (« ATCO2 »). À faire ici : construire les binaires avec les
  `-ldflags` ; vérifier `backend` sur le vrai sidecar ; `runways_in_use` compte les pistes de
  l'ensemble actif (De Gaulle face à l'est = 4), à confronter à l'attente de la session design.
  **Étapes 2 et 3 de la migration versées** (`35058d0`) après le coup d'œil du propriétaire :
  familles de phase, palette d'altitude F choisie par lui sur une scène de 34 avions (du chaud au
  froid ; la rampe de gris refusée), barre du haut.
- 13/10 (soir) : **étape 1 versée dans `local`** (907b719) après relecture conforme de la session
  design : le système dans `www/` à comportement identique (copies vérifiées par le filet,
  jetons comme utilitaires Tailwind à côté des huit noms d'origine gardés, pas de surcharge
  des rayons). Mesuré : 305 signatures balise + classes, 25 propriétés calculées, 304 identiques,
  la seule différence une pastille d'état ; CSS compilé identique règle par règle. **Étape 2
  livrée sur `migration`** (6aa293d, cinq familles de phase et rampe de clarté, une source
  `www/phase/phase-families.js`, badges pleins pour T/O et T/D) : relecture design, puis soirée
  d'écoute du propriétaire avant versement.
- 13/10 (soir) : **la migration du design commence — étape 0 livrée** (plan validé par le
  propriétaire, `docs-fr/design/04-migration.md` ; référence figée `design-reference-2026-10-13` ;
  étude versée dans `local`, 1febe52 et e786aeb). Étape 0 : branche `migration` dans un arbre de
  travail à part (`~/Dev/Aero/atc-scribe-migration`) ; l'instance d'essai a sa configuration
  (`runs/son-test/config-migration.toml`, `static_dir` sur ce `www/`, lanceur
  `son-test-migration.command`), vérifiée par un fichier témoin servi depuis l'arbre et absent de
  `local` ; le filet vérifie les copies du système dans `www/` quand elles existent
  (`ds-export.mjs --check`, éprouvé : fidèle → OK, retouchée → échec), l'étape 1 les y mettra ;
  la sonde de rendu se rejoue en signet (`tools/design/probe-bookmarklet.mjs`). Rien à l'écran,
  rien en production. Qui fait quoi : le propriétaire décide et valide à l'écoute ; cette session
  implémente, publie et tient les registres ; la session design garde la référence, relit chaque
  étape sur l'instance d'essai et complète le système quand il manque quelque chose.
- 13/10 (soir) : **la bulle de dernière transmission fusionne dans l'étiquette de l'avion**
  (demande du propriétaire : la bulle du 12/10 recouvrait l'étiquette d'identification, et
  elle n'était pas dans le design). La dernière transmission devient la **troisième ligne de
  l'étiquette** (entre guillemets, 40 caractères, dix minutes), dessinée avec elle ; la bulle à
  part, son fichier, ses styles et son câblage sont retirés (garde-fou 7). Le réglage garde sa
  clé (`voiceBubbles`), relibellé « Last transmission on the label ». Le clic sur l'avion ouvre
  sa fiche, où la transmission se lit ; le saut direct vers la ligne du fil reviendra avec le
  fil unifié de la migration (D77). **Vérification « aucune fonction perdue »** par le
  propriétaire : la table de `09-ecrans.md` couvre les seize fonctions déplacées ; l'inventaire
  de la page (51 actions, 28 champs de réglage, 13 raccourcis, 43 clés) relevé contre les écrans
  trouve **huit réglages non dessinés**, à inscrire comme gardés : les six opacités de calques,
  la longueur des traînées et la limite de pistes, la bulle de dernière transmission, les trois
  réglages d'animation et le bouton WebSocket de « Performance and debug », l'exclusion des
  avions au sol des autres aérodromes ; et le choix « accès local / compte » de la page de
  connexion, qui passe au premier lancement et aux réglages (déplacé, pas perdu). **Intégré le
  soir même** par la session design : huit lignes de plus dans la table de `09-ecrans.md`, les
  curseurs d'opacité et la longueur des traînées dessinés dans `a-calques`, les réglages
  d'animation, de trace, d'accès et le bouton WebSocket dans `reglages` (System › Performance
  and debug) ; le composant d'étiquette porte la troisième ligne, aucun composant bulle. La
  table couvre maintenant chaque fonction d'aujourd'hui : c'est elle que chaque étape coche.
- 13/10 (après-midi) : **trois petits chantiers pour le panneau « Right now »** des esquisses
  design, validés par le propriétaire après l'état des lieux (ce que la page peut afficher
  aujourd'hui, vérifié ligne par ligne) : (1) le journal d'événements survit au rechargement
  (localStorage, borné, clés gardées, `recent(minutes)` pour un résumé) ; (2) **messages ADS-B
  par seconde** dans `/api/v1/status` (`messages_per_second`, compteur du récepteur sur les dix
  derniers relevés, 0 sans compteur ; remise à zéro du récepteur gérée ; test unitaire) et dans
  le voyant ADS-B du bandeau ; (3) **la raison d'un non-appariement** gardée avec la
  transmission (D72 complétée : `unmatched: …`, sans score ; le matcher rend le meilleur
  candidat refusé). Puis, les esquisses validées (D77), **les deux autres** : (4) la **séquence
  d'arrivée hors de la PAR** : le placement des avions dans les cadres de piste sort de la vue
  dans `www/par/par-arrivals.js` (fonctions pures, testées en node, 3 tests) ; le store la calcule
  toutes les 5 s pour tous les aérodromes suivis (`store.arrivals`, `store.nextTouchdown`) et la
  PAR dessine depuis le même calcul, à l'identique. Vérifié sur l'instance d'essai, PAR fermée :
  Orly 25 en service, prochain toucher à 2,9 NM dans 80 s, le suivant 9 NM derrière ; la PAR
  ouverte ensuite dessine De Gaulle à pleine largeur. (5) **Gabarits français des consignes** :
  chaque consigne en clair porte `text_fr` (serveur, un gabarit par rôle, « niveau 100 » pour un
  niveau de vol ; 9 cas testés) ; la page les assemble en une phrase (`plainFrench`) avec le nom
  de la fréquence quand c'est une fréquence écoutée, montrée dans la bulle d'aide en attendant la
  migration. Binaires reconstruits. Dans la foulée, un défaut
  de la PAR relevé par le propriétaire et diagnostiqué par la session design : les cadres
  prenaient leur largeur au premier rendu, conteneur encore caché (plancher 480 px dans une
  colonne de 949), et un onglet en arrière-plan ne se rattrapait pas ; un rendu à l'image
  suivante et un `ResizeObserver` sur le conteneur règlent les deux cas.
- 13/10 : **le produit s'appelle Clairance** (D74, choix du propriétaire sur les propositions
  de la session design). Renommage en une fois : dépôt `chatelp/clairance`, page, lanceur,
  dossier de configuration, documents ; dossier local, module Go, binaire et sessions gardent
  l'ancien nom (voir D74). La veille, **D75** : position à deux décimales, adresses génériques,
  historique réécrit et poussé de force ; l'étude communauté est livrée (recommandation :
  consultation d'avocat d'abord) ; l'étude design est livrée sauf la marque, qui démarre sur
  le nom.
- 12/10 (soir) : **le garde-fou 3 était troué, et c'est la session design qui l'a vu** en
  écrivant son plan de migration : le serveur sert `www` relatif au répertoire courant
  (`routes.go`), et les lanceurs de production et d'essai se placent tous deux à la racine du
  dépôt. **La production, quand elle tourne, servait les fichiers en cours d'édition** au
  prochain rechargement de la page ; l'instance d'essai n'isolait rien. Correctif : un réglage
  `server.static_dir` (vide : `www`, comme avant), et `runs/production.command` prend une
  **copie figée de `www/` dans `runs/production/www`** à chaque démarrage (rsync, 3,2 Mo,
  vérifiée identique), que la configuration de production désigne. L'instance d'essai continue
  de lire `www/` en direct : c'est là qu'une modification se vérifie avant d'aller en
  production, qui ne la verra qu'à son prochain lancement. Binaire `bin/co-atc` reconstruit ;
  la production ne tourne pas, le prochain lancement prend le tout. Pour la migration du design,
  la session design propose en plus de lancer l'instance d'essai depuis son arbre de travail.
- 12/10 (fin d'après-midi) : **deux retours de l'inventaire de la session design**
  (`docs-fr/design/01-inventaire.md`, branche `design`), traités dans `www/` par cette session.
  (1) Avec le fond noir, le défaut, les avions étaient peints avec la palette prévue pour les
  cartes claires (test « le nom du fond finit par dark », faux pour `none`) : croisière
  `#1D4ED8` sur noir à 3,1:1 au lieu de `#E8ECEF` à 17,7:1 ; les fonds clairs sont nommés
  (`46dc425`). (2) **51 des 123 sélecteurs de `style.css` n'avaient plus aucune occurrence** :
  l'ancien rendu des avions en DOM (marqueurs, étiquettes, traînées, cercles de proximité,
  étiquettes de pistes et de balises, bandes d'altitude par filtre CSS, double curseur de
  l'amont) et quatre `@keyframes` qu'eux seuls appelaient ; retirés après vérification
  (grep par sélecteur hors `vendor/`, puis ce qui restait : seules `ol-attribution` et `ol-zoom`,
  générées par OpenLayers), avec le survol de la vue de proximité qui cherchait un marqueur DOM
  dans une table toujours vide. `style.css` passe de 957 à 482 lignes, le CSS compilé ne change
  que d'empreinte, filet OK, page vérifiée sur l'instance d'essai. Le nom et le positionnement
  « espace aérien français » sont confiés à la session design (D74, Q53).
- 12/10 (après-midi) : **bulles de dernière transmission sur la carte** (`9809277`), demande du
  propriétaire hors plan : au-dessus de chaque avion nommé par la radio dans les dix dernières
  minutes, sa dernière transmission (coupée à 64 caractères, texte complet dans la bulle d'aide) ;
  un clic ouvre le fil de la fréquence à cette transmission. Interrupteur dans Affichage.
  Vérifié sur l'instance d'essai (bulle, suivi de l'avion animé, clic → fil ouvert et ligne
  visible). Trois sessions en parallèle depuis le 12/10 : labo (précision par indices),
  communauté (`~/Dev/Aero/atc-scribe-communaute`, cadre légal fait ; **le nom change**, décision
  de Pierre : la DSNA a un outil « Scribe » et le nom est contesté par le SNCTA), design
  (`docs-fr/design/MISSION.md`, branche `design`).
- 12/10 : **première séance du propriétaire sur la version des paliers 0 à 4** (journal
  d'événements, réglages, curseur d'altitude en QNH/FL, réécoute, fiche de vol, séquence
  d'arrivée, consignes en clair, bandeau d'état, barre radio, bibliothèques locales) :
  « une NETTE amélioration UX par rapport à co-atc de base ». Garde-fou 5 (validation à
  l'usage) tenu pour ces paliers ; aucun défaut remonté à cette séance.

- 09/10 : cadre posé ; trois études menées en parallèle (Sonnet), rapports dans
  `docs-fr/audit-2026-10-09/` ; jugement du propriétaire, propositions et **consolidation**
  inscrits. `atc_chat.enabled` passé à `false`. D19 et CLAUDE.md corrigés sur l'empreinte.
- 10/10 : chat « AI Advisory » retiré (arbitrage 1), interface en anglais (arbitrage 3),
  règle « ne rien casser » (sept garde-fous), question des sources audio instruite ;
  **alignement avec les textes fondateurs, D71 validée**, documents mis à jour ; premier
  code sous D71 : options d'entrée ffmpeg, puis retrait du lecteur SRT. Ordre tranché (0, 1,
  2 puis 3), lanceur de barre de menus au lieu de la relance automatique.
- 10/10 : **palier 0 fait, sauf 0.2** (fond de carte, étude en cours) : WebSocket, journaux,
  index, déconnexion, chacun dans son commit, vérifiés sur l'instance d'essai. `bin/co-atc`
  reconstruit ; l'ancien est gardé (`runs/co-atc.avant-2026-10-09`, binaire du 02/10) pour
  revenir en une minute. La production ne tourne pas : le prochain lancement prendra le
  nouveau, et la validation à l'usage (garde-fou 5) se fera à cette séance.
- 10/10 : **le crédit cloud du propriétaire (expire le 5/11) est mis au service du chantier.**
  Ce qui se vérifie sans la station part au cloud par fiches autonomes
  (`docs-fr/fiches-cloud/`) ; chaque branche revient sur le Mac, où elle est relue et vérifiée
  (garde-fous) avant d'être intégrée. Vague 1, en parallèle : A, retraits côté serveur (1.2,
  1.3, 1.4) ; B, retraits côté interface (1.1, étapes 0 à 5 du plan) ; C, tests de storage,
  audio et frequencies (5.4). Vague 2 après intégration : configuration réduite (5.2),
  découpage (5.3), tests de l'API. Restent sur le Mac : l'accueil (1.5), le lanceur (1.6),
  les réglages et les alertes (palier 2), tout ce qui demande la station ou le navigateur.
