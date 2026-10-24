# Co-ATC amont — vérifié dans le code

> **Document historique — revue du 20/10.** Il décrit l'amont tel qu'il a été lu et exécuté le
> 15/09 ; le corps du texte n'est pas réécrit. Depuis le 10/10 (D71), Clairance est un produit
> à part et s'écarte de ce qui est décrit ici : le chemin OpenAI, le chat IA, la simulation
> d'avions et le Docker de l'amont sont **retirés** ; ont été **ajoutés** l'authentification
> (Argon2id, sessions, rôles), le sidecar de transcription MLX, la grammaire de phraséologie,
> l'état du serveur, les sources ajoutées en marche, une interface refaite. La liste à jour est
> dans le README (« What is different from upstream »). Deux repères ont bougé : le commit amont
> `0e4d685` cité plus bas s'appelle **`de57f07`** (même contenu, même date du 3 mai 2026) depuis
> la réécriture de l'historique du 12/10 (D75, vérifié par `git log upstream/main`), et le clone
> `upstream/` n'existe plus — l'amont se lit par le remote `upstream`.

> **Document réécrit le 15 septembre 2026** après lecture, compilation et exécution des
> sources. La version précédente venait d'une lecture de la page du dépôt ; ce qu'elle
> annonçait est confirmé, corrigé ou infirmé ci-dessous, avec la mesure à l'appui.
>
> Pour les appels à l'API OpenAI en particulier, voir `07-carte-openai.md`, plus détaillé.

Dépôt : <https://github.com/yegors/co-atc>
Clone local : `upstream/`, branche `main`, commit `0e4d685` du 3 mai 2026.

## Licence : MIT — tranché

`LICENSE`, branche `main` : « MIT License — Copyright (c) 2025 Yegor S ». Le fork est
publiable. Seule obligation : conserver la notice et le texte dans toute copie.
Voir D6 et D7 dans `05-decisions.md`.

## Ce que fait le produit

Confirmé. Supervision aéronautique croisant télémétrie ADS-B et communications VHF :
carte OpenLayers, trajectoires, détection des phases de vol et de la piste en service,
transcription, extraction de clairances, météo, assistant vocal.

La détection de phase fonctionne : mesurée sur notre trafic, elle a classé 128 avions en
`CRZ` (68), `UNK` (30), `ARR` (17), `DEP` (9), `NEW` (3) et `T/O` (1).

## Architecture — corrections

| | Annoncé | Vérifié |
|---|---|---|
| Go | « 1.21+ » | **`go.mod` exige 1.23.2.** Compilé ici avec go 1.25.6 |
| Base | SQLite, un fichier par jour | confirmé : `data/co-atc-AAAA-MM-JJ.db`, via **`modernc.org/sqlite`, pur Go — pas de cgo**, donc compilation croisée triviale |
| Audio | FFmpeg | confirmé, **et SRT natif** via `github.com/datarhei/gosrt` — *lecteur natif retiré le 10/10 (D71) : ffmpeg lit SRT lui-même, et toute source passe par lui* |
| Config | `configs/config.toml` | **le dépôt ne livre que `configs/config.toml.example`** (18 kio). Il faut le copier |
| Arborescence | `cmd/server`, `internal`, `pkg/logger`, `www`, `configs`, `assets`, `docker`, `prompts` | confirmé, **plus `docs/`, `scripts/` et `.claude/`** |
| Port | non annoncé | **8000**, pas 8080. `[server] port` |

`go vet` signale une fuite de contexte réelle dans `internal/frequencies/service.go:57`
et `:78` (« the procCancel function is not used on all paths »). Candidat à une
contribution amont, indépendante du fork.

## Un document de conception à lire absolument

**`docs/LOCAL-STT.md`** — 536 lignes, l'auteur amont a déjà conçu le remplacement de la
transcription OpenAI par faster-whisper en local. **Ce n'est qu'un plan : rien n'en est
implémenté** (ni `sidecar/`, ni option `backend`, ni processeur local). Détail et
conséquences dans `07-carte-openai.md`.

## Sources ADS-B

Cinq modes, confirmés dans `configs/config.toml.example` : `tar1090`, `readsb-api`,
`readsb-file`, `external-rapidapi`, `external-opensky`.

**Le mode `tar1090` fonctionne sur notre station sans rien adapter.** Mesuré le 15/09 :
`tar1090_base_url = "http://station.lan:8080/data/"`, validation réussie au démarrage,
128 avions suivis dont 96 positionnés.

À noter : **`external-opensky` en mode anonyme ne demande aucune clé** et fonctionne.
La configuration par défaut du dépôt l'utilise, ce qui explique qu'elle démarre telle
quelle.

## Configuration — la liste réelle des sections

