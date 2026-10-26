"""The faster-whisper engine: CTranslate2 on an NVIDIA GPU, or on the processor (Q54).

For Linux (and later Windows) machines, where MLX does not run. The models are
CTranslate2 conversions (sidecar/README.md says how to make them); the HTTP
contract, the voice gate and everything else in whisper_server.py are the same.

The decoding is set to what mlx_whisper does wherever faster-whisper has the same
option, so that what was measured on the Mac has a chance to hold here; the
non-regression itself is measured on a real GPU (D73), not assumed.
"""
from __future__ import annotations

import logging
import os
import shutil
import subprocess
import time

import numpy as np

log = logging.getLogger("stt")

# mlx_whisper samples at most n_text_ctx // 2 = 224 tokens a window. faster-whisper,
# unless told, goes up to the whole context less the prompt -- 444 on a first
# window -- and a loop, which is what reaches the cap, runs twice as long. It also
# refuses a window whose prompt and max_new_tokens add up to more than 448, and the
# prompt can reach 227 (the previous text, cut to 223, and four markers): so 221,
# never refused, three tokens short of MLX's cap on a window that loops to its end.
MAX_NEW_TOKENS = 221

# mlx_whisper.transcribe's defaults, written out where faster-whisper's differ or
# might: mlx_whisper decodes greedily and has no beam search (its decoder raises on
# beam_size), and draws one sample per temperature above 0, where faster-whisper
# defaults to a beam of 5 and five samples.
DECODING = dict(
    beam_size=1,
    best_of=1,
    temperature=[0.0, 0.2, 0.4, 0.6, 0.8, 1.0],
    compression_ratio_threshold=2.4,
    log_prob_threshold=-1.0,
    no_speech_threshold=0.6,
    condition_on_previous_text=True,
    prompt_reset_on_temperature=0.5,
    suppress_blank=True,
    suppress_tokens=[-1],
    without_timestamps=False,
    max_initial_timestamp=1.0,
    word_timestamps=False,
    # The sidecar's own voice gate runs first (whisper_server.py).
    vad_filter=False,
    max_new_tokens=MAX_NEW_TOKENS,
)


def _local(path: str) -> str:
    return os.path.expanduser(path) if path.startswith(("/", "~", "./")) else path


def cuda_devices() -> int:
    import ctranslate2
    try:
        return ctranslate2.get_cuda_device_count()
    except Exception:
        return 0


def gpu_name() -> str:
    """The first NVIDIA GPU's name, as torch (installed for the voice gate) or
    nvidia-smi gives it; "CUDA" when neither does. A label, never a reason not
    to start."""
    try:
        import torch
        if torch.cuda.is_available():
            return torch.cuda.get_device_name(0).strip()[:48] or "CUDA"
    except Exception:
        pass
    smi = shutil.which("nvidia-smi")
    if smi:
        try:
            out = subprocess.run([smi, "--query-gpu=name", "--format=csv,noheader"],
                                 capture_output=True, text=True, timeout=5).stdout
            name = out.strip().splitlines()[0].strip() if out.strip() else ""
            if name:
                return name[:48]
        except Exception:
            pass
    return "CUDA"


class CT2Engine:
    name = "faster-whisper"

    def __init__(self, device: str = "auto", compute_type: str = "auto"):
        if device == "auto":
            device = "cuda" if cuda_devices() > 0 else "cpu"
        if compute_type == "auto":
            compute_type = "float16" if device == "cuda" else "int8"
        self.device, self.compute_type = device, compute_type
        self._models: dict = {}
        self._detector = None
        # faster-whisper says "Processing audio with duration ..." at INFO for
        # every transmission; the sidecar's own line says more.
        logging.getLogger("faster_whisper").setLevel(logging.WARNING)

    def backend(self) -> str:
        """The GPU's name on CUDA, "CPU" on the processor."""
        return gpu_name() if self.device == "cuda" else "CPU"

    def load(self, path: str):
        """Build the model, and run its encoder once: a GPU without the CUDA
        libraries CTranslate2 needs (cuBLAS, cuDNN) fails here, at start with
        --preload, rather than on the first transmission."""
        if path not in self._models:
            t0 = time.time()
            from faster_whisper import WhisperModel
            model = WhisperModel(_local(path), device=self.device, compute_type=self.compute_type)
            fe = model.feature_extractor
            model.encode(fe(np.zeros(fe.sampling_rate, dtype=np.float32), padding=fe.n_samples)
                         [..., :fe.nb_max_frames])
            self._models[path] = model
            log.info("model loaded: %s on %s, %s (%.2fs)", path, self.device, self.compute_type,
                     time.time() - t0)
        return self._models[path]

    def transcribe(self, model: str, audio, language: str,
                   initial_prompt: str = "", temperature: float | None = None) -> dict:
        m = self.load(model)
        options = dict(DECODING)
        if temperature is not None:
            options["temperature"] = temperature
        segments, _ = m.transcribe(audio, language=language,
                                   initial_prompt=initial_prompt or None, **options)
        # A generator: the decoding happens here, on the model thread.
        segments = [{"text": s.text, "start": s.start, "end": s.end, "temperature": s.temperature,
                     "avg_logprob": s.avg_logprob, "compression_ratio": s.compression_ratio,
                     "no_speech_prob": s.no_speech_prob} for s in segments]
        return {"text": "".join(s["text"] for s in segments), "segments": segments,
                "language": language}

    def detector(self, path: str):
        """The language-detection model, loaded once."""
        if self._detector is None:
            t0 = time.time()
            self._detector = self.load(path)
            log.info("language detector loaded: %s (%.2fs)", path, time.time() - t0)
        return self._detector

    def french_probability(self, detector: str, audio) -> float:
        """Whisper's language detection on the first 30 s: one encoder pass and
        one decoder step. The audio is padded with 30 s of silence before the
        spectrogram, as the MLX engine does, rather than the spectrogram with
        zeros, as faster-whisper's own detect_language does: what the model sees
        after a 3-second transmission is then the same on both engines."""
        model = self.detector(detector)
        fe = model.feature_extractor
        features = fe(np.asarray(audio, dtype=np.float32), padding=fe.n_samples)[..., :fe.nb_max_frames]
        results = model.model.detect_language(model.encode(features))[0]
        probs = {token[2:-2]: p for token, p in results}
        return float(probs.get("fr", 0.0))
