# 26 — Ce que co-atc interprète et affiche, par la voix et par l'ADS-B

> **Document historique — revue du 20/10.** Il décrit l'état du 23/09 ; le corps du texte n'est
> pas réécrit. L'interface a été reconstruite sur le système de design du 13 au 15/10 (migration,
> étapes 0 à 13), et les §5 et suivants, qui décrivent l'interface de l'amont, ne la décrivent
> plus. L'état courant : `33-pilotage.md` et le README. Ce qui a changé, vérifié dans le code :
>
> - **un journal d'événements** (`www/events.js`) remplace la barre d'alertes de l'amont :
>   squawks d'urgence (7500, 7600, 7700), remises des gaz, changements de piste en service ;
> - **plus de chat IA ni d'avions simulés** (retirés, D71) ; l'accès local est enregistré et se
>   change dans les réglages (D51) ;
> - **un volume par fréquence**, la relecture de l'audio archivé de chaque transmission
>   (`store.playClip`) ;
> - **la fiche d'un avion** en onglets Story / Details / Track / Nearby, et la colonne
>   « Right now » / « Traffic » ; **un seul fil radio** pour toutes les fréquences (D77) ;
> - **la vue PAR**, redessinée sur le système de design (étape 9e) ; **les noms officiels de
>   l'AIP** (procédures entendues, Q55) et **les routes publiées** (Q56) ; **la piste et la sortie
>   probables** sur l'étiquette de l'avion (Q57) ;
> - **la preuve de l'appariement** gardée avec la transmission et montrée au survol (D72) ;
> - **les réglages** réunis dans une page à six sections (Listening, Callsign matching, Regional
>   data, Display, Account, System), avec trois préréglages d'appariement (D73) : voir doc 30.

*État de l'implémentation au 23 septembre 2026, branche `local`. Mis à jour le même jour
après D49 : les phases sont désormais jugées par rapport à un aéroport de référence
réglable.*

Document de référence. Il répond à une question simple : **de ce que co-atc reçoit — le son
de la radio et les messages ADS-B — qu'est-ce qu'il comprend, et qu'est-ce qu'il montre ?**

**Chaque ligne a été vérifiée dans le code ou dans le journal d'exécution**, pas déduite de
la documentation de l'amont ni de la configuration seule. Le fichier source est indiqué
pour qu'on puisse revérifier. Si une ligne devient fausse, c'est ce document qui est
périmé : le corriger dans le même geste (règle de `CLAUDE.md`).

Légende :

- ✅ **actif et utile ici** ;
- ⚠️ **actif, mais limité sur cette installation** — la raison est donnée ;
- ❌ **présent dans le code, inopérant ici**, ou absent.

---

## 1. Ce que l'ADS-B apporte, brut

Reçu de `readsb` via `tar1090` (`adsb.source_type = "tar1090"`), un message par avion et
par seconde environ. Champs lus par co-atc (`internal/adsb/models.go`, type `ADSBTarget`) :

| Famille | Champs | |
|---|---|---|
| Identité | adresse OACI (hex), indicatif, catégorie | ✅ |
| Position | latitude, longitude, distance et relèvement au récepteur | ✅ |
| Altitude | barométrique, géométrique, au sol ou non | ✅ |
| Vitesse | sol, indiquée, vraie, Mach | ✅ |
| Direction | route, cap magnétique, cap vrai, taux de virage, inclinaison | ✅ |
| Vertical | taux barométrique, taux géométrique | ✅ |
| **Intentions de l'équipage** | **altitude affichée au pilote automatique** (MCP) et au FMS, **cap sélectionné**, **calage altimétrique réglé à bord** (QNH) | ✅ quand l'avion les émet |
| Environnement mesuré par l'avion | vent (direction, force), température extérieure et totale | ✅ quand l'avion les émet |
| Transpondeur | code (squawk), alerte, identification (SPI) | ✅ reçu — mais voir §5, rien ne l'exploite |
| Qualité | NIC, NACp, NACv, SIL, RSSI, multilatération, TIS-B | ✅ |

> **Le point à retenir** : les champs d'*intentions* portent exactement ce que le contrôleur
> ordonne — un niveau, un cap, un calage. Ils sont émis surtout par les avions de ligne
> (Mode S amélioré, ADS-B v2), c'est-à-dire par le trafic qui intéresse le propriétaire.
> **Aucune fonction ne les confronte à la voix aujourd'hui** (voir §4).

**Ce qui est gardé en historique.** L'historique permanent de la station
(`/opt/adsb/globe_history`, décodé par `whisper-lab/scripts/heatmap.py`) ne conserve que position,
altitude, vitesse sol et indicatif. La base SQLite de co-atc ne garde l'enregistrement brut
complet que pour le **dernier état** de chaque avion (D32). **Les intentions d'équipage ne
sont donc pas rejouables après coup** : toute vérification qui s'en sert doit se faire en
direct.

