"""Local speech-to-text sidecar for Clairance.

Implements the HTTP contract described in upstream's `docs/LOCAL-STT.md`, which
was designed but never built: the Go side posts raw PCM and gets a transcript
back, so the audio pipeline stays unchanged and no audio leaves the machine.

Two things are deliberately added to that design, and both come from measurement
on a real station rather than from taste:

  1. A voice-activity gate in front of the model. Up to 98% of night-time squelch
     openings carry no speech, and Whisper fed near-silence does not return an
     empty string — it invents a plausible one.

  2. Language routing with a refusal path. An English ATC model fed French audio
     produces fluent, confident, wrong English: "cleared to land runway zero five"
     over a French flying club. Passed downstream, that fabricates a landing
     clearance for a real aircraft on the map. When the expected language has no
     model configured, this server returns an empty transcript and says why,
     which is the only safe answer.

Run:
    python sidecar/whisper_server.py [--engine mlx|faster-whisper] --model-en <model> [--model-fr <model>]

The models are MLX conversions on Apple Silicon, CTranslate2 ones with
faster-whisper (engines.py, README.md).
"""
from __future__ import annotations

import asyncio
import functools
import io
import json
import logging
import sys
import os
import threading
import time
import unicodedata
import wave
import uuid
from dataclasses import dataclass

import numpy as np
from fastapi import FastAPI, Header, Request
from fastapi.responses import JSONResponse

sys.path.insert(0, __file__.rsplit("/", 1)[0])
from config import Config, from_args  # noqa: E402
import engines  # noqa: E402
from loops import cut_loops  # noqa: E402
from lanes import FIRST, SECOND, PriorityWorker  # noqa: E402

WHISPER_RATE = 16000

log = logging.getLogger("stt")
cfg = Config()
app = FastAPI(title="Clairance STT sidecar")

# What turns audio into text (engines.py), chosen at startup (main).
engine = None
# What the models run on, as /health declares it: the GPU's name when MLX or
# CUDA gives it, else "mlx" or "CPU". Read once at startup, on the model thread
# (main).
_backend = "mlx"
_vad = None
_counters = {"second_opinion_ok": 0, "second_opinion_failed": 0,
             "second_opinion_by_detector": 0, "detector_failed": 0,
             "second_opinion_loops_cut": 0,
             "transcribed": 0, "rejected_no_speech": 0, "audio_save_failed": 0,
             "archiving_stopped": 0}


# --------------------------------------------------------------------------- audio

