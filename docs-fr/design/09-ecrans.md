# Les écrans finaux

*13/10/2026, session design. Quatorze écrans assemblés avec le design system seul (`08-design-system.md`) :
ses jetons et ses classes `cl-`, aucun style propre. Ils sont la référence d'acceptation de la
migration : chaque étape livrée se compare à eux.*

- Source : `tools/design/ecrans.py`, qui écrit `ecrans/*.html` (ouvrir `ecrans/index.html` en servant
  `docs-fr/design/`). Le cadre PAR vient de `tools/design/par-mini.py`.
- Galerie publiée (privée, au propriétaire) : https://claude.ai/artifact/2gZSBGbTkEySxThbNRYhMF.
  La copie publiée intègre les feuilles dans chaque écran ; elle se refait par
  `python3 tools/design/ecrans.py <dossier>`.
- Règle de fabrication : un écran qui a besoin d'un style absent du système révèle un composant
  manquant ; on l'ajoute au système, pas à l'écran. Seuls sont permis en ligne la position d'une
  fenêtre flottante et les illustrations qui tiennent lieu d'un moteur de rendu (la carte, la PAR).
  Vérifié le 13/10 : aucune classe hors du système, aucune balise `<style>`.

## Les quatorze écrans

| Écran | Ce qu'il montre |
|---|---|
| `a-maintenant` | A par défaut : le fil, le ciel, la colonne sur « Right now » (synthèse, séquence, « Worth knowing », météo, compteurs) |
| `a-trafic` | A sur « Traffic » : recherche, altitude avec histogramme, préréglages et QNH, Air / Ground / Heard, familles, Active / Signal lost, tri ; la carte suit les filtres |
| `a-frequence` | le menu d'une fréquence (volume, « seulement elle dans le fil ») ; « ↓ 3 new exchanges » après avoir remonté le fil |
| `b-replie` | B : la colonne repliée, le fil en transcription, « Right now » en carte sur la carte |
| `c-suivre` | C : l'histoire du vol dans la colonne, sa trace sur la carte centrée sur lui, son approche dans le cadre PAR |
| `c-details` | C, onglet Details : historique des phases, consignes entendues, informations de base, sections repliées |
| `a-calques` | les calques et fonds de carte ; la fiche d'un aérodrome |
| `a-journal` | le journal (la cloche) : ses six sortes d'événements et ses sons |
| `par` | la vue PAR existante, son en-tête et sa séquence, à la place du ciel |
| `reglages` | la page des réglages par-dessus l'écran |
| `connexion`, `premier-lancement` | la connexion, « le ciel » : la page est le scope du signe, le nom en grand, la console de la station, le formulaire au centre ; la question du premier lancement dans le même cadre |
| `etats` | rien à l'antenne, connexion perdue, « cliquez pour reprendre l'écoute » |
| `telephone` | trois volets du téléphone, barre d'onglets en bas, tailles tactiles |

## Où va chaque fonction d'aujourd'hui

Tiré de l'inventaire de `www/` fait le 13/10 sur `origin/main` (≈ 150 fonctions), puis contrôlé
par la session atc-scribe le même jour (51 actions, 28 champs de réglage, 13 raccourcis, 43 clés) :
les lignes des réglages ci-dessous viennent de ce contrôle. Ne sont listées que les fonctions qui
changent de place ou de forme ; le reste garde la sienne (carte, PAR, liste, fiche).

