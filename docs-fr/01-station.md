# La station de réception

*Relevé le 15 septembre 2026 sur la machine. La référence d'exploitation complète vit
dans le projet Claude « Radio » du propriétaire ; ce document en extrait ce dont
Clairance a besoin.*

## Machine

**`station`, station.lan** — Mac mini 2014, Intel i5-4278U (Haswell, 2 cœurs /
4 fils, AVX2), 7,6 Gio de RAM, disque 219 Go rempli à 21 %. **Fedora Linux 44**, sans
écran, en fonctionnement continu. Accès `ssh pierre@station.lan` sans mot de passe.

**Position : 48.81 / 2.05 (à deux décimales dans les documents publics, D75 ; la valeur exacte est dans la configuration locale), antenne à 152 m** (sol IGN 139,0 m + 13 m au-dessus du
sol). Fontenay-le-Fleury, Yvelines.

Ces trois valeurs sont celles à mettre dans `[station]` de la configuration Co-ATC.
Attention : **152 m est une altitude au-dessus du niveau de la mer**, pas une hauteur
d'antenne au-dessus du sol (qui vaut 13 m). Les deux champs existent selon les outils et
la confusion a déjà coûté une erreur de configuration chez FlightAware.

**Machine de développement : un Mac mini M4, 192.0.2.28.** C'est là que Clairance
tournera. Les deux machines sont sur le même réseau local, mais la station est câblée
dans une borne mesh dont le lien de retour est radio : **8,7 ms de latence et 68 Mbit/s
utiles** vers la station, pas du gigabit. Des boîtiers CPL sont commandés pour corriger
ça. Dimensionner en conséquence : quelques centaines de kbit/s d'audio passent sans
problème, un flux IQ brut à 16 Mbit/s non.

> *À re-vérifier côté station (revue du 20/10)* : la latence, le débit et l'arrivée des
> boîtiers CPL datent du 15/09 et n'ont pas été remesurés depuis.

## Deux chaînes indépendantes, deux antennes, deux clés

| Chaîne | Clé RTL-SDR | Statut |
|---|---|---|
| **ADS-B 1090 MHz** | index `0`, série `1090` | en continu, **jamais interrompue** |
| **VHF aéro 118-137 MHz** | index `1`, série `vhf` | RTLSDR-Airband **ou** SDR++ à distance, jamais les deux |

> ⚠️ **La chaîne ADS-B ne doit jamais être touchée.** Elle alimente FlightAware et
> Flightradar24 en continu, **et OpenSky Network depuis le 30/09** (service `opensky`,
> ajouté par l'agent de la station avec l'accord du propriétaire, MLAT comprise, position
> masquée ; les neuf services existants n'ont pas été redémarrés — compte rendu dans
> `station/echanges/station-vers-atc-scribe-2026-09-30-opensky.md`), et participe à la
> multilatération avec 161 récepteurs voisins. Une erreur de position y injecte des
> microsecondes d'erreur chez tous les autres.

## Ce que la station expose déjà

Onze services Docker dans `/opt/adsb/docker-compose.yml`, neuf en permanence ; **douze et dix
depuis le 30/09**, avec `opensky`.
Les noms d'hôte `*.lan` sont résolus par un Pi-hole en 192.0.2.2 — **depuis le M4 ils
fonctionnent ; depuis un conteneur ou une machine tierce, utiliser l'IP.**
*À re-vérifier côté station (revue du 20/10) : le nombre de services et leur état.*

| Ce dont Co-ATC a besoin | Où le prendre |
|---|---|
| **ADS-B temps réel** | `http://station.lan:8080/data/aircraft.json` — c'est un tar1090/readsb standard, avec `receiver.json` et `stats.json` à côté |
| **Carte tar1090** | `http://adsb.lan/` |
| **Audio VHF** | Icecast, `http://audio.lan/aero.mp3` (brut) et `/aero-clair.mp3` (filtré voix) — **voir la réserve majeure plus bas** |
| **État de la chaîne VHF** | `http://station.lan/radio/etat` **avec l'en-tête `Host: station.lan`** |

