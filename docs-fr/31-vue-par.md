# 31 — La vue PAR : le ciel de profil, piste par piste

> **Document historique — revue du 20/10.** Conception et suivi arrêtés le 01/10 ; le corps du
> texte n'est pas réécrit. La vue PAR est **en production**, redessinée sur le système de design à
> l'étape 9e de la migration (f34cb62) ; le suivi continue dans `32-audit-produit.md` et
> `33-pilotage.md`.

> **État au 29/09 : implémentation commencée le jour même.** Proposition validée par le
> propriétaire (D67) ; tranché le même jour : 15° côté départs (V6), altitude de transition relevée
> (V2), réglages modifiables dans l'outil, calcul dans le navigateur (V4), bascule de l'écran
> principal seul (V5). Où en est chaque étape : tableau de la dernière partie.
> Ce document est à la fois la conception et le **suivi de l'implémentation** : le tableau de la
> dernière partie se tient à jour à chaque étape, avec ce qui a été mesuré pour la valider.

## La demande

Le propriétaire, le 29/09 : *« j'aimerais bien avoir une vue "horizon" switcheable dans co-atc pour
chaque aéroport suivi individuellement. Au lieu d'une vue du dessus, ça montre les avions en live,
qui se posent, décollent… »*

Puis, sur les propositions :

- *« si ça existe vraiment c'est encore mieux, dans ce cas on peut coller aux conventions des outils
  "PAR" (tout en restant accessible comme co-atc en général) »* ;
- *« oui profil le long de piste comme proposé. Effectivement il faudrait une vue par piste »* ;
- *« ça serait bien qu'on ait plus loin que phases d'approche ou décollage. Mais c'est sûr que ça
  pose la question du rattachement à quelle piste si on n'a pas l'axe de piste »* ;
- *« toutes les pistes côte à côte (les unes en dessous des autres ?) »* ;
- *« j'aimerais bien que le ciel soit découpé avec les FL standards par ex. surtout que co-atc est
  capable de l'inférer de l'ADS-B et/ou transmissions audio pour un avion donné »* ;
- *« profil et azimut ne peuvent pas être affichés conjointement ou ça n'a pas de sens ? »* — si :
  c'est justement la convention du PAR (ci-dessous).

## La référence : l'écran du PAR

