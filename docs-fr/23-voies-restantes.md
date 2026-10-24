# 23 — Où en est la chaîne, et ce qu'il reste comme voies

> **Document historique — revue du 20/10.** Bilan du 16/09 ; le corps du texte n'est pas réécrit.
> Les quatre points de « Ce qu'il reste » (partie B) sont réglés : le test mélange contre canal par
> canal (Q28, puis le flux par fréquence en production, D64 : ×4 d'avions justes) ; le détecteur
> d'erreur par les valeurs lues (juge J2, doc 29) ; la base qui ne tournait jamais (Q26, corrigée
> par D22) ; les annotations du propriétaire (D44, vérité terrain). L'état courant est dans
> `33-pilotage.md`.

*16 septembre 2026, 13 h. Bilan de deux jours de mesures, puis les voies restantes pour
la reconnaissance vocale anglaise et l'enrichissement de co-atc — hors entraînement
d'un modèle français, exclu de la question.*

Ce document est en deux parties. **La première est technique et exhaustive** : l'état
mesuré de chaque étage, tout ce qui a été testé et éliminé avec les chiffres, ce qui
reste ouvert, et les voies proposées par un panel indépendant qui a lu le dossier et
le code puis tenté de réfuter chaque proposition. **La seconde, à la fin, est la
version courte** pour décider.

---

# Partie A — l'analyse détaillée

## A.1 La chaîne telle qu'elle tourne à 13 h

```
aero.mp3 (flux mixé, 4 à 7 canaux)
  → segmenteur énergie     silence_threshold 0,005 · 600 ms de silence termine · 400 ms min · préroll 200 ms
  → sidecar mlx-whisper    VAD Silero 0,5 / 250 ms / 300 ms · modèle EN = large-v3-atco2 · FR = bofenghuang (branché, non utilisé : language="en")
  → SQLite                 transcriptions(content, speaker_type, callsign) · clearances · phraseology_values
  → grammaire              parse.go : rôles, lettres, locuteur, clearances, normalisation
  → appariement ADS-B      3 chiffres min · fenêtre 60 s · ambigus refusés · pas de règle « à un près » · contexte 120 s (égalités seulement)
  → co-atc                 filtre « Heard on the radio » · panneau Radio par avion · valeurs en pastilles · texte des clearances
```

Tout tourne en local, sans clé, sans qu'un octet d'audio ni de texte quitte le Mac.
Le groupe écouté est choisi sur la station (`/opt/adsb/aero-mode`), aujourd'hui
`en-route-et-descente-cdg` : les mêmes sept canaux, avec sortie fichier par transmission.
Les deux gabarits de la nuit du 15 au 16 (`…-nuit-continu` et `…-nuit-reference`) ont été
archivés le 16/09, l'identification étant faite.

## A.2 Ce que chaque étage vaut, mesuré

### Transcription

| grandeur | valeur | source |
|---|---|---|
| modèle anglais retenu | `sfabriece/whisper-large-v3-atco2-asr-mlx` | D12 — 94,0 appariements vrais contre 62,6 pour `jacktol medium.en`, sur les cinq fréquences sans exception |
| temps de calcul par transmission | 0,72 s médian (captation), 0,80–0,90 s observé en direct | doc 18, journal du 16/09 |
| première transcription après démarrage | 8,4 s (chargement du modèle) | journal du 16/09 |
| ouvertures de squelch sans parole, la nuit | **88,1 %** en moyenne, de 7,7 % à 99,1 % selon le canal | doc 21, 4 126 ouvertures sur 28 h |
| VAD contre modèle, VAD désactivé | 30/30 vides sur la classe « sans parole », 29/30 avec texte sur la classe « avec » | doc 21 |
| parasite secteur, peigne à 100 Hz | **exactement quatre canaux** : 132,733 · 132,825 · 133,000 · 133,250 | doc 21, spectre |
| silence numérique dans le flux mixé | 87,3 % des fenêtres de 20 ms à exactement zéro | doc 19 |
| dégénérescence en direct (groupe tours) | 2,2 % à 12 % selon l'échantillon, sur audio français passé au modèle anglais | doc 19 |

### Grammaire

| grandeur | valeur | source |
|---|---|---|
| transmissions livrant au moins une valeur structurée | **66 %** | doc 19 |
| transmissions portant une clearance | 4 % | doc 19 |
| clearances détectées → stockées, première heure | 6 → **1** ; les 5 perdues faute d'avion identifiable | doc 19 |
| locuteur attribué | 54 % (captation), 46–47 % (direct) — avant : 10 % | doc 19, D15 |
| justesse du locuteur | **non établie** ; contrôle d'alternance à 61 % / 59 % contre 50 %, sur 35 paires | Q20 |
| valeurs extraites sur la matinée (rattrapage) | FL 27 · fréquence 26 · altitude 15 · vitesse 15 · piste 12 · cap 9 · QNH 6 · transpondeur 1 | doc 19 |
| transmissions **sans aucun indicatif à trois chiffres** | **668 / 1 415 = 47 %** | doc 22 |

