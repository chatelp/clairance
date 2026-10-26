"""Configuration for the local STT sidecar."""
from __future__ import annotations

import argparse
from dataclasses import dataclass, field


@dataclass
class Config:
    host: str = "127.0.0.1"
    port: int = 8178

    # Models are chosen per language, not per frequency: the caller passes the
    # language it expects and the sidecar honours it. See docs-fr/13-strategie-bilingue.md.
    model_en: str = "sfabriece/whisper-large-v3-atco2-asr-mlx"
    model_fr: str = ""          # e.g. a converted bofenghuang/whisper-large-v3-french
    preload: bool = False

    # Voice activity detection. Measured on this station: up to 98% of night-time
    # squelch openings carry no speech at all, and a model fed silence hallucinates
    # plausible text. The gate is a correctness requirement, not an optimisation.
    vad_enabled: bool = True
    vad_threshold: float = 0.5
    vad_min_speech_ms: int = 250
    vad_min_silence_ms: int = 300
    min_speech_seconds: float = 0.25

    # Decoding. The upstream transcription prompt measurably hurts on short
    # transmissions: it multiplies degeneration loops sevenfold and leaks into the
    # output. Empty by default. See docs-fr/10-mesure-amorces.md.
    initial_prompt_en: str = ""
    initial_prompt_fr: str = ""

    # Second opinion. When the primary transcript carries French words, ask the
    # French model too and return both. Measured on 1 378 real transmissions
    # (docs-fr/05-decisions.md, Q29): +8.5% true callsign matches, precision
    # unchanged at 72%, and 15% more compute -- because the gate only opens on
    # 12.5% of transmissions. Asking both models every time was measured too and
    # is worse: +13.7% matches but precision falls to 66%, which means words
    # attributed to the wrong aircraft.
    second_opinion: bool = False

    # Language detection for the second opinion (docs-fr/05-decisions.md, Q50).
    # The word gate misses French that the English model turned into English:
    # forced into English it anglicises -- "Air France Five One Air we conceded
    # report three zero" -- and no French word survives. A multilingual Whisper
    # that was not fine-tuned on English ATC (the ATC model's own detector says
    # "English" on French) gives the probability of French, and above
    # fr_threshold the French model reads the transmission too. Measured on 665
    # transmissions: +8% true matches at equal precision with large-v3-turbo at
    # 0.1, for about 0.6 s more per transmission that has no French word; the
    # small and base models detect worse (+4% and +3%). Empty: the word gate
    # alone, as before.
    fr_detector: str = ""
    fr_threshold: float = 0.1

    # Read French once, at temperature 0, instead of mlx_whisper's fallback of up
    # to six decodes at rising temperature (Q50). The fallback fires on the
    # French model's loops, and those are what made the second opinion expensive:
    # measured on two benches, the mean French decode falls from 3.83 to 1.77 s
    # and from 2.73 to 1.41 s, reads over 10 s from 70 to 2 and 46 to 0, and the
    # true matches stay within one aircraft (192.2 against 192.8, 202.0 against
    # 202.8). Off: the fallback, as before.
    fr_no_fallback: bool = False

    # Cut the loops out of the French second reading, after the model (loops.py).
    # Read without fallback, the French model loops two to three times as often;
    # cutting where a run of words repeats back to back brings the loops below
    # what the fallback left (4.2 and 2.5 % of texts against 9.8 and 4.8 %), true
    # matches within noise. The manifest keeps the uncut text. Off: as read.
    fr_cut_loops: bool = False

    # Keep every transmission's audio, with a manifest line beside it. The whole
    # dossier rests on a single afternoon's capture: every model comparison, every
    # threshold, every control. A second corpus on different frequencies is what
    # lets any of it be checked against independent data, and the audio is the one
    # thing that cannot be recovered afterwards -- the transcript can always be
    # recomputed from it, never the reverse.
    #
    # Rejected clips are kept too, on purpose: whether the voice gate was right
    # to drop them is an open question (Q27) and it cannot be answered from
    # clips that were thrown away.
    save_audio: str = ""
    # Archiving stops on its own below this much free space. An archive that
    # fills the disk takes the database down with it, and the database is the
    # thing that cannot be rebuilt from anything else.
    save_audio_min_free_gb: float = 8.0
    # MLX's buffer cache, bounded (device.limit_cache): unbounded, it took the
    # sidecar from 8 to 16 GB in 25 minutes on 26/10. 0 leaves it unbounded.
    mlx_cache_limit_gb: float = 1.0

    # Stop when the process that started this one goes away.
    #
    # co-atc stops its sidecar on the way out, but it cannot do so when it is
    # killed outright -- and a sidecar that outlives it holds port 8178, which is
    # how seventeen servers came to share one in a night. Three orphans were
    # observed on this machine in a single day. The parent cannot prevent this;
    # the child can notice.
    #
    # Off when this process was already orphaned at startup, so `nohup ... &`
    # followed by closing the terminal still works. --no-exit-with-parent turns
    # it off outright.
    exit_with_parent: bool = True

    log_level: str = "info"


