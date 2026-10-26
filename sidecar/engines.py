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

The engine is chosen once, at start (pick).
"""
from __future__ import annotations


def pick(name: str = "mlx"):
    """The engine called `name`."""
    if name == "mlx":
        from engine_mlx import MLXEngine
        return MLXEngine()
    raise ValueError(f"unknown engine {name!r}")