### Appariement

| grandeur | valeur | source |
|---|---|---|
| captation du 15/09, `atc-multi`, réglages de production | 144 appariés · 40,8 de hasard · **72 % au-dessus** · **103,2 vrais** | doc 19 (après retrait de la règle « à un près ») |
| groupe tours, 51 min | 19 acceptés (1 ambigu refusé) · 1,6 hasard · **91 %** · 17,4 vrais · 76 avions en fenêtre | doc 19, remesuré sous la règle de production |
| groupe gros-porteurs, 51 min | 40 (2 refusés) · 8,8 · **78 %** · 31,3 vrais · 93 avions | doc 19 |
| amas 132-133, 51 min *(10:48, fenêtre interrompue par 4 redémarrages)* | 41 (3 refusés) · 3,8 · **91 %** · 37,3 vrais · 95 avions | doc 19 |
| amas 132-133, 51 min *(12:34, fenêtre propre : un seul démarrage, zéro erreur)* | 36 (0 refusé) · 5,5 · **85 %** · 30,5 vrais · 100 avions · **214 transmissions, 4,2/min** | doc 19 |
| résolution de l'instrument (8 témoins) | écart-type du hasard ≈ 2–3 appariements par fenêtre, soit **± 5 à 7 points** de précision | doc 19 |

> **Ce que la fenêtre propre change.** Le débit de l'amas était bien sous-estimé
> (175 → 214 transmissions), mais les appariements vrais tombent de 40,5 à 30,9 : ce
> n'était pas seulement l'interruption, c'était aussi l'heure. **Une fenêtre de 51 min
> ne suffit pas à classer finement deux groupes** — l'écart entre deux fenêtres du même
> groupe (40,5 / 30,9) est du même ordre que l'écart entre groupes. Ce qui tient :
> les tours rapportent moitié moins que les deux autres ; gros-porteurs et amas se
> valent en volume ; l'amas est nettement plus précis (90–93 % contre 80 %).
| indicatifs faits de lettres seules | inappariables **sans corroboration** : +0,30 < plancher 0,60 ; avec l'opérateur nommé (+0,40) ou l'altitude (+0,40) ils passent à 0,70 — et c'est ainsi que 5 indicatifs à queue de lettres ont été appariés le 16/09 | doc 19, corrigé par le panel (doc 23, A.5) |
| opérateurs absents du lexique (`airlines.dat`, ~2014) | 23 / 77 = **30 %** | Q23 |
| effet du complément d'opérateurs relevé sur la station | **nul** sur les appariements (20/42/44 identiques) ; hasard 3,6 → 5,4 sur l'amas | Q23, Q25 |

### Écran

| ce que co-atc montre de la voix | depuis |
|---|---|
| texte brut et normalisé, locuteur en couleur, indicatif — liste par fréquence | amont |
| clearances par avion (heure, type, piste, statut, âge) | amont ; **texte** ajouté le 16/09 |
| filtre « Heard on the radio », 375 avions → 20 | 16/09, D16 |
| panneau Radio : transmissions rattachées à l'avion, valeurs en pastilles | 16/09 |
| champ `voice` dans l'API (transmissions, dernière heure, dernier texte) | 16/09 |

## A.3 Tout ce qui a été testé et éliminé

À ne pas re-proposer sans mesure nouvelle. Chaque ligne a coûté une mesure ; c'est
leur valeur.

