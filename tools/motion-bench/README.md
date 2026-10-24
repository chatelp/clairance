# Motion bench

The aircraft motion measured on screen, in a real browser
(docs-fr/fiches-cloud/M-mouvement-ecran.md). The real server, built from this
checkout, reads synthetic traffic from `gen/` (internal/adsb/simtraffic) as it
would a tar1090; the page it serves runs in Chrome, driven through the DevTools
protocol; `probe.js` reads in the page, after every frame the map draws, the
pose actually set on each aircraft's OpenLayers feature. The truth comes from
the generator only. Nothing reaches a receiver or leaves the machine: the
place is the made-up receiver of the replay bench, 45.0 / 4.67.

    node tools/motion-bench/bench.mjs                  # the six scenarios, ~15 min
    node tools/motion-bench/bench.mjs --scenario e     # one of them
    node tools/motion-bench/bench.mjs --headed         # a visible window (the real screen)
    node tools/motion-bench/bench.mjs --label X --save-reference   # keep the metrics in reference/X.json
    node tools/motion-bench/analyse.mjs OUT/raw-<label>-<scenario>.json   # measure a run again
    node tools/motion-bench/bench.mjs --profile 20      # also a CPU profile of the page, 20 s

Needs Go, Node 22 (its built-in WebSocket) and Chrome or Chromium (`--chrome
PATH`; the default is Google Chrome on macOS). Ports 8790 (generator) and 8791
(server) must be free, or set others with `--gen-port` and `--port`. Runs, logs and raw poses go to `$TMPDIR/motion-bench`
(`--out DIR`).

| scenario | what |
|---|---|
| a | the server restarted and the page reloaded, measured from the first second (`--restart server` keeps the page, `--restart page` keeps the server) |
| b | 60 aircraft, the column on Right now |
| c | 150 aircraft, the column on Traffic |
| d | 60 aircraft, a zoom from 10 to 50 NM and drags of the map while they fly |
| e | 60 aircraft, the window minimised 30 s, then shown again |
| f | 60 aircraft, one of them 18 s without a position at 45 NM, then a fix |

Each runs 60 s after a minute of warm-up (`--warmup`, `--measure`), but a.
The metrics are defined at the top of `metrics.mjs`; those of the replay bench
(www/aircraft-replay.test.js) keep their names so the two compare.
`--engine JSON` sets the animation engine's config in the page before
measuring, e.g. `--engine '{"strategy":"B"}'`.
