"""The sidecar's HTTP contract, against the real sidecar and a real model.

    SIDECAR_TEST_MODEL=<multilingual model> python -m unittest sidecar/test_contract.py

SIDECAR_TEST_MODEL is a multilingual Whisper the engine reads (a CTranslate2
directory with faster-whisper, an MLX one or a hub id with MLX); it serves as the
English model, the French model and the detector at once. SIDECAR_TEST_ENGINE
chooses the engine (default auto). Skipped without a model. The speech is
synthesised by ffmpeg's flite filter, so ffmpeg must have it; with go on the PATH,
the Go client is run against the same sidecar (TestAgainstARealSidecar).

The keys checked are the ones co-atc reads: internal/transcription/sidecar.go
(Health), local.go (the /transcribe and /second answers) and
internal/audioarchive (the manifest).
"""
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import unittest
import urllib.error
import urllib.request
import wave

HERE = os.path.dirname(os.path.abspath(__file__))
MODEL = os.environ.get("SIDECAR_TEST_MODEL", "")
ENGINE = os.environ.get("SIDECAR_TEST_ENGINE", "auto")
SPEECH = ("Air France one two three, descend flight level eight zero, "
          "contact Paris approach one two four decimal three five, goodbye.")


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def flite(text: str, path: str) -> None:
    """Speech as co-atc sends it: s16le, mono, 24 kHz. ffmpeg's flite filter,
    or macOS's own voice where ffmpeg has no flite (Homebrew's has none)."""
    if has_ffmpeg_flite():
        src = ["-f", "lavfi", "-i", f"flite=text='{text}':voice=slt"]
    else:
        aiff = path + ".aiff"
        subprocess.run(["say", "-o", aiff, text], check=True)
        src = ["-i", aiff]
    subprocess.run(["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", *src,
                    "-ar", "24000", "-ac", "1", "-f", "s16le", path], check=True)


def has_ffmpeg_flite() -> bool:
    if not shutil.which("ffmpeg"):
        return False
    out = subprocess.run(["ffmpeg", "-hide_banner", "-filters"], capture_output=True, text=True).stdout
    return " flite " in out


def has_flite() -> bool:
    return has_ffmpeg_flite() or bool(shutil.which("ffmpeg") and shutil.which("say"))


class Sidecar:
    def __init__(self, *args: str):
        self.port = free_port()
        self.url = f"http://127.0.0.1:{self.port}"
        self.log = tempfile.NamedTemporaryFile("w+", suffix=".log", delete=False)
        env = {k: v for k, v in os.environ.items() if k != "COATC_SPAWNED"}
        self.proc = subprocess.Popen(
            [sys.executable, os.path.join(HERE, "whisper_server.py"), "--port", str(self.port),
             "--engine", ENGINE, "--no-exit-with-parent", *args],
            stdout=self.log, stderr=subprocess.STDOUT, env=env)
        deadline = time.time() + 600
        while time.time() < deadline:
            if self.proc.poll() is not None:
                raise RuntimeError(f"the sidecar exited: {self.output()}")
            try:
                self.get("/health")
                return
            except OSError:
                time.sleep(0.5)
        self.stop()
        raise RuntimeError(f"the sidecar did not answer: {self.output()}")

    def output(self) -> str:
        self.log.flush()
        with open(self.log.name) as f:
            return f.read()[-3000:]

    def get(self, path: str):
        with urllib.request.urlopen(self.url + path, timeout=600) as r:
            return r.status, json.load(r)

    def post(self, pcm: bytes, **headers):
        h = {"X-Sample-Rate": "24000", "X-Channels": "1", "X-Language": "en", "X-Frequency-Id": "124350"}
        h.update({k.replace("_", "-"): v for k, v in headers.items()})
        req = urllib.request.Request(self.url + "/transcribe", data=pcm, headers=h, method="POST")
        with urllib.request.urlopen(req, timeout=600) as r:
            return json.load(r)

    def stop(self):
        self.proc.terminate()
        try:
            self.proc.wait(10)
        except subprocess.TimeoutExpired:
            self.proc.kill()
        os.unlink(self.log.name)