> ⚠️ **Piège vérifié** : la route `/radio/*` n'est déclarée que dans le bloc
> `http://station.lan` du Caddyfile. Une requête sur l'IP renvoie **200 avec un
> corps vide**, ce qui n'a pas l'air d'une erreur. Toujours forcer l'en-tête `Host`.

> ⚠️ **Icecast n'est pas exposé sur le port 8000 de l'hôte.** Depuis la station,
> `http://localhost:8000/` répond « connection refused ». Tout passe par Caddy.

## L'obstacle principal : un flux, pas N flux

> **Réglé (revue du 20/10).** La section qui suit est écrite au futur, comme l'état du
> 15/09 ; elle est **faite**. La station diffuse **un flux Icecast par fréquence** (flux
> séparés en service dès le 26/09), et Clairance les suit depuis le 28/09 (D57, D64, Q45) :
> `radio-ctl-sync` lit l'état de la station et donne à co-atc, pour chaque fréquence, le
> `flux_url` publié par `/radio/etat`, sans jamais basculer la station. Le mélangeur
> `aero.mp3` continue de fonctionner pour l'écoute, mais n'est plus transcrit. L'implantation,
> les mesures de charge et le piège des sorties fichier ci-dessous restent valables comme
> raisonnement ; ce qui s'y présente comme à faire ne l'est plus.

**C'est la première chose à traiter, et elle vient avec une contrainte ferme.**

RTLSDR-Airband tourne en **multicanal** : il démodule 5 à 7 fréquences en parallèle
(aucun saut, donc aucun hachage) puis **les mélange dans un seul flux stéréo**, chaque
canal ayant sa position dans l'image gauche-droite. C'est un choix fait pour l'écoute
humaine : on entend tout à la fois et la position stéréo dit quelle fréquence parle.

Co-ATC attend l'inverse : une entrée `[[frequencies.sources]]` **par fréquence**, donc
un flux par fréquence.

### La contrainte : ne rien casser

> **Le mode par canal ne remplace pas le mélangeur, il s'y ajoute.** Le propriétaire
> écoute sa station tous les jours et le flux mélangé `aero.mp3` doit continuer de
> fonctionner exactement comme aujourd'hui, avec sa répartition stéréo et son filtre
> voix. Le mode par canal est un **troisième mode de diffusion, activable et
> désactivable à la demande**, au même titre que la bascule existante entre
> « à l'antenne » et « exploration ».

La station a déjà cette mécanique : `radio-ctl` expose `/radio/on` (rtl_tcp),
`/radio/spy` (SpyServer) et `/radio/off` (retour à l'antenne), et le pupitre du tableau
de bord peint les boutons correspondants. **Le mode par canal est un quatrième état à
brancher dans cette même machine**, pas un système parallèle.

### L'implantation recommandée

Chaque bloc de canal de rtl_airband peut porter plusieurs sorties. Le gabarit décrit
aujourd'hui une sortie `type = "mixer"` par canal. Il suffit d'**ajouter** une sortie
`type = "icecast"` par canal, avec son propre `mountpoint`, quand le mode est actif.

Deux façons de le faire, à arbitrer (`05-decisions.md`, décision D2) :

- **Dupliquer les gabarits** en variantes `<mode>-parcanal.tmpl`. Simple, mais double le
  nombre de fichiers et fait diverger deux sources de vérité pour les mêmes fréquences.
- **Injecter les sorties à la génération.** `radio-ctl` lit déjà le gabarit et y
  substitue le mot de passe Icecast avant d'écrire `rtl_airband.conf` ; il peut tout
  aussi bien y insérer les sorties par canal selon un drapeau. **Une seule source de
  vérité pour les fréquences, un seul endroit à maintenir.** C'est l'option
  recommandée.

Convention de nommage suggérée pour les points de montage : `/ch-118700.mp3`, dérivé de
la fréquence — stable, sans ambiguïté, et directement exploitable pour associer un flux
à une entrée de configuration Co-ATC.

Deux choses à mesurer avant de le faire :

- **la charge CPU.** Sept encodeurs LAME continus **en plus** du mélangeur, sur deux
  cœurs Haswell déjà à ~20 %. Airband tourne actuellement à ~10 % avec 7 canaux. Une
  sortie Icecast continue n'a rien à voir avec une sortie fichier (voir le piège
  ci-dessous), mais ça se mesure — et c'est précisément pour ça que le mode doit être
  désactivable.