---

## 2. Ce que co-atc calcule à partir de l'ADS-B

### Enrichissement

| | Source | |
|---|---|---|
| Nom de la compagnie | `assets/airlines.dat` (OpenFlights, 2014) + corrections `telephony-overrides.csv` (D47) | ✅ |
| Immatriculation, type, constructeur, propriétaire | `assets/aircraft.csv` (tar1090-db), par adresse OACI | ✅ |
| Base BaseStation (`.sqb`) | aucun fichier présent | ❌ |

### Géométrie et cinématique — `atc_derived.go`, `service.go`

| | |
|---|---|
| Distance à la station (NM) | ✅ |
| Vent de face ou arrière, vent de travers | ✅ |
| Pente de trajectoire, gradient de montée (ft/NM) | ✅ |
| Taux de virage | ✅ |
| Heure estimée de passage à la station | ✅ |
| Distance, relèvement et écart d'altitude **par rapport à un avion choisi** (vue *Proximité*, 5 NM par défaut) | ✅ |

### Trajectoires — `trajectory_prediction.go`

| | |
|---|---|
| Traînée (positions passées) | ✅ |
| **Prévision à 2 minutes**, modèle cinématique avec accélération et taux de virage | ✅ |
| **Rétro-prévision à 60 s** : où était l'avion avant qu'on le reçoive (régression, R² ≥ 0,80) | ✅ |

### Phases de vol — `trajectory_phase.go`