| piste | mesure | résultat | référence |
|---|---|---|---|
| `jacktol medium.en` comme modèle anglais | ADS-B, 5 fréquences, témoin mélangé | 62,6 vrais contre 94,0 pour atco2 ; au hasard (−12 %) sur 124,625 | D12, doc 18 |
| amorce de transcription (`initial_prompt`) | 25 clips × 2 fréquences × 3 configurations | dégénérescence **×7**, fuite de l'amorce dans le texte | doc 10 |
| concaténer les transmissions consécutives avant transcription | protocole doc 12 | **nul, voire négatif** — piste fermée | doc 12 |
| détection automatique de la langue | 16 % de français et 28 % d'anglais mal classés | routage par le catalogue, pas par le modèle | D-langue, doc 13 |
| seuil de score supplémentaire 0,9 | balayage | −30 % de vrais (104,7 → 73,4) pour +2 points | doc 19 |
| seuil 1,0 (corroboration exigée) | balayage | 88 % de précision mais **26 appariements et 3 fréquences sur 5 éteintes** | doc 19 |
| quatre chiffres minimum | balayage | 41,1 vrais contre 94,7 pour +2 points | doc 19 |
| fenêtre ADS-B 30 s / 120 s | balayage | 91,1 / 91,9 vrais contre 94,7 à 60 s | doc 19 |
| accepter les appariements ambigus | avec / sans | refuser ne coûte rien et gagne en précision | doc 19 |
| règle « chiffres à un près » | avec / sans, 8 témoins | 8 appariements dont 1,5 vrai : **81 % de bruit** | doc 19 |
| correction lexicale sans IA (dictionnaire fermé) | plafond puis effet | `that → thai` ; avec garde-fou de langue : **+1 transmission, −4 candidats** | doc 22 |
| correction lexicale par LLM local | non testée, mais même propriété du problème | les erreurs qui coûtent sont dans les **chiffres**, intouchables sans connaître la réponse | doc 22 |
| contexte : répétition interne | plafond | 15 / 1 415 = 1,1 % ; règle en place, effet nul | doc 22 |
| contexte : valeur partagée entre voisines | plafond | **0** sur 668 transmissions sans indicatif | doc 22 |
| contexte : propagation par proximité | — | **non mesurable** avec le témoin mélangé ; refusée pour cette raison | doc 22 |
| complément de lexique d'opérateurs | avec / sans, 3 groupes | 0 appariement de plus ; hasard en hausse | Q23, Q25 |
| sortie fichier par transmission sur la station | relevés salves.py | fragmente le direct (médiane 0,1 s au lieu de 4,6 s) ; sortie continue sans effet mesuré de jour | docs 15, 20 |

## A.4 Ce qui est ouvert et déjà nommé

- **Q1** — la branche française (hors périmètre ici, mais 26 % du groupe tours).
- **Q19** — pourquoi 125,825 est la moins bien transcrite (hypothèse saturation, non vérifiée).
- **Q20** — justesse de la détection du locuteur.
- **Q23** — la source d'un lexique d'opérateurs republiable.
- **Q25** — le bonus « opérateur nommé » discrimine-t-il ? Il monte le hasard ; à mesurer comme `-fuzzy`.
- **D2** — le mode de diffusion par canal sur la station (décision du propriétaire).
- Non numéroté, doc 19 : *« one two zero heading »* — nombre avant son mot-clé, `heading` absent des rôles suffixes.
- Non numéroté, doc 22 : 47 % des transmissions ne portent aucun indicatif — **aucune astuce ne crée une information absente**.
- Non mesuré : `large-v3-turbo` n'a été comparé qu'en identification de langue et boucles (docs 10, 14), **jamais sur la métrique ADS-B contre atco2**.
- **Non observable en production : le taux de rejet du VAD.** Le sidecar renvoie
  `rejected: no_speech` mais ne le journalise pas (`whisper_server.py:196` ne trace
  que les transcriptions) ; côté Go, `local.go:194` le trace en **Debug**, absent du
  journal au niveau courant. Sur la fenêtre propre de 13 h, 216 segments transcrits
  sont visibles et le nombre de segments rejetés est **inconnu** — le « 0 » qu'on
  obtient en comptant est un angle mort, pas un chiffre. Le 88,1 % de la nuit vient
  d'une mesure hors ligne (doc 21). Conséquence : la question *« le parasite est-il
  vraiment absent le jour ? »* ne peut pas être répondue depuis la chaîne en service
  tant que ce compteur n'existe pas.

## A.5 Le panel : quinze propositions, cinq survivantes, et quatre corrections à ce dossier

**Méthode.** Cinq lecteurs indépendants (Sonnet), chacun avec une lentille — audio et
ASR, grammaire, appariement, enrichissement, mesure et exploitation — ont lu `docs-fr/`
et le code en lecture seule, avec interdiction de toucher au binaire en service, à la
station et au GPU. Chacun a rendu au plus trois propositions, chacune obligée de dire
en quoi elle diffère de A.3 et comment elle se mesure. Puis **un réfutateur (Opus) par
proposition**, chargé de la casser avec le dossier, le code, et — quand c'était
possible — une mesure faite sur place avec `cmd/phraseology` ou `sqlite3`. Vingt
agents, 600 lectures ou commandes, quinze verdicts. Aucune proposition n'a été
« gardée » sans réserve ; c'est le signe que le dossier avait déjà fermé les portes
faciles.

Le panel a surtout produit quelque chose que je n'attendais pas : **quatre corrections
à mes propres mesures**, listées d'abord parce qu'elles changent des chiffres publiés.