@unittest.skipUnless(MODEL, "SIDECAR_TEST_MODEL not set")
class Contract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not has_flite():
            raise unittest.SkipTest("ffmpeg, with the flite filter or macOS's say, is needed to synthesise speech")
        cls.tmp = tempfile.mkdtemp(prefix="sidecar-contract-")
        cls.speech_path = os.path.join(cls.tmp, "speech.raw")
        flite(SPEECH, cls.speech_path)
        with open(cls.speech_path, "rb") as f:
            cls.speech = f.read()
        cls.archive = os.path.join(cls.tmp, "archive")
        # The French gate opens on every transmission (threshold 0), so that a
        # second reading follows without French audio.
        cls.full = Sidecar("--model-en", MODEL, "--model-fr", MODEL, "--preload", "--second-opinion",
                           "--fr-detector", MODEL, "--fr-threshold", "0", "--fr-no-fallback",
                           "--save-audio", cls.archive, "--save-audio-min-free-gb", "0")
        # No French model, and nothing loaded: the refusal comes before any model.
        cls.english_only = Sidecar("--model-en", MODEL)

    @classmethod
    def tearDownClass(cls):
        cls.full.stop()
        cls.english_only.stop()
        shutil.rmtree(cls.tmp, ignore_errors=True)

    def test_health_says_everything_the_server_reads(self):
        status, h = self.full.get("/health")
        self.assertEqual(status, 200)
        self.assertIn(h["status"], ("ok", "degraded"))
        self.assertEqual(h["degraded"], [])
        self.assertEqual(h["pid"], self.full.proc.pid)
        self.assertIsInstance(h["backend"], str)
        self.assertTrue(0 < len(h["backend"]) <= 48)
        self.assertEqual(h["model_en"], MODEL)
        self.assertIs(h["second_opinion"], True)
        for key in ("second_opinion_ok", "second_opinion_failed", "second_pending"):
            self.assertIsInstance(h[key], int)
        self.assertEqual(set(h["queue"]), {"first", "second"})
        self.assertIs(h["archiving"], True)
        self.assertIs(h["archiving_stopped_low_disk"], False)
        self.assertEqual(set(h["models"]), {"en", "fr", "detector"})
        for m in h["models"].values():
            self.assertEqual(m["id"], MODEL)
            self.assertIn(m["kind"], ("path", "hub"))
            self.assertIs(m["available"], True)
        self.assertEqual(self.english_only.get("/health")[1]["languages"], ["en"])

    def test_speech_is_transcribed(self):
        before = self.full.get("/health")[1]["transcribed"]
        r = self.full.post(self.speech)
        self.assertIn("france", r["text"].lower())
        self.assertEqual(r["language"], "en")
        self.assertFalse(r.get("rejected"))
        self.assertAlmostEqual(r["duration"], len(self.speech) / 2 / 24000, places=2)
        self.assertGreater(r["speech_seconds"], 3)
        self.assertGreater(r["realtime_factor"], 0)
        self.assertEqual(r["frequency_id"], "124350")
        for s in r["segments"]:
            self.assertEqual(set(s), {"text", "start", "end", "no_speech_prob"})
        # Without X-Second the second reading comes in the same answer.
        self.assertTrue(r["second_text"])
        self.assertEqual((r["second_language"], r["second_gate"], r["second_pending"]), ("fr", "detector", ""))
        self.assertIsInstance(r["p_fr"], float)
        self.assertEqual(self.full.get("/health")[1]["transcribed"], before + 1)

    def test_without_speech_nothing_is_transcribed(self):
        before = self.full.get("/health")[1]["rejected_no_speech"]
        r = self.full.post(b"\0\0" * 24000 * 2)
        self.assertEqual((r["text"], r["rejected"], r["segments"]), ("", "no_speech", []))
        self.assertEqual(r["duration"], 2.0)
        self.assertEqual(self.full.get("/health")[1]["rejected_no_speech"], before + 1)

    def test_a_language_without_model_is_refused(self):
        r = self.english_only.post(self.speech, X_Language="fr")
        self.assertEqual((r["text"], r["rejected"], r["language"]), ("", "no_model_for_fr", "fr"))

    def test_a_second_reading_asked_for_later_is_fetched_once(self):
        r = self.full.post(self.speech, X_Second="later")
        self.assertIn("france", r["text"].lower())
        self.assertEqual(r["second_text"], "")
        key = r["second_pending"]
        self.assertTrue(key)
        status, second = self.full.get(f"/second/{key}?wait=300")
        self.assertEqual(status, 200)
        self.assertEqual(second["status"], "done")
        self.assertTrue(second["second_text"])
        self.assertEqual((second["second_language"], second["second_gate"]), ("fr", "detector"))
        for path in (f"/second/{key}?wait=0", "/second/no-such-id?wait=0"):
            with self.assertRaises(urllib.error.HTTPError) as e:
                self.full.get(path)
            self.assertEqual(e.exception.code, 404)

    def test_the_archive_keeps_each_transmission_as_it_came(self):
        self.full.post(self.speech, X_Created_At="2026-10-26T12:00:00Z", X_Segment_At="2026-10-26T12:00:00.123Z")
        lines = []
        for root, _, files in os.walk(self.archive):
            if "manifeste.jsonl" in files:
                with open(os.path.join(root, "manifeste.jsonl")) as f:
                    lines += [json.loads(line) for line in f]
        line = [x for x in lines if x.get("created_at") == "2026-10-26T12:00:00Z"][0]
        self.assertEqual((line["frequency_id"], line["segment_at_utc"]), ("124350", "2026-10-26T12:00:00.123Z"))
        self.assertEqual((line["langue"], line["modele"], line["porte"]), ("en", MODEL, "detector"))
        self.assertTrue(line["texte"] and line["texte_second"])
        self.assertTrue(line["segments"] and line["segments_second"])
        with wave.open(os.path.join(self.archive, line["fichier"])) as w:
            self.assertEqual((w.getframerate(), w.getnchannels(), w.getnframes()), (24000, 1, len(self.speech) // 2))

    @unittest.skipUnless(shutil.which("go"), "go not on the PATH")
    def test_the_go_client_against_this_sidecar(self):
        env = dict(os.environ, CLAIRANCE_SIDECAR_URL=self.full.url, CLAIRANCE_SIDECAR_PCM=self.speech_path,
                   CLAIRANCE_SIDECAR_EXPECT_SECOND="1")
        out = subprocess.run(["go", "test", "-count=1", "-v", "-run", "TestAgainstARealSidecar",
                              "./internal/transcription/"], cwd=os.path.dirname(HERE), env=env,
                             capture_output=True, text=True, timeout=900)
        self.assertEqual(out.returncode, 0, out.stdout[-3000:] + out.stderr[-2000:])
        self.assertIn("--- PASS: TestAgainstARealSidecar", out.stdout)


if __name__ == "__main__":
    unittest.main()