def decode_pcm(raw: bytes, sample_rate: int, channels: int) -> np.ndarray:
    """Raw s16le (or a WAV container) to mono float32 at Whisper's 16 kHz."""
    if raw[:4] == b"RIFF":
        with wave.open(io.BytesIO(raw)) as w:
            sample_rate, channels = w.getframerate(), w.getnchannels()
            raw = w.readframes(w.getnframes())

    audio = np.frombuffer(raw, dtype=np.int16).astype(np.float32) / 32768.0
    if channels > 1:
        audio = audio.reshape(-1, channels).mean(axis=1)

    if sample_rate != WHISPER_RATE:
        from math import gcd
        from scipy.signal import resample_poly
        g = gcd(int(sample_rate), WHISPER_RATE)
        audio = resample_poly(audio, WHISPER_RATE // g, int(sample_rate) // g)
    return np.ascontiguousarray(audio, dtype=np.float32)


@dataclass
class Speech:
    seconds: float
    fraction: float


def measure_speech(audio: np.ndarray) -> Speech:
    """Seconds of actual speech, via Silero. Cheap next to the model itself."""
    global _vad
    if _vad is None:
        from silero_vad import load_silero_vad
        _vad = load_silero_vad()
    import torch
    from silero_vad import get_speech_timestamps

    spans = get_speech_timestamps(
        torch.from_numpy(audio), _vad, sampling_rate=WHISPER_RATE,
        threshold=cfg.vad_threshold,
        min_speech_duration_ms=cfg.vad_min_speech_ms,
        min_silence_duration_ms=cfg.vad_min_silence_ms,
    )
    speech = sum(s["end"] - s["start"] for s in spans) / WHISPER_RATE
    total = len(audio) / WHISPER_RATE
    return Speech(speech, speech / total if total else 0.0)


# --------------------------------------------------------------------------- models

def model_for(language: str) -> tuple[str | None, str]:
    """Return (model, resolved language), or (None, language) if we must refuse."""
    if language == "fr":
        return (cfg.model_fr or None), "fr"
    return (cfg.model_en or None), "en"


# The French model on this station is a symlink to an external drive. Unplug it
# and the second opinion stops working, the primary transcript still arrives, and
# nothing anywhere says the station just lost 8.5% of its callsign matches. That
# is the failure this reports: not a crash, a silence.
def model_status(model: str) -> dict:
    if not model:
        return {"id": "", "kind": "none", "available": False, "detail": "not configured"}
    # A local path is a directory we can look at; anything else is a hub id we
    # cannot check without reaching the network, so we do not claim to know.
    if model.startswith("/") or model.startswith("./") or model.startswith("~"):
        path = os.path.expanduser(model)
        if os.path.isdir(path):
            return {"id": model, "kind": "path", "available": True}
        return {"id": model, "kind": "path", "available": False,
                "detail": "path not found -- an external drive may be unplugged"}
    return {"id": model, "kind": "hub", "available": True, "detail": "not verified"}


_disk = {"checked": 0.0, "ok": True}


def _disk_has_room() -> bool:
    """Free space, rechecked at most once a minute.

    The archive is the expendable half: transcription and the database must
    survive a full disk, so archiving is what stops. It says so once rather than
    on every transmission -- a log line per clip would itself be a problem.
    """
    now = time.time()
    if now - _disk["checked"] < 60:
        return _disk["ok"]
    _disk["checked"] = now
    try:
        # The archive directory does not exist until the first clip is written,
        # and statvfs on a missing path raises. Walking up to an existing parent
        # is the difference between a guard and a guard that fails open -- which
        # is what this did on its first test.
        probe = os.path.abspath(cfg.save_audio or "/")
        while probe != "/" and not os.path.isdir(probe):
            probe = os.path.dirname(probe)
        st = os.statvfs(probe)
        free_gb = st.f_bavail * st.f_frsize / 1073741824
    except Exception as e:
        if _disk["ok"]:
            log.warning("cannot read free space, archiving continues: %s", e)
        return _disk["ok"]
    ok = free_gb >= cfg.save_audio_min_free_gb
    if ok != _disk["ok"]:
        if ok:
            log.info("audio archiving resumed, %.1f GB free", free_gb)
        else:
            log.warning("audio archiving STOPPED: %.1f GB free, floor is %.1f -- "
                        "transcription continues", free_gb, cfg.save_audio_min_free_gb)
        _counters["archiving_stopped"] = 0 if ok else 1
    _disk["ok"] = ok
    return ok


def keep_audio(raw: bytes, rate: int, channels: int, record: dict) -> str:
    """Write one transmission's audio and append its manifest line.

    The audio is written exactly as it arrived, before resampling: a transcript
    can always be recomputed from the audio, never the reverse, so the archive
    has to hold the thing that cannot be rebuilt.
    """
    if not cfg.save_audio:
        return ""
    if not _disk_has_room():
        return ""
    day = time.strftime("%Y%m%d")
    folder = os.path.join(cfg.save_audio, day, record.get("frequency_id") or "unknown")
    os.makedirs(folder, exist_ok=True)
    name = time.strftime("t_%Y%m%d_%H%M%S") + f"_{int(time.time()*1000) % 1000:03d}.wav"
    path = os.path.join(folder, name)
    try:
        with wave.open(path, "wb") as w:
            w.setnchannels(channels)
            w.setsampwidth(2)
            w.setframerate(rate)
            w.writeframes(raw)
        record["fichier"] = os.path.relpath(path, cfg.save_audio)
        # One manifest per day, beside that day's audio: a single file would
        # grow without end, and a day is the unit the archive is used in.
        with open(os.path.join(cfg.save_audio, day, "manifeste.jsonl"), "a") as m:
            m.write(json.dumps(record, ensure_ascii=False) + "\n")
        return path
    except Exception as e:
        # Losing the archive must never lose the transmission.
        _counters["audio_save_failed"] += 1
        log.warning("could not keep audio: %s", e)
        return ""


def identity(frequency_id: str, received: float, segment_at: str, created_at: str) -> dict:
    """The fields that tie a manifest line to its transcription in co-atc's
    database. `created_at` is co-atc's own value for the row, sent with the
    audio: under a queue the sidecar sees a transmission up to a minute after it
    ended, so its own clock cannot be the join key."""
    rec = {
        "at": time.strftime("%Y-%m-%dT%H:%M:%S", time.localtime(received)),
        "recu_utc": time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(received)) + f".{int(received * 1000) % 1000:03d}Z",
        "frequency_id": frequency_id,
    }
    if segment_at:
        rec["segment_at_utc"] = segment_at
    if created_at:
        rec["created_at"] = created_at
    return rec