### Ce que le panel a corrigé dans ce dossier

1. **L'outil `-db` n'appliquait pas la règle de production.** `-strict`, `-min-score`
   et `-context` étaient déclarés et jamais transmis au chemin SQLite : les quatre
   fenêtres de groupes comptaient les ambigus comme attachés, et mes commandes
   `-context 120` ne faisaient rien. Corrigé le jour même (le chemin `-db` porte
   maintenant la mémoire par fréquence et refuse les ambigus), remesuré : au plus
   3 appariements et 5 points d'écart, aucune conclusion ne bouge (doc 19, tableau
   corrigé). Deux divergences restent : la flotte hors ligne ne porte **jamais la
   phase**, donc les pénalités −0,25 « en croisière, pas en atterrissage » de
   `matcher.go` ne sont validées par **aucune** mesure du dossier ; et la fenêtre
   hors ligne est symétrique (± 60 s) quand la production ne regarde que le passé
   (`GetAllAircraftWithLastSeenFilter(1)`).

2. **« Structurellement inappariable » était faux.** Le bonus de lettres s'ajoute
   *avant* le test `score == 0`, donc un indicatif à queue de lettres passe le plancher
   avec une seule corroboration. Et c'est exactement ce qui s'est produit : les cinq
   appariements du 16/09 portés par le bonus d'opérateur sont tous du type
   `letters + operator named = 0,70` — AFR89VR, AFR44SA, AFR78HN, AFR65AR, AFR74UP.

3. **Q25 avait sa prémisse à l'envers, et sa réponse est dans le code.** Le +0,40
   « opérateur nommé » ne *discrimine* rien par construction : `restrict` garantit que
   tous les candidats survivants portent l'opérateur, ou aucun ; c'est un décalage
   constant, qui ne change ni le classement ni l'écart d'ambiguïté. Son seul effet est
   sur le plancher absolu — et là il est décisif pour une classe précise : 6 appariements
   sur 168 dans la journée dépendent de lui (3,6 %), 5 en fenêtres de 51 min, pour
   **0,38 de hasard sur huit témoins : 93 % au-dessus**. C'est l'inverse exact de la
   règle « à un près » (81 % de bruit). Le retirer éteindrait les indicatifs à queue de
   lettres — ceux que doc 19 crédite de tirer la précision des tours vers le haut. Quant
   à l'effet du complément de lexique sur le hasard, remesuré avec et sans sur les
   quatre fenêtres : ± 1 selon la fenêtre, **sous la résolution de l'instrument**. Ni
   ma hausse ni la baisse relevée par le panel ne sont établies.

4. **Le « 0 rejet VAD de jour » était un angle mort**, déjà noté en A.4 ; le panel a
   confirmé qu'aucun des deux côtés ne compte les rejets au niveau de journal courant.

### Ce qui a résisté (avec réserve)

**R1 — Arrêter de mélanger les fréquences avant la transcription (asr).** Le
mécanisme est vérifié ligne à ligne : `ffmpeg_channels = 1`, `X-Channels: 1` en dur,
segmenteur sur un signal mono — la séparation stéréo que fait le mélangeur de la
station est détruite avant que Whisper ne voie l'audio. Le réfutateur a mesuré le
chevauchement entre fréquences sur la captation du 15/09 : **850 transmissions sur
1 468 (57,9 %) chevauchent une transmission d'une autre fréquence**, et ces
transmissions-là portent **72,3 des 101,0 appariements vrais** — elles sont 1,83 fois
plus productives que les autres, parce qu'on se marche dessus quand la bande
travaille. Conséquence : le volet « détecter la collision par la balance stéréo et
écarter la fenêtre » est **réfuté** (il coûterait 72 vrais sur 102, la forme exacte des
résultats qui ont tué `-fuzzy` et `min_score 1.0`). Le volet « un flux par canal » (Q4,
D2) n'est **ni réfuté ni établi** : la seule comparaison propre-contre-mélangé du
dossier — production mélangée 30,5 vrais / 51 min à 85 %, corpus par canal 33,0 vrais
/ 51 min à 72 % — donne le même ordre de grandeur, sur deux jours et deux groupes
différents. *Mesure qui tranche* : sommer numériquement les fichiers par canal du 15/09
(mêmes échantillons, alignés à la trame), transcrire ce mélange synthétique avec le
sidecar de production, et comparer aux 102,3 vrais du même audio par canal. Un passage
GPU d'une heure environ, sur un audio identique — la seule expérience A/B possible sans
rien enregistrer. Le lecteur a déjà chiffré l'exposition sur l'amas nocturne : 56,6 %
des segments du mélange recollent l'activité de deux canaux, 22,7 % ont deux paroles
humaines réellement simultanées.

