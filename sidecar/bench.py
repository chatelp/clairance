"""Time a running sidecar on one transmission, and read its memory.

    python sidecar/bench.py speech.raw [--url http://127.0.0.1:8178] [--rate 24000]
                            [--runs 10] [--language en] [--later]

speech.raw is s16le mono PCM at --rate, as co-atc sends it. The first answer is
not counted (the voice gate loads then). Prints, as JSON: the clip's length, each
run's `elapsed` as the sidecar measures it, the median and its real-time factor
(the clip's length over the time to transcribe it, the sidecar's own
`realtime_factor`), and the sidecar's resident memory now and at its peak
(Linux; now only elsewhere). The same protocol on the processor and on a GPU.
"""
import argparse
import json
import statistics
import subprocess
import urllib.request


def memory_mb(pid: int) -> dict:
    try:
        with open(f"/proc/{pid}/status") as f:
            fields = dict(line.split(":", 1) for line in f if ":" in line)
        return {k: round(int(fields[v].split()[0]) / 1024) for k, v in (("rss_mb", "VmRSS"), ("peak_mb", "VmHWM"))}
    except OSError:
        out = subprocess.run(["ps", "-o", "rss=", "-p", str(pid)], capture_output=True, text=True).stdout
        return {"rss_mb": round(int(out.strip() or 0) / 1024)}


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("pcm")
    p.add_argument("--url", default="http://127.0.0.1:8178")
    p.add_argument("--rate", type=int, default=24000)
    p.add_argument("--runs", type=int, default=10)
    p.add_argument("--language", default="en")
    p.add_argument("--later", action="store_true", help="send X-Second: later, as co-atc does")
    a = p.parse_args()
    with open(a.pcm, "rb") as f:
        pcm = f.read()
    headers = {"X-Sample-Rate": str(a.rate), "X-Channels": "1", "X-Language": a.language,
               "X-Frequency-Id": "bench"}
    if a.later:
        headers["X-Second"] = "later"

    def send() -> dict:
        req = urllib.request.Request(a.url + "/transcribe", data=pcm, headers=headers, method="POST")
        with urllib.request.urlopen(req, timeout=900) as r:
            return json.load(r)

    send()
    runs = [send() for _ in range(a.runs)]
    with urllib.request.urlopen(a.url + "/health", timeout=60) as r:
        health = json.load(r)
    elapsed = [r["elapsed"] for r in runs]
    duration = runs[0]["duration"]
    median = statistics.median(elapsed)
    print(json.dumps({
        "backend": health.get("backend"), "model": health.get("model_en"),
        "duration_s": duration, "elapsed_s": elapsed, "median_s": median,
        "realtime_factor": round(duration / median, 2),
        "text": runs[-1]["text"], "second_text": runs[-1].get("second_text", ""),
        **memory_mb(health["pid"]),
    }, ensure_ascii=False, indent=1))


if __name__ == "__main__":
    main()