| Aujourd'hui | Demain |
|---|---|
| Barre radio : tuile par fréquence (écouter / couper, MHz ou « mixed », échec / nouvel essai) | `ThreadHead` : un bouton de fréquence en deux parties |
| Volume par fréquence (curseur dans la tuile) | menu de la fréquence (▾), `ChannelMenu` |
| « all » écouter / couper tout | « Mute all » / « Listen to all » en tête du fil |
| Fil de transcription par fréquence (fenêtre, recherche, non-lus) | un seul fil, filtre « Show » par fréquence, recherche du fil, une pastille « ↓ N new » |
| Niveau et secondes depuis la dernière émission | `LevelMeter` dans le fil ; l'âge dans le menu |
| METAR, TAF, NOTAM (barre du bas) | « Right now » › `Weather` (METAR lu en clair, TAF et NOTAM en boutons) |
| Piste en service par aérodrome (barre du bas) | la phrase de synthèse et l'en-tête de la PAR |
| Bandeau d'état (antenne, transcription, ADS-B) | `TopBar` |
| Horloges locale / UTC, cliquables pour choisir l'heure affichée | les deux dans `TopBar` ; le choix passe dans Réglages › Display |
| Engrenage et cloche en haut à gauche de la carte | `TopBar`, à droite |
| Titre « Tracked Targets » | l'onglet « Traffic » |
| Fiche avion : Timeline, Details, Tracks, Proximity | onglets Story, Details, Track, Nearby de la colonne (C) |
| Fiche avion : pas de bouton fermer au bureau | « ‹ Back to Traffic » dans la colonne, ✕ « Stop following » dans `TopBar` |
| Tab / Maj-Tab : avion suivant | J / K, ↓ / ↑ dans la liste (choix du 13/10) ; Tab rend la main au navigateur |
| Téléphone : Map, PAR, List | Radio (accueil), Sky (carte ou PAR), Traffic, Right now |
| Écran « Connection Lost » plein écran | `ErrorBanner` en haut du ciel, la carte reste utilisable |
| Bulle de dernière transmission posée sur la carte (retirée le 13/10 à la demande du propriétaire : elle couvrait l'étiquette) | troisième ligne de l'étiquette de l'avion, entre guillemets, 40 caractères, dix minutes après l'écoute (`MapLabel`) ; réglage « Last transmission on aircraft labels », clé `voiceBubbles` |
| Opacité des six calques (anneaux, espace aérien, aérodromes, héliports, balises, autres pistes) | panneau des calques, un curseur par calque (`a-calques`) |
| Longueur des traînées (`trailLength`) | panneau des calques, « Tracks, last N min » (`a-calques`) |
| Lissage, images par seconde (trois clés d'animation) | Réglages › Display (`reglages`) |
| Points de trace gardés (`tracksLimit`) | Réglages › Display |
| Masquer les avions au sol des autres aérodromes (`excludeOtherAirportsGrounded`) | Réglages › Display |
| Pause des mises à jour (WebSocket, « Performance and debug ») | Réglages › System, section repliée |
| Accès : cette machine ou un compte (`switchAccess`, page de connexion et réglages) | premier lancement (`premier-lancement`) et Réglages › Account |

## Six propositions, validées par le propriétaire le 13/10

Présentées une à une, avant / après sur les écrans réels ; toutes retenues telles quelles.


1. **Le volume dans le menu de chaque fréquence** (▾), puisque la barre du bas disparaît.
2. **Un seul fil et une seule pastille de nouveaux échanges**, au lieu d'un fil et d'un compteur
   par fréquence ; le filtre « Show » remplace l'ouverture d'un fil particulier.
3. **La météo dans « Right now »**, le METAR lu en clair en une phrase.
4. **Le choix UTC / heure locale dans les réglages**, les deux horloges restant affichées.
5. **Le téléphone à quatre onglets**, la radio en accueil.
6. **Les consignes entendues sans verdict** : la colonne « Status » de la fiche (prévue pour
   issued / complied / deviation, mais qui n'affiche jamais que « issued » : rien n'écrit les deux
   autres) devient « Then » et dit ce que l'avion a fait (« level at 4 000 ft, 3 min later » ; « — » tant qu'une clairance n'a pas de suite visible ; phrases arrêtées par la fiche cloud K), sans
   qualifier l'écart. C'est le principe « constater, ne pas juger » appliqué à une fonction qui
   existe ; le propriétaire a confirmé qu'il s'applique ici.

## La page de connexion (13/10, le soir)

À la demande du propriétaire (« pas une bête fenêtre de login, la présence de marque doit être
assurée, et quelques infos geek avant même d'être loggé »), trois directions ont été dessinées
(`connexion/`, `tools/design/connexion.py`) : A le scope, B un échange, C le ciel. **Il a retenu
C** : la page entière est le scope du signe (anneaux, balayage figé, lueur, un plot, quelques
silhouettes de notre famille), le nom en grand, la console de la station en haut à droite, le
formulaire au centre. **Et les constantes agrégées avant la connexion** : avions suivis, messages
par seconde, portée maximale, canaux à l'antenne, transmissions du jour, modèle de transcription,
taux d'appariement, aérodromes et pistes, temps de fonctionnement, version ; jamais une position,
un indicatif ni une fréquence nommée. La page n'existe qu'en mode « réseau avec mot de passe » :
ces chiffres sont vus de tout appareil du réseau ou du VPN, d'où une route publique qui ne sert
qu'eux, et un réglage pour couper la console.

## Écarts volontaires

| Date | Écart | Pourquoi | Suite |
|---|---|---|---|
| 13/10 | Les préréglages d'altitude de l'onglet Traffic : l'écran de référence avait remplacé, par erreur, la tranche « FL100–195 » par un plafond « ≤ FL 195 », ce qui retirait une fonction | relevé à la relecture de l'étape 4 | la tranche revient (« FL 100–195 »), dans un contrôle segmenté dense (`cl-seg--dense`) pour tenir sur une ligne ; écran `a-trafic` corrigé |
| 14/10 | La barre du haut au téléphone : la référence n'y montrait ni la cloche ni les réglages, et la page, sans règle pour le petit écran, débordait jusqu'à 780 px (cloche et réglages hors de l'écran, donc inaccessibles) | relevé à la relecture de l'étape 7, à 375 × 812 | règle ajoutée au système (`TopBar` : signe, un voyant coupé d'une ellipse, UTC, cloche, réglages à 44 px ; les autres voyants seulement en alerte) ; écran `telephone` corrigé ; à appliquer sans attendre l'étape 12 |
| 13/10 | « Pilot replied, 3 s later » sous une consigne n'est pas affiché | étape 6 : l'API ne garde aucun lien entre une consigne et sa réponse | accepté : la ligne revient quand le serveur saura lier une réponse à sa consigne ; fiche `Exchange` |
| 13/10 | « being matched… » à la place des puces, quelques secondes, avant l'appariement | étape 6 : l'appariement local n'est pas instantané | accepté, ajouté à la fiche `Exchange` |
| 13/10 | Les noms de fréquence longs (« Approche De Gaulle + Le Bourget ») coupés d'une ellipse, les boutons sur une rangée | étape 6 : deux boutons passaient sur deux rangées, l'en-tête prenait 40 px | règle ajoutée au système (`cl-threadhead__row--channels`, `cl-channel__name`) ; nom entier dans le `title` et le menu |
| 13/10 | La phrase de synthèse de référence : « De Gaulle lands on 26L since 11:40 » est un calque du français ; avec « since », l'anglais veut le present perfect | relevé à la relecture de l'étape 5 (« uses runway 25 since… » sur l'instance) | « has been using runway 26L since 11:40 » ; « uses » plutôt que « lands on », le serveur ne distinguant pas arrivées et départs ; écrans et fiche `SummarySentence` corrigés |
| 13/10 | TAF, NOTAM et METAR précédents (« Earlier », gardé de l'ancienne bulle METAR) s'ouvrent en carte dans la colonne, sous les boutons, et non en bulle flottante | étape 5 : un NOTAM est long, et rien ne flotte sur la carte | accepté ; fiche `Weather` mise à jour, bouton ouvert `aria-expanded`, dessiné sélectionné |
| 13/10 | Un tableau d'arrivées par piste en service ayant des avions, au lieu d'un seul | étape 5 : De Gaulle, Orly et Saint-Cyr suivis ensemble | accepté : la plus proche arrivée d'abord, cinq lignes s'il est seul, trois s'il y en a plusieurs ; fiche `ArrivalSequence` |
| 13/10 | La carte « Right now » repliée se place sous la bascule Radar / PAR, pas au bout de `cl-sky__top` | étape 5 : la bascule n'a pas encore migré | provisoire : elle rejoint `cl-sky__top` avec la bascule, à l'étape 9 (ajoutée au plan) |
| 13/10 | L'altitude des avions sur la carte garde sa palette par teinte d'avant (fond sombre : `#D35A48`, `#EA8A4A`, `#F4B55A`, `#F8D857`, `#E8ECEF` du plus bas à la croisière ; fond clair : la série bleu / orange), au lieu de la rampe de gris du système | soirée d'écoute de l'étape 2 : « le dégradé de gris ne marche pas, trop difficile de distinguer les uns des autres » | **réglé le 13/10** : quatre palettes mesurées et dessinées sur une même scène ; le propriétaire choisit F, du chaud au froid (jaune, rose, violet, cyan, ardoise), qui tient toutes les paires sous les quatre visions sans rappeler l'urgence ni une famille de phase ; jetons `color.altitude.*` mis à jour, `03-systeme.md` § Altitude |

## Demandes ouvertes

| Date | Demande | Suite |
|---|---|---|
| 13/10 | Le propriétaire : des étiquettes d'avion allégées à faible zoom, par exemple l'indicatif seul sous un seuil (relayé par la session atc-scribe) | **décidé le 15/10** sur le ciel réel de l'instance, mesures et planches à l'appui : trois paliers selon la largeur du ciel (étiquette complète jusqu'à 24 NM, indicatif seul de 24 à 67 NM sauf les avions qui comptent, au-delà seulement eux), et le dernier message dans une bulle sous l'étiquette au lieu d'une troisième ligne ; règles dans la fiche `MapLabel` |
| 16/10 | Q56, les procédures publiées sur la carte : la route d'une puce de procédure, et une couche (relayé par la session atc-scribe, direction validée par le propriétaire) | **dessiné et livré le 16/10**, fiche `ProcedureRoute`, jeton `color-map-procedure` ; relu sur l'instance avec mesures. La première livraison écrivait un même nom jusqu'à 21 fois (une fois par route) et ne nommait aucun point d'arrivée à aucune largeur (les STAR finissent à 30-40 NM, au-delà du palier de 67 NM pris aux étiquettes). Après correction : un nom par point à toutes les largeurs, 4 noms d'arrivée à 67 NM et 5 à 102 NM (12 de départ), la note tient à 1 440 px. Une STAR finit au point d'approche initiale, sans indicatif de piste |
| 16/10 | Q57, la piste et la sortie probables sur l'étiquette, lues sur la trace ADS-B (demande du propriétaire, mesures de la session atc-scribe) | **forme proposée et validée le 16/10, en production le même jour** (9fcc281) : champ en fin de 2ᵉ ligne (« →26L/R » puis « →26L », « →OKASI +2 » puis seul), l'incertitude dite par le contenu et non par un style ; clé « To … » de la fiche ; réglage allumé par défaut. Ajout du propriétaire, relu après coup : un départ sélectionné montre les routes de ses sorties possibles ; la relecture du code y a trouvé les noms doublés de Q56 et une route de puce ajoutée aux candidates, corrigés et mesurés sur l'instance à 17:17 (aucun nom doublé, couche allumée ou non ; la route de la puce l'emporte). Chevauchements, réglage éteint puis allumé : inchangés (45 % à 24 NM, 37 % à 34 NM ; 2 à 3 avions concernés) |
| 28/10 | D86, les réglages en pages groupées (analyse `docs-fr/38-reglages-analyse.md`), la mise en forme confiée à la session design | **proposée le 28/10** : une page à la fois, la navigation groupée (This browser · Station · Administration), la page élargie à 1 080 px et les champs gardés à 592 px de lecture, les cartes en deux colonnes à partir de 1 024 px (comptes, usage, rapports), une page qui demande un regard marquée ▲ ou ■ dans la navigation, le viewer en lecture seule (valeurs en texte), la liste puis la page au téléphone (« ‹ Settings ») ; fiche `SettingsPage`, écrans `reglages*.html` et `telephone-reglages*.html` (`tools/design/reglages.py`) |

## Ce qui n'est pas dessiné

Les volets Track et Nearby de C (ils gardent leur contenu actuel dans les composants du système),
les fiches de balise et d'héliport (même forme que celle d'un aérodrome), le détail des onze
règles d'appariement et des rapports système dans les réglages (repliés, contenu inchangé).
