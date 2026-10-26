"""What turns audio into text, behind the sidecar's model thread.

The HTTP contract, the voice gate, the two lanes, the French gate and the archive
do not depend on it (whisper_server.py). What does is kept behind five calls, all
made on the model thread (lanes.py), one at a time:

    backend()                   what /health declares in `backend`
    load(model)                 get a model ready (a path or a hub id)
    transcribe(model, audio, language, initial_prompt="", temperature=None)
                                audio is mono float32 at 16 kHz; the answer has
                                mlx_whisper's shape: {"text", "language",
                                "segments": [{"text", "start", "end", "temperature",
                                "avg_logprob", "compression_ratio", "no_speech_prob"}]}
                                temperature None: the engine's fallback over rising
                                temperatures; a number: that one decode only
    detector(model)             get the language detector ready
    french_probability(detector, audio)
                                Whisper's language detection on the first 30 s:
                                the probability that the audio is French

Two engines: MLX (mlx-whisper, Apple Silicon: what this station runs, and what
every figure in docs-fr was measured with) and faster-whisper (CTranslate2: an
NVIDIA GPU or the processor, on Linux; Q54). The engine is chosen once, at start
(pick).
"""
from __future__ import annotations

import importlib.util


def available(module: str) -> bool:
    """Whether a module can be found, without importing it: importing MLX at
    start would move its cost out of the first request, which is where it has
    always been paid."""
    try:
        return importlib.util.find_spec(module) is not None
    except (ImportError, ValueError):
        return False


def resolve(name: str = "auto") -> str:
    """The engine `auto` stands for: MLX where it can be imported, else
    faster-whisper."""
    if name != "auto":
        return name
    if available("mlx_whisper"):
        return "mlx"
    if available("faster_whisper"):
        return "faster-whisper"
    raise RuntimeError("no transcription engine is installed: install sidecar/requirements-mlx.txt "
                       "(Apple Silicon) or sidecar/requirements-ct2.txt (Linux, NVIDIA or processor)")


def pick(name: str = "auto", device: str = "auto", compute_type: str = "auto"):
    """The engine called `name`: auto, mlx or faster-whisper. device and
    compute_type are faster-whisper's; MLX has neither."""
    name = resolve(name)
    if name == "mlx":
        from engine_mlx import MLXEngine
        return MLXEngine()
    if name == "faster-whisper":
        if not available("ctranslate2"):
            raise RuntimeError("faster-whisper is not installed: install sidecar/requirements-ct2.txt")
        from engine_ct2 import CT2Engine
        return CT2Engine(device=device, compute_type=compute_type)
    raise ValueError(f"unknown engine {name!r}")