**R2 — Calculer la concordance voix / ADS-B, et l'afficher — mais pas comme un
statut (enrichissement).** Le réfutateur a refait la mesure avec le témoin qui
manquait : sur 62 niveaux prononcés portant un indicatif, **49 concordent à ≤ 500 ft**
avec le meilleur d'`alt_baro` / `nav_altitude_mcp` (79 %), contre **8,6 %** pour un
avion quelconque présent dans la même fenêtre ; 47 écarts sont à ≤ 25 ft, puis on saute
à 650, 2 975, 4 600 — la coupure est nette, et elle tient si l'on serre la fenêtre
temporelle. Trois réserves qui changent la forme du résultat :
- **`clearances.status` a un plafond de zéro** : la grammaire n'extrait que décollage,
  atterrissage et approche, 3 lignes dans toute la journée dont 2 mal analysées
  (« cleared to land runway three eight » → piste `380`), et rien dans l'ADS-B ne
  tranche une clairance d'atterrissage.
- **Le vert est en partie tautologique** : le matcher utilise déjà l'accord niveau dit /
  `alt_baro` (+0,40 à ≤ 1 500 ft), donc 7 des 33 concordances observées n'existent que
  parce qu'elles concordent, et 6 des 10 discordances survivent à exactement 0,60. La
  concordance ne valide pas l'appariement : elle en réexpose une entrée.
- **Le rouge n'est jamais une déviation** : les 13 discordances de la journée sont
  toutes des fautes de la chaîne — « flight level three four » lu FL34 face à un
  avion à 34 000 ft (EZY2070, une troncature ASR), « one thousand feet or greater »
  pris pour une altitude, LSM350 dont « Floor Low Three Five Zero » (le niveau) a servi
  d'indicatif. Peindre ça en « deviation » afficherait l'erreur de co-atc comme un
  écart de l'avion.

Ce qui reste, et qui vaut : **le calcul, avec un rouge qui veut dire « à vérifier »**.
Treize fautes de grammaire ou de transcription repérées en 4 h 20 **sans une seule
annotation** — c'est le seul détecteur automatique d'erreur que le projet ait, et son
rendement ne dépend pas du volume. Plafond du volet affiché : 67 valeurs corroborables
sur 391 (17 %), soit ~14 pastilles par heure, presque toutes des niveaux — le cap
prononcé est inutilisable (1 sur 5 à moins de 15° de la route, les autres sont du
charabia de grammaire : « two thousand degrees », « left five degrees ») et le
transpondeur inexistant (1 occurrence dans la journée, 0 dans la captation).

**R3 — `nav_altitude_mcp` dans le corroborateur d'altitude (appariement).** Deux
réfutateurs, deux verdicts, un même chiffre : **+3 appariements sur 158 (1,9 %)**,
mesuré en l'implémentant, sous le bruit de l'instrument (écart-type du hasard 4,3 sur
huit témoins). L'altitude sélectionnée est le champ le plus réactif à une clairance
qui vient d'être donnée, et un seul champ ne suffit pas — HRN223 « flight level two
zero four climbing two six zero » est juste contre `alt_baro` pour le premier niveau
et contre `nav_altitude_mcp` pour le second. Gardé **pour l'affichage** (R2), pas pour
le score du matcher, où la règle d'altitude entière ne vaut que 7 appariements.

**R4 — Réparer l'instrument `-db` (mesure).** Fait le jour même, voir correction 1.
Reste à porter la phase dans la flotte hors ligne (`phase_changes` existe dans la base)
pour que les pénalités de phase soient enfin mesurées.

**R5 — La base ne tourne jamais (exploitation).** Le point le plus grave du panel,
vérifié dans le code par le réfutateur puis par moi : `sqliteStorage` est ouvert **une
seule fois** au démarrage sur le fichier du jour, et jamais rouvert ;
`ensureTodayDatabaseFile` (`main.go:415`) se contente de **créer un fichier vide** pour
le nouveau jour et le referme ; `cleanupOldDailyDatabases` (`main.go:437`) **saute
explicitement le fichier actif**. Un processus qui tourne une semaine écrit donc *un*
fichier qui grossit sans limite et que la rétention ne touche pas, pendant que des
fichiers journaliers vides s'accumulent et se font purger. Chiffres du jour :
`adsb_targets` à 64,7 lignes/s, **1,49 Go pour 4 h 15** (page_count × 4 096, freelist
0), soit ~8,4 Go par jour pleine ; **54 % du volume est `raw_data`**, une copie JSON de
colonnes déjà analysées dans la même ligne. À 48 Gio libres, le disque est plein en
**~6 jours** de marche continue. C'est un défaut de l'amont, et un bloqueur pour toute
marche sans surveillance. *Le correctif est du code* — rouvrir la base à minuit, ou
cesser de stocker `raw_data` (qui ramène 8,4 à 3,9 Go/jour) — pas un veilleur : le
veilleur dirait au jour 5 ce que la lecture du code dit au jour 0.

