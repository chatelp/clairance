# Fiche N — Le sidecar de transcription hors du Mac : Linux et NVIDIA, repli processeur

*Écrite le 26/10/2026. Plan : `docs-fr/35-plan.md`, étapes E1 et G1. Décision : Q54 dans
`docs-fr/05-decisions.md` (cible tranchée par le propriétaire le 12/10). Lis d'abord les règles
communes de `README.md`.*

## Pourquoi

Le sidecar (`sidecar/whisper_server.py`, 761 lignes) transcrit avec **MLX**, donc sur Apple Silicon
seulement (D10). Pour que Clairance s'installe ailleurs (le jalon « installable ailleurs » du plan),
il doit tourner sur **Linux x86-64 avec une carte NVIDIA**, par **faster-whisper / CTranslate2**,
avec un **repli sur le processeur** (int8) dans la même pile. Windows viendra ensuite, avec la même
pile (Q54). Le serveur Go ne doit rien voir de la différence : **le contrat HTTP du sidecar ne change
pas.**

## Ce que tu as, et ce que tu n'as pas

Ni GPU, ni audio réel, ni la station. Tu travailles sur **processeur**, avec un **petit modèle**
(`tiny` ou `base`, anglais) pour les tests, et de l'audio **synthétique** ou des fichiers publics
libres de droits (par exemple des voix de synthèse que tu génères). La mesure de non-régression sur
le vrai modèle et le vrai GPU se fera sur le Mac et au labo, pas chez toi.

## Le contrat à garder (lis-le dans le code, ne le devine pas)

- `POST /transcribe` : PCM 16 bits brut, en-têtes `X-Sample-Rate`, `X-Channels`, `X-Language`,
  `X-Frequency-Id`, `X-Created-At`, `X-Segment-At`, `X-Second` ; réponse JSON (`text`, `language`,
  `duration`, `speech_seconds`, `rejected`, `realtime_factor`, `second_text`, `second_language`,
  `second_pending`).
- `GET /second/{id}` : la seconde lecture demandée plus tard.
- `GET /health` : toutes les clés lues par `internal/transcription/sidecar.go` (`Health`) et par
  `internal/api` (`status`, `degraded`, `pid`, `backend`, `queue`, `second_pending`, `archiving`,
  `models`…). Le test `internal/api/vitals_test.go` et `server_state_test.go` montrent ce qui est lu.
- Les deux voies de la file (`sidecar/lanes.py`), la détection de voix (silero-vad), la coupe des
  boucles (`loops.py`), l'archive des extraits (`--save-audio`).

## Les étapes (un commit chacune, sur `cloud/n-sidecar-linux`)

1. **Une interface de moteur.** Isoler dans `sidecar/` ce qui appelle MLX (`decode`, `load`,
   `detector`, `french_probability`, le préchargement) derrière une petite interface : charger un
   modèle, transcrire un tableau numpy à 16 kHz avec une langue, dire le nom du calcul (`backend`).
   MLX reste le moteur par défaut sur Apple Silicon. **Aucun changement de comportement sur le Mac** :
   les tests existants (`test_*.py`) passent sans retouche.
2. **Le moteur faster-whisper.** Choisi par une option (`--engine faster-whisper`, ou automatiquement
   quand MLX n'est pas importable), `device` `cuda` si disponible sinon `cpu`, `compute_type` `float16`
   sur GPU, `int8` sur processeur. Mêmes options de décodage que le moteur MLX quand elles existent
   (langue, température, `condition_on_previous_text`, etc. : lis ce que `decode` passe aujourd'hui) ;
   dis dans le rapport ce qui n'a pas d'équivalent.
3. **Les modèles.** Le modèle anglais de production est `sfabriece/whisper-large-v3-atco2-asr-mlx`
   (une conversion MLX). Trouve le modèle d'origine qu'il convertit (les cartes de modèle sur Hugging
   Face le disent) et écris **la commande de conversion en CTranslate2** (`ct2-transformers-converter`)
   dans `sidecar/README.md`, sans pousser de poids dans le dépôt. Même chose pour le détecteur français
   (`whisper-large-v3-turbo`). Si une conversion publique existe déjà, cite-la.
4. **Les dépendances.** `sidecar/requirements.txt` séparé par moteur (`requirements-mlx.txt`,
   `requirements-ct2.txt`, ou des extras), versions fixées ; une ligne d'installation pour chacun dans
   `sidecar/README.md`.
5. **Les tests.** Sur processeur avec `tiny` : le contrat de `/transcribe` (réponse, rejet sans parole,
   langue sans modèle), de `/health` (chaque clé lue par le serveur), de `/second/{id}` ; un test qui
   lance le serveur Go (`go test ./internal/transcription/...` contre ton sidecar, si un test existant
   s'y prête) ou, à défaut, un script qui envoie un PCM synthétique et vérifie la réponse. Mesure et
   écris dans le rapport : temps de chargement, facteur temps réel sur processeur pour 10 s d'audio,
   mémoire.

## Ce qui ne doit pas bouger

Le comportement sur le Mac (MLX), le contrat HTTP, les noms d'options déjà utilisés par
`configs/config.toml.example`, l'archive (`--save-audio`) et ses manifestes, lus par
`internal/audioarchive`.

## Le rapport (`N-rapport.md`)

Les commits ; la commande d'installation et de lancement sur Linux, avec et sans GPU ; les modèles et
leur conversion ; ce qui n'a pas d'équivalent entre MLX et faster-whisper ; les mesures sur processeur ;
ce qu'il reste à mesurer sur un vrai GPU (non-régression de l'appariement à 95 %, D73).
