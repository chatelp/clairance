# Relier une transmission à un avion : les règles, leurs réglages, ce qu'elles rapportent

*Document de référence, état au 27 septembre 2026, **revu le 20/10** (les passages corrigés ou ajoutés à cette date le disent). Il couvre tout ce qui se passe **après** la
transcription : du texte à l'avion, par des règles écrites à la main, sans modèle. Le modèle de
langage et le son qui lui arrive sont dans `29-approches-reconnaissance.md`. À tenir à jour à chaque
règle ajoutée ou mesurée ; le détail de chaque mesure est dans `05-decisions.md`.*

## Comment on juge

Une transmission est **rattachée** quand co-atc lui donne l'indicatif ADS-B d'un avion. On ne sait
pas, transmission par transmission, si c'est le bon avion ; on mesure donc en bloc :
- **hasard** : on refait le même calcul en donnant à chaque transmission le ciel d'un autre moment,
  où aucun avion ne peut être celui qui parle (moyenne de cinq tirages) ;
- **avions justes** = rattachés − hasard ; **précision** = avions justes / rattachés.

L'outil est `cmd/phraseology` (`-capture` sur la capture du 24/09, `-db` sur une base de co-atc).

## 1. Les règles réglables au panneau

Page *Settings* de co-atc → section **Callsign matching** (la page compte six sections : Listening,
Callsign matching, Regional data, Display, Account, System ; revue du 20/10). Chaque changement
s'applique à la transmission suivante et s'enregistre dans `runtime-settings.json`, à côté du
fichier de configuration, sous la clé `matching`. **Trois préréglages** en tête de section
règlent les onze règles d'un coup (ci-dessous) ; les règles restent modifiables une à une, et la
page affiche « custom » quand elles ne correspondent à aucun préréglage.

| Libellé au panneau | Clé | Défaut | Ce que fait la règle | Ce qu'elle rapporte (mesuré) | Réf. |
|---|---|---|---|---|---|
| Letters in flight numbers | `letters` | activé | lit un nombre suivi de lettres épelées comme un numéro de vol : « seven uniform echo » = 7UE (59 % des indicatifs au-dessus de Paris ont des lettres) | avec la suivante, **+27 % d'avions justes**, précision 73 → 76 % | D58 |
| Airline heard roughly | `approx_operators` | activé | reconnaît une compagnie mal entendue (« welling » = Vueling), à une ou deux lettres près ou par ses consonnes, **parmi les compagnies présentes dans le ciel** seulement | compris dans les +27 % ci-dessus | D58 |
| Shortest flight number | `min_digits` | 3 | nombre de chiffres minimum pour qu'un nombre prononcé soit un numéro de vol | 2 chiffres : +28 % d'avions justes mais précision 72 → 52 % | Q30 |
| By its last letters | `context_letters` | activé | un avion entendu **sur la même fréquence dans les 2 dernières minutes** peut être rappelé par ses dernières lettres (« Sierra Bravo ») | **+8 avions justes**, sans hasard mesurable | D58 |
| By its last two digits | `context_digits` | activé | même chose avec ses deux derniers chiffres, si aucun autre avion récent ne finit ainsi | **+13 avions justes** | D58 |
| By its airline alone | `context_names` | **désactivé** | même chose avec la compagnie seule, si un seul avion récent en est | +68 au-dessus du hasard, mais l'ADS-B ne peut pas dire si c'est le bon avion : à vérifier à l'oreille | D58 |
| Only aircraft in the frequency's sector | `sectors` | activé (depuis le 26/09 au soir) | ne compare qu'aux avions que la fréquence peut avoir en ligne : dans un rayon autour de son aéroport, sous un plafond ; dans un secteur, un numéro de vol amputé de sa dernière lettre (« seven uniform » pour 7UE) est accepté | **quatre mesures** : hasard divisé par 2 à 2,5 (sauf deux approches de De Gaulle seules : −28 %), précision 76 → 84 %, 91 → 96 %, 88 → 91 %, 88 → 95 % | D59 |
| (rayon, plafond par nature) | `sector.approach`, `.departure`, `.tower`, `.ground` → `radius_nm`, `max_alt_ft` | approche et départ 60 NM / 20 000 ft ; tour 15 / 6 000 ; sol 5 / 1 500 | taille du secteur selon la nature de la fréquence ; les fréquences de contrôle n'ont pas de secteur | approche mesurée le 24/09 ; tour et sol à mesurer | D59 |
| First two digits, airline named | `prefix_digits` | **désactivé** | accepte les deux premiers chiffres d'un numéro de trois ou plus quand ils suivent immédiatement le nom de la compagnie, et qu'un seul avion de celle-ci commence ainsi (« Air France Five One » = AFR511) | capture du 24/09 : +4,8 avions justes pour +6,2 de hasard (85 → 84 %) ; nuit et matin du 27/09 : +5,2 pour +1,8 (94 % inchangée). Trop mince pour être activé par défaut | ci-dessous |
| Accept one digit off | `one_digit_off` | désactivé | accepte un numéro à un chiffre près | 81 % de bruit | doc 19 |