### Ce qui a été écarté, et pourquoi c'est utile

| proposition | plafond ou mesure du réfutateur | verdict |
|---|---|---|
| fixer les paramètres de décodage de mlx-whisper (température, `no_speech`, compression) | blanchir les 46 transmissions « lentes » du corpus ne change **rien** à l'appariement ; plafond 3,1 % du corpus ; aucune valeur n'a jamais été choisie pour de l'ATC, mais seul un WER pourrait les évaluer | écarté — **à reprendre après les annotations** |
| le seuil de silence de 600 ms colle deux avions sur 125,825 | 387 fermetures de squelch de 0,2–0,6 s avalées sur 125,825 (41 % des frontières) — mais le dommage par fusion est mesuré nul (doc 12), 125,825 n'est pas en production, et 125,933 cumule 26 % d'avalement et la meilleure précision | écarté ; **Q19 reste ouverte** |
| apparier par lettres seules | 74 transmissions (5 %) portent des lettres sans chiffres ; **les deux outils de mesure les excluent** alors que la production les tente ; garde-fou relâché : 8 appariés, 1,75 de hasard, ≤ 6,25 vrais | écarté comme gain, **gardé comme angle mort d'instrument** |
| exiger une corroboration pour le palier « suffixe » seul | 26 des 143 appariements (18 %) reposent sur le suffixe seul, 29 % sur 125,825 ; l'exiger coûte **−14,5 vrais pour +3 points** | écarté — même forme que `min_score 1.0` |
| `heading` comme rôle suffixe (« one two zero heading ») | 9 transmissions sur 1 056 dans la base du jour, ≤ 2 caps réellement corrects, et la règle transforme un indicatif en cap | écarté |
| corroborer par le niveau sélectionné dans le score | +3 / 158, sous le bruit ; aucun appariement n'est aujourd'hui perdu à une fausse contradiction | écarté du score, gardé pour l'affichage |
| trancher Q25 par un drapeau | la mesure est faite sans drapeau, par `-v` ; le bonus est un décalage constant, porteur de 5 appariements vrais | écarté — **Q25 est répondue** |
| corroborer par le cap (route ADS-B) | aucun des deux instruments ne peut le trancher ; 1 cap sur 5 est sain ; le `track` est une route sol, le cap ATC un état futur | écarté ; **nettoyer les caps de la grammaire** reste utile |
| un second témoin par l'état physique de l'avion | même taux d'accord d'altitude sur le tirage réel (33/172) et sur les témoins (18/97) — le témoin relit un terme que le matcher optimise déjà | écarté |

### Ce que le panel a relevé sans le proposer

- **Q19, réponse partielle** : la saturation divise presque par deux les appariements
  vrais sur 125,825 entre moitié chargée et moitié calme — mais 124,350 et 124,625
  montrent l'effet **inverse**. « La charge nuit à la précision » n'est pas une loi sur
  ce corpus ; c'est un fait mesuré sur une fréquence. Et l'occupation instantanée
  suggérée par Q19 n'est pas calculable depuis la base : `transcriptions` ne porte ni
  durée ni occupation.
- **Une classe d'erreur ASR nommée** : le dernier chiffre qui tombe. « three four » pour
  « three four zero » (FL34 face à 34 000 ft), « level three two » face à 38 000 ft
  (CNV4258). `plausibleFlightLevel` accepte FL30–FL660, donc FL34 passe. C'est la
  concordance (R2) qui les attrape, pas la grammaire.
- **Un bug de lecture de piste** : « runway three eight » devient `380`, la lecture
  gloutonne d'un nombre ne s'arrête pas à deux chiffres pour une piste.
- **`min_speech_seconds` est inerte** : égal à `vad_min_speech_ms`, il ne rejette
  jamais rien que Silero n'ait déjà rejeté, et le second n'est pas exposé en ligne de
  commande.
- **Aucun paramètre de décodage n'est passé** à `mlx_whisper.transcribe` hormis le
  modèle, la langue et l'amorce ; les défauts (repli de température 0 → 1,0,
  compression 2,4, `no_speech` 0,6, `condition_on_previous_text` vrai) n'ont jamais été
  choisis pour de l'ATC.