- **le débit.** 7 × 64 kbit/s = 448 kbit/s en plus, sur un lien qui en offre 68 000.
  Sans objet.

**Contrôle obligatoire après toute modification de la chaîne audio** : le propriétaire a
une règle, `salves.py`, qui mesure la structure des salves du flux mélangé. Une bonne
configuration donne une **médiane de 3 à 5 secondes et 0 % de salves sous une seconde**.
Si l'activation du mode par canal dégrade ce chiffre, le mode est mauvais, quelle que
soit l'impression à l'oreille. Comparer à une référence prise **à la même heure**.

> ⚠️ **Le piège des sorties fichier.** Donner à un canal une sortie fichier *par
> transmission* (`split_on_transmission`) en plus du mélangeur **fragmente le flux en
> direct** : RTLSDR-Airband ouvre et ferme un encodeur LAME à chaque ouverture de
> squelch, dans le fil de sortie audio. Mesuré : médiane de salve 4,6 s → **0,1 s**, et
> 0 % → 64 % de salves sous la seconde, **avec le processeur à 6 %**. Ce n'est pas un
> problème de puissance mais de latence. Les sorties Icecast sont continues et ne
> devraient pas souffrir de ça — mais c'est une hypothèse, pas une mesure : vérifie-la.

## Les groupes de fréquences

*À re-vérifier côté station (revue du 20/10) : la table des groupes est celle du 15/09 ; les
groupes et leurs fréquences ont pu changer depuis (le groupe `amas-132-133` a déjà été
renommé, plus bas).*

La station ne peut pas tout écouter : **une clé RTL-SDR ne voit que 2,56 MHz d'un coup,
la bande aéro en fait 19**. Le propriétaire a donc dix « groupes d'écoute », des
gabarits dans `/opt/adsb/modes/*.tmpl`, entre lesquels il bascule.

| Groupe | Centre | Fréquences |
|---|---|---|
| `decollages-atterrissages` | 118,7125 | 118,000 · 118,150 · 118,700 · 118,925 · 119,250 · 119,425 |
| `le-bourget` | 119,0000 | 118,150 · 118,883 · 118,892 · 118,925 · 119,850 |
| `de-gaulle-sol` | 121,3710 | 120,900 · 121,150 · 121,608 · 121,842 |
| `orly-approche` | 124,9042 | 123,875 · 124,350 · 124,625 · 125,825 · 125,933 |
| `gros-porteurs` | 125,3900 | 124,350 · 124,625 · 125,825 · 126,425 |
| `paris5` | 128,6400 | 127,750 · 127,850 · 128,142 · 128,950 · 129,525 |
| `paris5-avec-enregistrement` | 128,6400 | idem + un fichier par transmission |
| `en-route-et-descente-cdg` | 132,7625 | 132,275 · 132,500 · 132,733 · 132,783 · 132,825 · 133,000 · 133,250 |
| `en-route` | 135,8375 | 135,400 · 135,967 · 136,275 |
| `fixe136` | 135,8005 | 136,275 seule |