def decode_details(result: dict) -> list:
    """Per-segment decoding figures for the archive. A segment decoded at a
    temperature above 0 is one the fallback re-read, which is how a loop shows."""
    out = []
    for s in result.get("segments", []):
        out.append({k: (round(s[k], 4) if isinstance(s.get(k), float) else s.get(k))
                    for k in ("start", "end", "temperature", "compression_ratio", "avg_logprob", "no_speech_prob")})
    return out


# One decode at a time, off the event loop. /transcribe is async, and a model
# call made directly in it held the whole server for the length of the decode:
# /health could not answer while a transmission was being transcribed. Measured
# on 23/09: not one answer during a 17 s decode, and the settings panel showing
# "not reached just now" whenever it asked at that moment. The lock keeps decodes
# one at a time, as they were -- nothing says the models are safe to call
# concurrently, and two decodes at once would only share the same GPU anyway.
_decode_lock = threading.Lock()

# ...and on one thread. MLX keeps a stream per thread, and what it creates lazily
# -- a model's weights, a cached filter bank -- belongs to the stream of the
# thread that created it. asyncio.to_thread spreads calls over a pool, and with
# the language detector loaded the sidecar aborted on its first request: "There
# is no Stream(gpu, 1) in current thread". One thread for all model work is what
# the bench did, and it never failed.
#
# With a stack of its own, and a large one. On 28/09 the sidecar died of a bus error
# on this thread's stack guard, deep in MLX's graph compilation (compile_dfs
# recursing on itself): Python gives its threads 16 MiB on macOS, and one decode
# needed more. 256 MiB is reserved, not used -- pages are only committed as the
# stack grows -- and the size applies to this thread alone.
_MLX_STACK = 256 * 1024 * 1024
# Two lanes on that one thread (lanes.py): the first reading of a transmission
# before any detector pass or second reading still waiting (14/10).
_model_thread = PriorityWorker(name="mlx", stack_size=_MLX_STACK)


async def on_model(fn, *args, lane=FIRST, **kwargs):
    return await asyncio.wrap_future(_model_thread.submit(functools.partial(fn, *args, **kwargs), lane))


def decode(model: str, audio, language: str, **options):
    """One reading of the audio by a model, on the model thread (engines.py)."""
    with _decode_lock:
        return engine.transcribe(model, audio, language, **options)


def french_probability(audio: np.ndarray) -> float:
    """The detector's probability that the transmission is French (engines.py),
    on the model thread."""
    with _decode_lock:
        return engine.french_probability(cfg.fr_detector, audio)


# --------------------------------------------------------------------------- routes