- **`large-v3-turbo`** : jamais comparé sur la métrique ADS-B, mais sa seule mesure
  qualitative (doc 08 : boucle et caractères hébreux sur de l'en route) et la marge
  d'atco2 sur jacktol rendent le test à faible valeur attendue.
- **De jour, 24 % des transmissions portent un niveau, une altitude ou un cap**, contre
  3,6 % la nuit : la matière pour R2 est diurne.
- **4 heures continues de la même condition d'écoute** existent maintenant (amas depuis
  10:48) — ce que la comparaison des groupes n'a jamais eu.

## A.6 Les voies, dans l'ordre où je les ferais

Chaque ligne dit ce qu'elle rapporte, comment on le saura, et ce que ça coûte. Rien ici
n'est promis : tout est borné par une mesure déjà faite ou par une mesure nommée.

### Reconnaissance vocale anglaise

| # | voie | ce qu'on saura, et comment | coût | attente |
|---|---|---|---|---|
| **V1** | **L'A/B mélangé contre par canal sur audio identique** (R1) : sommer les fichiers par canal du 15/09, transcrire le mélange, comparer aux 102,3 vrais | tranche enfin la question D2 *pour la transcription* ; la borne de plausibilité dit « même ordre », la resynthèse dit « 22,7 % de paroles simultanées » | ~1 h de GPU, un script, aucun changement de station | **inconnue, et c'est la seule question ASR qui vaille une heure de GPU** |
| V2 | **Le chiffre final qui tombe** : marquer suspects les niveaux à deux chiffres après « flight level » sur une fréquence en route, et les compter | via la liste rouge de R2 ; EZY2070 et CNV4258 sont les cas types | quelques heures | petit sur l'appariement ; supprime des fausses contradictions |
| V3 | **Le taux de rejet VAD en production** : journaliser `no_speech` en Info ou le compter | répond à « le parasite est-il absent le jour ? » — sans réponse aujourd'hui | une heure | un chiffre qu'on n'a pas |
| V4 | **Paramètres de décodage et `condition_on_previous_text`** | plafond 0 sur l'appariement ; **seul un WER les évalue** | dépend des 120 annotations | à faire **après** les annotations, pas avant |
| V5 | `large-v3-turbo` sur la métrique ADS-B | une passe GPU sur la captation | ~1 h | faible attente ; optionnel |

### Grammaire et appariement

| # | voie | mesure | coût | attente |
|---|---|---|---|---|
| **V6** | **Garder le bonus d'opérateur** (Q25 répondue) et **ne pas toucher au lexique** : neutre sous la résolution de l'instrument | faite | 0 | — |
| V7 | **Piste à deux chiffres** : borner la lecture d'un numéro de piste à 01–36 + L/R/C | table `clearances` | une heure | corrige des lignes fausses, volume infime |
| V8 | **Caps de la grammaire** : « two thousand degrees », « left five degrees » sortent en `heading` ; les nettoyer | comptage sur `phraseology_values` | quelques heures | rend la pastille cap affichable ; aucun effet sur l'appariement |
| V9 | **Mesurer le chemin lettres seules** : inclure les 74 transmissions sans chiffres dans les deux outils | plafond ≤ 6,25 vrais / 1 415 | quelques heures | fermer un angle mort, pas chercher un gain |
| V10 | **La phase dans la flotte hors ligne** (`phase_changes`) | rend enfin mesurables deux pénalités de −0,25 que la production applique et qu'aucune mesure n'a validées | une demi-journée | inconnue — c'est le point |
| V11 | Afficher le **score et la raison** de l'appariement sur la fiche avion, pour que « suffixe seul à 0,60 » se voie | UI seulement | une heure | lisibilité, pas justesse |

### Enrichissement de co-atc

| # | voie | mesure | coût | attente |
|---|---|---|---|---|
| **V12** | **Concordance niveau dit / ADS-B sur les pastilles du panneau Radio** (R2, R3) : vert à ≤ 500 ft contre le meilleur d'`alt_baro` et `nav_altitude_mcp`, **« à vérifier »** sinon — jamais « déviation » | 79 % contre 8,6 % de hasard ; 13 fautes de chaîne détectées en 4 h 20 sans annotation | une demi-journée, frontend déjà touché | **le seul détecteur d'erreur automatique du projet** |
| V13 | `clearances.status` | plafond **zéro** avec la grammaire actuelle | — | ne pas faire |
| V14 | Corroboration par cap ou transpondeur | pas de matière (0 squawk, 1 cap sain sur 5) | — | ne pas faire |

### Exploitation

