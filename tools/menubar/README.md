# Menu bar launcher (macOS)

An icon in the menu bar, and by default in the Dock, that says whether co-atc's
production runs and whether it is healthy, starts and stops it, and opens the
page. It notifies when production goes deaf, stops by itself or degrades.
Nothing starts on its own: production runs only when asked. The forms and the
words are docs-fr/design/10-lanceur.md; the checks and their measures are
docs-fr/34-lanceur.md.

Below, every co-atc running on this Mac, production and test alike, read from
the kernel by PID and from its own config and server: the page it serves (the
branch and commit when its `www` is a checkout's), the server's version, its
aircraft and transmissions of the day, its transcription (real model or fake,
the sidecar's health and queue), its station link (`radio-ctl-sync`), the app
it depends on for macOS, its ports, config, databases and log. Production is
the co-atc running `configs/config.toml` of the checkout. A test instance can
be stopped from there: by its own PIDs, co-atc, its station link and a sidecar
co-atc did not start, after a confirmation. Sidecars and station links running
without an instance are listed apart. No process is ever found or stopped by a
pattern in its command line.

It does not run production itself. It runs the script a double-click would
(`production_command`, by default `runs/production.command`), so sign-in, the
station link and the clean stop stay in one place, and stops it with SIGTERM to
that script, co-atc's parent, which its trap turns into a clean stop of co-atc,
its sidecar and the station link. A script started from Terminal is stopped the
same way.

That script is the installation's own, not part of the repository (`runs/` is not
tracked): it starts `bin/co-atc` with its configuration, which starts the
transcription sidecar, then the station link `bin/radio-ctl-sync`, and stops all
three on SIGTERM.

```sh
tools/menubar/build.sh --install   # builds Clairance.app, copies it to ~/Applications
```

Needs Xcode's command line tools and macOS 13 or later. Optional settings in
`~/.config/clairance/launcher.json` (outside the repository):

```json
{
  "repo": "/path/to/clairance",
  "production_command": "runs/production.command",
  "production_config": "configs/config.toml",
  "production_log": "runs/production.log",
  "station_status_url": "http://your-station/radio/etat",
  "tailscale_url": "http://100.x.y.z:8000/",
  "planning_file": "/path/to/a/markdown/table/of/gpu/use.md",
  "silence_minutes": 10
}
```

`~/.config/clairance/codesign-identity`, if present, names a signing identity
of this Mac's keychain that build.sh signs the app with (otherwise ad hoc): the
app then keeps one identity from one build to the next.

The app's own settings (Settings…, ⌘,) say whether it also shows in the Dock;
by default it does, and a click on its Dock icon opens them.

The app's icon is `AppIcon.icon`, an Icon Composer document: since macOS 26 the
system draws an app's icon from such a file, and shrinks an older one into a
frame. build.sh compiles it with Xcode's actool when Xcode is installed; with
the command line tools alone it uses `AppIcon.icns`, which `icon.sh` makes
from the same document.

The server the script starts says which build it is (settings panel, sign-in
page) only if it was built with its version and commit; a script that builds
co-atc before starting it should pass them:

```sh
go build -ldflags "-X main.version=$(git describe --tags --match '[0-9]*' --always) -X main.commit=$(git rev-parse --short HEAD)" \
  -o bin/co-atc ./cmd/server
```

## What production depends on

For macOS, a process depends on the app that started it, and runs on that
app's Local Network permission. When that app quits, the process keeps running
but loses the local network at once and for good: no station, nothing
transcribed (measured on 24/10/2026; this is what happened from 16 to 18/10,
when production depended on a Claude session that restarted).

So the launcher does not start production as its own child. It opens a
carrier: another instance of Clairance.app, opened by the system, that waits
for the station to answer, runs the production script, and leaves with it. Test
instances run the same way. Measured on 24/10:

- what a carrier runs depends on the carrier, not on the menu;
- the menu can quit while what it carries keeps hearing the station;
- a SIGTERM to the carrier stops what it carries, and the carrier leaves when
  what it carries stops.

But the app must not be replaced on disk while a carrier runs from it: on
26/10 an install of a new build took production's local network away at once,
though the carrier ran on (an earlier test had replaced the app with an
identical build, which the deterministic compiler had made). `build.sh
--install` refuses while one runs: stop production, install, start it again.

While a carrier runs, macOS takes Clairance for open: opening it again (a
double-click, Spotlight, `open`) reaches the carrier, which then opens the
menu. The launcher itself, run from a terminal, has the system open it again
and leaves. A process that still depends on the menu, or on another app, is
shown as such in its details ("Depends on:"), and quitting the menu asks first
while something depends on it.

## The guard

Every 5 s the launcher reads production: its process, its server
(`/api/v1/health`, and `/api/v1/health/detail`, open to this Mac only), its
sidecar, the station's own page, the disks and macOS's memory pressure. An
incident opens once its condition has held for its time, closes once it has
been gone for 10 s, and is written to `events.jsonl` beside the socket:

| Incident | When | Sign |
|---|---|---|
| Stopped by itself | co-atc gone, no stop asked (by the launcher, or the script's "arret demande") | Down |
| Did not start | a start that ended without production answering; its cause read in co-atc's log | Down |
| Not answering | the server silent for 45 s | Down |
| Deaf | no ADS-B read for 30 s, held 15 s; the launcher reads the station itself to tell "the station does not answer" from "no local network for co-atc" | Fault |
| No local network | production depends on no app any more | Fault |
| Started from another app | production depends on Terminal, Claude… | Fault |
| No audio on a frequency | nothing decoded for 60 s | Fault |
| Station link stopped | radio-ctl-sync gone, held 15 s | Fault |
| Transcription | sidecar silent 45 s, or a language not loaded for 60 s | Fault |
| Nothing transcribed | no transmission cut for 10 min from 8 h to 23 h, 45 min at night, while audio arrives and the station is on air | Fault |
| Archive disk missing, disk under 20 GB (5 GB), memory pressure for 2 min | | Fault |
| A test instance running 12 h, a helper left 10 min without an instance | | — |

Notifications are macOS's own, one per incident and replaced by a second when
it ends; their buttons that stop or restart ask first. They need Clairance
allowed in System Settings > Notifications: switched off there, macOS refuses
them without asking. `clairance --status` says whether they are allowed.

## A test bench

A second launcher can run beside the owner's on its own settings and folder,
to provoke failures on a test instance without touching the station or
production: `CLAIRANCE_LAUNCHER_CONFIG` names its launcher.json,
`CLAIRANCE_SUPPORT_DIR` the folder of its socket and journals (both passed to
`open --env`, and to the command line).

## From a terminal, a Shortcut or an agent

```sh
C=~/Applications/Clairance.app/Contents/MacOS/clairance
$C --status
$C --start                                   # or --stop
$C --start-test runs/my-test/config.toml --for 2h [--binary path/to/co-atc]
$C --stop-test runs/my-test/config.toml
```

`--start` and `--start-test` never start anything themselves: they ask the
running launcher, through a socket in `~/Library/Application Support/Clairance/`
open to this user only, and open the launcher first if it does not run. What
starts then runs in a carrier, not under the terminal or the agent that typed
the command; the command shows what the launcher does, and ends when the
server answers (with production: once its station link runs and its sidecar is
ready). `--stop` goes through the launcher too when it runs.

A test instance runs the `co-atc` beside its config if there is one, else the
checkout's `bin/co-atc`, from the checkout, with its output in `co-atc.log`
beside the config. It stops at the end of its time (2 h by default, 24 h at
most), even if the launcher was closed and reopened in between: what the
launcher started is kept in `started.json`, beside the socket.