@app.get("/health")
def health():
    models = {"en": model_status(cfg.model_en), "fr": model_status(cfg.model_fr)}
    if cfg.fr_detector:
        models["detector"] = model_status(cfg.fr_detector)
    # degraded, not unhealthy: the primary model answers, so transcription works;
    # what is lost is the second opinion, and losing it quietly is the problem.
    degraded = [lang for lang, m in models.items()
                if m["kind"] == "path" and not m["available"]]
    return {
        "status": "degraded" if degraded else "ok",
        # So a caller that started this process can tell it apart from one that
        # was already holding the port. A health probe answering says something
        # is there, not that it is yours.
        "pid": os.getpid(),
        "backend": _backend,
        "degraded": degraded,
        "models": models,
        "second_opinion": cfg.second_opinion,
        "second_opinion_ok": _counters["second_opinion_ok"],
        "second_opinion_failed": _counters["second_opinion_failed"],
        # How many second opinions the detector opened where no French word did,
        # and at what threshold (Q50).
        "fr_detector": cfg.fr_detector or None,
        "fr_threshold": cfg.fr_threshold if cfg.fr_detector else None,
        "second_opinion_by_detector": _counters["second_opinion_by_detector"],
        "fr_no_fallback": cfg.fr_no_fallback,
        "fr_cut_loops": cfg.fr_cut_loops,
        "second_opinion_loops_cut": _counters["second_opinion_loops_cut"],
        "detector_failed": _counters["detector_failed"],
        # Q27 asked for this and nothing answered it: the voice gate's rejection
        # rate in production was logged only at Debug, so "is the night-time
        # parasite absent in daylight?" had no instrument.
        "transcribed": _counters["transcribed"],
        # The model's queue now, by lane (lanes.py), and the second readings
        # asked for later and not fetched yet.
        "queue": _model_thread.waiting(),
        "second_pending": sum(1 for v in _pending.values() if not v["done_at"]),
        "rejected_no_speech": _counters["rejected_no_speech"],
        "archiving": bool(cfg.save_audio) and _disk["ok"],
        "archiving_stopped_low_disk": bool(_counters["archiving_stopped"]),
        "audio_save_failed": _counters["audio_save_failed"],
        # kept for callers written against the earlier shape
        "model_en": cfg.model_en,
        "model_fr": cfg.model_fr or None,
        "vad": cfg.vad_enabled,
        "min_speech_seconds": cfg.min_speech_seconds,
        "languages": ["en"] + (["fr"] if cfg.model_fr else []),
    }


# Words that do not exist in air-traffic English, so an English model only writes
# them when it actually heard French. Deliberately short and common: this opens a
# gate, it does not measure a proportion. The same list produced the Q29 figures;
# keep it in step with whisper-lab/scripts/q1-francais.py or the measurement stops
# describing what runs.
FRENCH_MARKERS = (
    "bonjour", "bonsoir", "au revoir", "aurevoir", "merci", "monsieur",
    "autoris", "niveau", "descendez", "montez", "contactez", "rappelez",
    "piste", "vent", "quittez", "maintenez", "approche", "tour de",
    "s'il vous", "bien recu", "bien recu", "roger merci",
)


def _fold(s: str) -> str:
    return "".join(c for c in unicodedata.normalize("NFD", s.lower())
                   if unicodedata.category(c) != "Mn")


def looks_french(text: str) -> bool:
    folded = _fold(text)
    return any(_fold(m) in folded for m in FRENCH_MARKERS)