| # | voie | mesure | coût | attente |
|---|---|---|---|---|
| **V15** | **Faire tourner la base** à minuit, ou cesser de stocker `raw_data` (R5) | 1,49 Go / 4 h 15, disque plein en ~6 jours | une demi-journée de code, candidat à une pull request amont | **bloqueur de toute marche sans surveillance** |
| V16 | Mesurer la **variance horaire** sur les 4 h continues de l'amas | la résolution de l'instrument est ± 5–7 points par fenêtre de 51 min ; combien par heure, sur une condition fixe ? | une heure de calcul | fixe ce qu'on peut affirmer avec une fenêtre |
| V17 | Les **120 annotations** débloquent V4, la justesse du locuteur (Q20), et un WER absolu pour tout comparatif de modèle | outil prêt sur `http://127.0.0.1:8777/` | 30 à 45 min du propriétaire | **la seule voie vers un chiffre absolu** |

### Ce que je ne recommande pas, malgré l'apparence

- Un **modèle de langue local** pour corriger le texte (docs 22) : les erreurs qui
  coûtent sont dans les chiffres, qu'aucun modèle ne peut réparer sans connaître la
  réponse. Le panel n'a rien trouvé qui rouvre cette porte.
- **Propager un indicatif par proximité** entre transmissions (doc 22) : non
  mesurable avec le témoin du projet ; le panel l'a confirmé.
- **Écarter les fenêtres de collision** entre fréquences (R1, volet b) : ce sont les
  transmissions les plus productives du corpus.
- **Relever `min_score`**, exiger une corroboration, ou quatre chiffres : trois fois
  mesuré, trois fois le même profil — de la propreté apparente et une mesure éteinte.

---

# Partie B — la version courte

## Où on en est

**La chaîne anglaise fonctionne, en local, sans rien qui sorte du Mac.** Elle écoute le
groupe que vous choisissez sur la station, transcrit chaque transmission en moins d'une
seconde, comprend qui parle et de quel avion il s'agit, et le montre sur la carte : un
filtre pour ne voir que les avions dont la radio a parlé, un panneau « Radio » sur
chaque fiche avec ce qui a été dit, et les niveaux, caps et fréquences en pastilles.

**Ce qu'elle vaut, en chiffres tenus à jour :** sur une heure d'écoute de l'espace
supérieur, environ 30 à 37 avions sont correctement rattachés à ce qu'on a entendu
d'eux, avec 85 à 91 % de fiabilité. Sur les tours locales, deux fois moins, mais plus
sûr encore. Et la nuit, le détecteur de voix évite de transcrire 88 % de bruit.

**Ce qu'elle ne fait pas :** le français (un quart du groupe tours) part vers un modèle
anglais ; presque la moitié des transmissions ne nomment aucun avion, et aucune astuce
n'y changera rien ; et il n'existe toujours **aucun taux d'erreur de mots**.

## Ce qu'on a appris en deux jours, en une phrase chacun

- Le bon modèle anglais n'était pas celui prévu ; l'ADS-B a tranché.
- Le détecteur de voix est une exigence de justesse, pas une optimisation.
- Les réglages de l'appariement sont mesurés, pas choisis, et toutes les façons
  « prudentes » de les durcir éteignent la mesure.
- Corriger le texte sans IA ne rapporte rien ; le faire avec une IA locale ne pourrait
  pas mieux, pour une raison de fond.
- Le contexte entre transmissions n'a presque pas de matière, et sa version rentable
  n'est pas mesurable.
- Le panel a corrigé quatre de mes chiffres. Aucune conclusion n'a bougé, mais je
  savais moins bien que je ne le croyais ce que mesurait mon propre outil.

## Ce qu'il reste, par ordre d'intérêt

1. **Un test d'une heure de GPU** qui dit enfin si mélanger les fréquences avant la
   transcription coûte quelque chose — sur un audio identique, par canal et mélangé.
   C'est la seule question de reconnaissance vocale qui vaille encore une expérience.
2. **Un détecteur d'erreur gratuit** : comparer le niveau prononcé à ce que l'avion
   affiche. Vert dans 79 % des cas contre 9 % par hasard ; et chaque rouge, jusqu'ici,
   était une faute de la chaîne — treize trouvées en quatre heures sans annoter.
   Une demi-journée.
3. **Un bloqueur d'exploitation** : la base de données ne tourne jamais, le fichier
   actif grossit de 8 Go par jour, et le disque est plein en six jours de marche
   continue. À corriger avant toute marche sans surveillance. C'est un défaut de
   l'amont, et un correctif à leur proposer.
4. **Vos 120 annotations**, toujours. Elles seules donnent un chiffre absolu, et elles
   seules permettent de régler les paramètres du modèle — dont aucun n'a jamais été
   choisi pour de l'ATC.

Tout le reste — bugs de grammaire, angles morts d'instrument, variance horaire — est
listé en A.6 avec son coût et son plafond, et aucun ne vaut plus de quelques heures.