Le **PAR** (*Precision Approach Radar*, radar d'approche de précision) est un radar posé au bord de
la piste qui guide un avion à la voix jusqu'au seuil. Son écran est exactement cette vue « de
profil » :

- **en haut, l'élévation** : l'altitude de l'avion en fonction de sa distance au seuil, avec la
  **pente de descente** tracée (3° d'ordinaire, environ 300 ft par NM) et ses lignes de tolérance ;
- **en bas, l'azimut** : l'écart latéral de l'avion à l'axe de piste, en fonction de la même
  distance, avec l'axe tracé et ses tolérances ;
- **les deux bandes partagent l'axe des distances** : un avion y apparaît deux fois, à la même
  abscisse, et l'œil lit d'un coup « trop haut et à gauche ».

La couverture d'un PAR est un secteur étroit devant la piste : d'après l'annexe 10 de l'OACI, de
l'ordre de 20° en azimut, 7° en élévation et 9 NM de portée (cité de mémoire, **non vérifié dans le
texte**). C'est le point de départ ; on l'élargit pour la demande « plus loin que les phases
d'approche ».

On en garde les conventions, pas l'austérité : les étiquettes, les couleurs de phase et le clic
restent ceux de la carte de co-atc.

## Ce qui est décidé

### Une vue par piste physique, les deux sens dans le même cadre

Une piste physique (06-24 à Orly) est un cadre. La piste est dessinée à plat, à son altitude, et
son axe est prolongé **des deux côtés** : devant un seuil arrivent les avions qui s'y posent, et
derrière l'autre extrémité partent ceux qui en décollent. Arrivées et départs du même sens
d'exploitation sont donc dans le même cadre, de part et d'autre de la piste.

Le cadre s'oriente pour que **les arrivées du sens en service aillent toujours de gauche à droite**,
vers la piste, comme sur un écran PAR. Quand le sens d'exploitation s'inverse (24 → 06), le cadre
se retourne.

```
 FL150 ┤        ╲ limite 7°
       │         ✈ AFR23P FL120 ↓ vise FL80
 FL100 ┤                    ╲
       │                           ╲                                              ↗ EZY41TK
       │· · · · ·⋱· · · · · · · · · · ·╲· · · · · · · · ·  transition · · · · · ·↗· · · · · · · ·
  5000 ┤                ⋱                 ╲                                    ↗
       │                      ⋱ pente 3°    ╲                               ↗
  3000 ┤                            ⋱          ╲                         ↗
       │                                  ✈ TVF62Y╲                    ↗
  1000 ┤                                         ⋱  ╲                ↗
   290 ┼───────────────────────────────────────────────▬▬▬▬▬▬▬▬▬▬▬▬───────────────────────────────
                20 NM     15        10        5       24         06         5         10
       ───────────────────────────────────────────────────────────────────────────────────────────
       │         · AFR23P
   az. ┼──────────────────────────────────✈ TVF62Y─────▬▬▬▬▬▬▬▬▬▬▬▬──────────────↗ EZY41TK────────
       │
       ───────────────────────────────────────────────────────────────────────────────────────────
```

*(croquis, pas à l'échelle, indicatifs inventés. Orly en 24 : les arrivées viennent de la gauche
sur la pente de 3°, les départs montent à droite après l'extrémité 06. Dessous, la bande d'azimut :
AFR23P est dans le cône mais à gauche de l'axe, il est estompé en haut.)*

### Toutes les pistes empilées, la piste en service en premier

Les cadres de toutes les pistes de l'aéroport sont **les uns sous les autres**, à la même échelle
des distances, **la piste en service en tête et mise en avant** (celle que co-atc déduit déjà par
aéroport, D62). Chaque cadre se replie en une ligne, ou se masque. Un aéroport par onglet, la vue
se bascule avec la carte.

### Deux bandes par piste : élévation et azimut

La bande d'élévation porte l'essentiel ; la bande d'azimut est mince (le quart de la hauteur
environ). Un avion **hors des tolérances d'azimut** reste dans la bande d'élévation, estompé : il
est dans le cône, mais pas sur l'axe.

### Le rattachement à une piste : la géométrie seule

La question posée par le propriétaire — à quelle piste rattacher un avion qui ne suit pas un axe —
se règle par un **cône de couverture** à chaque extrémité de chaque piste, comme celui d'un PAR mais
plus large :

- **20 NM** de portée depuis le seuil ;
- **±20°** autour de l'axe prolongé ;
- **jusqu'à 7°** au-dessus de l'horizontale, soit environ 15 000 ft à 20 NM (davantage pour un
  avion qui s'éloigne de la piste : voir V6).

Un avion dans un cône apparaît dans le cadre de cette piste, **quelle que soit sa phase** : c'est
ce qui donne « plus loin que l'approche et le décollage ». Un avion dans les cônes de **deux pistes
parallèles** va à celle dont l'axe est le plus proche. Les trois valeurs sont des réglages.

**Deux garde-fous, ajoutés le 29/09 après un cas réel** (AFR1855, en finale sur la 08R de De Gaulle,
était affiché sur la 02-20 d'Orly : De Gaulle est à 13 NM au nord d'Orly, dans le prolongement de
cet axe, et ses finales tombent dans le cône nord d'Orly) :

- **un avion que co-atc rattache à un autre aéroport suivi n'est pas montré ici** : l'attribution
  de D62, celle qui sert aux phases, fait foi ;
- **un avion qui coupe l'axe à plus de 60° est de passage**, et n'est pas montré, sauf à moins de
  2 NM de la piste, là où les avions virent en finale ou après le décollage. Les finales de De
  Gaulle coupent l'axe 02-20 d'Orly à 68°. Ce garde-fou vaut aussi quand un seul aéroport est suivi.

Un vrai PAR n'aurait pas vu AFR1855 : sa couverture s'arrête vers 9 NM, et c'est l'élargissement
à 20 NM qui fait entrer ce trafic. Et un PAR montre des échos anonymes, alors que co-atc met un
indicatif sur une piste, ce qui revient à dire « cet avion est pour cette piste ».

Hors ces deux cas, ce rattachement est **indépendant de l'attribution d'aéroport de D62** (axe à 0,5 NM, 10 NM, sous
5 000 ft) : celle-ci décide des phases ; le cône ne décide que de l'affichage. Un avion peut donc
figurer dans l'onglet d'Orly et dans celui de De Gaulle s'il est dans leurs deux cônes.

### Le ciel découpé en niveaux standard

L'axe vertical est gradué **en niveaux de vol tous les 1 000 ft au-dessus du niveau de transition**
(FL60, FL70… FL150) et **en pieds QNH en dessous de l'altitude de transition** (1 000, 2 000…),
avec la couche de transition marquée entre les deux.

L'axe mesure l'**altitude QNH**, la hauteur vraie à la température près : c'est ce qui permet de
tracer une pente de 3° qui part du seuil et de comparer un avion à cette pente. Les graduations de
niveaux de vol sont placées là où ces niveaux se trouvent ce jour-là ; avec un QNH de 1 016 hPa,
FL70 est à environ 7 075 ft. L'écart est petit (environ 27 ft par hPa), mais il est juste.

### Pour chaque avion : le niveau où il est, celui qu'il vise, celui qu'on a entendu

- **Où il est** : l'altitude barométrique de l'ADS-B (`alt_baro`), corrigée du QNH sous la
  transition. C'est la position du symbole.
- **Ce qu'il vise** : l'altitude sélectionnée au pilote automatique (`nav_altitude_mcp`), en repère
  sur l'axe et en flèche depuis l'avion. Mesuré le 29/09 à 15:27 : **73 avions positionnés sur 77**
  l'émettaient.
- **Ce qu'on a entendu** : le dernier niveau ou altitude **lu par la grammaire** dans une
  transmission rattachée à cet avion. Juge J2 (doc 29) : **84 % des niveaux lus étaient justes** le
  27/09, avant la correction des valeurs impossibles du 29/09.
- **Accord** : discret (une coche) quand le niveau entendu et le niveau visé concordent ; **en
  désaccord, les deux sont montrés, sans jugement**. co-atc ne sait pas qui a raison, et la
  transcription se trompe bien plus souvent que l'équipage : un niveau lu sur six est faux (J2).

C'est la première place où co-atc confronterait une instruction entendue à ce que l'équipage a
affiché — une case « ❌ absent » du doc 26.

### Le reste, comme la carte

Étiquette d'indicatif, altitude et flèche de montée ou de descente, courte traîne, couleurs de
phase de la carte, clic qui sélectionne l'avion comme sur la carte. Au sol, seuls les avions sur la
piste même apparaissent.

**Ce que la vue n'est pas** : un outil de contrôle. Elle n'alerte sur rien, ne juge aucun écart à
la pente, et ne dit jamais qu'un avion est « trop bas ».

## Les données : d'où vient chaque chose

| Donnée | Source | Vérifié le 29/09 |
|---|---|---|
| Position des seuils | `assets/runways.csv` (OurAirports), servie par `/api/v1/station`, champ `airports[].runways` | ✅ servie, **position seule** |
| Altitude du seuil, cap vrai, seuil décalé | `assets/runways.csv` : `le_elevation_ft`, `le_heading_degT`, `le_displaced_threshold_ft` | ✅ présents pour les 7 pistes d'Orly et De Gaulle ; **pas servis** à l'interface |
| Piste en service | `RunwayInUseTracker`, un par aéroport (D62), `airports[].runway_in_use` | ✅ servie |
| Avions : position, altitude, niveau visé, QNH affiché | ADS-B par le websocket existant : `alt_baro`, `nav_altitude_mcp`, `nav_qnh`, `baro_rate` | ✅ 77 positionnés : `alt_baro` 77, `nav_altitude_mcp` 73, `nav_qnh` 73, `baro_rate` 75 |
| QNH du jour | **à trancher** (V1) : le calage affiché par les avions bas, ou le METAR | 9 avions sur 10 sous 6 000 ft émettaient `nav_qnh`, entre 1 014 et 1 016 hPa |
| Pente et hauteur au seuil de chaque piste | eAIP : AD 2.19 (alignements de descente ILS) et AD 2.14 (PAPI) ; les VAC le montrent aussi | ✅ **3° pour les 13 ILS** d'Orly (5) et De Gaulle (8), hauteur au seuil de 50 à 57 ft ; la 20 d'Orly, sans ILS, a un **PAPI à 3,4°** (eAIP en vigueur, AD 2.14) |
| Altitude des terrains | cartes VAC | ✅ 291 ft à Orly, 392 ft à De Gaulle, cohérentes avec `runways.csv` |
| Altitude de transition | cartes d'approche aux instruments (IAC) de l'eAIP : l'ENR 1.7 dit qu'elle y est publiée, TMA par TMA ; **pas sur les VAC** | ✅ **5 000 ft à Orly et à De Gaulle** (IAC ILS 06 d'Orly, IAC ILS 08L de De Gaulle, cycle AIRAC du 03/09/2026) |
| Niveau de transition | donné par le contrôle selon le QNH ; règle de l'ENR 1.7 : le plus bas niveau de vol à au moins 1 000 ft au-dessus de l'altitude de transition | calculé (voir les réglages) ; **à confronter à l'ATIS** |
| Niveau entendu | la grammaire (`phraseology`), rôles « altitude » et « niveau de vol » | ❌ **pas rattaché aux avions aujourd'hui** : les autorisations gardées par avion (`ClearanceData`) ne sont que décollage et atterrissage |

**Les cartes** : les VAC du propriétaire sont archivées dans `charts/vac/`, à la racine du dépôt mais
hors git (`.gitignore`) — LFPB et LFPG du 11 JUN 2026, **LFPO du 10 JUL 2025**, plus ancienne, et
LFPX. Les valeurs de ce document viennent de l'eAIP du cycle en vigueur (03/09/2026), qui fait foi ;
les VAC ont servi à les recouper.

Deux remarques sur les seuils :

- la position fournie est l'**extrémité physique** de la piste ; le seuil décalé, quand il existe
  (984 ft pour la 06 d'Orly, 1 427 ft pour la 25, 1 969 ft pour la 26R et la 27L de De Gaulle), est
  là où la pente de descente arrive : il faut l'appliquer ;
- les altitudes de seuil vont de 277 à 392 ft : les ignorer abaisserait toute la pente d'autant,
  soit plus d'un mille nautique sur une pente de 3°.

## Les réglages : des valeurs par défaut, modifiables dans l'outil

Demande du propriétaire, 29/09 : *« il faudrait penser à des valeurs par défaut mais paramétrables
dans l'outil »*. Chaque valeur ci-dessous a un défaut qui marche sans rien toucher, et se modifie au
panneau de réglages, comme les règles d'association (doc 30) : défaut dans `config.toml`, valeur
modifiée enregistrée dans `runtime-settings.json`, bornes vérifiées avant d'être appliquée. Trois
portées :

- **générale** : vaut pour tous les aéroports ;
- **par aéroport** : un aéroport suivi peut avoir sa propre valeur, sinon il prend la générale ;
- **par extrémité de piste** : même principe, sous l'aéroport.

| Réglage | Portée | Défaut | D'où vient le défaut |
|---|---|---|---|
| Portée du cône | générale | 20 NM | au-delà de l'approche (10 NM pour les phases, D62), en restant lisible |
| Demi-ouverture en azimut | générale | 20° | le double du PAR, pour attraper les avions en dernier virage |
| Élévation maximale, avion qui s'approche | générale | 7° | celle du PAR ; environ 15 000 ft à 20 NM |
| Élévation maximale, avion qui s'éloigne | générale | **15°** | **décidé le 29/09** (V6) : aucun des 4 départs mesurés ne la dépasse |
| Plafond absolu | générale | 15 000 ft | garde dehors les avions en croisière que 15° laisserait entrer (V6) |
| Angle de croisement maximal | générale | 60° | au-delà, l'avion coupe l'axe au lieu de le suivre ; les finales de De Gaulle coupent l'axe 02-20 d'Orly à 68° |
| Rayon sans condition d'angle | générale | 2 NM | près de la piste, les avions virent en finale ou après le décollage |
| Tolérance au bout de piste | générale | 0,5 NM | prolongée le long de sa route, celle d'une arrivée doit rejoindre l'axe avant le bout de piste, celle d'un départ en venir ; 0,3 ou 1 NM donnent le même résultat (mesuré le 29/09) |
| Hystérésis entre deux axes | générale | 0,05 NM | bien sous les 0,21 NM qui séparent les parallèles de De Gaulle ; un départ, lui, reste dans le cadre de sa piste tant qu'il y est |
| Altitude de transition | par aéroport | 5 000 ft | eAIP, Orly et De Gaulle. Elle change d'un pays à l'autre (18 000 ft en Amérique du Nord) : l'exemple de configuration le dira |
| Niveau de transition | par aéroport | automatique : le plus bas niveau à au moins 1 000 ft au-dessus de l'altitude de transition, avec le QNH du moment | règle de l'ENR 1.7 ; ou une valeur fixe, pour suivre ce qu'annonce l'ATIS |
| Source du QNH | par aéroport | automatique : les avions bas, puis le METAR, puis aucun | V1 ; ou une valeur fixe |
| Pente de descente | par extrémité de piste | 3° | eAIP : 3° pour les 13 ILS d'Orly et De Gaulle. Notre configuration mettra 3,4° pour la 20 d'Orly (PAPI) |
| Hauteur au seuil | par extrémité de piste | 50 ft | eAIP : de 50 à 57 ft sur ces 13 ILS |
| Tolérance d'élévation | générale | ±0,7° | l'ordre de la pleine échelle d'un glide ILS |
| Tolérance d'azimut | générale | ±2,5° | l'ordre de la pleine échelle d'un localizer |
| Traîne | générale | 60 s | celle d'une lecture « d'où il vient » sans encombrer |

Ce qui relève de la façon de regarder, et pas du ciel — l'onglet ouvert, les pistes repliées ou
masquées — reste dans le navigateur de chacun, comme aujourd'hui le style de carte ou l'affichage
des étiquettes (`localStorage`).

## Questions ouvertes

- **V1 — Le QNH.** Le calage affiché par les avions bas autour de l'aéroport (`nav_qnh`, médiane des
  avions sous l'altitude de transition dans les cônes) est présent, frais, et dit ce que les
  équipages ont réellement affiché. Le METAR n'est chargé que pour l'aéroport principal (D62) et
  arrive brut (`interface{}`), à décoder. **Recommandation** : l'ADS-B d'abord, le METAR à défaut,
  et sinon l'altitude pression, en le disant sur l'axe.
- **V2 — L'altitude et le niveau de transition** — *tranchée le 29/09* : 5 000 ft relevés dans
  l'eAIP pour Orly et De Gaulle ; niveau de transition calculé par la règle de l'ENR 1.7, ou fixé au
  panneau. Reste à confronter le calcul à ce qu'annonce l'ATIS.
- **V3 — Les pistes sécantes.** Orly a 02-20 en travers de 06-24 et 07-25 : un avion peut être dans
  les cônes de deux pistes non parallèles. **Proposition** : la règle de l'axe le plus proche vaut
  aussi pour elles, et un avion à égale distance de deux axes reste dans le cadre où il était, pour
  ne pas sauter d'une piste à l'autre à chaque mise à jour.
- **V4 — Où calculer la géométrie** — *tranchée le 29/09 : dans le navigateur*, comme recommandé.
  Tout ce qu'il faut y arrive déjà. Le calcul vit dans un module pur, `www/par/par-geometry.js`, testé
  avec `node --test 'www/par/*.test.js'` : ce sont les premiers tests JavaScript du dépôt.
- **V5 — Où loger la vue** — *tranchée le 29/09 par le propriétaire* : *« il faut bien intégrer à
  l'interface actuelle de CO-ATC »*, *« c'est l'écran principal uniquement (celui avec la vue radar)
  qui bascule »*, *« il faut pouvoir switcher d'aéroport en fonction de ceux qui sont suivis dans les
  settings »*. Une bascule en haut à droite de l'écran principal : **Radar**, puis un bouton par
  aéroport suivi (le principal et les « Also follow »). Les panneaux de gauche et de droite ne
  bougent pas ; la vue prend les filtres de la carte, et un clic sur un avion le sélectionne dans le
  panneau de droite.
- **V6 — Le plafond des départs** — *tranchée le 29/09 par le propriétaire : 15°*. Le cône teste l'angle sous lequel on voit l'avion **depuis le
  bout de piste**. Mesuré le 29/09 de 15:38 à 15:44 (35 relevés ADS-B, De Gaulle face à l'est, chaque
  avion rattaché à l'axe le plus proche) : **les 4 départs de De Gaulle ont tous dépassé 7°**, entre
  9,2° et 13,5°, **sur leurs 4,5 à 6,4 premiers NM**. Ils rentrent ensuite dans le cône en se
  stabilisant vers 4 000 à 5 000 ft. Avec 7°, la vue perdrait donc chaque décollage juste après la
  piste, là où on veut le voir. Avec 15°, aucun des quatre n'en sort. En revanche, deux avions en
  croisière vus au même moment, à FL180-200 et FL257 entre 13 et 20 NM, étaient sous 11,5° et 12,4° :
  **15° seul les ferait entrer**, ce que le plafond absolu d'environ 15 000 ft empêche. **Proposition**
  : 15° pour un avion qui s'éloigne de la piste, avec ce plafond absolu. Limites : 6 minutes, un seul
  aéroport, un seul sens d'exploitation ; aucun départ d'Orly n'a été capté.

- **V7 — Un seul juge de la piste d'un avion** — *tranchée le 30/09 (D69) : le serveur, avec la règle « établi sur l'axe », validée contre FlightAware* (demande du propriétaire, 29/09) : *« puisqu'on est
  capable de relier les avions aux pistes, si on valide bien le matching, ça pourrait être une info
  affichée dans la feuille détaillée d'un avion (parti ou arrivé sur telle piste) — évidemment ça
  doit être cohérent partout »*. Aujourd'hui, deux calculs distincts répondent à deux questions
  différentes :
  - **« quelles pistes l'aéroport utilise-t-il ? »** : le serveur, qui compte les approches,
    atterrissages et montées initiales de la dernière heure ; approche = à moins de 10 NM du seuil,
    0,5 NM de l'axe, sous 5 000 ft ;
  - **« qui est devant cette piste en ce moment ? »** : les cadres PAR, par la géométrie seule
    (20 NM, ±20°), y compris des avions qui passent ou ne sont pas encore alignés.

  Ils ne peuvent pas toujours concorder. **Proposition, à discuter** : le serveur devient le seul
  juge. Pour chaque avion, il fixe la piste d'arrivée une fois l'avion établi en finale, et la piste
  de départ une fois vu en montée dans l'axe, avec la même règle que la vue (l'axe le plus proche).
  Il la garde pour le reste du vol et la sert avec l'avion. La feuille détaillée l'affiche. La vue
  PAR s'en sert pour le cadre et pour distinguer un avion qui atterrit ou décolle d'un avion qui
  passe. Les pistes en service se comptent en avions et non en mises à jour ADS-B. **Validation
  avant tout affichage** : l'atterrissage donne une vérité terrain. Les derniers points en vol d'un
  avion qui se pose survolent la piste qu'il a prise ; sur une journée de la base, on compte les
  avions à qui la règle a donné cette piste-là.

## Suivi de l'implémentation

Chaque étape se valide par ce qu'elle permet de **mesurer ou vérifier**, inscrit dans le journal
ci-dessous au moment où elle est faite.

| # | Étape | Vérification | État |
|---|---|---|---|
| 1 | **Pistes complètes servies** : altitude du seuil, seuil décalé, pente, pour chaque aéroport suivi | **déjà servies par `/api/v1/airports/{code}`** (`RunwayInfo` : altitude, cap vrai, seuil décalé des deux extrémités) ; vérifié sur les 3 pistes d'Orly, rien à coder côté Go. La pente et la hauteur au seuil par piste viendront des réglages (étape 8) | ✅ |
| 2 | **Géométrie** : distance le long de l'axe, écart latéral, angle d'élévation, test du cône, piste la plus proche | 11 tests `node --test` ✅ ; **reste** : sur une heure de trajectoires de la base, les avions posés sur une piste sont sur cette piste à 5 NM du seuil dans au moins 98 % des cas, et la part des départs gardés jusqu'à 10 NM | 🟡 |
| 3 | **Cadre d'une piste, sans avion** : axe gradué en niveaux et en pieds, transition, pente et tolérances, bande d'azimut | vu en direct le 29/09 à Orly et De Gaulle (journal) | ✅ |
| 4 | **Avions en direct** : symbole, étiquette, flèche verticale, traîne, couleurs de phase, clic | vu en direct le 29/09 ; **reste** la mesure de la correction du QNH au toucher (±100 ft de l'altitude du seuil sur une journée d'atterrissages) | 🟡 |
| 5 | **Pistes empilées** : pistes en service en tête, repli, masquage, retournement quand le sens change | pistes en service en tête (plusieurs à De Gaulle) et repli vus le 29/09 ; **retournement vu en direct le 30/09 au soir**, De Gaulle passé face à l'ouest : « 08R → 26L » est devenu « 26L → 08R » | ✅ |
| 6 | **Niveau visé et niveau entendu** : `nav_altitude_mcp` affiché ; le dernier niveau lu par la grammaire rattaché à l'avion et servi | niveau visé affiché ✅ ; **reste** le niveau entendu (côté Go) et le taux d'accord entendu / visé, à comparer au J2 du doc 29 | 🟡 |
| 7 | **Un onglet par aéroport**, bascule avec la carte | Orly et De Gaulle basculés le 29/09 ; **reste** un avion dans les deux cônes vu dans les deux | 🟡 |
| 8 | **Les réglages** : défauts dans `config.toml`, section du panneau, `runtime-settings.json`, portées générale, par aéroport et par extrémité de piste | tests Go de validation des bornes et de l'héritage (piste → aéroport → général) ; une valeur changée au panneau redessine la vue sans redémarrer ; un réglage enregistré avant reste valide | 🔲 |
| 9 | **La piste de chaque avion, jugée par le serveur** (D69) : servie avec l'avion, affichée dans la feuille détaillée, utilisée par la vue PAR ; pistes en service comptées en avions | règle mesurée contre FlightAware avant d'être codée ✅ ; 8 tests Go ✅ ; vu en direct le 30/09 ✅ ; **verdicts de la nuit du 30/09 : 368 sur 368 conformes à FlightAware, arrivées et départs** ✅ | ✅ |

États : 🔲 à faire · 🟡 en cours · ✅ fait.

### Journal

**29/09 — premier incrément (étapes 1, 3 et 7, amorce de 2, 4, 5, 6).** Fichiers :
`www/par/par-geometry.js` (géométrie pure) et ses 11 tests, `www/par/par-view.js` (rendu SVG, un
cadre par piste, rafraîchi chaque seconde), la bascule dans `www/index.html`, `mainView` et
`setMainView` dans le store de `www/app.js`, les styles dans `www/style.css`. Aucune ligne de Go.

Vu en direct sur une instance de test (port 8012, sans audio ni transcription), de 16:36 à 16:40 :

- **QNH estimé par l'ADS-B : 1 015 hPa sur 8 avions à Orly, autant qu'à De Gaulle ; le METAR d'Orly
  dit Q1015.** La première source prévue (V1) tombe juste.
- **Orly, 25 en service** (le compteur de pistes de co-atc, 100 %) : son cadre passe en tête. Trois
  arrivées y descendent sur la pente de 3°, à 3 400, 2 300 et 1 200 ft. La dernière affiche déjà
  2 000 ft sélectionnés : l'altitude de remise des gaz, que les équipages affichent une fois la
  pente captée.
- **De Gaulle, 09R en service** : une arrivée à gauche, deux départs à droite qui montent vers FL100
  et FL110.
- Des avions qui quittent Orly vers le nord sont rangés dans le cadre de la 02-20 : ils sont dans
  son cône, et c'est son axe qui est le plus proche. C'est la règle voulue (géométrie seule).

Quatre corrections faites en regardant :

- le compteur de pistes nomme une extrémité « 07-25/25 » (paire, puis extrémité) : la comparaison
  à « 25 » échouait, et la piste en service n'était pas trouvée ;
- **l'altitude sélectionnée arrive par pas de 16 ou 32 ft** : 3 000 ft est émis 3 024. Arrondie à
  la centaine à l'affichage ;
- la barre d'alertes de co-atc, au-dessus de l'écran principal, masquait l'en-tête : la vue
  commence sous elle, quelle que soit sa hauteur, et défile dessous ;
- les étiquettes de niveau se chevauchaient en haut de l'axe : une étiquette trop proche de la
  précédente est omise, la ligne reste.

**Un choix à juger à l'œil par le propriétaire** : l'axe des altitudes est en racine carrée, pas
linéaire. Les derniers milles et la montée initiale y ont de la place, et les niveaux jusqu'à FL150
tiennent quand même dans 230 px. En contrepartie, la pente de 3° y est une courbe et non une droite,
ce qui s'écarte de l'écran PAR.

**29/09, fin d'après-midi — ce que le propriétaire a relevé, et ce qui en est sorti.**

- **« Runway in use 09R (100 %) » alors que deux avions sont en approche sur la 09L.** Ce n'était
  pas la vue, c'était la détection de co-atc, en amont. Parmi les pistes dont l'axe passe à moins de
  0,5 NM de l'avion, elle retenait **celle dont le seuil est le plus proche**. À De Gaulle, les
  seuils de chaque paire sont décalés : celui de la 09R est à 0,48 NM plus à l'ouest que celui de la
  09L, à 383 m de côté. Mesuré sur la géométrie réelle, sans la correction : **les finales de la
  09L comptaient pour la 09R, celles de la 08R pour la 08L, et les départs de la 09L comme de la 09R
  pour la 08R.** Face à l'est, les deux pistes d'atterrissage étaient confondues avec les pistes de
  décollage. Corrigé (D68) : l'axe le plus proche d'abord, le seuil ensuite. Quatre tests sur les
  coordonnées de De Gaulle et d'Orly, qui échouent sans la correction.
- **Les deux paires de De Gaulle, 08 et 09, n'étaient jamais actives ensemble** : co-atc reconnaît
  les pistes parallèles à leur numéro, et 08 n'est pas 09, alors qu'elles sont à 1° l'une de
  l'autre. Les approches sur la 08R étaient rejetées dès que la 09 était active. Elles sont
  maintenant reconnues par leur cap, à 5° près ; Orly, dont les 06 et 07 sont à 12°, n'est pas
  touché.
- **L'en-tête ne nommait qu'une piste** : il nomme maintenant toutes celles que co-atc compte en
  service (au moins 15 % des indices, son propre seuil), et chaque cadre concerné porte le badge.
  Relu à 17:15 sur l'instance de test, corrigée et redémarrée : **« 08R 60 % · 09L 35 % »**, les
  deux pistes d'atterrissage de De Gaulle face à l'est, et leurs deux cadres en tête.
- **Reste un écart connu** : les décollages sont mal comptés par co-atc. La 09R n'est qu'à 6 % et la
  08L n'apparaît pas, alors que la vue montre bien les départs. La détection de montée initiale
  demande un décollage vu au sol, ou un avion à moins de 5 NM du point de référence de l'aéroport,
  deux conditions que notre réception remplit rarement à De Gaulle. C'est la question V7.

Ajouts demandés le même jour :

- **L'aide au survol**, pour quelqu'un qui ne connaît pas le contrôle aérien : chaque élément de la
  vue a son explication (QNH, niveaux, pente, cônes, seuils, azimut…), sauf les avions, qui ont
  leur panneau. Les lignes fines ont une zone de survol élargie, invisible. 231 zones sur les
  quatre pistes de De Gaulle.
- **La trace de l'avion sélectionné** : toute sa trajectoire enregistrée, en blanc, dans les deux
  bandes. Vue sur AFR61AX : l'arrivée par l'est au-dessus de FL100, le vent arrière le long de
  l'aéroport, décalé sur le côté dans la bande d'azimut, le virage à 17 NM et 5 000 ft, puis la
  descente sur la pente.
- La barre d'alertes change de hauteur au gré des alertes, et **toute la vue sautait sous le
  pointeur** : la place qui lui est réservée ne fait plus que grandir.

**29/09, soir — la validation de la piste d'un avion (V7), première mesure.** Script :
`tools/runway-check/validate.py`, en lecture seule sur les bases quotidiennes. Quatre journées (23,
24, 25 et 28/09) : **1 116 approches vues vers De Gaulle, 390 vers Orly.**

- **Couverture.** On perd les avions en finale **à 3,9 NM du seuil en médiane, vers 1 250 ft**
  (quartiles 3,1-4,3 NM à De Gaulle, 2,7-4,7 NM à Orly). Le toucher n'est jamais vu, comme le
  prévoyait le propriétaire. Moins de 4 % des approches sont suivies sous 2 NM.
- **La vérité prise faute de mieux** : l'axe que l'avion suit à son dernier point, quand ce point
  est à moins de 4 NM du seuil (629 approches à De Gaulle, 194 à Orly). Elle ne laisse pas de
  doute à ce stade : l'avion est à **4 m de son axe en médiane, 15 m au 95e centile**, et l'axe
  parallèle voisin est à 276 m au moins.
- **Point par point, comme le serveur** :

  | Règle | De Gaulle | Orly |
  |---|---|---|
  | ancienne (seuil le plus proche) | **0,8 %** des points justes | 90,2 % |
  | axe le plus proche (D68) | 92,2 % | 99,5 % |
  | axe le plus proche, parallèles décalées admises jusqu'à 1 NM au-delà | **99,9 %** | 99,5 % |

  Les 7,8 % manquants de la deuxième ligne sont un artefact : sur le premier demi-mille de chaque
  finale de la 09L, son seuil est encore au-delà des 10 NM et celui de la 09R, décalé de 0,48 NM,
  ne l'est plus. C'est ce qui donnait 6 % à la 09R en direct. Corrigé par la troisième ligne
  (D68), avec son test.
- **Stabilité, pour un juge par avion** : la piste lue à 8 NM (ou à 6 NM) est celle du dernier
  point pour **628 approches sur 629 à De Gaulle et 193 sur 194 à Orly**. Les deux exceptions
  (FPO4UD, 09L puis 09R ; VLG33CT, 07 puis 06) sont à regarder une par une.

**Ce qui manque** : une vérité indépendante de notre réception, qui voie le toucher. **FlightAware
AeroAPI la publie** : champs `actual_runway_on` et `actual_runway_off`, « piste d'arrivée réelle à
destination, quand elle est connue » (spécification OpenAPI publique, lue le 29/09). Le
propriétaire alimente FlightAware : **son compte « Personal » donne 10 $ d'usage gratuit par
mois**, et la liste des arrivées d'un aéroport coûte 0,005 $ par tranche de 15 vols. Une journée
de De Gaulle (environ 700 arrivées) coûte donc 0,23 $. Flightradar24 publie aussi la piste dans
son API, mais **n'offre pas d'accès gratuit à ses contributeurs** (FAQ de l'API).

**29/09, soir — un avion pour De Gaulle affiché sur une piste d'Orly.** Relevé par le propriétaire :
AFR1855, en finale sur la 08R de De Gaulle à 1 350 ft, apparaissait dans le cadre 02-20 d'Orly,
à 15,4 NM du bout 20, 1,1 NM de l'axe, 0,7° au-dessus de l'horizon. Il était dans le cône, mais il
le traversait à 68°. Corrigé par les deux garde-fous décrits plus haut (« Le rattachement à une
piste »), avec un test ; revu en direct, Orly n'affiche plus que son trafic.

**29/09, soir — les réglages vérifiés sur 3 h 20 de trafic réel** (instance de test, 16:35-19:55,
645 000 positions ; `tools/runway-check/par-replay.js` rejoue la base dans le code même de la vue).

*Précision.*
- **Les arrivées établies passent 60 ft sous la pente de 3° en médiane** (-0,1°), aux deux
  aéroports. L'écart grandit avec la hauteur (Orly : -51 ft à 3-5 NM, -65 à 5-7, -81 à 7-10). C'est
  l'erreur des altimètres par air chaud : environ 4 % de la hauteur à 26 °C, soit 53 ft à 4 NM, et
  on mesure -51. La correction du QNH, les altitudes des seuils et les pentes sont donc justes, à
  la température près. Sans la correction du QNH, l'écart passait à -100 ft.
- **Le QNH estimé par l'ADS-B**, de 1014,4 à 1015,2 hPa sur la soirée, **concorde avec les METAR
  d'Orly** (Q1014 et Q1015).
- Les marges couvrent les approches normales : **99,8 à 100 % des points dans ±0,7°** en
  élévation, **99,3 à 100 % dans ±2,5°** en azimut.

*Deux défauts trouvés en mesurant, corrigés.*
- **L'hystérésis de 0,3 NM dépassait l'écart des parallèles de De Gaulle (0,21 NM).** Un avion qui
  rejoint la 09L par le sud croise d'abord l'axe de la 09R, prenait son cadre et y restait jusqu'à
  la piste. D'où un azimut à 44,5 % seulement dans la marge entre 2 et 4 NM ; **97,6 % après**.
  Ramenée à 0,05 NM. Un départ, lui, reste dans le cadre de sa piste tant qu'il y est : sans ça,
  un avion qui monte de la 09R et dérive de 100 m au nord passait dans le cadre de la 09L.
- **Les avions d'affaires qui décollent du Bourget** (5 NM au sud-ouest de De Gaulle, non suivi)
  longent la finale de la 08R à 1,25 NM au sud, en montée : dans le cône, jamais sur l'axe, ils
  étaient montrés comme des arrivées. Règle ajoutée : prolongée le long de sa route, celle d'une
  arrivée doit rejoindre l'axe avant le bout de piste, celle d'un départ en venir (0,5 NM de
  tolérance). Les quatre jets relevés (VLJ361Q, NJE322M, NJE5RT, IFA6555) sortent entièrement des
  cadres ; les changements de cadre à Orly passent de 61 à 40.

*Pertinence de chaque réglage*, en le faisant varier seul sur le même trafic :

| Variante | Orly : points d'arrivée montrés, dès | Orly : avions montrés | De Gaulle : arrivées, dès | De Gaulle : départs montrés |
|---|---|---|---|---|
| **réglages retenus** | 65,2 %, dès 16,7 NM | 98 | 59,3 %, dès 17,5 NM | 54,5 % |
| demi-ouverture 10° | 63,9 %, dès 16,2 NM | 96 | 58,7 % | 54,2 % (Orly : 60,7 % contre 65,1 %) |
| demi-ouverture 30° | identique | 103 | identique | identique |
| croisement 45° | 62,1 %, dès 15,9 NM | 96 | 57,8 % | 53,2 % |
| croisement 75° | 66,2 % | **205** | 59,8 % | 55,7 % |
| tolérance au bout de piste 0,3 ou 1 NM | identique | 98 à 101 | identique | identique |
| portée 15 NM | dès 14,9 NM | 95 | dès 14,9 NM | identique |

Les points d'arrivée non montrés (un tiers environ) sont les branches vent arrière et base, hors
de l'axe : c'est voulu. Ce qui est montré est à 94 % du trafic d'arrivée ou de départ de la piste,
le reste des avions attendus en approche. Les 15° des départs sont confirmés : avec les 7° d'un
PAR, on ne verrait que 3,9 % des points de départ à Orly (65 % avec 15°) et 16,8 % à De Gaulle.

**Ce qui n'est pas vérifié** : le niveau de transition calculé (FL060) contre celui de l'ATIS, que
nous n'enregistrons pas.

**30/09, matin — une nuit entière.** L'instance de test a tourné 16 h 27, **sans une erreur** dans son
journal, 360 Mo de mémoire ; la base a changé de fichier à minuit (608 Mo pour le 29, 405 Mo pour le
30 à 9:52). Rejouées, les deux journées confirment la soirée (plus de 570 arrivées) :

| | 29/09 Orly | 29/09 De Gaulle | 30/09 Orly | 30/09 De Gaulle |
|---|---|---|---|---|
| Arrivées suivies | 110 | 211 | 55 | 201 |
| Écart médian sous la pente | -59 ft | -55 ft | -46 ft | -35 ft |
| Dans ±0,7° / ±2,5° | 100 / 99,9 % | 99,8 / 99,7 % | 100 / 100 % | 99,8 / 98,8 % |
| Arrivées montrées dès | 16,2 NM | 16,7 NM | 16,9 NM | 16,9 NM |
| Départs montrés (7° à la place de 15°) | 74,9 % (9,3 %) | 52,5 % (17,6 %) | 55,2 % (1,0 %) | 51,9 % (14,6 %) |
| Trafic étranger à la piste | 0,0 % | 0,0 % | 0,0 % | 0,0 % |

**L'écart sous la pente suit la température** : les METAR d'Orly donnaient 26-27 °C le 29 au soir et
21-22 °C le 30 au matin, et l'écart passe de -55/-59 à -35/-46 ft. C'est l'erreur des altimètres
par air chaud, pas un défaut du modèle.

**Pistes en service la nuit** : De Gaulle a posé à égalité sur la 08R et la 09L (50 % chacune), puis
sur la 09R la nuit vers 23:42 ; Orly est resté sur la 25 (fermé de 23:30 à 6:00). Pas de changement
de sens : l'étape 5 reste à observer.

**Corrigé** : avec deux pistes à égalité, la « première » a changé 18 fois dans la nuit, et les
cadres, rangés par score, échangeaient leur place à l'écran. Les cadres en service viennent
maintenant en tête dans l'ordre de leurs noms.

**Les changements de cadre** (environ un par avion) sont presque tous des arrivées qui rejoignent
leur axe par le côté et traversent d'abord celui de la piste parallèle voisine : 235 entre 08L et
08R, 184 entre 09L et 09R sur les deux jours. C'est la géométrie. Le juge unique de V7, qui fixerait
la piste d'un avion une fois établi en finale, les ferait disparaître.

**30/09 — OpenSky Network comme vérité indépendante (V7) : ce qu'on a appris.** Le propriétaire a
créé un compte gratuit ; les identifiants de l'API sont dans `~/.config/clairance/opensky.json`,
hors du dépôt, et n'ont jamais été affichés. Script : `tools/runway-check/opensky_compare.py`
(cache des réponses dans `~/.cache/clairance/opensky/`, hors du dépôt : ce sont les données
d'OpenSky, seuls des décomptes sont publiés).

- **Ça marche pour un vol ancien** : la trajectoire d'un Istanbul–De Gaulle du 28/09 revient en
  590 points, **les derniers au seuil de la 09L**, bien plus bas que la station ne le suit.
- **Le coût dépend de l'âge des données** (documentation de l'API, vérifié) : 4 crédits pour moins
  de 24 h, 30 pour un ou deux jours, 60 × N au-delà. Relevé : 180 crédits pour le vol du 28/09,
  sur 4 000 par jour.
- **Mais OpenSky traite les journées avec retard** : le 30/09 à 10 h, la liste des arrivées à De
  Gaulle du 29/09 était vide, et la trajectoire d'un vol vu la veille à 15:08 UTC introuvable ;
  même une trajectoire « en direct » d'un avion en approche est revenue introuvable (le service est
  dit expérimental). En pratique, on ne peut vérifier qu'une vingtaine d'avions par jour.
- **Demande transmise à l'agent de la station** (décision du propriétaire, 30/09) : ajouter un
  conteneur d'envoi vers OpenSky, à côté de `piaware` et `fr24`, sans toucher à l'existant. Le
  quota passerait à 8 000 crédits par jour. FlightAware AeroAPI, qui donne la piste directement,
  reste la voie la plus rapide.

**30/09 — la validation indépendante (V7) : FlightAware confirme la règle.** Clé AeroAPI du
propriétaire dans `~/.config/clairance/aeroapi.key`, hors du dépôt, jamais affichée ; réponses en
cache dans `~/.cache/clairance/aeroapi/`, hors du dépôt (licence Personal : usage personnel, seuls
des décomptes sont publiés). Script : `tools/runway-check/aeroapi_compare.py`. Coût : 196 pages,
**0,98 $**. FlightAware donne `actual_runway_on`, la piste d'atterrissage réelle, pour chaque vol :
il voit le toucher, pas nous.

Six journées (23, 24, 25, 28, 29 et 30/09), approches suivies à moins de 6 NM du seuil. L'appariement
se fait par indicatif et par heure. La station entend l'**indicatif radio** (AFR16MC), et
FlightAware range le vol sous son numéro (AFR1234), avec l'indicatif radio en `atc_ident` : faute de
ce champ, 60 % des vols manquaient au premier passage. Retrouvés : **1 501 sur 1 566 à De Gaulle,
446 sur 453 à Orly.**

| | De Gaulle | Orly |
|---|---|---|
| Axe le plus proche au dernier point vu = piste réelle | **1 495 sur 1 501 (99,6 %)** | **445 sur 446 (99,8 %)** |
| Axe le plus proche dès 8 NM = piste réelle | 1 493 sur 1 501 (99,5 %) | 446 sur 446 (100 %) |
| Points de finale justes, ancienne règle (seuil le plus proche) | **3,5 %** | 95,1 % |
| Points de finale justes, règle de D68 | **99,5 %** | 100,0 % |

Les huit désaccords restants à De Gaulle, à regarder un par un :
- **Un vrai changement de piste tardif**, que FlightAware confirme : FPO4UD, sur l'axe de la 09L à
  8 NM, posé sur la 09R. Le dernier point de la station le donnait déjà sur la 09R.
- **Des désaccords qui ressemblent à des erreurs de FlightAware**, dont la piste est donnée « quand
  elle est connue » :
  - CPA261 un autre jour : co-atc dit 08L, FlightAware dit 27L, soit le sens opposé ;
  - HVN19 et SAS73C : co-atc les voit sur la 09R à 3,5 et 4,7 NM, FlightAware dit 08R, de l'autre
    paire, à 1,6 NM de là.
- **Quatre cas entre les deux pistes d'une même paire, à trancher** : AFR41WP, CPA261, MAU14 et
  AFR39PA.

À Orly, le seul désaccord (VLG33CT) donne raison à la lecture à 8 NM : 07, comme FlightAware. Le
dernier point, à 2,2 NM, disait 06 : près d'Orly, les axes de la 06 et de la 07 se rapprochent.

**Conclusion** : la règle de l'axe le plus proche est validée contre une source indépendante qui
voit le toucher, et la lecture dès 8 NM suffit pour un juge unique par avion (V7), à 99,5 % et
100 %. L'ancienne règle se trompait à De Gaulle pour 96,5 % des points.

**30/09, soir — le juge des pistes (D69), construit.**

- **Serveur** : `internal/adsb/runway_judge.go`, un fichier à part, contribuable. À chaque position
  reçue, le juge regarde si l'avion est établi sur un axe d'un aéroport suivi (185 m, 8 NM, 15°,
  sous le plafond d'approche). La piste d'arrivée est la dernière sur laquelle il s'est établi ;
  la piste de départ est jugée sur la montée initiale dans l'axe, au-delà du bout de piste.
  - Une remontée juste après une approche est une remise des gaz, pas un départ.
  - Un départ jugé plus de 20 minutes après une arrivée ouvre un nouveau vol et efface l'arrivée.
  - Le verdict est servi avec l'avion (champ `runway`), sur tous les chemins : REST, websocket et
    différences en direct.
- **Pistes en service comptées en avions** : chaque avion garde un indice par type (approche,
  atterrissage, montée), sur la dernière piste où il a été vu. Avant, un avion en longue finale
  pesait cent fois un avion vu brièvement.
- **Feuille détaillée** : une ligne « Runway », par exemple « Arriving 09L (LFPG) », « Landed on
  09L (LFPG) » ou « Departed 08L (LFPG) ».
- **Vue PAR** : un avion jugé sur une piste va dans le cadre de cette piste, même pendant qu'il
  traverse l'axe voisin pour rejoindre le sien. Son symbole est **plein** ; un avion présent dans le
  cône sans être jugé sur la piste a un symbole **creux**. Un avion jugé sur un autre aéroport n'est
  pas montré.
- **Corrigé au passage** : les phases envoyées en direct par websocket perdaient leur aéroport,
  dont la vue a besoin pour écarter le trafic des autres aéroports.
- **Tests** : 8 tests Go du juge, sur les coordonnées réelles de De Gaulle (arrivée, interception
  qui traverse la 09R, changement tardif de FPO4UD, départ, remise des gaz, rotation, avion qui
  n'est pas en finale, décompte en avions). Un test m'a d'ailleurs repris : un point à 300 m au sud
  de la 09L est à 83 m de l'axe de la 09R, et le juge l'y met à raison.
- **Reste à vérifier en direct** : une instance de test, la feuille détaillée et les symboles de la
  vue sur du trafic réel.

**30/09, 21:10-21:30 — le juge vu en direct**, sur une instance de test relancée depuis
`runs/par-test/`. Le redémarrage du Mac à 20:52 (installation de macOS 27.0.1) avait vidé `/tmp`,
et avec lui l'instance précédente et ses bases du 29 et du 30/09 ; leurs résultats sont consignés
plus haut, mais on ne peut plus les rejouer.

- **Feuille détaillée** : « Runway: Arriving 25 (LFPO) » pour TAP438T, en approche sur Orly.
- **Un départ de la 20 d'Orly** (TVF37LC), vérifié sur sa trajectoire : sur l'axe à 4-13 m, de 1,4 à
  5,3 NM au-delà du bout 02, cap 198, en montée de 2 200 à 4 700 ft.
- **Vue PAR, De Gaulle face à l'ouest** :
  - pleins : AFR99ZC « landing on 26L », AFR22CT « took off from 26R », AFR69DF « took off from
    27L » ; c'est la répartition de De Gaulle face à l'ouest, atterrissages sur les pistes
    extérieures et décollages sur les intérieures ;
  - creux : KQA114, dans le cône sans être établi.
- **En-tête** : « Runways in use 27R 50 % · 26L 50 % ».
- **Le cadre s'est retourné** avec le changement de sens (étape 5).
- **Les verdicts de la nuit** sont enregistrés par `tools/runway-check/judge_log.py` dans
  `runs/par-test/verdicts.jsonl`, pour être comparés à FlightAware au matin, départs compris
  (`actual_runway_off`).

**01/10, matin — le juge validé en direct, sur une nuit, contre FlightAware.** Verdicts
enregistrés de 21:10 à 07:29 par `judge_log.py` (445 verdicts) et comparés par
`tools/runway-check/judge_vs_aeroapi.py` à la piste réelle d'atterrissage (`actual_runway_on`)
et de décollage (`actual_runway_off`). Coût : 73 pages, 0,36 $.

| | Verdicts | Retrouvés chez FlightAware | Concordants |
|---|---|---|---|
| Arrivées De Gaulle | 170 | 162 | **162 sur 162** |
| Arrivées Orly | 53 | 52 | **52 sur 52** |
| Départs De Gaulle | 125 | 117 | **117 sur 117** |
| Départs Orly | 38 | 37 | **37 sur 37** |

**368 verdicts sur 368 justes.** C'est le code du serveur lui-même qui est vérifié ici, départs compris,
là où la mesure du 30/09 vérifiait la règle hors ligne, sur les seules arrivées. De Gaulle était face
à l'ouest toute la nuit.

**Couverture** : 222 avions sont passés en approche (phase APP) avant d'avoir un verdict. **Trois
seulement n'en ont jamais eu** (NJE089B et DAL224 à De Gaulle, TVF62SF à Orly) ; les autres ont été
jugés une fois établis sur l'axe.

**À savoir pour lancer une instance de test** : la mettre sous `runs/` (ignoré par git), jamais dans
`/tmp`, que macOS vide à chaque redémarrage. Lancé depuis la session de l'agent, le binaire
co-atc n'atteint pas la station (« no route to host »), alors que `curl` y arrive. C'est la
protection du réseau local de macOS. Lancé depuis Terminal, il y arrive.
