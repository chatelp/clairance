"""python3 -m unittest sidecar/test_engines.py -- needs numpy, no model and no engine installed."""
import os
import sys
import types
import unittest
from collections import namedtuple

import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import engine_ct2  # noqa: E402
import engines  # noqa: E402

Segment = namedtuple("Segment", "text start end temperature avg_logprob compression_ratio no_speech_prob")


class FakeFeatures:
    sampling_rate, n_samples, nb_max_frames = 16000, 480000, 3000

    def __init__(self, calls):
        self.calls = calls

    def __call__(self, audio, padding=160):
        self.calls.append(("features", len(audio), padding))
        return np.zeros((80, (len(audio) + padding) // 160), dtype=np.float32)


class FakeCT2:
    def detect_language(self, encoded):
        return [[("<|en|>", 0.7), ("<|fr|>", 0.2), ("<|de|>", 0.1)]]


class FakeWhisperModel:
    """What CT2Engine uses of faster_whisper.WhisperModel, recording each call."""
    calls = []

    def __init__(self, path, device, compute_type):
        self.calls.append(("init", path, device, compute_type))
        self.feature_extractor = FakeFeatures(self.calls)
        self.model = FakeCT2()

    def encode(self, features):
        self.calls.append(("encode", features.shape))
        return "encoded"

    def transcribe(self, audio, **kw):
        self.calls.append(("transcribe", len(audio), kw))
        return iter([Segment(" Air France 123,", 0.0, 1.5, 0.0, -0.2, 1.1, 0.01),
                     Segment(" descend level 80.", 1.5, 3.0, 0.0, -0.3, 1.2, 0.02)]), None


class EngineChoice(unittest.TestCase):
    def setUp(self):
        self.real = engines.available

    def tearDown(self):
        engines.available = self.real

    def test_auto_is_mlx_where_mlx_can_be_imported(self):
        engines.available = lambda m: True
        self.assertEqual(engines.resolve("auto"), "mlx")

    def test_auto_is_faster_whisper_without_mlx(self):
        engines.available = lambda m: m == "faster_whisper"
        self.assertEqual(engines.resolve("auto"), "faster-whisper")

    def test_without_any_engine_it_says_what_to_install(self):
        engines.available = lambda m: False
        with self.assertRaisesRegex(RuntimeError, "requirements-mlx.txt.*requirements-ct2.txt"):
            engines.resolve("auto")

    def test_a_named_engine_is_taken_as_named(self):
        engines.available = lambda m: True
        self.assertEqual(engines.resolve("faster-whisper"), "faster-whisper")
        self.assertEqual(engines.pick("mlx").name, "mlx")

    def test_finding_a_module_does_not_import_it(self):
        self.assertFalse(engines.available("no_such_module_here"))
        self.assertTrue(engines.available("unittest"))


class FasterWhisper(unittest.TestCase):
    def setUp(self):
        FakeWhisperModel.calls = []
        fake = types.ModuleType("faster_whisper")
        fake.WhisperModel = FakeWhisperModel
        sys.modules["faster_whisper"] = fake
        self.devices = engine_ct2.cuda_devices

    def tearDown(self):
        sys.modules.pop("faster_whisper", None)
        engine_ct2.cuda_devices = self.devices

    def engine(self, gpus=0, **kw):
        engine_ct2.cuda_devices = lambda: gpus
        return engine_ct2.CT2Engine(**kw)

    def test_the_gpu_in_float16_when_there_is_one_else_the_processor_in_int8(self):
        e = self.engine(gpus=1)
        self.assertEqual((e.device, e.compute_type), ("cuda", "float16"))
        e = self.engine(gpus=0)
        self.assertEqual((e.device, e.compute_type), ("cpu", "int8"))
        self.assertEqual(e.backend(), "CPU")

    def test_device_and_compute_type_can_be_named(self):
        e = self.engine(gpus=1, device="cpu", compute_type="float32")
        self.assertEqual((e.device, e.compute_type), ("cpu", "float32"))
        e = self.engine(gpus=1, compute_type="int8_float16")
        self.assertEqual((e.device, e.compute_type), ("cuda", "int8_float16"))

    def test_a_model_is_built_once_and_its_encoder_run_at_load(self):
        e = self.engine()
        e.load("/models/en")
        e.load("/models/en")
        inits = [c for c in FakeWhisperModel.calls if c[0] == "init"]
        self.assertEqual(inits, [("init", "/models/en", "cpu", "int8")])
        self.assertIn(("encode", (80, 3000)), FakeWhisperModel.calls)

    def test_the_decoding_is_mlx_whispers(self):
        e = self.engine()
        out = e.transcribe("/models/en", np.zeros(16000, dtype=np.float32), "en")
        kw = [c for c in FakeWhisperModel.calls if c[0] == "transcribe"][0][2]
        self.assertEqual(kw["language"], "en")
        self.assertEqual((kw["beam_size"], kw["best_of"]), (1, 1))
        self.assertEqual(kw["temperature"], [0.0, 0.2, 0.4, 0.6, 0.8, 1.0])
        self.assertEqual(kw["max_new_tokens"], 221)
        self.assertIsNone(kw["initial_prompt"])
        self.assertFalse(kw["vad_filter"])
        self.assertTrue(kw["condition_on_previous_text"])
        self.assertEqual(out["text"], " Air France 123, descend level 80.")
        self.assertEqual(out["language"], "en")
        self.assertEqual(set(out["segments"][0]), {"text", "start", "end", "temperature", "avg_logprob",
                                                   "compression_ratio", "no_speech_prob"})

    def test_one_temperature_is_one_decode_and_a_prompt_is_passed(self):
        e = self.engine()
        e.transcribe("/models/fr", np.zeros(16000, dtype=np.float32), "fr",
                     initial_prompt="Roissy", temperature=0.0)
        kw = [c for c in FakeWhisperModel.calls if c[0] == "transcribe"][0][2]
        self.assertEqual((kw["temperature"], kw["initial_prompt"], kw["language"]), (0.0, "Roissy", "fr"))
        self.assertEqual(engine_ct2.DECODING["temperature"], [0.0, 0.2, 0.4, 0.6, 0.8, 1.0])

    def test_the_french_probability_reads_30_s_padded_with_silence(self):
        e = self.engine()
        p = e.french_probability("/models/detector", np.zeros(48000, dtype=np.float32))
        self.assertAlmostEqual(p, 0.2)
        self.assertIn(("features", 48000, 480000), FakeWhisperModel.calls)
        self.assertEqual([c for c in FakeWhisperModel.calls if c[0] == "encode"][-1], ("encode", (80, 3000)))

    def test_the_detector_is_shared_with_a_model_of_the_same_path(self):
        e = self.engine()
        e.load("/models/turbo")
        e.detector("/models/turbo")
        self.assertEqual(len([c for c in FakeWhisperModel.calls if c[0] == "init"]), 1)


if __name__ == "__main__":
    unittest.main()
