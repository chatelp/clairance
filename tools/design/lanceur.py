#!/usr/bin/env python3
"""The launcher's Dashboard window (step 2, 20/11): an approximate mock-up.

    python3 tools/design/lanceur.py

Writes docs-fr/design/lanceur/*.html. The real window is SwiftUI with the
system's controls (a grouped Form, a unified toolbar); this page imitates them
in the system font, dark and light, to show the layout and the words
(docs-fr/design/11-lanceur-fenetre.md). It is not part of the web design
system: its styles are its own.
"""
import os

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
OUT = os.path.join(ROOT, 'docs-fr/design/lanceur')
IDENT = os.path.join(ROOT, 'docs-fr/design/identite')


def sign(name):
    svg = open(os.path.join(IDENT, name)).read()
    svg = svg[svg.index('<svg'):]
    return svg.replace('width="18" height="18"', 'width="48" height="48"', 1)


STYLE = '''
:root { --win: #1f1f1f; --bar: #2a2a2a; --group: #2b2b2b; --line: rgba(255,255,255,.08); --text: rgba(255,255,255,.88);
  --sec: rgba(255,255,255,.55); --btn: rgba(255,255,255,.12); --ok: #32d74b; --warn: #ff9f0a; --bad: #ff453a; --desk: #3a4048; }
.light { --win: #efeff1; --bar: #e6e6e8; --group: #ffffff; --line: rgba(0,0,0,.08); --text: rgba(0,0,0,.85);
  --sec: rgba(0,0,0,.5); --btn: rgba(0,0,0,.06); --ok: #28cd41; --warn: #ff9500; --bad: #ff3b30; --desk: #b9c2cc; }
* { box-sizing: border-box; }
body { margin: 0; font: 13px/1.35 -apple-system, BlinkMacSystemFont, system-ui, sans-serif; color: var(--text); background: var(--desk);
  display: grid; place-items: center; min-height: 100vh; -webkit-font-smoothing: antialiased; }
.win { width: 560px; height: 680px; background: var(--win); border-radius: 12px; overflow: hidden; display: flex; flex-direction: column;
  box-shadow: 0 0 0 .5px rgba(0,0,0,.5), 0 20px 50px rgba(0,0,0,.45); }
.bar { height: 52px; flex: none; display: flex; align-items: center; gap: 8px; padding: 0 12px 0 20px; background: var(--bar); border-bottom: .5px solid var(--line); }
.lights { display: flex; gap: 8px; margin-right: 12px; } .lights i { width: 12px; height: 12px; border-radius: 50%; display: block; }
.lights i:nth-child(1) { background: #ff5f57; } .lights i:nth-child(2) { background: #febc2e; } .lights i:nth-child(3) { background: #28c840; }
.title { font-weight: 600; font-size: 13px; }
.tools { margin-left: auto; display: flex; gap: 6px; }
.tool { display: inline-flex; align-items: center; gap: 5px; height: 28px; padding: 0 10px; border-radius: 7px; background: var(--btn); font-size: 12px; }
.tool svg { width: 14px; height: 14px; }
.tool.off { opacity: .4; }
.body { flex: 1; overflow-y: auto; padding: 20px 20px 0; }
.head { display: grid; grid-template-columns: 48px 1fr; column-gap: 14px; align-items: start; margin-bottom: 18px; }
.head svg { grid-row: span 4; }
.state { font-size: 17px; font-weight: 600; line-height: 22px; margin-top: 2px; }
.cause, .version { color: var(--sec); }
.version .mono { font-family: ui-monospace, Menlo, monospace; font-size: 12px; }
.acts { display: flex; gap: 8px; margin-top: 6px; }
.btn { display: inline-flex; align-items: center; height: 22px; padding: 0 9px; border-radius: 6px; background: var(--btn); font-size: 12px; white-space: nowrap; }
.btn.danger { color: var(--bad); margin-left: auto; }
h2 { font-size: 13px; font-weight: 600; margin: 0 0 6px 4px; display: flex; }
h2 .n { color: var(--sec); font-weight: 400; margin-left: 4px; }
.group { background: var(--group); border-radius: 10px; margin-bottom: 18px; border: .5px solid var(--line); }
.row { display: flex; align-items: flex-start; gap: 12px; min-height: 34px; padding: 8px 12px; border-top: .5px solid var(--line); }
.row:first-child { border-top: 0; }
.row .k { display: flex; gap: 6px; flex: none; min-width: 120px; }
.row .v { margin-left: auto; text-align: right; color: var(--sec); font-variant-numeric: tabular-nums; }
.row .v.left { margin-left: 0; text-align: left; }
.s { width: 12px; text-align: center; font-size: 11px; line-height: 17px; }
.s.ok { color: var(--ok); } .s.warn { color: var(--warn); } .s.bad { color: var(--bad); } .s.grey { color: var(--sec); }
.who { font-weight: 600; min-width: 80px; }
.lines { color: var(--sec); }
.disc { display: flex; align-items: center; gap: 6px; }
.disc::before { content: "›"; display: inline-block; width: 10px; color: var(--sec); font-size: 15px; line-height: 1; }
.disc.open::before { transform: rotate(90deg); }
.sub .row { padding-left: 28px; }
.mono { font-family: ui-monospace, Menlo, monospace; font-size: 12px; }
.btns { display: flex; flex-wrap: wrap; gap: 6px; padding: 8px 12px 10px 28px; border-top: .5px solid var(--line); }
.empty { color: var(--sec); }
'''