@app.post("/transcribe")
async def transcribe(
    request: Request,
    x_sample_rate: int = Header(default=24000),
    x_channels: int = Header(default=1),
    x_language: str = Header(default="en"),
    x_frequency_id: str = Header(default=""),
    x_segment_at: str = Header(default=""),
    x_created_at: str = Header(default=""),
    x_second: str = Header(default=""),
):
    """Transcribe raw PCM.

    With `X-Second: later` the answer comes as soon as the first reading is
    done, with `second_pending` set when a second reading may follow; that one
    is fetched from GET /second/{id}. Without it, the answer waits for both,
    as before (the lab's tools ask that way).

    `X-Language` is the caller's expectation, taken from the frequency catalogue.
    It is honoured, not second-guessed: measured language detection is wrong on
    16% of French and 28% of English transmissions here, and its failures are
    concentrated on clips that hold no usable speech anyway.
    """
    started = time.time()
    raw = await request.body()
    if not raw:
        return JSONResponse({"error": "empty body"}, status_code=400)

    audio = decode_pcm(raw, x_sample_rate, x_channels)
    duration = len(audio) / WHISPER_RATE

    speech = Speech(duration, 1.0)
    if cfg.vad_enabled:
        speech = measure_speech(audio)
        if speech.seconds < cfg.min_speech_seconds:
            _counters["rejected_no_speech"] += 1
            keep_audio(raw, x_sample_rate, x_channels, {
                **identity(x_frequency_id, started, x_segment_at, x_created_at),
                "duree": round(duration, 3),
                "speech_seconds": round(speech.seconds, 3),
                "rejected": "no_speech",
                "texte": "",
            })
            return {
                "text": "", "segments": [], "language": None,
                "duration": round(duration, 3),
                "speech_seconds": round(speech.seconds, 3),
                "rejected": "no_speech",
                "frequency_id": x_frequency_id,
                "elapsed": round(time.time() - started, 3),
            }

    language = (x_language or "en").lower()[:2]
    model, language = model_for(language)
    if model is None:
        # Refusing is the safe answer: transcribing French with an English model
        # yields confident, well-formed, wrong clearances.
        return {
            "text": "", "segments": [], "language": language,
            "duration": round(duration, 3),
            "speech_seconds": round(speech.seconds, 3),
            "rejected": f"no_model_for_{language}",
            "frequency_id": x_frequency_id,
            "elapsed": round(time.time() - started, 3),
        }

    prompt = cfg.initial_prompt_fr if language == "fr" else cfg.initial_prompt_en
    result = await on_model(decode, model, audio, language, initial_prompt=prompt)
    text = " ".join(result["text"].split())
    segments = [
        {"text": " ".join(s["text"].split()),
         "start": round(s["start"], 2), "end": round(s["end"], 2),
         "no_speech_prob": round(s.get("no_speech_prob", 0.0), 4)}
        for s in result.get("segments", [])
    ]
    later = x_second.strip().lower() == "later"
    can_second = bool(cfg.second_opinion and language == "en" and cfg.model_fr and text)
    keep = functools.partial(keep_reading, raw, x_sample_rate, x_channels,
                             identity(x_frequency_id, started, x_segment_at, x_created_at),
                             duration, speech, language, model, text, result)

    if later and can_second:
        pending = new_pending()
        asyncio.get_running_loop().create_task(
            second_later(pending, audio, text, x_frequency_id, keep))
        second, gate, p_fr = None, "", None
    else:
        pending = ""
        second, gate, p_fr = (await second_reading(audio, text, x_frequency_id)
                              if can_second else (None, "", None))
        keep(second, gate, p_fr)
    _counters["transcribed"] += 1

    elapsed = time.time() - started
    log.info("%s %.1fs speech=%.1fs %s %.2fs %r",
             x_frequency_id or "-", duration, speech.seconds, language, elapsed, text[:80])
    return {
        "text": text,
        "segments": segments,
        "language": language,
        "second_text": (second or {}).get("text", ""),
        "second_language": (second or {}).get("language", ""),
        "second_model": (second or {}).get("model", ""),
        "second_gate": gate if second else "",
        "second_pending": pending,
        "p_fr": None if p_fr is None else round(p_fr, 4),
        "duration": round(duration, 3),
        "speech_seconds": round(speech.seconds, 3),
        "speech_fraction": round(speech.fraction, 3),
        "model": model,
        "frequency_id": x_frequency_id,
        "elapsed": round(elapsed, 3),
        "realtime_factor": round(duration / elapsed, 2) if elapsed else None,
    }


