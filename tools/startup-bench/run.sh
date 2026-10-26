#!/usr/bin/env bash
# Startup bench: build the server, start it on a copy of the example
# configuration against a fake tar1090 and a fake sidecar, and check that it
# comes up, serves, shows the aircraft and stops cleanly on SIGINT. The log,
# timestamps removed, is written to <out>/server.log for comparison with a run
# of another commit:
#
#   tools/startup-bench/run.sh /tmp/before && git checkout <other> &&
#   tools/startup-bench/run.sh /tmp/after && tools/startup-bench/compare.py /tmp/before /tmp/after
#
# Ports: BENCH_PORT (default 18700) to BENCH_PORT+5, and BENCH_PORT+10 for the fakes.
set -euo pipefail

out=${1:?usage: run.sh <output dir>}
repo=$(cd "$(dirname "$0")/../.." && pwd)
here="$repo/tools/startup-bench"
port=${BENCH_PORT:-18700}
fake=$((port + 10))

mkdir -p "$out"
out=$(cd "$out" && pwd)
work=$(mktemp -d)
fakepid=""; srvpid=""
trap 'kill $fakepid 2>/dev/null || true; kill -9 $srvpid 2>/dev/null || true; rm -rf "$work"' EXIT

fail() { echo "FAIL: $*" >&2; echo "log: $out/raw.log" >&2; exit 1; }

(cd "$repo" && go build -o "$work/co-atc" ./cmd/server)
# No www/ beside it: the binary serves the page built into it.
ln -s "$repo/assets" "$work/assets"

# The example, pointed at the fakes, logging to stdout. In Python rather than
# sed, which differs between macOS and Linux.
python3 - "$repo/configs/config.toml.example" "$work/config.toml" "$port" "$fake" <<'PY'
import re, sys
src, dst, port, fake = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
text = open(src).read()
subs = [
    (r"(?m)^port = 8000\b", f"port = {port}"),
    (r"(?m)^additional_ports = .*$", f"additional_ports = {[port + i for i in range(1, 5)]}"),
    (r'(?m)^source_type = .*$', 'source_type = "tar1090"'),
    (r'(?m)^tar1090_base_url = .*$', f'tar1090_base_url = "http://127.0.0.1:{fake}/data/"'),
    (r'(?m)^url = "[^"]*"', f'url = "http://127.0.0.1:{fake}/audio"'),
    (r'(?m)^file = "runs/co-atc.log"', 'file = ""'),
    (r"(?m)^\[transcription.local\]$", f'[transcription.local]\nserver_url = "http://127.0.0.1:{fake}"'),
    (r"(?m)^\[wx\]$", f'[wx]\napi_base_url = "http://127.0.0.1:{fake}/wx"'),
]
for pattern, repl in subs:
    text, n = re.subn(pattern, repl, text)
    if n == 0:
        sys.exit(f"the example no longer has {pattern!r}")
open(dst, "w").write(text)
PY

python3 "$here/fake.py" "$fake" & fakepid=$!
for _ in $(seq 50); do curl -sf "http://127.0.0.1:$fake/health" >/dev/null && break; sleep 0.1; done

(cd "$work" && exec ./co-atc -config config.toml) > "$out/raw.log" 2>&1 & srvpid=$!

for _ in $(seq 600); do grep -q "Starting HTTP server" "$out/raw.log" && break; kill -0 $srvpid 2>/dev/null || fail "server exited"; sleep 0.1; done
grep -q "Starting HTTP server" "$out/raw.log" || fail "no 'Starting HTTP server' within 60 s"

code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/api/v1/health")
[ "$code" = 200 ] || fail "/api/v1/health answered $code"
code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/")
[ "$code" = 200 ] || fail "/ answered $code"
seen=""
for _ in $(seq 150); do
  if curl -s "http://127.0.0.1:$port/api/v1/aircraft" | grep -q '"c0ffee"'; then seen=1; break; fi
  sleep 0.1
done
[ -n "$seen" ] || fail "the aircraft never came back from /api/v1/aircraft"

kill -INT $srvpid
for _ in $(seq 200); do kill -0 $srvpid 2>/dev/null || break; sleep 0.1; done
kill -0 $srvpid 2>/dev/null && fail "still running 20 s after SIGINT"
wait $srvpid && status=0 || status=$?
[ "$status" = 0 ] || fail "exit status $status after SIGINT"
grep -q "Server fully stopped" "$out/raw.log" || fail "no 'Server fully stopped'"

# Timestamps off, the temporary directory named the same way every run.
python3 - "$out/raw.log" "$out/server.log" "$work" <<'PY'
import re, sys
raw, dst, work = sys.argv[1:]
with open(raw, encoding="utf-8", errors="replace") as f, open(dst, "w") as out:
    for line in f:
        line = re.sub(r"^\d{4}-\d{2}-\d{2}T\S+\s+", "", line)
        out.write(line.replace(work, "WORK"))
PY
echo "PASS: started, /api/v1/health 200, / 200, aircraft served, clean stop on SIGINT ($out/server.log)"
