# Menu bar launcher (macOS)

An icon in the menu bar that says whether co-atc's production runs, starts and
stops it, and opens the page. Nothing starts on its own: production runs only
when asked.

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
  "station_status_url": "http://your-station/radio/etat",
  "tailscale_url": "http://100.x.y.z:8000/",
  "planning_file": "/path/to/a/markdown/table/of/gpu/use.md"
}
```

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
- the menu can quit, and the app can be replaced on disk by an update, while
  what it carries keeps hearing the station;
- a SIGTERM to the carrier stops what it carries, and the carrier leaves when
  what it carries stops.

While a carrier runs, macOS takes Clairance for open: opening it again (a
double-click, Spotlight, `open`) reaches the carrier, which then opens the
menu. The launcher itself, run from a terminal, has the system open it again
and leaves. A process that still depends on the menu, or on another app, is
shown as such in its details ("Depends on:"), and quitting the menu asks first
while something depends on it.

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