> **Le groupe `amas-132-133` s'appelle `en-route-et-descente-cdg` depuis le 16 septembre.**
> Le mot « amas » disait ce qu'on ignorait : sept fréquences vues au balayage dans moins
> d'un mégahertz, sans rien qui les relie. La nuit du 15 au 16 les a identifiées canal par
> canal — voir `24-identification-amas.md`. Les documents datés d'avant le 16 gardent
> l'ancien nom, qui était le bon ce jour-là ; la station, elle, ne le connaît plus.

**Conséquence pour Co-ATC** : la liste des fréquences n'est pas figée, elle change quand
le propriétaire bascule de groupe. Trois attitudes possibles, à arbitrer —
*(tranché : la deuxième, hors de co-atc, par `radio-ctl-sync` qui suit l'état de la station
et ajoute ou retire les sources en marche, D36, D57 ; revue du 20/10)* —

- déclarer toutes les fréquences dans la config et accepter que la plupart soient
  muettes selon le groupe actif ;
- interroger `/radio/etat` au démarrage et se configurer dynamiquement ;
- ne gérer qu'un groupe à la fois et recharger sur changement.

La route `/radio/etat` renvoie déjà le mode actif, la liste de ses fréquences, et un
champ `accord` qui dit si ce qui sort d'Icecast correspond bien au gabarit chargé.

## Le catalogue des fréquences

Le catalogue (`06-catalogue.csv`) compte **68 lignes, dont 31 identifiées** ; le « 52 fréquences
identifiées » écrit ici le 15/09 ne correspond à aucun décompte du fichier, Q10 reste ouverte
(revue du 20/10). Les principales pour l'usage :

| MHz | Service | Langue attendue |
|---|---|---|
| 118,000 | Saint-Cyr Tour (LFPZ), 2 km | français dominant |
| 118,700 | Orly Tour | bilingue |
| 118,925 | Le Bourget Tour (LFPB) | bilingue |
| 119,425 | Villacoublay Approche (LFPV) | français dominant |
| 120,750 | Toussus Tour (LFPN) | français dominant |
| 123,875 | Orly Approche | bilingue |
| 124,350 · 125,825 · 126,425 · 118,150 · 119,850 · 121,150 | Approche De Gaulle **et Le Bourget** (secteurs partagés) | anglais dominant |
| 124,625 | Paris Contrôle, secteur DG | anglais dominant |
| 127,750 | Orly Départs | bilingue |
| 127,850 | Reims Contrôle, secteur KN | bilingue |
| 128,950 | Villacoublay Tour (LFPV), base militaire | **français**, VFR |
| 129,525 | Chavenay-Villepreux Tour (LFPX), 6,5 km, aéroclub | **français**, très bavarde |
| 133,375 · 136,275 | De Gaulle Approche | anglais dominant |
| 135,400 | Paris Contrôle, secteur DO | anglais dominant |

**Balises permanentes à exclure de toute transcription** : 131,025 (ATIS Saint-Cyr,
+49 dB, émet en continu), 127,475 (ATIS Toussus), 128,225 et 127,125 (ATIS De Gaulle),
120,000 (ATIS Le Bourget), 125,275 (ATIS Chavenay), 132,742 (porteuse permanente non
identifiée). Un ATIS est une boucle enregistrée : elle ne ferme jamais le squelch et
n'apporte rien au trafic, sauf à vouloir en extraire le contenu — idée séparée, traitée
en Q9 et Q32.

> **Corrigé le 21/09** : ce paragraphe disait que l'ATIS de Saint-Cyr « sature ».
> Mesuré — il ne sature pas, il se transcrit au gain quotidien. En revanche la boucle de
> 21 h ne porte **pas** de météo : ni vent, ni QNH, ni piste en service. Détail en Q32.