async def second_reading(audio, text: str, frequency_id: str):
    """The French gate and, when it opens, the second reading: (second, gate, p_fr).

    Second opinion. The two models fail on different recordings -- on French
    transmissions they agreed on only 8 of the 34 they matched -- so a second
    reading of the same audio reaches transmissions the first one misses. The
    gate matters: asked on every transmission the French model also answers
    where it has nothing to say, and invents callsigns on English audio. Both
    the detector and the reading run in the SECOND lane: a first reading of the
    other frequency waiting passes before them.
    """
    second = None
    gate, p_fr = "", None
    if looks_french(text):
        gate = "words"
    elif cfg.fr_detector:
        # Only where no French word opened the gate: the detector costs an
        # encoder pass, and a French word already says what it would.
        try:
            p_fr = await on_model(french_probability, audio, lane=SECOND)
            if p_fr >= cfg.fr_threshold:
                gate = "detector"
        except Exception as e:
            # A detector that fails leaves the word gate, as before; counted
            # so /health can say it is happening.
            _counters["detector_failed"] += 1
            log.warning("language detection failed (%d so far): %s", _counters["detector_failed"], e)
    if gate:
        try:
            t1 = time.time()
            fr_kwargs = {"temperature": 0.0} if cfg.fr_no_fallback else {}
            r2 = await on_model(decode, cfg.model_fr, audio, "fr", lane=SECOND, **fr_kwargs)
            raw_second = " ".join(r2["text"].split())
            text_second = cut_loops(raw_second) if cfg.fr_cut_loops else raw_second
            if text_second != raw_second:
                _counters["second_opinion_loops_cut"] += 1
            second = {
                "text": text_second,
                "raw": raw_second,
                "language": "fr",
                "model": cfg.model_fr,
                "elapsed": round(time.time() - t1, 3),
                "details": decode_details(r2),
            }
            _counters["second_opinion_ok"] += 1
            if gate == "detector":
                _counters["second_opinion_by_detector"] += 1
            log.info("%s second opinion (fr, %s%s) %r", frequency_id or "-", gate,
                     f" p_fr={p_fr:.2f}" if p_fr is not None else "", second["text"][:80])
        except Exception as e:
            # A failed second opinion is not a failed transcription: the primary
            # text stands and the caller is told nothing was added. But it is
            # counted, so /health can say it is happening.
            _counters["second_opinion_failed"] += 1
            log.warning("second opinion failed (%d so far): %s",
                        _counters["second_opinion_failed"], e)
    return second, gate, p_fr


def keep_reading(raw, rate, channels, ident, duration, speech, language, model, text, result,
                 second, gate, p_fr):
    """The kept audio's record, once both readings are known."""
    keep_audio(raw, rate, channels, {
        **ident,
        "duree": round(duration, 3),
        "speech_seconds": round(speech.seconds, 3),
        "langue": language,
        "modele": model,
        "texte": text,
        "texte_second": (second or {}).get("text", ""),
        # As the model read it, before any loop was cut, so the cut can be
        # measured again (whisper-lab).
        "texte_second_brut": (second or {}).get("raw", ""),
        "modele_second": (second or {}).get("model", ""),
        "porte": gate,
        "p_fr": None if p_fr is None else round(p_fr, 4),
        "segments": decode_details(result),
        "segments_second": (second or {}).get("details", []),
    })


# Second readings asked for later: id -> {"event", "result", "at"}. An entry
# leaves when it is fetched, or PENDING_KEEP_SECONDS after it is done.
_pending: dict = {}
PENDING_KEEP_SECONDS = 600


def new_pending() -> str:
    now = time.time()
    for key in [k for k, v in _pending.items() if v["done_at"] and now - v["done_at"] > PENDING_KEEP_SECONDS]:
        _pending.pop(key, None)
    key = uuid.uuid4().hex
    _pending[key] = {"event": asyncio.Event(), "result": None, "done_at": None}
    return key


async def second_later(key, audio, text, frequency_id, keep):
    try:
        second, gate, p_fr = await second_reading(audio, text, frequency_id)
    except Exception as e:  # never left pending for ever
        log.warning("second reading lost: %s", e)
        second, gate, p_fr = None, "", None
    keep(second, gate, p_fr)
    entry = _pending.get(key)
    if entry is None:
        return
    entry["result"] = {
        "status": "done",
        "second_text": (second or {}).get("text", ""),
        "second_language": (second or {}).get("language", ""),
        "second_model": (second or {}).get("model", ""),
        "second_gate": gate if second else "",
        "p_fr": None if p_fr is None else round(p_fr, 4),
    }
    entry["done_at"] = time.time()
    entry["event"].set()