```toml
[server]              # host, port (8000 par défaut)
[adsb]                # source_type et paramètres des cinq modes
[logging]
[storage]             # SQLite
[station]             # latitude, longitude, elevation_feet, airport_code, portées
[reference]           # chemins des six CSV de référence
[frequencies]         # réglages ffmpeg communs
[[frequencies.sources]]  # une entrée par fréquence
[transcription]       # clé OpenAI, modèle, réglages ffmpeg et VAD
[post_processing]     # section distincte : modèle, lot, intervalle
[flight_phases]       # seuils de détection des phases
[wx]                  # météo Windy
[atc_chat]            # clé OpenAI de l'assistant vocal
```

Deux corrections par rapport à la version précédente de ce document : `[transcription]`
ne contient pas que la clé d'API, et **le post-traitement est une section à part**.

> ⚠️ **`elevation_feet` est en PIEDS.** C'est exactement le piège d'unités décrit dans
> `01-station.md`. Les 152 m de la station donnent **499 ft**. Écrire 152 ici placerait
> l'antenne à 46 m.

La configuration par défaut vise bien **Toronto Pearson (CYYZ)** : `latitude = 43.6777`,
`longitude = -79.6248`, `elevation_feet = 569`.

## Données de référence : l'affirmation la plus fausse de la version précédente

Le document annonçait qu'« il faudra fournir les données d'Île-de-France ». **C'est faux.**

`assets/` est **livré peuplé dans le dépôt** (52 Mio) et les données sont **mondiales** :
`airports.csv`, `runways.csv`, `navaids.csv` et `airport-frequencies.csv` viennent
d'OurAirports, `aircraft.csv` de tar1090-db, `airlines.dat` d'OpenFlights.

Vérifié terrain par terrain :

| | présent | pistes |
|---|---|---|
| LFPG, LFPO, LFPB | oui | 4, 3, 3 |
| LFPZ, LFPX, LFPN | oui | 2, 2, 2 |
| LFPV | oui | 1 |

Au démarrage sur notre position : **307 aérodromes et 43 balises dans les 100 NM,
67 pistes, dont les 2 de LFPZ comme terrain de référence.** La fréquence ATIS de
Saint-Cyr (131,025) figure même dans `airport-frequencies.csv`.

**Rien à fournir.** Il restera à vérifier la justesse des données françaises, pas à les
créer.

## Dépendances externes

| Dépendance | Rôle | Sort |
|---|---|---|
| **API OpenAI** | transcription, post-traitement, clairances, assistant vocal | **à remplacer** — voir `07-carte-openai.md` |
| **Windy** (`node.windy.com/airports`) | METAR, TAF, NOTAM | à remplacer (Q7). Fonctionne sans clé ; **renvoie 204 sur LFPZ**, qui n'a pas de METAR — confirmation empirique de Q9 |
| OpenSky | source ADS-B alternative | inutile ici |

**Aucune bibliothèque cliente OpenAI dans `go.mod`** : tous les appels sont en
`net/http` et `gorilla/websocket` écrits à la main.

## Sécurité

Confirmé dans le README : pas d'authentification, pas d'autorisation, pas de durcissement,
et l'auteur déconseille explicitement l'exposition sur Internet. **On garde la
contrainte** (D5).

À noter : `[server] host` vaut `127.0.0.1` par défaut. Le service n'écoute que sur la
boucle locale tant qu'on ne le change pas — il faudra `0.0.0.0` pour y accéder depuis une
autre machine du réseau local, et c'est le moment où la question de l'exposition se pose
concrètement.

## Comportement sans aucune clé d'API — mesuré

Lancé avec `configs/config.toml.example` copié tel quel, sans aucune clé :

**Ce qui marche.** Démarrage complet, aucun plantage. Schéma SQLite créé. 618 428 avions
et 6 972 compagnies chargés en 0,3 s. ADS-B OpenSky anonyme : avions détectés en 2 s.
Météo Windy : 3 récupérations réussies sur CYYZ. Interface web servie, WebSocket
connecté, carte peuplée.

**Ce qui se désactive proprement**, avec un avertissement explicite au démarrage :
transcription, post-traitement, assistant vocal. Les trois disent pourquoi.

**Ce qui casse.** Les sources audio par défaut, toutes distantes : les flux LiveATC
répondent `EOF` immédiatement et la source SRT expire. Co-ATC relance alors ffmpeg
**toutes les 4 à 5 secondes, indéfiniment**, trois processus en parallèle,
`total_bytes_processed: 0` à chaque fois. Ce n'est pas fatal mais c'est une boucle sans
recul de cadence. **À surveiller pour nous** : une fréquence muette n'est pas une source
morte, et il faudra vérifier que Co-ATC ne confond pas les deux sur un flux Icecast
silencieux.

## Ce qui reste à vérifier

- Rechargement à chaud de la configuration des fréquences (Q3) — pas encore cherché.
- Comportement de ffmpeg sur un flux Icecast qui se tait plusieurs minutes.
- Justesse des pistes et des navaids français d'OurAirports.
