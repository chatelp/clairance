"""The MLX engine: mlx-whisper on Apple Silicon (D10).

What this station runs, and what every figure in docs-fr was measured with. The
code is the sidecar's own, moved here unchanged from whisper_server.py.
"""
from __future__ import annotations

import logging
import os
import sys
import time

from device import backend_name

log = logging.getLogger("stt")


class MLXEngine:
    name = "mlx"

    def __init__(self):
        self._models: dict[str, object] = {}
        self._kept_models: dict = {}
        self._detector = None

    def backend(self) -> str:
        """The GPU's name when MLX gives it, else "mlx" (device.py)."""
        return backend_name()

    def load(self, path: str):
        if path not in self._models:
            t0 = time.time()
            import mlx_whisper
            self.keep_every_model()
            # mlx-whisper resolves the model lazily on first transcribe; touching it
            # here just makes the cost visible in the logs instead of in the first request.
            self._models[path] = mlx_whisper
            log.info("model registered: %s (%.2fs)", path, time.time() - t0)
        return self._models[path]

    def keep_every_model(self) -> None:
        """One model kept in memory per path, instead of mlx_whisper's single place.

        mlx_whisper keeps one model at a time (ModelHolder): each switch between the
        English and the French reading built the model again, and the first decode
        of a model just built pays its start-up. Measured by whisper-lab on 14/10
        (latence-etapes, 200 + 50 transmissions of the 02-04/10 archive, compared in
        pairs): the English reading took 4.58 s alternating against 0.90 s kept, the
        French one 4.66 s against 0.99 s -- about 7.5 s more of the model's thread
        for every transmission whose French gate opens. The cost is one model more
        in memory, about 3 GB in fp16. The models are built where they are used, on
        the model's thread (lanes.py): MLX ties them to the stream that made them.
        The same way as the lab's bench, so that their figures stay comparable.
        """
        holder = sys.modules["mlx_whisper.transcribe"].ModelHolder
        if getattr(holder, "_clairance_keeps_every_model", False):
            return
        from mlx_whisper.load_models import load_model
        kept = self._kept_models

        def get_model(cls, path, dtype):
            key = (path, dtype)
            if key not in kept:
                t0 = time.time()
                kept[key] = load_model(path, dtype=dtype)
                log.info("model built: %s (%.2fs)", path, time.time() - t0)
            return kept[key]

        holder.get_model = classmethod(get_model)
        holder._clairance_keeps_every_model = True

    def transcribe(self, model: str, audio, language: str,
                   initial_prompt: str = "", temperature: float | None = None) -> dict:
        mlx_whisper = self.load(model)
        kwargs = {"path_or_hf_repo": model, "language": language}
        if initial_prompt:
            kwargs["initial_prompt"] = initial_prompt
        if temperature is not None:
            kwargs["temperature"] = temperature
        return mlx_whisper.transcribe(audio, **kwargs)

    def detector(self, path: str):
        """The language-detection model, loaded once. Unlike the transcription
        models it is held here: detection calls the model directly, not
        mlx_whisper.transcribe."""
        if self._detector is None:
            t0 = time.time()
            import mlx.core as mx
            from mlx_whisper.load_models import load_model
            model = load_model(os.path.expanduser(path)
                               if path.startswith(("/", "~", "./")) else path)
            # Loaded now rather than at the first detection, which would add the
            # load to that transmission's wait. Called on the model thread.
            mx.eval(model.parameters())
            self._detector = model
            log.info("language detector loaded: %s (%.2fs)", path, time.time() - t0)
        return self._detector

    def french_probability(self, detector: str, audio) -> float:
        """Whisper's language detection on the first 30 s: the probability that the
        transmission is French. Costs one encoder pass -- about 0.6 s with
        large-v3-turbo on this machine -- and no decoding."""
        import mlx.core as mx
        from mlx_whisper.audio import N_FRAMES, N_SAMPLES, log_mel_spectrogram, pad_or_trim
        model = self.detector(detector)
        mel = log_mel_spectrogram(mx.array(audio), n_mels=model.dims.n_mels, padding=N_SAMPLES)
        _, probs = model.detect_language(pad_or_trim(mel, N_FRAMES, axis=-2).astype(mx.float16))
        return float(probs.get("fr", 0.0))