@app.get("/second/{key}")
async def second(key: str, wait: float = 60.0):
    """A second reading asked for later: waits up to `wait` seconds for it.

    {"status": "done", "second_text": ...} (empty when the gate stayed shut),
    {"status": "pending"} when it is not done yet, 404 for an unknown id.
    """
    entry = _pending.get(key)
    if entry is None:
        return JSONResponse({"error": "unknown"}, status_code=404)
    try:
        await asyncio.wait_for(entry["event"].wait(), timeout=max(0.0, min(wait, 300.0)))
    except asyncio.TimeoutError:
        return {"status": "pending"}
    _pending.pop(key, None)
    return entry["result"]


def watch_parent(interval: float = 5.0) -> None:
    """Stop when the process that started this one goes away.

    co-atc stops its sidecar on the way out, but it cannot do so when it is
    killed outright, and a sidecar that outlives it keeps port 8178 -- which is
    how seventeen servers came to share one in a night. The parent cannot
    prevent that; the child can notice.

    A process that is already orphaned at startup is left alone, so running this
    under `nohup ... &` and closing the terminal still works.
    """
    # co-atc sets this when it spawns the sidecar. Guessing instead -- watching
    # whenever the parent looks alive -- would kill a sidecar started by hand
    # with `nohup ... &` the moment its terminal closes, because its parent
    # becomes init too. The caller knows; it should say so rather than be
    # inferred.
    if os.environ.get("COATC_SPAWNED") != "1":
        log.info("not spawned by co-atc; not watching for a parent")
        return

    original = os.getppid()
    if original == 1:
        log.info("already orphaned at startup; not watching for a parent")
        return

    def loop() -> None:
        while True:
            time.sleep(interval)
            if os.getppid() != original:
                log.warning("parent %d is gone, stopping", original)
                # The hard exit is deliberate: uvicorn's graceful shutdown waits
                # on in-flight requests, and the client that would have finished
                # them is the process that just died.
                os._exit(0)

    threading.Thread(target=loop, daemon=True, name="parent-watch").start()
    log.info("watching parent %d", original)


def main() -> None:
    global cfg, engine, _backend
    cfg = from_args()
    logging.basicConfig(level=cfg.log_level.upper(),
                        format="%(asctime)s %(levelname)s %(name)s %(message)s")
    try:
        engine = engines.pick(cfg.engine, cfg.device, cfg.compute_type)
    except RuntimeError as e:
        sys.exit(str(e))
    # The default English model is an MLX conversion, which faster-whisper cannot
    # read: finding out on the first transmission would be finding out too late.
    if engine.name != "mlx" and cfg.model_en == Config.model_en:
        sys.exit(f"{Config.model_en} is an MLX conversion, which {engine.name} cannot read: "
                 "pass a CTranslate2 conversion of jlvdoorn/whisper-large-v3-atco2-asr with "
                 "--model-en (sidecar/README.md)")
    # On the model thread: MLX ties what it creates to the thread that created it.
    _backend = _model_thread.submit(engine.backend).result()
    log.info("Engine: %s", engine.name)
    log.info("Backend: %s", _backend)
    log.info("English model: %s", cfg.model_en)
    log.info("French model: %s", cfg.model_fr or "(none — French will be refused)")
    if cfg.fr_detector:
        log.info("Language detector: %s, second opinion above p(fr) %.2f", cfg.fr_detector, cfg.fr_threshold)
    if cfg.preload:
        _model_thread.submit(functools.partial(engine.load, cfg.model_en)).result()
        if cfg.model_fr:
            # A French model that cannot be loaded costs French, not the sidecar
            # (a missing path shows as degraded in /health): the English one is
            # what must load. MLX only registers a model here and does not fail.
            try:
                _model_thread.submit(functools.partial(engine.load, cfg.model_fr)).result()
            except Exception as e:
                log.warning("French model %s could not be loaded, French is off until it can: %s",
                            cfg.model_fr, e)
        if cfg.fr_detector and cfg.second_opinion and cfg.model_fr:
            _model_thread.submit(functools.partial(engine.detector, cfg.fr_detector)).result()
    if cfg.exit_with_parent:
        watch_parent()

    import uvicorn
    uvicorn.run(app, host=cfg.host, port=cfg.port, log_level=cfg.log_level)


if __name__ == "__main__":
    main()