PLAY = '<svg viewBox="0 0 16 16"><path fill="currentColor" d="M4.5 2.8v10.4L13 8z"/></svg>'
STOP = '<svg viewBox="0 0 16 16"><rect x="3" y="3" width="10" height="10" rx="2" fill="currentColor"/></svg>'
RESTART = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M13 8a5 5 0 1 1-1.6-3.7"/><path d="M11.8 1.8v2.9H8.9"/></svg>'
OPEN = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="12" height="12" rx="3"/><path d="M6.5 9.5 10.5 5.5M7 5.5h3.5V9"/></svg>'


def row(k, v, s=None, left=False):
    shape = f'<span class="s {s[0]}">{s[1]}</span>' if s else ''
    return f'<div class="row"><span class="k">{shape}<span>{k}</span></span><span class="v{" left" if left else ""}">{v}</span></div>'


def page(theme, head, sections, tools):
    return f'''<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Dashboard — mock-up</title><style>{STYLE}</style></head>
<body class="{theme}"><div class="win">
<div class="bar"><span class="lights"><i></i><i></i><i></i></span><span class="title">Dashboard</span><span class="tools">{tools}</span></div>
<div class="body">{head}{sections}</div></div></body></html>
'''


def health(rows):
    return '<h2>Health</h2><div class="group">' + ''.join(row(k, v, s) for k, v, s in rows) + '</div>'


OK = ('ok', '●')
WARN = ('warn', '▲')

if __name__ == '__main__':
    os.makedirs(OUT, exist_ok=True)
    # running, on air, two accounts, a test instance unfolded (dark)
    tools = (f'<span class="tool">{STOP}Stop</span><span class="tool">{RESTART}Restart</span>'
             f'<span class="tool">{OPEN}Open Clairance</span>')
    head = (f'<div class="head">{sign("barre-menus-ecoute.svg")}<div class="state">Production running · 2 h 14 min</div>'
            '<div class="version">Version of 20 November 2026 · <span class="mono">9a4b5ce</span> · up to date</div></div>')
    sec = health([('Station', 'On air 124.350, 126.425', OK), ('ADS-B', '107 aircraft, 700 msg/s', OK),
                  ('Audio', '2 frequencies, both receiving', OK), ('Transcription', 'Queue 0, 1.0 s a transmission', OK),
                  ('Disk', '212 GB free · archive 57 of 60 GB', OK), ('Memory', 'Normal', OK)])
    sec += ('<h2>Connected<span class="n">· 2</span></h2><div class="group">'
            '<div class="row"><span class="who">famille</span><span class="lines">1 page open for 5 min · sound off</span></div>'
            '<div class="row"><span class="who">pierre</span><span class="lines">2 pages open for 2 h 14 min<br>Listening to 124.350 for 40 min</span></div></div>')
    sub = ''.join(row(k, v, left=False) for k, v in [
        ('Page', '<span class="mono">runs/test/www</span>'), ('Server', '20 Nov 2026 · <span class="mono">9a4b5ce</span>'),
        ('Transcription', 'Simulated, answering'), ('Station link', 'Running, 2 h 10 min'), ('Depends on', 'Clairance'),
        ('Stops at', '18:00 (in 2 h 10 min)'), ('Ports', '8020')])
    sec += ('<h2>Instances</h2><div class="group">'
            '<div class="row"><span class="disc">Production · port 8000</span></div>'
            '<div class="row"><span class="disc open">Test · port 8020 · stops at 18:00</span></div>'
            f'<div class="sub">{sub}</div>'
            '<div class="btns"><span class="btn">Open in Browser</span><span class="btn">Copy Address</span><span class="btn">Show Log</span>'
            '<span class="btn">Show Config</span><span class="btn danger">Stop Instance…</span></div></div>')
    with open(os.path.join(OUT, 'fenetre-marche.html'), 'w') as f:
        f.write(page('', head, sec, tools))

    # a fault, a permission missing, no one connected (light)
    head = (f'<div class="head">{sign("barre-menus-defaut.svg")}<div class="state"><span class="s warn" style="font-size: 15px;">▲</span> Production running · 2 h 14 min</div>'
            '<div class="cause">Transcription · sidecar silent for 1 min, 12 waiting</div>'
            '<div class="version">Version of 20 November 2026 · <span class="mono">9a4b5ce</span> · 4 commits behind local</div></div>')
    sec = ('<h2>Permissions</h2><div class="group"><div class="row"><span class="k"><span class="s warn">▲</span><span>Clairance · notifications not allowed</span></span>'
           '<span class="v"><span class="btn">Open Notification Settings…</span></span></div></div>')
    sec += health([('Station', 'On air 124.350, 126.425', OK), ('ADS-B', '107 aircraft, 700 msg/s', OK),
                   ('Audio', '2 frequencies, both receiving', OK), ('Transcription', 'Sidecar silent for 1 min, 12 waiting', WARN),
                   ('Disk', '212 GB free · archive 57 of 60 GB', OK), ('Memory', 'Normal', OK)])
    sec += '<h2>Connected</h2><div class="group"><div class="row"><span class="empty">No one connected</span></div></div>'
    sec += ('<h2>Instances</h2><div class="group"><div class="row"><span class="disc">Production · port 8000</span></div></div>')
    with open(os.path.join(OUT, 'fenetre-defaut.html'), 'w') as f:
        f.write(page('light', head, sec, tools))
    print('docs-fr/design/lanceur/fenetre-marche.html\ndocs-fr/design/lanceur/fenetre-defaut.html')