def from_args(argv: list[str] | None = None) -> Config:
    d = Config()
    p = argparse.ArgumentParser(description="Local STT sidecar for Clairance")
    p.add_argument("--host", default=d.host)
    p.add_argument("--port", type=int, default=d.port)
    p.add_argument("--model-en", default=d.model_en,
                   help="MLX model (path or HF repo) for English audio")
    p.add_argument("--model-fr", default=d.model_fr,
                   help="MLX model for French audio; leave empty to refuse French")
    p.add_argument("--preload", action="store_true",
                   help="load models at startup rather than on first use")
    p.add_argument("--no-vad", dest="vad", action="store_false")
    p.add_argument("--min-speech-seconds", type=float, default=d.min_speech_seconds)
    p.add_argument("--log-level", default=d.log_level)
    p.add_argument("--second-opinion", dest="second", action="store_true",
                   help="on a transcript that looks French, transcribe again with the French model")
    p.add_argument("--fr-detector", default=d.fr_detector, metavar="MODEL",
                   help="multilingual MLX Whisper (path or HF repo) whose language detection also "
                        "opens the second opinion; empty for the word gate alone")
    p.add_argument("--fr-threshold", type=float, default=d.fr_threshold,
                   help="probability of French above which the detector opens the second opinion (default 0.1)")
    p.add_argument("--fr-no-fallback", dest="fr_no_fallback", action="store_true",
                   help="read French once at temperature 0, without the temperature fallback (Q50)")
    p.add_argument("--fr-cut-loops", dest="fr_cut_loops", action="store_true",
                   help="cut the loops out of the French second reading, after the model (Q50)")
    p.add_argument("--save-audio", default=d.save_audio, metavar="DIR",
                   help="keep every transmission's audio and a manifest line, rejected ones included")
    p.add_argument("--save-audio-min-free-gb", type=float, default=d.save_audio_min_free_gb,
                   help="stop archiving below this much free disk space (default 8)")
    p.add_argument("--mlx-cache-limit-gb", type=float, default=d.mlx_cache_limit_gb,
                   help="bound MLX's cache of freed buffers, in GB (default 1; 0 for no bound)")
    p.add_argument("--no-exit-with-parent", dest="exit_with_parent", action="store_false",
                   help="keep running after the process that started this one goes away")
    a = p.parse_args(argv)
    return Config(host=a.host, port=a.port, model_en=a.model_en, model_fr=a.model_fr,
                  preload=a.preload, vad_enabled=a.vad,
                  min_speech_seconds=a.min_speech_seconds, log_level=a.log_level,
                  second_opinion=a.second, fr_detector=a.fr_detector,
                  fr_threshold=a.fr_threshold, fr_no_fallback=a.fr_no_fallback,
                  fr_cut_loops=a.fr_cut_loops,
                  save_audio=a.save_audio,
                  save_audio_min_free_gb=a.save_audio_min_free_gb,
                  mlx_cache_limit_gb=a.mlx_cache_limit_gb,
                  exit_with_parent=a.exit_with_parent)
