# Local STT sidecar

Implements the HTTP contract described in upstream's `docs/LOCAL-STT.md` — designed
there, never built. The Go side posts raw PCM, gets a transcript back, and no audio
leaves the machine.

```bash
python3 -m venv .venv
.venv/bin/pip install -r sidecar/requirements-mlx.txt    # Apple Silicon (requirements.txt is the same)
.venv/bin/pip install -r sidecar/requirements-ct2.txt    # Linux: NVIDIA GPU or processor, below
.venv/bin/python sidecar/whisper_server.py \
    [--engine auto|mlx|faster-whisper [--device auto|cuda|cpu] [--compute-type auto|<type>]] \
    [--model-en <model-or-hf-repo>] \
    [--model-fr <model-or-hf-repo>] [--preload] \
    [--second-opinion [--fr-detector <multilingual-model> [--fr-threshold 0.1]] [--fr-no-fallback [--fr-cut-loops]]] \
    [--save-audio <dir> [--save-audio-min-free-gb 8]]
```

The models are MLX conversions with the MLX engine, CTranslate2 ones with
faster-whisper (see [Two engines](#two-engines)). `--model-en` defaults to
`sfabriece/whisper-large-v3-atco2-asr-mlx`, which only MLX reads; without
`--model-fr` French audio is refused. It listens on `127.0.0.1:8178` (`--host`,
`--port`); `--preload` loads the models at start instead of on first use;
`--no-vad` and `--min-speech-seconds` set the voice gate; `--log-level`. You seldom
run it yourself: co-atc starts it from `[transcription.local] command`, restarts it
if it dies, and it exits on its own when the process that started it goes away
(`--no-exit-with-parent` to keep it).

`GET /health` reports which languages are served, and in `backend` what the
models run on: the GPU's name when MLX or CUDA gives it, else `mlx` or `CPU`. `POST /transcribe` takes raw
`s16le` PCM (or a WAV container) with `X-Sample-Rate`, `X-Channels`, `X-Language`
and `X-Frequency-Id` (and optionally `X-Created-At` / `X-Segment-At`, see the
archive below), and returns the transcript plus `duration`,
`speech_seconds`, `realtime_factor`, and `rejected` when nothing was transcribed.

## Two engines

**MLX** (`--engine mlx`): mlx-whisper on Apple Silicon. What this station runs
(D10), and what every figure in `docs-fr/` was measured with.

**faster-whisper** (`--engine faster-whisper`): CTranslate2, on Linux x86-64
(Q54). `--device auto` is the NVIDIA GPU when CTranslate2 sees one, else the
processor; `--compute-type auto` is `float16` on the GPU and `int8` on the
processor, and any CTranslate2 type can be named (`int8_float16` for a GPU short
of memory). Loading a model runs its encoder once, so a GPU without the CUDA
libraries fails at start with `--preload`, not on the first transmission.

`--engine auto`, the default, is MLX where `mlx_whisper` can be found, else
faster-whisper. Everything else is the same: the HTTP contract, the voice gate,
the two lanes, the French gate and the archive. The decoding is mlx_whisper's,
written out in `engine_ct2.py`: greedy, one sample per temperature, the same
six temperatures and thresholds, at most 221 new tokens a window (MLX: 224).
What has no equivalent: when every temperature fails, MLX keeps the last decode
and faster-whisper the most probable one, reported at the last temperature; and
MLX skips the fallback on a window whose `no_speech_prob` is over 0.6,
faster-whisper only when its log-probability is also under -1.

### Installing faster-whisper on Linux

```bash
python3 -m venv .venv                     # Python 3.12 or later
.venv/bin/pip install -r sidecar/requirements-ct2.txt
.venv/bin/python sidecar/whisper_server.py --model-en models/whisper-large-v3-atco2-asr-ct2 --preload
```

As pinned, the venv takes 6.0 GB, 4.4 of them torch and the CUDA 13 libraries
PyPI's torch brings along, for a voice gate that runs on the processor. A
processor-only torch, installed first, leaves them out (not tried here: the
PyTorch index was out of this session's reach):

```bash
.venv/bin/pip install torch==2.14.1 --index-url https://download.pytorch.org/whl/cpu
```

With an NVIDIA GPU, CTranslate2 4.8.4 loads cuBLAS for CUDA 12
(`libcublas.so.12`), which that torch does not bring. faster-whisper's README asks
for cuDNN 9 as well; CTranslate2 4.8.4's library names none. Not tried here (no
GPU):

```bash
.venv/bin/pip install nvidia-cublas-cu12
export LD_LIBRARY_PATH=$(.venv/bin/python -c 'import os, nvidia.cublas.lib; print(os.path.dirname(nvidia.cublas.lib.__file__))')
```

`LD_LIBRARY_PATH` must be set before Python starts: in the environment co-atc is
started from, when co-atc starts the sidecar. Without it, `--preload` stops the
sidecar at start with CTranslate2's own message.

### Models for faster-whisper

faster-whisper reads CTranslate2 conversions, not MLX ones:

| Role | MLX, on this station | Converted from | For faster-whisper |
|---|---|---|---|
| English, `--model-en` | `sfabriece/whisper-large-v3-atco2-asr-mlx` | `jlvdoorn/whisper-large-v3-atco2-asr` (docs-fr/27) | convert it, below |
| French, `--model-fr` | an MLX conversion of `bofenghuang/whisper-large-v3-french` | the same | its model card describes a `ctranslate2/` folder in the repository; else convert it |
| Detector, `--fr-detector` | `mlx-community/whisper-large-v3-turbo` | `openai/whisper-large-v3-turbo` | `large-v3-turbo`: faster-whisper fetches `mobiuslabsgmbh/faster-whisper-large-v3-turbo` |

Converting needs no GPU, and is done once:

```bash
python3 -m venv .venv-convert
.venv-convert/bin/pip install ctranslate2==4.8.4 transformers==5.20.2 torch
.venv-convert/bin/ct2-transformers-converter --model jlvdoorn/whisper-large-v3-atco2-asr \
    --output_dir models/whisper-large-v3-atco2-asr-ct2 \
    --copy_files tokenizer.json preprocessor_config.json --quantization float16
```

`--copy_files` is not optional. Without `preprocessor_config.json` faster-whisper
computes 80 mel bands where large-v3 takes 128, and refuses every transmission;
without `tokenizer.json` it fetches whisper-tiny's tokenizer, whose special tokens
sit one place off large-v3's (large-v3 added a language). If the repository lacks
either file, take it from `openai/whisper-large-v3`. `--quantization float16`
stores the weights in half precision; `--compute-type` decides at load what they
run in. The French model's own CTranslate2 folder, if the card is right:

```bash
.venv/bin/python -c "from huggingface_hub import snapshot_download; \
    snapshot_download('bofenghuang/whisper-large-v3-french', local_dir='models/whisper-large-v3-french', \
    allow_patterns='ctranslate2/*')"
# --model-fr models/whisper-large-v3-french/ctranslate2
```

No weights belong in this repository.

## Two departures from the upstream design, both measured

**A voice-activity gate in front of the model.** On this station up to 98% of
night-time squelch openings carry no speech. Whisper fed near-silence does not
return an empty string, it invents a plausible one — `"Thank you."` over 2.2s of
noise. Silero costs ~10ms and answers `rejected: "no_speech"` instead.

**Language routing with a refusal path.** An English ATC model fed French audio
produces fluent, confident, wrong English. Same 7.5s clip from a French flying
club tower, same server, only the routing differs:

```
X-Language: fr  →  "Fox Alpha Charlie, 28 au tourisme d'atterrissage, vent 282,
                    5 kt. On atterrit, piste 28, Fox Alpha Charlie."
X-Language: en  →  "lufthansa seven eight two so request heading five one two seven
                    climbing ten eight five to lufthansa seven eight two lufthansa…"
```

The second is not noise — it is well-formed ATC English that a downstream
post-processor will happily turn into a clearance for an aircraft that exists.
When the expected language has no model configured the server returns
`rejected: "no_model_for_fr"` and an empty transcript, which is the only safe
answer.

`X-Language` is the caller's expectation, taken from the frequency catalogue, and
it is honoured rather than second-guessed: measured language detection is wrong on
16% of French and 28% of English transmissions here, and its failures cluster on
clips that hold no usable speech anyway.

## No initial prompt by default

Upstream's transcription prompt is 150 words written for a continuous realtime
stream. On isolated 3-second transmissions it measurably hurts: sevenfold more
degeneration loops on French, decode time doubled, and the prompt text leaking
into the transcript (`"Aircraft are at cruise level and use ICA-7, Yankee Papa…"`).
See `docs-fr/10-mesure-amorces.md`.

## Second opinion and its gate

With `--second-opinion` and a French model, a transmission transcribed in English
is read again by the French model when the English text carries a French word
("bonjour", "niveau", "descendez"...). With `--fr-detector`, a multilingual
Whisper not fine-tuned on English (e.g. `whisper-large-v3-turbo`) also opens it
when its language detection gives French a probability of at least
`--fr-threshold`: the English model turns much French into English, and then no
French word is left to find. The detector only runs when no French word opened
the gate. This is not the language routing the paragraph above refuses: the
primary transcript still uses the language the caller asked for.

The response says which gate opened (`second_gate`: `words` or `detector`) and
the detector's probability (`p_fr`, when it ran); `/health` counts the second
opinions the detector opened (`second_opinion_by_detector`) and its failures
(`detector_failed`). All model work runs on one thread: MLX ties lazily created
arrays to the thread that created them.

That thread has two lanes (`lanes.py`, 14/10): the first reading of a transmission
runs before any detector pass or second reading still waiting, so the French
reading of one frequency no longer delays the English reading of another (the
wait had reached 26.5 s at the 90th percentile when both spoke at once). With the
header `X-Second: later` the answer comes as soon as the first reading is done,
with `second_pending` set when a second reading may follow; `GET /second/{id}?wait=60`
waits for it and answers `{"status": "done", "second_text": ...}` (empty when the
gate stayed shut), `{"status": "pending"}`, or 404 for an id it does not know.
Without the header the answer waits for both readings, as before. `/health`
reports the queue by lane (`queue`) and the readings not yet fetched
(`second_pending`).

`--fr-no-fallback` reads French once, at temperature 0, instead of mlx_whisper's
fallback of up to six decodes at rising temperature. The fallback fires on the
French model's loops: on two benches it made the mean French read 3.8 and 2.7 s,
against 1.8 and 1.4 s without it, for the same matches within one aircraft. But the
fallback is also what broke those loops: without it, 22.4 and 12.0 % of French texts
loop, against 9.8 and 4.8 %. `--fr-cut-loops` cuts them after the model, where a run
of words repeats back to back (`loops.py`: a word four times in a row, a group of two
to eight words three times), bringing them to 4.2 and 2.5 %. The manifest keeps the
uncut text (`texte_second_brut`). Not applied to the English reading, where the cut
cost true matches.

## The audio archive

`--save-audio <dir>` keeps every transmission exactly as it arrived, before
resampling, rejected ones included, as `<dir>/<YYYYMMDD>/<frequency>/t_*.wav`,
with one manifest per day, `<dir>/<YYYYMMDD>/manifeste.jsonl`. A line holds, beside
the clip (`fichier`), its frequency (`frequency_id`), its length (`duree`), the
language and models used (`langue`, `modele`, `modele_second`) and `at`:
- the texts (`texte`, `texte_second`), the gate (`porte`) and `p_fr`;
- `speech_seconds`, and `rejected` for a clip the voice gate dropped;
- per decoded segment, `temperature`, `compression_ratio`, `avg_logprob` and
  `no_speech_prob`, for both readings (`segments`, `segments_second`). A
  temperature above 0 marks a segment the fallback re-read, which is how a loop
  shows;
- `recu_utc`, when the sidecar received it;
- `created_at` and `segment_at_utc`, when the caller sends `X-Created-At` and
  `X-Segment-At`. co-atc sends the date its database row will carry, so a clip
  joins its transcription exactly. Under a queue, the sidecar sees a transmission
  up to a minute after it ended.

Archiving stops, and transcription goes on, when free space falls below
`--save-audio-min-free-gb`.
