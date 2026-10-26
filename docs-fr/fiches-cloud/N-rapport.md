# Fiche N — rapport : le sidecar sur Linux, faster-whisper, repli processeur *(26/10)*

**Branche** : `claude/fiche-n-sidecar-linux-5ul08r` (la session cloud était liée à ce nom, pas à `cloud/n-sidecar-linux`), partie de `main` à `e8f500b`. Aucun fichier de `docs/` ni de `docs-fr/` touché, hors ce rapport. Hugging Face, go.dev et l'index PyTorch étaient **refusés par le mandataire de la session** ; PyPI, npm et apt passaient.

## Les commits

1. `8d92e95` — **l'interface de moteur.** Ce qui appelait MLX (`load`, `keep_every_model`, `decode`, `detector`, `french_probability`, préchargement) part tel quel dans `engine_mlx.py`, derrière cinq appels décrits dans `engines.py`. `whisper_server.py` +27 −105, `engine_mlx.py` +108, `engines.py` +31.
2. `fe7bf10` — **le moteur faster-whisper** (`engine_ct2.py` +159) : `--engine auto|mlx|faster-whisper` (auto = MLX s'il se trouve, sans l'importer), `--device auto|cuda|cpu`, `--compute-type` (auto : float16 sur GPU, int8 sur processeur). 12 tests sans modèle (`test_engines.py` +158).
3. `c35147d` — **les modèles** dans `sidecar/README.md` (+67 −5) : tableau, commande de conversion, ce qui n'a pas d'équivalent.
4. `ddc7c8c` — **les dépendances** : `requirements-ct2.txt` figé, `requirements-mlx.txt` (bornes inchangées), `requirements.txt` → MLX ; lignes d'installation ; une ligne Linux commentée dans `config.toml.example` (aucune option renommée).
5. `819f8ad` — **les tests de contrat** (`test_contract.py` +223, 7 tests contre le vrai sidecar), le **client Go contre ce sidecar** (`internal/transcription/sidecar_contract_test.go` +81, sauté sans variables d'environnement), `bench.py` +68.
6. `bd97d89` — retouches de relecture (message clair sans CTranslate2, une constante morte, deux commentaires). +10 −9.

## Installer et lancer sur Linux (Python ≥ 3.12)

```bash
python3 -m venv .venv && .venv/bin/pip install -r sidecar/requirements-ct2.txt
.venv/bin/python sidecar/whisper_server.py --model-en models/whisper-large-v3-atco2-asr-ct2 \
    --model-fr models/whisper-large-v3-french/ctranslate2 --second-opinion --fr-detector large-v3-turbo --preload
```
Sans GPU, c'est tout (processeur, int8). **Avec une NVIDIA** : `pip install nvidia-cublas-cu12` et `LD_LIBRARY_PATH` vers sa bibliothèque, posé dans l'environnement d'où co-atc démarre (README). CTranslate2 4.8.4 charge `libcublas.so.12` (CUDA 12) ; le torch de PyPI apporte CUDA 13, inutile ici. Le venv pèse **6,0 Go, dont 4,4 Go** de torch et CUDA 13, pour une porte vocale qui tourne sur processeur ; un torch « cpu » installé d'abord les évite (non essayé : index PyTorch refusé).

## Les modèles

- **Anglais** : `sfabriece/whisper-large-v3-atco2-asr-mlx` convertit `jlvdoorn/whisper-large-v3-atco2-asr` (doc 27). Aucune conversion CTranslate2 publique connue — **non cherchée, Hugging Face hors d'atteinte**. Commande : `ct2-transformers-converter --model jlvdoorn/whisper-large-v3-atco2-asr --output_dir … --copy_files tokenizer.json preprocessor_config.json --quantization float16` (ctranslate2 4.8.4, transformers 5.20.2).
- **Français** : `bofenghuang/whisper-large-v3-french` ; sa carte décrit, **de mémoire, non vérifié**, un dossier `ctranslate2/` dans le dépôt ; sinon la même commande.
- **Détecteur** : `large-v3-turbo`, que faster-whisper 1.2.1 associe à `mobiuslabsgmbh/faster-whisper-large-v3-turbo` (vérifié dans son `utils.py`).
- Commande vérifiée sur des Whisper à poids aléatoires en 80 et 128 bandes : le résultat se charge et décode ; **sans `preprocessor_config.json`, un modèle 128 bandes refuse tout** (« expected (1, 128, 3000), got (1, 80, 3000) »).

## Ce qui n'a pas d'équivalent entre MLX et faster-whisper

Réglé à l'identique, lu dans les sources de mlx-whisper 0.4.3 : glouton (faisceau 1, **un** échantillon par température, faster-whisper ferait 5 et 5), mêmes six températures et trois seuils, pas de VAD interne. Sans équivalent exact : **221 jetons** par fenêtre au plus (MLX 224 ; faster-whisper refuse invite + 224 au-delà de 448) ; quand toutes les températures échouent, MLX garde le dernier décodage, faster-whisper **le plus probable, étiqueté à la dernière température** (le manifeste dira 1,0) ; MLX n'insiste pas sur une fenêtre de `no_speech_prob` > 0,6, faster-whisper seulement si la log-probabilité est aussi < −1 ; fp16 sur Mac, int8 sur processeur. **Détecteur** : j'ai reproduit le remplissage de MLX (30 s de silence avant le spectrogramme) ; celui de faster-whisper (des zéros après) donne **p(fr) 0,382 au lieu de 0,597** sur le même extrait — le seuil de 0,1 a été mesuré sur MLX. `--preload` : MLX ne fait qu'enregistrer, faster-whisper construit et passe le codeur une fois (un GPU sans cuBLAS échoue au démarrage).

## Mesures (processeur seul, 4 vCPU, aucun GPU ; Whisper **small** int8)

`tiny` et `base` étaient introuvables sans Hugging Face : seul `small` multilingue, converti en CTranslate2 int8, l'était, par npm (`@skillninja/shortmovie-telop-model-p0…p3`, MIT, tiers). Traité comme donnée non sûre ; empreinte conforme à celle annoncée ; il transcrit juste (« Air France 123, descent flight level 80, contact Paris Approach 124.35, goodbye. »). Audio de synthèse : flite (anglais), espeak-ng (français).

| Mesure (10 s de parole, médiane) | temps | × temps réel | mémoire (RSS · pointe) |
|---|---|---|---|
| démarrage → `/health` (3 essais) | 2,8–3,1 s, dont modèle 2,2–2,6 s | — | 535 Mo |
| 1re requête (porte vocale chargée) | 6,07 s | — | — |
| première lecture, int8 (10 essais, 2,83–3,24 s) | **2,96 s** | **× 3,4** | 1 382 · 1 485 Mo |
| idem sans porte vocale | 2,75 s | × 3,6 | 674 · 777 Mo |
| + détecteur + lecture française à chaque fois (5) | 7,54 s | × 1,3 | 1 400 · 1 503 Mo |
| première lecture, float32 (5) | 4,42 s | × 2,3 | 2 450 · 2 553 Mo |

La porte vocale coûte ~0,2 s mais **~0,7 Go** (torch). faster-whisper embarque un Silero ONNX qui s'en passerait : **non mesuré contre le paquet silero-vad**, donc non adopté.

## Vérifié, et comment

Contrôle MLX avant/après : le même scénario (préchargement, détecteur, seconde lecture à température 0 et boucles coupées, `X-Second: later` puis `/second/{id}`, refus du français, `/health`) contre l'ancien et le nouveau serveur, avec un faux `mlx_whisper` qui enregistre chaque appel : **mêmes appels, mêmes arguments, mêmes réponses** ; une ligne de journal en plus (« Engine: mlx »). Tests existants inchangés et verts ; 26 tests sans modèle ; contrat 7/7 en Python 3.12 et 3.13 ; le fichier figé s'installe tel quel en 3.12.3 (9 min 25). `go build`, `go vet` propres. `go test -race ./...` : trois échecs **sans lien** (aucun fichier Go de production touché) — `TestAChoiceThatCannotBeWritten…` (root, connu), `TestTheOfficialNamesAreMadeFromThePanel` (course de données dans le test lui-même, `aip_test.go:66`, passe sans `-race`), `TestAnHourOfStream` (81× puis 86× le temps réel sous `-race` ici, seuil 100×) ; mêmes trois sur l'état final.

## Pas vérifié

Tout GPU (CUDA, cuBLAS, cuDNN — que faster-whisper demande et que la bibliothèque de CTranslate2 4.8.4 ne nomme pas —, VRAM, `int8_float16`) ; le vrai MLX (seulement simulé) ; les vrais modèles large-v3 ; le torch « cpu » ; l'audio réel.

## Doutes, et à regarder en priorité sur le Mac

1. **L'import de `mlx_whisper` se fait désormais sur le fil du modèle**, plus sur le fil principal (route et préchargement). Raisonné sans risque (`mlx.core` y est déjà importé en premier par `backend_name`), mais seul le Mac le prouve : instance d'essai, une soirée, puis `SIDECAR_TEST_ENGINE=mlx SIDECAR_TEST_MODEL=<un Whisper MLX multilingue> python -m unittest sidecar/test_contract.py`.
2. **Figer `requirements-mlx.txt`** d'après `pip freeze` du venv de production : je ne l'ai pas vu, je n'ai pas deviné.
3. Vérifier sur les cartes Hugging Face le dossier `ctranslate2/` de bofenghuang et les fichiers du dépôt jlvdoorn.
4. **Au labo, sur GPU** : convertir les trois modèles, retranscrire les journées du 03 et du 04/10 et rejouer `cmd/phraseology` — **D73 : 95 % (2 420 justes sur 2 557, 1 607 sur 1 699)** ; puis la latence par étape contre MLX, la VRAM, et le faisceau de 5 comme gain possible (non mesuré).