**Les trois préréglages** (D73, valeurs validées par le propriétaire le 12/10 ; `www/server-settings.js`,
*(ajouté le 20/10)*). Mesurés dans les conditions de production sur les journées du 03/10
(8 322 transmissions) et du 04/10 (5 512), contre cinq ciels témoins mélangés :

| Préréglage (libellé de la page) | Règles | 03/10 : appariés · justes · précision | 04/10 : appariés · justes · précision |
|---|---|---|---|
| **Mesuré** (*Measured*, **le défaut**) | les règles activées par défaut dans le tableau ci-dessus : lettres, compagnie approximative, 3 chiffres, dernières lettres, deux derniers chiffres, secteurs ; compagnie seule, deux premiers chiffres et « un chiffre près » éteints | 2 557 · 2 420 · **95 %** | 1 699 · 1 607 · **95 %** |
| **Prudent** (*Cautious*) | Mesuré + chiffres exacts exigés : **`matching.min_score` = 0,9** (« Exact flight number required ») ; un numéro entendu en partie, ou une abréviation seule, ne désigne plus d'avion | 1 866 · 1 809 · 97 % | 1 263 · 1 224 · 97 % |
| **Expérimental** (*Experimental*) | Mesuré + `context_names` + `prefix_digits` : ce que le ciel témoin ne sait pas juger (deux avions d'une même compagnie sur la fréquence peuvent être confondus) ; à juger à l'oreille | 3 207 · 3 055 · 95 % | 2 124 · 2 020 · 95 % |

`matching.min_score` est un **réglage du panneau** (0 par défaut) : il s'applique s'il dépasse le
`[post_processing] min_score` du fichier (section 2). Les tailles de secteur ne font pas partie d'un préréglage, et « un chiffre près » reste éteint dans les trois. Écartés par la mesure : un
« prudent » qui retirerait la compagnie approximative et les deux derniers chiffres (3 %
d'appariements perdus, précision 94 %, sans gain).

**Les aéroports suivis**, même page, section *Listening* (*revue du 20/10 : c'était la section
« Reference airport »*) :

| Libellé | Clé | Défaut | Rôle pour l'association | Réf. |
|---|---|---|---|---|
| Reference airport | `reference_airport` | `[station] airport_code` | aéroport principal : phases des avions (qui servent de corroboration), météo | D49 |
| Also follow | `also_airports` (3 au plus) | aucun | chaque avion est jugé contre l'aéroport dont il suit l'axe de piste, sinon le plus proche ; sa phase porte cet aéroport | D62 |

**Le secteur d'une fréquence** ne se règle pas au panneau : il vient de la source. Pour les flux
séparés de la station, `radio-ctl-sync` donne à chaque fréquence son aéroport et sa nature, d'après
`cmd/radio-ctl-sync/secteurs-frequences.csv` (copie du tableau eAIP de la station) ; pour une source
du fichier de configuration, `sector_airport` et `sector_kind` dans `[[frequencies.sources]]`.

## 2. Les réglages du fichier de configuration

Dans `config.toml`, lus au démarrage. **Deux clés ont changé de section** (revue du 20/10,
`internal/config/config.go`) : `airlines_dat_path` est dans `[reference]` et `airport_range_nm`
dans `[station]`. L'ancien emplacement est encore lu, avec un avertissement au démarrage qui
dit où écrire la clé.

| Clé | Défaut | Rôle | Réf. |
|---|---|---|---|
| `[post_processing] min_score` | 0 | plancher de score en plus de celui des règles (0,6) ; 0 = aucun | doc 19 |
| `[post_processing] min_digits` | 3 | valeur de départ de `min_digits`, remplacée par le panneau | Q30 |
| `[post_processing] fleet_last_seen_minutes` | 1 | ancienneté maximale d'une cible ADS-B pour être candidate | doc 19 |
| `[post_processing] interval_seconds`, `batch_size` | 10, 20 | rythme du traitement des transmissions | — |
| `[reference] airlines_dat_path` *(ancien : `[post_processing]`)* | `assets/airlines.dat` | table des compagnies (OpenFlights, figée vers 2014) | Q23 |
| `[station] airport_range_nm` *(ancien : `[flight_phases]`)* | 5 | « près de l'aéroport » : atterrissage à la perte du signal, aéroport d'un décollage ou d'un atterrissage | D62 |
| `[flight_phases] approach_max_distance_nm`, `approach_centerline_tolerance_nm`, `approach_heading_tolerance_deg`, `approach_max_altitude_ft` | 10, 0,5, 30, 5 000 | tolérances d'approche ; ce sont aussi celles de l'axe de piste qui attribue un avion à un aéroport | D62 |

## 3. Les règles fixes

Écrites dans le code (`internal/transcription/phraseology/`, `grammar_processor.go`) ou dans les
tables de `assets/`, sans réglage :

| Règle | Détail | Réf. |
|---|---|---|
| Barème | chiffres exacts 0,9 ; fin de numéro 0,6 ; partie vol exacte (7UE) 0,9, amputée 0,5 (0,6 dans un secteur) ; lettres +0,3 ; compagnie nommée +0,4 ; altitude qui concorde +0,4, qui contredit −0,3 ; chiffres répétés +0,15 ; rappel d'un avion récent 0,7 ; atterrissage donné à un avion en croisière −0,25 ; **autorisation de décollage donnée à un avion en croisière ou en arrivée −0,25** (ajouté le 20/10 : `phraseology/matcher.go`, « airborne, not departing »). **Seuil : 0,6** | docs 17, 19 ; D58 |
| Compagnie nommée et présente | si la transmission nomme une compagnie qui est dans le ciel, seuls ses avions sont candidats | doc 19 |
| Refus de l'ambigu | deux avions presque ex æquo : aucun rattachement | doc 19 |
| Indicatifs radio qui trompent | `assets/telephony-overrides.csv` (« France Soleil » = Transavia, « Bee Line »…) | D47 |
| Formes entendues | `assets/spoken-operators.csv` : ce que le modèle écrit vraiment (« mazda » = Malta Air, **« France » seul = Air France**) | Q23, D61 |
| **Attendre l'avion** | une transmission restée sans avion est retentée à chaque passe pendant **60 s** (`retryFor`), le temps que l'ADS-B décode un avion qui vient d'entrer en portée | D60 |
| Mémoire de la fréquence | les avions rattachés sur la fréquence dans les **2 dernières minutes** départagent deux candidats égaux, et servent aux rappels de la section 1 | D58 |
| **Preuve gardée** *(ajouté le 20/10)* | le score et les raisons de l'appariement (« digits exact + operator named + altitude agrees ») sont écrits avec la transmission, colonnes `match_score` et `match_reason`, et montrés au survol de l'indicatif ; une transmission non appariée garde sa raison, préfixée `unmatched: ` (personne dans la portée, personne de nommé, ambigu entre X et Y, meilleur candidat sous le plancher). C'est la **force des indices**, pas une probabilité. Avant le 11/10, rien n'était gardé | D72 |
| **Noms de procédures** *(ajouté le 20/10)* | ils **ne servent pas à l'association** : la règle de grammaire qui les aurait lus a été mesurée et écartée (35 appariements perdus, presque tous justes, 0 gagné), l'indice de direction aussi. Ils servent à **nommer** dans le fil les procédures entendues (« elcope seven golf » → ELCOB 7G), d'après la liste de l'AIP fabriquée autour de la station : réglages *Regional data*, « Name the procedures heard in the thread », trois niveaux de certitude mesurés sur 77 cas d'écoute (*Surer* 1,10 : 9 noms, tous justes ; *Measured* 0,95, le défaut : 18 noms, tous justes ; *More* 0,90 : 19 noms, 18 justes) | Q55 |
| Seconde lecture | quand la lecture française existe, elle est essayée aussi ; la lecture anglaise l'emporte en cas de désaccord. *Le déclenchement de la lecture française relève du modèle : doc 29.* | D24, Q29 |

## 4. Ce que ces règles ont rapporté, dans l'ordre

Sur la **capture de référence du 24/09** (quatre fréquences de Roissy, canal par canal) :

| Étape | Avions justes | Précision | Réf. |
|---|---|---|---|
| Règles d'origine (chiffres seuls) | 240 | 73 % | Q46 |
| + indicatifs à lettres, compagnie mal entendue | 306 | 76 % | D58 |
| + mémoire de 2 min (référence de la suite) | 307 | 76 % | Q46 |
| + secteur, partie vol amputée, rappel par lettres et par chiffres | 368 | 84 % | D59 |
| *même chose, ciel lu comme en direct (15 s après, au lieu de 60)* | *343* | *84 %* | D60 |
| + attendre l'avion une minute | 372 | 84 % | D60 |
| + « France » seul | **387** | **85 %** | D61 |

**En direct**, avec toutes ces règles (secteur désactivé pendant l'enregistrement, mesuré avec et
sans ensuite) :

| Séance | Transmissions | Avions justes (avec secteur) | Précision | Réf. |
|---|---|---|---|---|
| 26/09, 16:44–18:35, Orly + trois fréquences de De Gaulle | 681 | 110,6 | 96 % | D59 |
| 26/09, 20:26–22:20, deux approches de De Gaulle, couplées la plupart du temps | 795 | 130,2 | 91 % | Q48 |
| nuit du 26 au 27/09, 124,350 + 126,425 | 1 484 | 173,4 | 95 % | *Mesure de la nuit* |

## 5. Pistes

| Piste | État |
|---|---|
| **Les deux premiers chiffres, compagnie nommée** (AFR511, 27/09) | ✅ codé et mesuré le 27/09, **désactivé par défaut** (section 1). Prise n'importe où dans le texte, la règle ajoutait autant de hasard que d'avions justes (« cleared to route one two » pour AFR1241) ; collée au nom de la compagnie, elle gagne 1 à 2 % |
| Le secteur des fréquences de contrôle, appris des données (où volent les avions rattachés) | 🔲 |
| Une petite IA pour l'association, jugée avec le contrôle par ciel mélangé | 🔲 (évaluée sur le papier le 23/09) |
| Dédoublonner une transmission reçue sur deux fréquences couplées | 🔲 constaté le 26/09 (Q48) ; seulement pour les comptes, pas pour l'affichage |
