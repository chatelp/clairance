# Menu bar launcher (macOS)

An icon in the menu bar that says whether co-atc's production runs, starts and
stops it, and opens the page. Nothing starts on its own: production runs only
when asked.

Below, every co-atc running on this Mac, production and test alike, found by
process name and read from its own config and server: the page it serves (the
branch and commit when its `www` is a checkout's), the server's version, its
aircraft and transmissions of the day, its transcription (real model or fake,
the sidecar's health and queue), its station link (`radio-ctl-sync`), its
ports, config, databases and log. A test instance can be stopped from there:
by its own PIDs, co-atc, its station link and a sidecar co-atc did not start,
after a confirmation. Sidecars and station links running without an instance
are listed apart.

It does not run production itself. It runs the script a double-click would
(`production_command`, by default `runs/production.command`), so sign-in, the
station link and the clean stop stay in one place, and stops it with SIGTERM,
which that script's trap turns into a clean stop of co-atc, its sidecar and the
station link. A script started from Terminal is stopped the same way.

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
  "repo": "/path/to/atc-scribe",
  "production_command": "runs/production.command",
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

The same actions from a terminal or a Shortcut, through the same code:

```sh
~/Applications/Clairance.app/Contents/MacOS/clairance --status   # or --start, --stop
```

A production started this way from a terminal belongs, for macOS, to the
application that terminal runs in: its Local Network permission is that
application's. If that application quits or updates itself, the server keeps
running but can no longer reach the station on the LAN ("no route to host"),
with no other sign (16-18/10/2026). Start production from the menu or by a
double-click on the script when it must outlive the terminal.