Dix phases : **NEW** (vu à l'arrêt), **TAX** (roulage), **T/O** (décollage), **CLB** (montée
initiale depuis l'aéroport), **DEP** (départ, s'éloigne de l'aéroport), **CRZ**
(croisière, FL180 et plus), **ARR** (arrivée, se rapproche de l'aéroport), **APP** (finale,
aligné sur une piste de l'aéroport), **T/D** (toucher), **UNK** (indéterminé). Changements
historisés, dates de décollage et d'atterrissage déduites.

✅ **Toutes ces phases sont jugées par rapport à l'aéroport de référence** (D49,
`adsb.PhaseReference`), **réglable dans les paramètres** de l'application — *Server →
Reference airport* —, qui propose les aéroports à moins de 50 NM dont les pistes sont
utilisables, triés par distance. Démarre sur `station.airport_code` ; le choix fait dans le
panneau est gardé dans `configs/runtime-settings.json` et survit au redémarrage.

L'amont jugeait tout depuis **la position du récepteur**, ce qui n'est juste que si le
récepteur est posé sur son aéroport. Avant D49, avec Saint-Cyr : aucune approche ni aucun
départ d'Orly ou de De Gaulle n'était jamais reconnu.

⚠️ **Limites mesurées, avec Orly comme référence** (récepteur à 14 NM, 3 jours d'historique) :

- **APP et CLB sont détectables** : 375 avions reçus entre 500 et 1 000 ft à moins de 5 NM
  d'Orly, plusieurs centaines plus haut. Observé en direct le 23/09 : 4 approches en
  9 minutes (TAP432, RAM642J suivi de 3 300 à 2 600 ft) ;
- **T/O et T/D ne le sont pas** : ils se jugent sur le passage sol/air, et **aucun avion
  n'est reçu sous 500 ft** à moins de 5 NM d'Orly ;
- **Le Bourget comme référence confond** probablement ses approches avec celles de De
  Gaulle, dont les pistes sont presque parallèles et à 4 NM (un Lufthansa en approche vu
  pendant l'essai).

### Piste en service — `runway_tracker.go`

Probabilité de chaque extrémité de piste d'après les approches, toucher et montées
observés, fenêtre glissante, pistes parallèles gérées. ✅ Pour l'aéroport de référence ;
les indices sont **oubliés quand on change d'aéroport** (ils portaient sur d'autres pistes).
Sert à écarter les fausses approches sur les pistes sécantes.

---

## 3. Ce que co-atc tire de la voix

### Transcription — `sidecar/whisper_server.py`, `internal/transcription/local.go`

| | |
|---|---|
| Détection de voix avant transcription (Silero, 0,25 s de parole minimum) — écarte les ouvertures de squelch vides | ✅ |
| Modèle anglais `whisper-large-v3-atco2-asr` (MLX, local, aucun appel externe) | ✅ |
| **Second avis français** (`bofenghuang/whisper-large-v3-french`), déclenché quand la sortie anglaise « a l'air française » — environ 1 transmission sur 8 (D31) | ✅ |
| Langue retenue par transmission (`en`, `fr`, `en>fr`), stockée en base | ✅ stockée — **jamais affichée** (§5) |

### Grammaire — `internal/transcription/phraseology/parse.go`

Remplace le post-traitement GPT-4o de l'amont, sans appel externe. Pour chaque transmission :

| Ce qui est extrait | Exemple | |
|---|---|---|
| **Qui parle** : contrôleur ou pilote | tempo des verbes : *descend* / *descending* | ✅ |
| **Niveau de vol**, altitude | « flight level one two zero » → FL120 | ✅ |
| **Cap** | « turn left heading two seven zero » → 270 | ✅ |
| **Vitesse** | | ✅ |
| **Fréquence** | « contact one two four decimal three five » → 124.35 | ✅ |
| **Piste** | → 27L | ✅ |
| **Calage** (QNH) | | ✅ |
| **Code transpondeur** | | ✅ |
| Lettres phonétiques (immatriculations d'aviation légère) | « fox kilo papa » → FKP | ✅ |
| **Autorisations** : décollage, atterrissage, approche, avec la piste | « cleared to land runway two seven right » | ✅ |

Anglais complet ; **français partiel** — les verbes d'instruction usuels sont reconnus
(*descendez, montez, maintenez, contactez, affichez*, et les réponses *on descend, on
monte*…).

### Rattachement à un avion — `phraseology/matcher.go`

Chaque transmission est rapprochée d'un avion vu en ADS-B au même moment, par :

- les **chiffres** de l'indicatif ;
- les **lettres** pour l'aviation légère ;
- le **nom d'opérateur** : s'il est prononcé et que l'opérateur est dans le ciel, **seuls
  ses avions sont candidats** — c'est ce qui empêche un « Ryanair 174 » de partir sur
  AFR174 (D47) ;
- la **concordance d'altitude** : niveau prononcé contre altitude barométrique mesurée.

Les cas ambigus sont signalés comme tels, pas tranchés au hasard.

### Fiabilité mesurée

| | Mesure | Source |
|---|---|---|
| Anglais, modèle anglais | retrouve **7,5 mots dits sur 10** ; **4,8 mots écrits sur 10** sont justes | D44, D45 |
| Français **d'aérodrome** | 5 sur 10 retrouvés ; 3,6 sur 10 justes | D45 — **pas la cible**, voir D48 |
| Français **des avions de ligne** (la cible) | **pas encore mesuré** sur vérité terrain ; +11,7 % d'indicatifs appariés grâce au second avis | D31, D48 |
| Clips qu'aucun humain ne comprend | **8 à 10 mots inventés** par clip ; aucun indice de confiance du modèle ne le signale | D45 |
| Transmission → avion identifié | environ **1 sur 10**, dont 8 sur 10 justes | Q30 |

Le défaut dominant est l'**invention** (insertions), pas la mauvaise écoute (D44).

---

## 4. Voix et ADS-B ensemble

| | |
|---|---|
| **Avion « entendu »** : nombre de transmissions qui l'ont nommé, heure de la dernière, dernier texte (`internal/adsb/voice.go`) | ✅ |
| **Autorisations rattachées à l'avion** (type, piste, texte, heure) | ✅ |
| **Statut des autorisations** : *émise*, *respectée*, *écart* | ❌ — voir ci-dessous |
| Concordance d'altitude voix / ADS-B, utilisée pour **choisir l'avion** | ✅ |
| Confrontation des **instructions** (niveau, cap, calage) aux **intentions affichées par l'équipage** | ❌ absent |

> ⚠️ **Le piège du modèle de données.** Chaque autorisation porte un champ `status` prévu
> pour *issued / complied / deviation*, et le stockage a une fonction pour le modifier
> (`UpdateClearanceStatus`, `internal/storage/sqlite/clearances.go`) — commentée « for Phase
> 2 compliance monitoring ». **Elle n'est appelée nulle part.** Toute autorisation reste
> « émise » pour toujours. L'interface affiche la colonne *Status* ; elle ne dit jamais autre
> chose.

---

## 5. Ce que l'interface montre

### Carte — `www/map/openlayers-map-manager.js`

| | |
|---|---|
| Avions (en vol et au sol, masquables séparément), étiquettes | ✅ |
| Traînées, prévisions | ✅ |
| Anneaux de distance autour de la station | ✅ |
| Aéroports, héliports, balises, toutes les pistes (`assets/*.csv`, rayon réglable) | ✅ |
| Limites d'espace aérien — tuiles OpenAIP via ADS-B Exchange, couverture mondiale | ✅ |
| Radar météo NEXRAD, radar et infrarouge NOAA | ❌ **services américains**, sans couverture utile sur la France |

### Liste des avions suivis

Tri par colonne ; filtres par statut, **par phase**, par altitude min/max, par ancienneté ;
recherche ; et, ajouté par ce fork, **« seulement les avions nommés à la radio »**, avec leur
nombre. ✅

### Fiche d'un avion

| Onglet ou section | Contenu | |
|---|---|---|
| **DETAILS** | altitude, cap (avec sa source), vitesses vraie et sol, taux vertical, phase courante, enrichissement | ✅ |
| **TRACKS** | positions passées et prévues : heure, altitude, cap, vitesses, distance | ✅ |
| **PROXIMITY** | avions voisins, distances et écarts relatifs | ✅ |
| Phases | historique des changements de phase | ✅ par rapport à l'aéroport de référence (§2) |
| Autorisations | type, piste, texte entendu, statut, âge | ✅ — statut toujours « émise » (§4) |
| **Radio** | transmissions rattachées à l'avion : heure, qui parle, **valeurs extraites en pastilles** (niveau, cap…), texte | ✅ |

### Fréquences et transcriptions

| | |
|---|---|
| Écoute de chaque fréquence en direct, vumètre | ✅ — **aucun réglage de volume** dans l'interface (le client audio en a le mécanisme, rien ne l'alimente ; constaté le 21/09) |
| Libellé de la fréquence, ou « mixed » pour un flux mélangé (D36) | ✅ — modifiable **par l'API** (`PUT /frequencies/{id}/label`), pas depuis l'interface |
| Transcriptions par fréquence : heure, contrôleur ou pilote, indicatif, texte, recherche | ✅ |
| **Langue détectée, second avis français** | ❌ **stockés en base, affichés nulle part** |

### Alertes

Un bandeau à chaque **autorisation entendue** — *AFR123 → LANDING CLEARANCE* —, coloré par
type (décollage, atterrissage, approche). ✅ C'est le **seul** déclencheur d'alerte
(`showClearanceAlert`, unique appel de `addAlert` dans `www/app.js`).

**Aucune alerte sur événement ADS-B** : ni décollage, ni atterrissage, ni **code d'urgence**
— 7500, 7600, 7700 n'apparaissent nulle part dans l'interface.

### Météo — `GET /wx`

✅ **Suit l'aéroport de référence.** Avec Saint-Cyr (qui ne publie ni METAR ni TAF), seuls
les NOTAM arrivaient — 158 réponses vides dans le journal du 21/09. Avec Orly : METAR, TAF
et NOTAM, vérifié le 23/09. Le cache est vidé au changement d'aéroport, et un résultat
arrivé pour l'ancien est jeté plutôt que rangé sous le nouveau.

⚠️ **L'API par défaut de l'amont est celle, privée, de Windy** (`node.windy.com/airports`),
qui répond elle-même : *« Do not steal this API. Download your own METARs for free from
aviationweather.gov »*. À remplacer dans un outil public.

### Le reste

| | |
|---|---|
| Premier lancement : choix « local » ou « accès externe avec compte » (fork) | ⚠️ **le choix « local » n'est pas enregistré** : l'écran revient à chaque visite (trouvé le 23/09) |
| **Aéroport de référence**, dans *Server* (fork, D49) | ✅ |
| Réglages serveur : niveau et rotation des journaux, rétention des bases (fork) | ✅ |
| Avions simulés pilotables (amont) | ✅ |
| **Conversation vocale avec un « contrôleur IA »** (amont, OpenAI Realtime) | ❌ activée dans la configuration, **clé vide** — inopérante, et hors de l'esprit « tout local » du fork |

---

## 6. Ce que l'inventaire met en évidence

Pas un plan : ce que ce relevé fait apparaître, pour la suite.

1. **La vérification des instructions par l'ADS-B est à moitié construite.** Le statut
   *respectée / écart* existe dans les données, et l'ADS-B transmet en direct l'altitude,
   le cap et le calage que l'équipage vient d'afficher. Relier les deux donnerait, pour la
   première fois, **une mesure de la reconnaissance vocale sans annotation humaine** — et
   sur le trafic visé, les avions de ligne étant tous équipés. Limite : en direct
   seulement, l'historique ne garde pas ces champs (§1).
2. ~~**Les phases et la piste en service sont calées sur Saint-Cyr**~~ — **corrigé par
   D49** : l'aéroport de référence est réglable. Reste qu'il n'y en a **qu'un** à la fois :
   Orly *ou* De Gaulle, pas les deux.
3. **Le travail bilingue est invisible.** Langue et second avis français sont en base ;
   les afficher est le plus simple des trois.
4. **Des réglages simples**, sans code : ~~faire pointer la météo sur LFPO ou LFPG~~ (fait
   avec l'aéroport de référence, D49) ; désactiver le chat IA dont la clé est vide.
5. **Aucune alerte d'urgence.** Le code transpondeur est reçu (§1) ; rien ne le surveille.
