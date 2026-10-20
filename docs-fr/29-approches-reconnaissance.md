# Améliorer la transcription : le modèle de langage et le son qui lui arrive

*Document de référence, **état au 20 octobre 2026** (revue du 20/10 ; chaque ligne garde la date de sa mesure, les lignes ajoutées le 20/10 sont signalées). Il se lit sans jargon ; chaque ligne renvoie au
document qui détaille la mesure. À tenir à jour à chaque résultat.*

*Depuis le 27/09, ce document ne couvre plus que **le modèle, la chaîne de transcription et la
réception**. Les règles qui relient ensuite le texte à un avion, avec leur réglage dans co-atc, sont
dans **`30-association-regles-et-reglages.md`** (l'ancienne section B de ce tableau).*

## Ce qu'on cherche, et comment on juge

**Le but** : que co-atc relie chaque transmission au bon avion — son indicatif — et qu'il
**n'invente pas**. Un indicatif inventé qui tombe sur un avion réel est pire qu'une absence : on
ne le voit pas.

**Deux juges** :
- **l'ADS-B** : une transmission est « juste » si l'indicatif compris correspond à un avion
  réellement dans le ciel à ce moment. On retire la part due au hasard en refaisant le même
  calcul avec un ciel pris à un autre moment (le « contrôle par ciel mélangé ») ;
- **l'oreille du propriétaire** : 55 transmissions transcrites à la main — mais peu en anglais, et
  presque toutes partielles.

**Le point de départ** (Q30) : sur 1 332 transmissions du 15/09, **environ 8 % reliées à un
avion juste**. Les pertes viennent surtout des **chiffres mal entendus** et de transmissions qui
ne portent aucun indicatif (47 %).

## Le tableau

✅ adopté · ❌ écarté · ⏳ en cours · 🔲 pas encore testé

### A. Le modèle et la chaîne de transcription

| | Approche | Ce qui a été mesuré | Verdict |
|---|---|---|---|
| 1 | Un modèle anglais spécialisé contrôle aérien, le plus gros | 94 avions justes contre 63 pour un modèle moyen spécialisé | ✅ (D12) |
| 2 | Tout faire tourner sur le Mac, sans service extérieur | 0,7 s par transmission | ✅ (D3, D10) |
| 3 | Une seconde lecture par un modèle français quand le texte contient du français | +8,5 % d'avions justes ; elle se déclenche sur 12,5 % des transmissions | ✅ (D24) |
| 4 | Un détecteur de voix pour écarter le bruit | la nuit, jusqu'à 98 % des ouvertures du squelch sont sans parole | ✅ (D27) |
| 5 | Donner au modèle un texte d'amorce | 7 fois plus de délires | ❌ (doc 10) |
| 6 | Donner en amorce les avions visibles à l'ADS-B | aucun effet net sur un petit échantillon | ❌ (D46) |
| 7 | Coller les transmissions consécutives avant de transcrire | nul ou négatif | ❌ (doc 12) |
| 8 | Laisser le modèle deviner la langue | 16 à 28 % d'erreurs ; remplacé par la langue connue de chaque fréquence | ❌ (doc 13) |
| 9 | Les réglages internes du modèle contre l'invention | aucun ne sépare l'invention de la bonne transcription | ❌ (D45) |
| 9 bis | Ne donner au modèle que la parole | l'invention baisse avec la marge (presque divisée par deux à 1 s) ; les avions justes ne bougent pas au-delà du bruit, mais la fréquence faible 125,825 en perd à toutes les marges | ⏳ pas adopté en l'état (Q41) |
| 9 ter | **L'aiguillage vers le modèle français** : aujourd'hui, la seconde lecture ne se déclenche que si le texte *anglais* contient un mot français repère (« bonjour », « niveau »…) | 27/09, 126,425 : le propriétaire entend « Air France 511 » dans des échanges en français ; forcé en anglais, le modèle **anglicise** (« Air France Five One Air we conceded report three zero »), aucun repère, pas de seconde lecture. La détection de langue de Whisper, avec notre modèle affiné sur l'ATC anglais, répond « anglais » sur ces mêmes passages | ✅ **en production depuis le 28/09 (D64, Q50)** : repères **ou** détection de langue (turbo, p(fr) ≥ 0,1) → **+7 à +8 % d'avions justes à précision égale**, sur trois bancs et en direct (+13,4 le 28/09). Français lu sans relance (coût ÷ 2) et **boucles coupées après le modèle** (sans relance, le français bouclait deux à trois fois plus ; avec la coupe, 4,2 et 2,5 % de textes qui bouclent, contre 9,8 et 4,8 % avec relance). Un français plus rapide (distil, medium) perd le gain |
| 9 quater | *(ligne ajoutée le 20/10)* **La première lecture d'abord, la seconde ensuite** : tout le travail du modèle passe par un seul fil, et la seconde lecture française d'une transmission passait devant la première lecture de la fréquence voisine. De la requête au texte, 300 transmissions sur 48 min (14/10) : 7,8 s en médiane, 31,7 s au 90ᵉ centile, dont 0,5 s et 26,5 s d'attente. Cause annexe : `mlx_whisper` ne gardait qu'un modèle en mémoire, chaque bascule le reconstruisait (lecture anglaise 4,58 → 0,90 s) | ✅ **en production (14/10, D81)** : deux voies sur le fil (`sidecar/lanes.py`), le sidecar répond dès la lecture anglaise et la française rejoint le fil en mise à jour, et les deux modèles restent en mémoire (7f35308). Première lecture : **9,2 s → 1,0 s en médiane** (28,6 s → 3,3 s au 90ᵉ centile), mesuré en direct sur 83 transmissions. La reconnaissance n'y perd rien |

### B. Du texte à l'avion

→ **déplacé dans `30-association-regles-et-reglages.md`** (règles, réglages du panneau, mesures).
Deux acquis postérieurs au 27/09, ajoutés le 20/10 :

- **Trois préréglages d'appariement** (D73, 12/10) : Mesuré (le défaut, **95 % de précision** sur
  les journées du 03 et du 04/10), Prudent (chiffres exacts exigés, 97 %, 27 % d'appariements en
  moins) et Expérimental (95 %, 25 % de plus, à juger à l'oreille). Détail au doc 30.
- **Les noms officiels de l'AIP** (Q55, 15-16/10) : un outil fabrique la liste des points et
  procédures autour de la station ; leur **affichage dans le fil** (les procédures entendues sont
  nommées d'après le son des mots, seuil 0,95 réglable) est **en production depuis le 16/10**,
  « usage 3 ». Les deux autres usages sont **écartés après mesure** : la règle de grammaire (35
  appariements perdus, presque tous justes, 0 gagné, sur 18 224 transmissions) et l'indice de direction
  (39 % des autres avions se rapprochent aussi du point).

### C. La réception, à la station

| | Approche | Ce qui a été mesuré | Verdict |
|---|---|---|---|
| 19 | **Recevoir chaque fréquence séparément au lieu du mélange** | **4 fois plus d'avions justes** (235 contre 60), précision égale — le gain le plus fort mesuré à ce jour | ✅ flux séparés en service à la station depuis le 26/09 ; co-atc les suit (D57) — séances du 26/09 et nuit du 26 au 27 ; reste le lancement de la production |
| 20 | Comprendre le hachage | le squelch se ferme par instants au milieu des transmissions ; **la position de la fréquence dans la bande n'y est pour rien** (compteurs de la station, 26/09) ; il varie d'un facteur 10 d'un quart d'heure à l'autre ; **le contrôleur, au sol, hache presque autant que les pilotes** (27/09, 2 h : 57 % contre 68 %) et le hachage ne croît pas avec la distance de l'avion : la cause est à la réception | ⏳ piste : un seuil de squelch plus bas, par blocs alternés (Q47) |
| 20 bis | Deux fréquences qui reçoivent la même chose | le contrôle couple ses secteurs le soir et la nuit (approches de De Gaulle : 65 à 100 % des transmissions en double) : écho à l'écoute, transcriptions en double | ✅ compris (Q48) : n'écouter qu'une des deux |
| 21 | Chasser le parasite de nuit | toujours là ; le bloc de la Freebox est innocenté | ⏳ coupable inconnu (Q44) |
| 22 | Baisser le gain de la réception | ça écrête, mais baisser le gain n'est pas le remède | ❌ (D42, D43) |
| 23 | Nettoyer le son par des filtres classiques | **le filtre voix de la station** (28/09, laboratoire) : passe-haut 300, passe-bas 2 800, compresseur et limiteur, rejoués à l'identique sur le Mac, sur le banc du 27/09, passe anglaise → 153,8 avions justes contre 166,5 sur les totaux, textes vides 2,0 % contre 0,5 %, boucles 7,7 % contre 6,2 %. **Corrigé le 30/09 (laboratoire), transmission par transmission** : 39 gagnées, 54 perdues (test du signe, p = 0,15) ; valeurs justes nettes (J2) 55,5 → 44,4, soit −20 %. Le « −8 % » des totaux n'est pas établi | ❌ on garde le flux brut pour la reconnaissance (Q42) : **il tend à nuire (J2 −20 %), ce n'est pas prouvé sur les rattachements, et rien ne plaide pour lui** |
| 23 bis | Le squelch : combler ses trous, ou s'en passer | **Combler les trous** (7 % de l'audio) d'un souffle limité à la bande → 162,7 avions justes contre 166,5, dans le bruit ; transmission par transmission (30/09, laboratoire), 23 gagnées et 25 perdues (p = 0,89) : **aucun effet, confirmé** ; les retirer → 158,1 : les zéros eux-mêmes ne font pas inventer le modèle. **Un vrai canal sans squelch** (28/09, 17:53-19:53) : 124,350 reçue deux fois à la fois, squelch 10 et squelch manuel à −120 dBFS ; 385 transmissions coupées aux mêmes instants, lues avec les réglages de production. Résultat : 139,3 avions justes avec squelch, 143,1 sans, précision 97 % des deux côtés ; transmission par transmission, 24 gagnées et 20 perdues (test du signe, p = 0,65) ; sur les 152 hachées, 10 gagnées et 13 perdues ; boucles un peu plus fréquentes sans squelch (anglais 5,2 → 6,5 %, français 1,0 → 2,3 %). Séance représentative : 39 % de transmissions hachées | ❌ **aucun gain mesurable, le canal sans squelch n'est pas retenu** (laboratoire). Réserve : une soirée, une fréquence ; un gain de quelques % n'est pas exclu |

### C bis. D'autres modèles de reconnaissance

| | Approche | Ce qui a été mesuré | Verdict |
|---|---|---|---|
| 23 quater | Débruiter par un réseau de neurones, ou par FFT | **les deux débruiteurs neuronaux effacent la voix radio** (30/09, laboratoire) : RNNoise la fait passer de −18 à −63 dB, DeepFilterNet 3 de −17 à −58 dB. Sur la voix propre d'ATCOSIM, DeepFilterNet la garde intacte (−26,7 → −26,5 dB) : c'est notre bande étroite qu'il ne reconnaît pas comme de la voix. **Le débruiteur FFT de ffmpeg** (`afftdn nr=20 nf=-30`, sans suivi du bruit) baisse le souffle de 10 dB sans toucher la voix, mais ne sert pas : sur le banc du 27/09, anglais seul, par paires, 42 transmissions gagnées et 53 perdues (p = 0,30) ; valeurs justes nettes 55,5 → 51,2 ; mots non classés 16,5 → 17,4 % | ❌ **les trois débruiteurs sont écartés** (30/09, laboratoire) |
| 23 ter | Kyutai STT (`stt-1b-en_fr`, MLX), bilingue par construction | sur 50 transmissions du 27/09 tirées au hasard : **43 textes vides**, 1 avec un mot de phraséologie ; le modèle anglais de production : 0 vide, 28 avec. Il lit bien la voix propre d'ATCOSIM, mais sur la radio d'UWB-ATCC il rend de l'hébreu ou rien | ❌ écarté (28/09, laboratoire) |
| 23 quinquies | Les modèles NVIDIA Parakeet TDT 0.6B v3 et Canary 1B v2 (conseil de Damien, un ami du propriétaire ; téléchargés avec son accord sur le disque externe, 2,5 et 6,4 Go) | sur les mêmes 50 transmissions du 27/09 que Kyutai, en textes vides / mots par texte / textes avec un mot de phraséologie / textes avec un nombre : **Parakeet 12 / 11,1 / 11 / 30 ; Canary** (langue donnée par le détecteur) **14 / 16,1 / 7 / 19** ; Kyutai 43 / 1,4 / 1 / 3 ; anglais de production 0 / 23,7 / 32 / 47 ; français de production 0 / 19,2 / 16 / 44. Parakeet entend plus que Kyutai et LINAGORA, mais trois fois moins de phraséologie que notre Whisper affiné | ❌ écartés en l'état (30/09, laboratoire) ; **Parakeet reste candidat à l'affinage**, après Whisper |

### D. Entraîner le modèle sur le bruit de la station (plan 27)

| | Étape | Ce qui a été mesuré | Verdict |
|---|---|---|---|
| 24 | Peut-on entraîner sur ce Mac ? | oui : ~5 s par pas, 9 Go de mémoire ; reconversion pour co-atc réussie | ✅ |
| 25 | Faire sonner des enregistrements propres comme la station | l'imitation reproduit la station sur tout ce qui se mesure | ✅ |
| 26 | Mini-essai 1 : un seul corpus, le modèle entier | invente **4 à 6 fois moins** sur du bruit, mais **deux fois moins d'avions justes** : il a appris le vocabulaire du corpus (« Rhein », « Lufthansa ») et perdu celui de Paris | ❌ en l'état |
| 27 | Mini-essai 2 : un mélange de trois corpus | 191 avions justes (essai 1 : 104 ; production : 236) ; invente toujours moins sur le bruit ; la fréquence faible 125,825 perd encore la moitié de ses avions | ❌ en l'état, mais la bonne direction |
| 28 | Mini-essai 3 : le mélange, en n'entraînant que « l'oreille » (pas la rédaction) | 158 avions justes, moins bien que l'essai 2 sur les quatre fréquences ; plus lent (textes à rallonge) | ❌ |
| 28 bis | Ce que les essais 2 et 3 ont en commun | sur les fréquences faibles, ils rendent **vide un tiers à la moitié des morceaux qui contiennent de la parole** (8 % en production) : la leçon « se taire sur le bruit » déborde sur la parole faible | à corriger au prochain essai |
| 28 ter | L'invention sur le bruit, là où elle compte | la baisse mesurée porte sur des ouvertures que co-atc n'envoie déjà pas au modèle ; sur les morceaux qu'il transcrit vraiment, **pas de gain** (texte trop long : 10,7 % production, 12,1 % essai 2) | ❌ l'acquis apparent ne tient pas |

### E. Qui parle, contrôleur ou pilote

| | Approche | Ce qui a été mesuré | Verdict |
|---|---|---|---|
| 29 | Deviner à partir des mots | pas fiable, et aucune référence pour le vérifier ; une règle plus prudente n'a pas fait mieux | ⏳ gardé faute de mieux (Q39) |
| 30 | Reconnaître le contrôleur à sa voix | pas encore essayé ; demande le son de chaque fréquence séparé | 🔲 |

### F. Juger le texte lui-même, et pas seulement l'indicatif (laboratoire, D63)

Juges de `whisper-lab/scripts/labo/juges.py`, sur la sortie `-json` de `cmd/phraseology`. Règle de
production (détecteur, français sans relance, boucles coupées), bancs du 27 et du 28/09.

| | Juge | Ce qui a été mesuré | Verdict |
|---|---|---|---|
| J2 | Les valeurs lues pour l'avion rattaché, confrontées à ce qu'il affiche en ADS-B, de 30 s avant à 150 s après : niveau ou altitude sélectionnés à 100 ft près (ou altitude baro à 300 ft), cap sélectionné à 5°, vitesse indiquée à 10 kt, squawk, QNH à 1 hPa. Le hasard : la même valeur confrontée aux autres avions à moins de 60 NM | **70 % des valeurs vérifiables justes le 27/09, contre 14 % par hasard** (74 sur 106 ; niveaux 84 %) ; **82 % contre 15 % le 28/09** (94 sur 114) | ✅ premier juge des chiffres au-delà de l'indicatif |
| J2 bis | Des valeurs impossibles : un nombre suivi de son mot de rôle avalait l'indicatif dit juste avant (« KLM One Four Zero Five one eighty knots » → vitesse 1405180) | la grammaire garde désormais la partie plausible du nombre et rend le reste à la lecture des indicatifs (29/09). Mesuré par J2 : impossibles 39 → 11 sur 149 (27/09) et 33 → 9 sur 153 (28/09) ; **valeurs justes nettes du hasard 59,5 → 68,1 et 76,9 → 82,0**. Les dernières, des QNH tronqués ou des vitesses à un chiffre (« QNH three »), sont du texte mal entendu : elles gardent leurs chiffres mais perdent leur rôle, comme un niveau de vol impossible le faisait déjà ; il n'en reste aucune. Rattachements : 1 gagné, 0 perdu, 0 changé | ✅ en service au prochain lancement |
| J3 | La part des mots que ni la grammaire ni un lexique de phraséologie et de politesse ne classent : un indicateur du charabia | anglais 16,5 et 17,4 %, français 28,0 et 28,9 % ; « ne donner que la parole » (Q41) n'y change rien (18,0 % contre 18,2 à 19,0 %) | ❌ **ne passe pas son étalonnage** sur les 60 transmissions annotées (J4) : Spearman −0,25 contre la précision annotée, pour un seuil de 0,5. Reste un indicateur grossier |
| J4 | **Le texte jugé par le propriétaire** : 60 transmissions de De Gaulle annotées à l'aveugle (40 du banc du 28/09, 20 du 24/09). Précision : part des mots écrits qu'il a entendus, un plancher, car 50 annotations sur 60 sont partielles. Rappel : part de ce qu'il a entendu que le texte contient | **Lecture anglaise : précision 31 %, rappel 55 %** (1 375 mots écrits pour 762 entendus). Parole anglaise (33 transmissions) : 30 % / 69 %. Parole française (19) : lecture anglaise 36 % / 41 %, lecture française 30 % / 46 %. Sur les 3 transmissions que le propriétaire n'a pas comprises, 11, 7 et 18 mots écrits quand même. **Texte affiché par co-atc** (anglais toujours, français seulement quand il a nommé l'avion, soit 3 transmissions sur 60) : parole anglaise 30 % / 69 %, parole française 31 % / 46 %, mixte 21 % / 45 %, pratiquement la lecture anglaise seule. **Deux mots écrits sur trois ne correspondent à rien d'entendu.** Mais **l'oreille n'est pas la vérité** : passées au même juge ADS-B, sur les 40 transmissions du 28/09, les annotations nomment 9 avions confirmés, contre 13 pour le modèle (7 que seul le modèle trouve, 3 que seule l'oreille trouve, 6 en commun dont 2 en désaccord) ; valeurs vérifiables justes : oreille 4 sur 6, modèle 3 sur 9. J4 mesure donc l'accord avec l'oreille, et sa précision est doublement un plancher : une partie des mots « non entendus » sont justes, surtout les indicatifs | ✅ mesure de référence (29/09, laboratoire) ; réserve : 60 transmissions |

## Ce qui reste à tester, par ordre d'intérêt

1. ~~**L'aiguillage vers le modèle français**~~ : fait, en production depuis le 28/09 (9 ter), avec
   la lecture sans relance et la coupe des boucles. La suite, mesurer et améliorer le texte
   lui-même, est le plan du laboratoire (`whisper-lab/PLAN-LABO.md`, D63).
2. **Un quatrième essai d'entraînement, s'il vaut la peine** : repartir de l'essai 2 (le modèle
   entier), avec **beaucoup moins de clips de silence** pour qu'il ne se taise plus sur la parole
   faible, un apprentissage plus doux, puis le vocabulaire de Paris (vos indicatifs par synthèse
   vocale). Aucun des trois essais ne bat la production, et leur baisse d'invention sur le bruit
   est déjà obtenue par le détecteur de voix : c'est un chantier de fond, pas un gain rapide.
3. **Un jeu de test fiable** : une centaine de transmissions des fréquences visées, transcrites en
   entier, avec l'aide de l'outil d'annotation. Sans lui, on ne juge que l'indicatif.
4. **Le squelch des deux fréquences faibles**, essayé avec retour automatique et jugé sur les
   compteurs de la station.
5. **Ne donner au modèle que la parole** : mesuré le 25/09 (Q41) — l'invention baisse nettement,
   les avions justes ne gagnent rien de mesurable, et la fréquence faible en perd. À reprendre
   avec un détecteur de voix plus sensible pour ce tri, jugé sur un autre jour.
6. ~~**Garder les deux modèles en mémoire**~~ : fait le 14/10 (7f35308, D81), avec les deux voies du
   sidecar : première lecture 9,2 s → 1,0 s en médiane (ligne 9 quater). L'attente derrière le
   modèle français faisait perdre des transmissions aux heures chargées (6 le matin du 25/09).
7. **Le nettoyage du son par filtres**, mesuré hors ligne avant tout réglage de la station (Q42).
8. **Le locuteur par la voix**.
9. **Le modèle « turbo »**, plus rapide, jamais comparé sur l'ADS-B.

## Ce qu'on a appris en chemin

- **Compter les transmissions gagnées et perdues, pas seulement le total.** Une petite différence de
  son fait changer d'avis le modèle sur une transmission rattachée sur trois (44 sur 385, canal
  sans squelch, 28/09). Un écart de quelques avions sur un total ne dit rien tant qu'on n'a pas
  compté les gagnées et les perdues, et fait un test du signe
  (`whisper-lab/scripts/labo/paire-canal-ouvert.py`).

- **Juger sur la vraie station.** L'imitation a flatté le premier mini-essai ; seule la station a
  montré qu'il perdait les indicatifs de Paris.
- **Les erreurs qui coûtent sont dans les chiffres et dans l'invention**, pas dans l'orthographe.
- **Un système qui répond toujours est pire qu'un qui s'abstient** : c'est pourquoi chaque essai
  se juge avec le contrôle par ciel mélangé et sur sa précision, pas seulement sur le volume.
- **Le mélange de quatre fréquences dans un seul flux coûte trois avions justes sur quatre.**