**Quinze fréquences actives ne sont dans aucune liste publique** et restent à
identifier ; sept d'entre elles sont en cours d'enregistrement nocturne. *(Dépassé en
partie : les sept canaux de la tranche 132-133, ceux qu'on enregistrait la nuit, ont été
examinés un à un le 16/09 et leur nature établie, `24-identification-amas.md` ; ce paragraphe
date du 15/09 et le décompte du reste est celui de Q10 ; revue du 20/10.)* Le catalogue
complet avec niveaux de crête et de médiane est dans l'artefact « Plan de bande aéro »
du propriétaire.

## Deux pièges de mesure qui ont déjà coûté cher

**Le critère qui sépare une balise d'un secteur saturé n'est pas la médiane, c'est son
écart à la crête.** Une balise permanente a une médiane collée à sa crête (131,025 :
+49,4 / +48,7, écart 0,7 dB). Un secteur très occupé a aussi une médiane haute mais une
crête loin au-dessus (126,425 : +37,0 / +27,7, écart 9,3 dB — c'est de la voix). Trois
fréquences ont été classées à tort comme porteuses sur la foi de la seule médiane.

**Deux canaux espacés de 8,33 kHz ne se séparent pas.** Avec `fft_size = 512` à
1,024 Msps, le canaliseur de RTLSDR-Airband a des cases de 2 kHz : le plus fort déborde
sur le plus faible et **la même émission est comptée deux fois**. Une fréquence entière
(133,241667) a été inventée comme ça avant d'être retirée du catalogue. Le test qui
tranche est une matrice de coïncidence temporelle : 74 % des transmissions de l'une
avaient une jumelle à moins de 1,5 s sur l'autre, contre 2 à 45 % pour toutes les autres
paires.

## Un parasite qui fausse tout entre 22 h et 7 h

Un appareil du logement branché sur le secteur rayonne assez fort pour **ouvrir le
squelch sur toute la tranche 132-133 MHz**, de 22 h à 7 h. Signature : peigne
d'harmoniques espacées de **100 Hz** (le double du 50 Hz secteur), identique sur quatre
fréquences à la fois, **aucune parole**, absent le jour. Les horaires sont ceux du tarif
heures creuses.

**Conséquence directe pour toi** : entre 22 h et 7 h, jusqu'à **98 % des déclenchements
de squelch ne contiennent pas de parole**. Contre 18 à 34 % en journée, qui est le
plancher normal d'un squelch à seuil 10. Une chaîne de transcription qui avale tout ce
qui déclenche va donc brûler l'essentiel de son calcul sur du bruit, la nuit.

> **Mesuré sur une nuit entière le 16/09** — 4 h continues, sept canaux, 4 126
> ouvertures : **88,1 % sans parole en moyenne, mais de 7,7 % à 99,1 % selon le canal.**
> Le « jusqu'à 98 % » est juste, et c'est bien une propriété de **quatre** fréquences
> — 132,733, 132,825, 133,000, 133,250 — qui sont exactement celles où l'analyse
> spectrale retrouve le peigne à 100 Hz décrit ci-dessus. Les trois autres canaux de
> ce groupe portent de la parole, dont un, **132,783, à 92 %**. Détail dans
> `21-nuit-132-133.md`.

**Il faut un détecteur de voix en amont du modèle.** Voir `03-transcription.md`.

## Les grandeurs qui comptent

- Une transmission de contrôle dure **2 à 8 secondes**. Une tour d'aéroclub bavarde
  peut aller à 44 s — c'est ce chiffre aberrant qui a permis d'identifier Chavenay.
- Une fréquence de contrôle ordinaire tourne à **3 à 5 % d'occupation**, soit 130 à
  260 secondes de parole par heure. Au-delà de 20 %, se méfier : soit le secteur est
  saturé, soit ce n'est pas de la parole.
- Le nombre d'avions suivis en ADS-B est un **témoin croisé** précieux : 14 avions dans
  le ciel à 4 h du matin ne peuvent pas produire 35 % d'occupation sur trois fréquences.
  C'est ce croisement qui a démasqué le parasite.
