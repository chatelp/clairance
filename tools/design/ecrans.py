#!/usr/bin/env python3
"""Final screens of the redesign, assembled from the design system only.

    python3 tools/design/ecrans.py

Writes docs-fr/design/ecrans/*.html. Each screen links the system's tokens.css
and components/bundle.css and nothing else: no <style>, no class that is not
cl-*. Inline style is allowed only for an illustration standing for a renderer
(the map, the PAR) and for the absolute position of a popover. If a screen
needs a style the system lacks, the system is incomplete: add the component
there, not here.
"""
import os

import shutil
import sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
OUT = os.path.join(ROOT, 'docs-fr/design/ecrans')
SYS = '../systeme/project'
# python3 tools/design/ecrans.py <dir>: a self-contained copy (stylesheets under <dir>/sys/), to publish
if len(sys.argv) > 1:
    OUT, SYS = os.path.abspath(sys.argv[1]), 'sys'
    os.makedirs(os.path.join(OUT, 'sys/components'), exist_ok=True)
    shutil.copy(os.path.join(ROOT, 'docs-fr/design/systeme/project/tokens.css'), os.path.join(OUT, 'sys/tokens.css'))
    shutil.copy(os.path.join(ROOT, 'docs-fr/design/systeme/project/components/bundle.css'), os.path.join(OUT, 'sys/components/bundle.css'))

# ---------------------------------------------------------------- glyphs
SPK_ON = '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M2 6h3l4-3v10L5 10H2z"/><path fill="none" stroke="currentColor" stroke-width="1.5" d="M11 5.5a3.5 3.5 0 0 1 0 5"/></svg>'
SPK_OFF = '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M2 6h3l4-3v10L5 10H2z"/><path fill="none" stroke="currentColor" stroke-width="1.5" d="m11 6 4 4m0-4-4 4"/></svg>'
CARET = '<svg width="10" height="10" viewBox="0 0 16 16" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="2" d="m3 6 5 5 5-5"/></svg>'
PLAY = '<svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M5 3.5v9l7-4.5z"/></svg>'
BELL = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M4 11V7a4 4 0 0 1 8 0v4l1.5 1.5h-11L4 11ZM6.5 14h3"/></svg>'
GEAR = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" aria-hidden="true"><path d="M6.43 2.83 L6.50 1.16 L9.50 1.16 L9.57 2.83 L10.55 3.24 L11.77 2.10 L13.90 4.23 L12.76 5.45 L13.17 6.43 L14.84 6.50 L14.84 9.50 L13.17 9.57 L12.76 10.55 L13.90 11.77 L11.77 13.90 L10.55 12.76 L9.57 13.17 L9.50 14.84 L6.50 14.84 L6.43 13.17 L5.45 12.76 L4.23 13.90 L2.10 11.77 L3.24 10.55 L2.83 9.57 L1.16 9.50 L1.16 6.50 L2.83 6.43 L3.24 5.45 L2.10 4.23 L4.23 2.10 L5.45 3.24Z"/><circle cx="8" cy="8" r="2.1"/></svg>'
SEARCH = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="7" cy="7" r="4.5"/><path d="m10.5 10.5 3.5 3.5"/></svg>'
LAYERS = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m8 2 6 3.5L8 9 2 5.5 8 2ZM2 8.5 8 12l6-3.5M2 11.5 8 15l6-3.5"/></svg>'
CLOSE = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8"/></svg>'
SIGN_ON = '<svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="7.4" fill="#163020"/><circle cx="8" cy="8" r="6.6" fill="none" stroke="#4caf50" stroke-width="1.6"/><path d="M8 8 L8 1.6 A6.4 6.4 0 0 1 12.525 3.475 Z" fill="#4caf50" fill-opacity=".45"/><path d="M8 8 L12.525 3.475" fill="none" stroke="#66c56a" stroke-width="1.6" stroke-linecap="round"/><circle cx="9.78" cy="4.19" r="1.15" fill="#e0e0e0"/></svg>'


def page(title, body, width=1440, height=900, folded=False, extra_class=''):
    cls = 'cl-root cl-app' + (' is-folded' if folded else '') + (' ' + extra_class if extra_class else '')
    return f'''<!doctype html>
<html lang="en" data-theme="dark">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title}</title>
<link rel="stylesheet" href="{SYS}/tokens.css">
<link rel="stylesheet" href="{SYS}/components/bundle.css">
</head>
<body style="margin: 0; width: {width}px; height: {height}px;">
<div class="{cls}">
{body}
</div>
</body>
</html>
'''


# ---------------------------------------------------------------- top bar
def topbar(following=None):
    follow = ''
    if following:
        follow = (f'<span class="cl-plain">Following</span><span class="cl-chip cl-chip--callsign is-selected">{following}'
                  f'<button class="cl-btn cl-btn--icon" aria-label="Stop following">{CLOSE}</button></span>')
    return f'''<header class="cl-topbar">
  <span class="cl-topbar__brand">{SIGN_ON}<span class="cl-topbar__name">Clairance</span></span>{follow}
  <span class="cl-light cl-light--ok">On air: Approche De Gaulle + De Gaulle App. (L), chosen at the station 1 h 38 ago</span>
  <span class="cl-light cl-light--ok">Transcription</span>
  <span class="cl-light cl-light--ok">ADS-B 107 · 700/s</span>
  <span class="cl-topbar__utc">13:13:20Z</span><span class="cl-topbar__local">15:13</span>
  <button class="cl-btn cl-btn--icon" aria-label="Event log">{BELL}</button>
  <button class="cl-btn cl-btn--icon" aria-label="Settings">{GEAR}</button>
</header>'''


# ---------------------------------------------------------------- thread
def channel(freq, name, age, on=False, menu=False):
    icon = SPK_ON if on else SPK_OFF
    cls = 'cl-channel cl-channel--split' + (' is-on' if on else '') + (' is-menu-open' if menu else '')
    return (f'<span class="{cls}"><button class="cl-channel__main" aria-pressed="{str(on).lower()}">{icon}'
            f'<span class="cl-channel__freq">{freq}</span> {name}</button>'
            f'<button class="cl-channel__caret" aria-label="{freq}: volume and options">{CARET}</button></span>')


def threadhead(filtered=None, menu=False):
    seg = ''.join(f'<button class="cl-seg__item" aria-pressed="{str(f == (filtered or "All")).lower()}">{f}</button>'
                  for f in ('All', '124.350', '126.425'))
    return f'''<div class="cl-threadhead">
  <div class="cl-threadhead__row"><h2 class="cl-threadhead__title">Radio</h2><span class="cl-exchange__when">5 exchanges in the last 2 min</span>
    <span class="cl-threadhead__end"><button class="cl-btn cl-btn--icon" aria-label="Search the thread">{SEARCH}</button><button class="cl-btn">Mute all</button></span></div>
  <div class="cl-threadhead__row">{channel('124.350', 'Approche', '8 s', on=True, menu=menu)}{channel('126.425', 'App. (L)', '1 min')}</div>
  <div class="cl-threadhead__row"><span class="cl-label">Show</span><div class="cl-seg">{seg}</div></div>
</div>'''


def chip(ph, fam, cs, extra=''):
    return f'<span class="cl-chip cl-chip--callsign{extra}"><span class="cl-phase cl-phase--{fam} cl-phase--small">{ph}</span>{cs}</span>'


WHEN = lambda t: f'<span class="cl-exchange__when">{t}</span>'
PLAYB = f'<button class="cl-exchange__play" aria-label="Play the exchange">{PLAY}</button>'
STR3 = '<span class="cl-strength cl-strength--3" title="Strong clues"><i></i><i></i><i></i></span>'
STR2 = '<span class="cl-strength cl-strength--2" title="Medium clues"><i></i><i></i><i></i></span>'


def exchanges(selected='AFR47KX', wide=False):
    sel = lambda cs: ' is-selected' if cs == selected else ''
    if wide:
        return f'''<article class="cl-exchange cl-exchange--wide">
  <div class="cl-exchange__who">{WHEN('13:10:02')}{WHEN('124.350')}</div>
  <div class="cl-exchange__what"><div class="cl-exchange__head"><span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('CLB', 'dep', 'TVF61WB')}{STR3}<span class="cl-exchange__reply"><span>↩</span><span>pilot replied, 3 s later</span></span>{PLAYB}</div>
  <p class="cl-exchange__text">Transavia six one Whiskey Bravo, climb flight level one two zero, direct Rambouillet.</p>
  <div class="cl-exchange__instr"><span class="cl-chip cl-chip--instr">Climb to FL 120</span><span class="cl-chip cl-chip--instr">Direct RBT</span></div></div>
</article>
<article class="cl-exchange cl-exchange--wide">
  <div class="cl-exchange__who">{WHEN('13:11:52')}{WHEN('124.350')}</div>
  <div class="cl-exchange__what"><div class="cl-exchange__head"><span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('ARR', 'arr', 'EZY82QT')}{STR3}<span class="cl-exchange__reply"><span>↩</span><span>pilot replied, 12 s later</span></span>{PLAYB}</div>
  <p class="cl-exchange__text">Easy eight two Quebec Tango, descend flight level seven zero, turn left heading two seven zero.</p>
  <div class="cl-exchange__instr"><span class="cl-chip cl-chip--instr">Descend to FL 070</span><span class="cl-chip cl-chip--instr">Turn left heading 270</span></div></div>
</article>
<article class="cl-exchange cl-exchange--wide{sel('AFR47KX')}" lang="fr">
  <div class="cl-exchange__who">{WHEN('13:12:31')}{WHEN('124.350')}</div>
  <div class="cl-exchange__what"><div class="cl-exchange__head" lang="en"><span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('APP', 'arr', 'AFR47KX', sel('AFR47KX'))}{STR2}<span class="cl-exchange__reply"><span>↩</span><span>pilot replied, 4 s later</span></span>{PLAYB}</div>
  <p class="cl-exchange__text">Air France quatre sept Kilo X-ray, autorisé ILS piste deux six gauche, contactez la tour un deux zéro décimal six cinq.</p>
  <div class="cl-exchange__instr" lang="en"><span class="cl-chip cl-chip--instr cl-chip--clearance">Cleared ILS runway 26L</span><span class="cl-chip cl-chip--instr">Contact 120.650</span></div>
  <div class="cl-exchange__explain">Autorisé à l'approche ILS, piste 26L. Contactez 120.650.</div></div>
</article>
<article class="cl-exchange cl-exchange--wide is-live">
  <div class="cl-exchange__who">{WHEN('now')}{WHEN('124.350')}</div>
  <div class="cl-exchange__what"><div class="cl-exchange__head"><span class="cl-level"><i style="height:6px"></i><i style="height:12px"></i><i style="height:9px"></i><i style="height:14px"></i><i style="height:7px"></i></span><span class="cl-plain">someone is speaking</span></div></div>
</article>'''
    return f'''<article class="cl-exchange">
  <div class="cl-exchange__head">{WHEN('12:58:31 · 126.425')}{chip('ARR', 'arr', 'AFR47KX', sel('AFR47KX'))}<span class="cl-exchange__to">→</span><span class="cl-chip cl-chip--atc">ATC</span>{STR3}{PLAYB}</div>
  <p class="cl-exchange__text" lang="fr">De Gaulle bonjour, Air France quatre sept Kilo X-ray, niveau un huit zéro en descente vers un cinq zéro.</p>
  <div class="cl-exchange__explain">First call on this frequency.</div>
</article>
<article class="cl-exchange">
  <div class="cl-exchange__head">{WHEN('13:10:02 · 124.350')}<span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('CLB', 'dep', 'TVF61WB', sel('TVF61WB'))}{STR3}{PLAYB}</div>
  <p class="cl-exchange__text">Transavia six one Whiskey Bravo, climb flight level one two zero, direct Rambouillet.</p>
  <div class="cl-exchange__instr"><span class="cl-chip cl-chip--instr">Climb to FL 120</span><span class="cl-chip cl-chip--instr">Direct RBT</span></div>
  <div class="cl-exchange__reply"><span>↩</span><span>Pilot replied, 3 s later</span></div>
</article>
<article class="cl-exchange">
  <div class="cl-exchange__head">{WHEN('13:11:52 · 124.350')}<span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('ARR', 'arr', 'EZY82QT', sel('EZY82QT'))}{STR3}{PLAYB}</div>
  <p class="cl-exchange__text">Easy eight two Quebec Tango, descend flight level seven zero, turn left heading two seven zero.</p>
  <div class="cl-exchange__instr"><span class="cl-chip cl-chip--instr">Descend to FL 070</span><span class="cl-chip cl-chip--instr">Turn left heading 270</span></div>
  <div class="cl-exchange__reply"><span>↩</span><span>Pilot replied, 12 s later</span></div>
</article>
<article class="cl-exchange is-unmatched">
  <div class="cl-exchange__head">{WHEN('13:12:04 · 126.425')}<span class="cl-chip cl-chip--callsign is-away">?</span><span class="cl-plain">no aircraft in range was named</span>{PLAYB}</div>
  <p class="cl-exchange__text">… roger, two seven zero, maintaining.</p>
</article>
<article class="cl-exchange{sel('AFR47KX')}" lang="fr">
  <div class="cl-exchange__head" lang="en">{WHEN('13:12:31 · 124.350')}<span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('APP', 'arr', 'AFR47KX', sel('AFR47KX'))}{STR2}{PLAYB}</div>
  <p class="cl-exchange__text">Air France quatre sept Kilo X-ray, autorisé ILS piste deux six gauche, contactez la tour un deux zéro décimal six cinq.</p>
  <div class="cl-exchange__instr" lang="en"><span class="cl-chip cl-chip--instr cl-chip--clearance">Cleared ILS runway 26L</span><span class="cl-chip cl-chip--instr">Contact 120.650</span></div>
  <div class="cl-exchange__explain cl-exchange__explain--boxed">Autorisé à l'approche ILS, piste 26L. Contactez 120.650.</div>
  <div class="cl-exchange__reply" lang="en"><span>↩</span><span>Pilot replied, 4 s later</span></div>
</article>
<article class="cl-exchange is-live">
  <div class="cl-exchange__head">{WHEN('now · 124.350')}<span class="cl-level"><i style="height:6px"></i><i style="height:12px"></i><i style="height:9px"></i><i style="height:14px"></i><i style="height:7px"></i></span><span class="cl-plain">someone is speaking</span></div>
</article>'''


def thread(menu=False, wide=False, selected='AFR47KX', newbelow=False):
    nb = '<button class="cl-newbelow">↓ 3 new exchanges</button>' if newbelow else ''
    return f'''<section class="cl-app__thread" aria-label="Radio">
{threadhead(menu=menu)}
<div class="cl-app__bottom">
{exchanges(selected, wide)}
{nb}
</div>
</section>'''


# ---------------------------------------------------------------- sky
AC = 'M0-7 1.3-2 6 .5v1.6L1.3 1 .9 4.6 2.6 6v1.2L0 6.4l-2.6.8V6l1.7-1.4-.4-3.6L-6 2.1V.5L-1.3-2Z'


def skymap(filtered=False, width=640, follow=False):
    """An illustration standing for the OpenLayers map: icons on the altitude
    ramp (color-altitude-dark-*), labels with the phase family colours."""
    def ac(x, y, rot, fill, scale=1):
        return f'<g transform="translate({x} {y})"><path d="{AC}" fill="{fill}" transform="rotate({rot}) scale({scale})"/></g>'

    def label(x, y, ph, col, cs, sel=False, line2=None, said=None, typ=''):
        lines = [f'{ph} {cs} {typ}'] + ([line2] if line2 else []) + ([said] if said else [])
        w = int(max(len(l) for l in lines) * 6.7 + 16)
        h = 6 + 14 * len(lines)
        out = f'<g transform="translate({x} {y})"><rect width="{w}" height="{h}" rx="4" fill="#0a0a0ad9" stroke="{"#4caf50" if sel else "none"}"/>'
        out += f'<text x="7" y="14" font-family="ui-monospace,Menlo,monospace" font-size="11"><tspan fill="{col}" font-weight="600">{ph}</tspan><tspan fill="#e0e0e0" font-weight="600"> {cs}</tspan><tspan fill="#b8b8b8"> {typ}</tspan></text>'
        if line2:
            out += f'<text x="7" y="28" font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#b8b8b8">{line2}</text>'
        if said:
            out += f'<text x="7" y="{14 + 14 * (2 if line2 else 1)}" font-family="system-ui,-apple-system,sans-serif" font-size="11" fill="#b8b8b8">{said}</text>'
        return out + '</g>'
    track = ''
    if follow:
        track = ('<path d="M150 40 C190 110 220 190 250 250 S 330 262 400 236" fill="none" stroke="#b8b8b8" stroke-opacity=".75" stroke-width="1.5" stroke-dasharray="5 4"/>'
                 '<g fill="#b8b8b8" fill-opacity=".75"><circle cx="150" cy="40" r="2"/><circle cx="220" cy="190" r="2"/><circle cx="300" cy="258" r="2"/></g>'
                 '<g font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#949494"><text x="160" y="36">12:58 FL 150</text><text x="230" y="186">13:04 4000 ft</text></g>')
    far = '' if filtered else ac(110, 140, 120, '#6e7b8f') + label(124, 140, 'CRZ', '#b8d3f2', 'DLH6TA', line2='FL 360 ↔ 452 kt', typ='A20N') + ac(470, 650, 200, '#6e7b8f')
    # following: the view is centred on the aircraft, in the part the PAR frame leaves visible
    vx, vy = (400 - width / 2, 236 - 300) if follow else (-(width - 640) / 2, 0)
    return f'''<svg class="cl-sky__map" viewBox="{vx} {vy} {width} 852" preserveAspectRatio="xMidYMid slice" role="img" aria-label="Map of the traffic around the station">
  <g fill="none" stroke="#333"><circle cx="320" cy="430" r="110"/><circle cx="320" cy="430" r="230"/><circle cx="320" cy="430" r="380"/></g>
  <g font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#949494" text-anchor="middle"><text x="320" y="314">5 NM</text><text x="320" y="194">10 NM</text></g>
  <g stroke="#b8b8b8" stroke-width="3" stroke-linecap="round"><line x1="290" y1="438" x2="350" y2="424"/><line x1="470" y1="210" x2="560" y2="192"/><line x1="476" y1="230" x2="566" y2="212"/></g>
  <g font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#b8b8b8"><text x="354" y="444">LFPZ</text><text x="570" y="228">LFPG</text></g>
  <line x1="380" y1="232" x2="470" y2="210" stroke="#4c9ce8" stroke-dasharray="4 4" opacity=".6"/>
  <g transform="translate(400 236)"><circle r="22" fill="none" stroke="#4caf50" stroke-width="1" opacity=".5"/><circle r="13" fill="none" stroke="#4caf50" stroke-width="2"/><path d="{AC}" fill="#e68cba" transform="rotate(78)"/></g>
  {label(87, 246, 'APP', '#4c9ce8', 'AFR47KX', sel=True, line2='2 400 ft ↓ 158 kt', said='“autorisé ILS piste deux six gauche, cont…”', typ='A320')}
  {ac(250, 336, 80, '#c2a3ff')}{label(266, 346, 'ARR', '#4c9ce8', 'EZY82QT', line2='5 000 ft ↓ 241 kt', typ='A21N')}
  {ac(200, 560, -35, '#c2a3ff')}{label(214, 570, 'CLB', '#ec8b34', 'TVF61WB', line2='5 800 ft ↑ 212 kt', said='“climb flight level one two zero, direct…”', typ='B738')}
  {ac(330, 420, 30, '#fff59a', .8)}
  {far}{track}
</svg>'''


def sky(par_seg=True, note=None, card=None, corner=True, width=620, extra='', pressed='Map', follow=False):
    seg = ('<div class="cl-seg">' + ''.join(f'<button class="cl-seg__item" aria-pressed="{str(x == pressed).lower()}">{x}</button>'
           for x in ('Map', 'PAR · LFPG', 'PAR · LFPO', 'PAR · LFPZ')) + '</div>') if par_seg else ''
    end = f'<div class="cl-sky__end">{card}</div>' if card else ''
    notes = f'<div class="cl-sky__note">{note}</div>' if note else ''
    corner_html = f'<div class="cl-sky__corner"><button class="cl-btn cl-btn--icon" aria-label="Layers and base map">{LAYERS}</button></div>' if corner else ''
    return f'''<section class="cl-app__sky" aria-label="Sky">
{skymap(filtered=bool(note) and not follow, width=width, follow=follow)}
{extra}
<div class="cl-sky__top">{seg}{notes}{end}</div>
{corner_html}
</section>'''


# ---------------------------------------------------------------- context column
def tabs(current):
    t = lambda name, count='': (f'<button class="cl-tabs__tab" role="tab" aria-selected="{str(name == current).lower()}">{name}'
                                + (f' <span class="cl-tabs__count">{count}</span>' if count else '') + '</button>')
    return (f'<div class="cl-tabs cl-tabs--column" role="tablist">{t("Right now")}{t("Traffic", "110")}'
            f'<span class="cl-tabs__end"><button class="cl-btn cl-btn--icon cl-mono" aria-label="Fold the column">»</button></span></div>')


SEQ = '''<table class="cl-table"><thead><tr><th></th><th>Aircraft</th><th class="is-num">To thr.</th><th class="is-num">Time</th><th class="is-num">Behind</th></tr></thead><tbody>
<tr><td class="is-dim">1</td><td><b>AFR47KX</b> <span class="is-dim">A320</span></td><td class="is-num">6.2 NM</td><td class="is-num">2:21</td><td class="is-num is-dim">first</td></tr>
<tr><td class="is-dim">2</td><td><b>EZY82QT</b> <span class="is-dim">A21N</span></td><td class="is-num">14.0 NM</td><td class="is-num">3:29</td><td class="is-num">+7.8 NM</td></tr>
<tr><td class="is-dim">3</td><td><b>DLH9KR</b> <span class="is-dim">A20N</span></td><td class="is-num">16.5 NM</td><td class="is-num">3:35</td><td class="is-num is-tight">+2.5 NM</td></tr>
</tbody></table>'''


def column_now():
    return f'''<aside class="cl-app__column" aria-label="Context">
{tabs('Right now')}
<div class="cl-app__scroll">
<div class="cl-section"><p class="cl-summary">De Gaulle has been using runway <b>26L</b> since 11:40. Three aircraft on final, next touchdown in about 2 min. Orly uses <b>06</b>.</p><p class="cl-summary__source">Read from the ADS-B tracks, not announced by the controllers.</p></div>
<div class="cl-section"><span class="cl-label">Arrivals to LFPG 26L</span>{SEQ}</div>
<div class="cl-section"><span class="cl-label">Worth knowing · last 30 min</span><ul class="cl-events"><li><time>13:09</time><span><span class="is-warn">▲ Go-around</span> EZY14LM on 27R at LFPG, now climbing back.</span></li><li><time>13:05</time><span>LFPG switched to landing west (26L, was 08R).</span></li></ul></div>
<div class="cl-section"><span class="cl-label">Weather at LFPG</span><span class="cl-metar">LFPG 131300Z 25012KT 9999 FEW030 17/09 Q1018</span><span class="cl-metar__plain">Wind from the west, 12 kt; visibility over 10 km; a few clouds at 3 000 ft; QNH 1018.</span><span class="cl-row"><button class="cl-btn">TAF</button><button class="cl-btn">NOTAM <span class="cl-tabs__count">4</span></button></span></div>
<div class="cl-section"><span class="cl-label">Traffic</span><div class="cl-stats"><span><b>107</b> airborne</span><span><b>12</b> heard</span><span><b>3</b> on the ground</span></div></div>
</div>
</aside>'''


def strip(fam, ph, phcls, cs, sub, v1, s1, v2, v3, extra=''):
    return (f'<div class="cl-strip cl-strip--{fam}{extra}"><span class="cl-phase {phcls}">{ph}</span><div class="cl-strip__col"><span class="cl-strip__v cl-strip__v--callsign">{cs}</span>'
            f'<span class="cl-strip__s">{sub}</span></div><div class="cl-strip__col cl-strip__col--end"><span class="cl-strip__v">{v1}</span><span class="cl-strip__s">{s1}</span></div>'
            f'<div class="cl-strip__col cl-strip__col--end"><span class="cl-strip__v">{v2}</span><span class="cl-strip__s">kt</span></div>'
            f'<div class="cl-strip__col cl-strip__col--end"><span class="cl-strip__v">{v3}</span><span class="cl-strip__s">NM</span></div></div>')


def column_traffic(with_tabs=True):
    histo = ''.join(f'<i class="is-in" style="height:{h}%"></i>' for h in (40, 70, 55, 30, 45, 60, 35, 50, 25)) + ''.join(f'<i style="height:{h}%"></i>' for h in (40, 65, 90, 100, 75, 30, 10))
    fam = lambda d, n, c, on=True: f'<button class="cl-toggle{" is-on" if on else ""}"><i class="cl-dot cl-dot--{d}"></i>{n} <span class="cl-toggle__count">{c}</span></button>'
    H = '<span class="cl-strip__heard">heard {}</span>'
    return f'''<aside class="cl-app__column" aria-label="Context">
{tabs('Traffic') if with_tabs else ''}
<div class="cl-section">
<input class="cl-input" type="search" placeholder="Callsign, registration, type" aria-label="Find an aircraft">
<div class="cl-altrange"><div class="cl-altrange__head"><span class="cl-label">Altitude <span class="cl-age">QNH 1018</span></span><span class="cl-altrange__value">SFC – FL 195 <small>· 64 of 107</small></span></div><div class="cl-altrange__histo">{histo}</div><div class="cl-slider"><span class="cl-slider__rail"></span><span class="cl-slider__fill" style="left:0;width:56%"></span><span class="cl-slider__thumb" style="left:0"></span><span class="cl-slider__thumb" style="left:56%"></span></div><div class="cl-altrange__marks"><span>SFC</span><span>TA 5000</span><span>FL 100</span><span>FL 195</span><span>∞</span></div></div>
<div class="cl-seg cl-seg--dense">''' + ''.join(f'<button class="cl-seg__item" aria-pressed="{str(x == "All").lower()}">{x}</button>' for x in ('All', '≤ 3000', '≤ TA', '≤ FL 100', 'FL 100–195', '≥ FL 195')) + f'''</div>
<div class="cl-row"><button class="cl-toggle is-on">Air <span class="cl-toggle__count">107</span></button><button class="cl-toggle is-on">Ground <span class="cl-toggle__count">3</span></button><button class="cl-toggle">Heard <span class="cl-toggle__count">12</span></button></div>
<div class="cl-row">{fam('gnd', 'Gnd', 3)}{fam('dep', 'Dep', 18)}{fam('crz', 'Crz', 61)}{fam('arr', 'Arr', 21)}{fam('unk', '?', 7, False)}</div>
</div>
<div class="cl-tabs cl-tabs--inset" role="tablist"><button class="cl-tabs__tab" role="tab" aria-selected="true">Active <span class="cl-tabs__count">110</span></button><button class="cl-tabs__tab" role="tab" aria-selected="false">Signal lost <span class="cl-tabs__count">22</span></button><span class="cl-tabs__end cl-plain">Sort: distance ▾</span></div>
<div class="cl-app__scroll">
{strip('arr', 'APP', 'cl-phase--arr', 'AFR47KX', 'A320 · ' + H.format('8 s'), '2 400<span class="cl-unit">ft</span> ↓', '−900 ft/min', '158', '6.2', ' is-selected')}
{strip('dep', 'T/O', 'cl-phase--dep cl-phase--fill', 'AFR19ME', 'A21N', '300<span class="cl-unit">ft</span> ↑', '+2 100 ft/min', '148', '9.8')}
{strip('arr', 'ARR', 'cl-phase--arr', 'F-GXQA', 'DR40 · ' + H.format('41 s'), '1 600<span class="cl-unit">ft</span> ↔', 'level', '92', '2.4')}
{strip('gnd', 'TAX', 'cl-phase--gnd', 'F-HBZC', 'C172 · LFPZ', 'GND', '—', '12', '0.8')}
{strip('arr', 'ARR', 'cl-phase--arr', 'EZY82QT', 'A21N · ' + H.format('27 s'), '5 000<span class="cl-unit">ft</span> ↓', '−1 100 ft/min', '241', '14.0')}
{strip('dep', 'CLB', 'cl-phase--dep', 'TVF61WB', 'B738 · <span class="cl-badge" title="Last known position: GPS lost">LP</span> <span class="cl-age">−12 s</span>', '5 800<span class="cl-unit">ft</span> ↑', '+1 800 ft/min', '212', '15.3')}
{strip('arr', 'ARR', 'cl-phase--arr', 'DLH9KR', 'A20N', 'FL 085 ↓', '−1 300 ft/min', '276', '16.5')}
{strip('dep', 'CLB', 'cl-phase--dep', 'AFR52PB', 'A320', '2 900<span class="cl-unit">ft</span> ↑', '+2 400 ft/min', '171', '13.1')}
{strip('unk', 'NEW', 'cl-phase--unk', '3c4b21', 'no callsign yet', 'FL 160', '—', '—', '31.0')}
</div>
</aside>'''


def channel_menu():
    return f'''<div class="cl-menu" style="position: absolute; left: 12px; top: 122px;" role="dialog" aria-label="124.350 Approche De Gaulle">
  <div class="cl-menu__head"><span class="cl-channel__freq cl-menu__title">124.350</span><span class="cl-menu__title">Approche De Gaulle</span></div>
  <div class="cl-menu__row"><span class="cl-light cl-light--ok">Listening in this browser</span></div>
  <div class="cl-menu__row"><span class="cl-label">Volume</span><div class="cl-slider"><span class="cl-slider__rail"></span><span class="cl-slider__fill" style="left:0;width:70%"></span><span class="cl-slider__thumb" style="left:70%"></span></div><span class="cl-channel__age">70 %</span></div>
  <div class="cl-menu__sep"></div>
  <label class="cl-check"><span class="cl-check__box"></span>Only this frequency in the thread</label>
  <span class="cl-plain">Last transmission 8 s ago · 41 today. On air because the station chose it.</span>
</div>'''


# ---------------------------------------------------------------- C: following a flight
def column_follow(tab='Story'):
    t = lambda n: f'<button class="cl-tabs__tab" role="tab" aria-selected="{str(n == tab).lower()}">{n}</button>'
    head = f'''<div class="cl-tabs cl-tabs--column"><button class="cl-btn cl-btn--icon cl-mono" aria-label="Back to Traffic">‹</button><span class="cl-plain">Back to Traffic</span><span class="cl-tabs__end"><button class="cl-btn cl-btn--icon cl-mono" aria-label="Fold the column">»</button></span></div>
<div class="cl-story__head">
  <div class="cl-story__id"><span class="cl-phase cl-phase--arr">APP</span><span class="cl-story__callsign">AFR47KX</span><span class="cl-badge" title="Followed airport">LFPG</span></div>
  <span class="cl-plain">Air France · Airbus A320 · F-HEPA</span>
  <div class="cl-story__grid">
    <div><div class="cl-label">Altitude</div><div class="cl-story__val">2 400<span class="cl-unit">ft</span> ↓</div></div>
    <div><div class="cl-label">To 26L</div><div class="cl-story__val">6.2<span class="cl-unit">NM</span> · 2:21</div></div>
    <div><div class="cl-label">Ground speed</div><div class="cl-story__val">158<span class="cl-unit">kt</span></div></div>
    <div><div class="cl-label">Heading</div><div class="cl-story__val">258°<span class="cl-unit">mag</span></div></div>
  </div>
</div>
<div class="cl-tabs cl-tabs--inset" role="tablist">{t('Story')}{t('Details')}{t('Track')}{t('Nearby')}</div>'''
    if tab == 'Story':
        body = '''<ol class="cl-story__events">
  <li><time>12:51</time><i></i><span>Seen by the receiver, FL 210, 61 NM north-east.</span></li>
  <li><time>12:58</time><i class="is-radio"></i><span>Arrival begins; first contact with De Gaulle App. (L).</span></li>
  <li><time>13:02</time><i class="is-radio"></i><span>Handed to Approche De Gaulle.</span></li>
  <li><time>13:08</time><i></i><span>On approach, 12 NM out.</span></li>
  <li><time>13:12</time><i class="is-now"></i><span>Cleared to land on 26L by ILS; leaves for the tower.</span></li>
  <li class="is-next"><time>≈13:15</time><i></i><span>Expected touchdown, from its distance and speed.</span></li>
</ol>'''
    else:
        body = '''<div class="cl-section"><button class="cl-disclosure" aria-expanded="true">Phase history</button>
<table class="cl-table"><thead><tr><th>Time</th><th>Phase</th><th class="is-num">Point</th></tr></thead><tbody>
<tr><td>13:08:12</td><td><span class="cl-phase cl-phase--arr cl-phase--small">APP</span></td><td class="is-num is-dim">#412</td></tr>
<tr><td>12:58:40</td><td><span class="cl-phase cl-phase--arr cl-phase--small">ARR</span></td><td class="is-num is-dim">#203</td></tr>
<tr><td>12:51:03</td><td><span class="cl-phase cl-phase--crz cl-phase--small">CRZ</span></td><td class="is-num is-dim">#1</td></tr></tbody></table></div>
<div class="cl-section"><button class="cl-disclosure" aria-expanded="true">Instructions heard</button>
<table class="cl-table"><thead><tr><th>Time</th><th>Instruction</th><th>Then</th></tr></thead><tbody>
<tr><td>13:12:31</td><td>Cleared ILS 26L</td><td class="is-dim">—</td></tr>
<tr><td>13:04:10</td><td>Descend to 4 000 ft</td><td class="is-dim">level at 4 000 ft, 3 min later</td></tr></tbody></table></div>
<div class="cl-section"><button class="cl-disclosure" aria-expanded="true">Basic info</button>
<dl class="cl-kv"><dt>Hex</dt><dd>39a4c1</dd><dt>Registration</dt><dd>F-HEPA</dd><dt>Squawk</dt><dd>4231</dd><dt>First seen</dt><dd>12:51:03</dd><dt>Operator</dt><dd>Air France</dd></dl></div>
<div class="cl-section"><button class="cl-disclosure" aria-expanded="false">Position</button></div>
<div class="cl-section"><button class="cl-disclosure" aria-expanded="false">Speed and direction</button></div>
<div class="cl-section"><button class="cl-disclosure" aria-expanded="false">Navigation and weather</button></div>
<div class="cl-section"><button class="cl-disclosure" aria-expanded="false">ADS-B and signal</button></div>'''
    return f'<aside class="cl-app__column" aria-label="AFR47KX">{head}<div class="cl-app__scroll">{body}</div></aside>'


import importlib.util
_spec = importlib.util.spec_from_file_location('par_mini', os.path.join(os.path.dirname(__file__), 'par-mini.py'))
_par = importlib.util.module_from_spec(_spec); _spec.loader.exec_module(_par)
PAR = _par.svg()


def sky_follow():
    parframe = f'''<div class="cl-parframe" style="position: absolute; left: 0; right: 0; bottom: 0;">
  <div class="cl-parframe__head"><b>26L → 08R</b><span class="cl-parframe__inuse">IN USE</span><span class="cl-age">3 aircraft</span><button class="cl-btn" style="margin-left: auto;">Open the PAR view</button></div>
  {PAR}
</div>'''
    return sky(par_seg=False, note='Centred on <b>AFR47KX</b> · its track since 12:51', extra=parframe, corner=False, follow=True)


# ---------------------------------------------------------------- map controls, airport popup
def map_controls():
    row = lambda name, on=True, opacity=None: (f'<div class="cl-menu__row"><span class="cl-switch{" is-on" if on else ""}" role="switch" aria-checked="{str(on).lower()}"></span>'
                                               f'<span class="cl-menu__label">{name}</span>' +
                                               (f'<div class="cl-slider cl-slider--short"><span class="cl-slider__rail"></span><span class="cl-slider__fill" style="left:0;width:{opacity}%"></span><span class="cl-slider__thumb" style="left:{opacity}%"></span></div>' if opacity else '') + '</div>')
    seg = ''.join(f'<button class="cl-seg__item" aria-pressed="{str(x == "None").lower()}">{x}</button>' for x in ('IGN dark', 'IGN', 'OSM dark', 'OSM', 'None'))
    return f'''<div class="cl-menu cl-menu--map" style="position: absolute; right: 392px; bottom: 56px;" role="dialog" aria-label="Layers and base map">
  <div class="cl-menu__head"><span class="cl-menu__title">Base map</span></div>
  <div class="cl-seg">{seg}</div>
  <div class="cl-menu__sep"></div>
  <div class="cl-menu__group"><span class="cl-label">Aircraft</span>{row('Labels')}{row('Tracks, last 3 min', True, 30)}</div>
  <div class="cl-menu__sep"></div>
  <div class="cl-menu__group"><span class="cl-label">Layers <span class="cl-age">switch · opacity</span></span>{row('Distance rings', True, 60)}{row('Airspace', True, 40)}{row('Airports', True, 80)}{row('Heliports', True, 50)}{row('Navaids', True, 50)}{row('Other runways', True, 50)}{row('Airport layout')}</div>
</div>'''


def airport_popup():
    return '''<div class="cl-card" style="position: absolute; left: 60px; top: 110px; width: 280px;" role="dialog" aria-label="LFPZ">
  <div class="cl-row"><span class="cl-menu__title cl-mono">LFPZ</span><span class="cl-menu__title">Saint-Cyr-l'École</span><button class="cl-btn cl-btn--icon" aria-label="Close" style="margin-left: auto;">''' + CLOSE + '''</button></div>
  <dl class="cl-kv"><dt>Type</dt><dd>small airport</dd><dt>Elevation</dt><dd>371 ft</dd></dl>
  <table class="cl-table"><thead><tr><th>Type</th><th>Desc</th><th class="is-num">MHz</th></tr></thead><tbody><tr><td>TWR</td><td>Saint-Cyr Tour</td><td class="is-num">119.250</td></tr><tr><td>AFIS</td><td>Saint-Cyr Info</td><td class="is-num">123.200</td></tr></tbody></table>
</div>'''


# ---------------------------------------------------------------- event log
def event_log():
    return '''<div class="cl-menu cl-menu--wide" style="position: absolute; right: 52px; top: 52px; width: 360px;" role="dialog" aria-label="Event log">
  <div class="cl-menu__head"><span class="cl-menu__title">Event log</span><span class="cl-age">5 events</span><span class="cl-row" style="margin-left: auto;"><button class="cl-btn">Sounds</button><button class="cl-btn cl-btn--quiet">Clear</button></span></div>
  <ul class="cl-events">
    <li class="is-clickable"><time>13:12</time><span><span class="is-urgent">■ 7700</span> AFR6TZ, emergency squawk, FL 110, 18 NM north-east.</span></li>
    <li class="is-clickable"><time>13:09</time><span><span class="cl-warn">▲ Go-around</span> EZY14LM on 27R at LFPG, now climbing back.</span></li>
    <li><time>13:05</time><span>LFPG switched to landing west (26L, was 08R).</span></li>
    <li class="is-clickable"><time>12:57</time><span><span class="is-info">●</span> AFR18GH, an A388, is in range, 34 NM west.</span></li>
    <li><time>12:51</time><span>The station switched to Approche De Gaulle + De Gaulle App. (L).</span></li>
  </ul>
  <div class="cl-menu__sep"></div>
  <div class="cl-menu__group"><span class="cl-label">Ring a bell for</span>
    <label class="cl-check is-on"><span class="cl-check__box"></span>Emergencies</label>
    <label class="cl-check"><span class="cl-check__box"></span>Go-arounds</label>
    <label class="cl-check"><span class="cl-check__box"></span>Runway changes</label>
    <label class="cl-check"><span class="cl-check__box"></span>Aircraft of interest</label>
    <label class="cl-check"><span class="cl-check__box"></span>Station and audio</label></div>
</div>'''


# ---------------------------------------------------------------- the PAR view
def par_view():
    seq = SEQ
    frame = lambda head, inuse, svg, table='': (f'<div class="cl-parframe{" is-in-use" if inuse else ""}"><div class="cl-parframe__head">{head}</div>{svg}{table}</div>')
    head1 = '<b>26L → 08R</b><span class="cl-parframe__inuse">IN USE</span><span class="cl-age">3 aircraft</span>'
    head2 = '<b>27R → 09L</b><span class="cl-age">1 aircraft</span>'
    return f'''<div class="cl-parview">
  <div class="cl-parhead"><span class="cl-parhead__airport">LFPG</span><span>Charles de Gaulle</span><a class="cl-link" href="#">VAC</a><span class="cl-parhead__sep">·</span><span>Runways in use <b>26L</b> 100 % · <b>27R</b> 100 %</span><span class="cl-parhead__sep">·</span><span>QNH <b>1018</b> hPa <span class="cl-age">(ADS-B, 12 aircraft)</span></span><span class="cl-parhead__sep">·</span><span>TA <b>5000</b> ft</span><span class="cl-parhead__sep">·</span><span>TL <b>FL060</b> <span class="cl-age">(computed)</span></span></div>
  {frame(head1, True, PAR, '<div class="cl-section" style="border: 0; padding: 0;"><span class="cl-label">Arrivals to 26L</span>' + seq + '</div>')}
  {frame(head2, False, '')}
</div>'''


# ---------------------------------------------------------------- settings
def settings():
    nav = ''.join(f'<a href="#" aria-current="{str(n == "Listening").lower()}">{n}</a>' for n in ('Listening', 'Callsign matching', 'Display', 'Account', 'System'))
    return f'''<div class="cl-page" role="dialog" aria-label="Settings"><div class="cl-page__inner">
<nav class="cl-page__nav" aria-label="Sections">{nav}</nav>
<div class="cl-page__body">
  <div class="cl-page__head"><h1 class="cl-page__title">Settings</h1><span class="cl-light cl-light--ok">Saved</span><span class="cl-row" style="margin-left: auto;"><button class="cl-btn">Reload from the server</button><button class="cl-btn cl-btn--icon" aria-label="Close">{CLOSE}</button></span></div>
  <section class="cl-page__section"><h3>Listening</h3>
    <div class="cl-field"><span class="cl-field__label">On air</span><span class="cl-plain">Approche De Gaulle 124.350 · De Gaulle App. (L) 126.425</span><span class="cl-field__help">Chosen at the station; this page plays what it receives.</span></div>
    <div class="cl-field"><span class="cl-field__label">Reference airport</span><select class="cl-select" aria-label="Reference airport"><option>LFPZ · Saint-Cyr-l'École · 2 NM</option></select><span class="cl-field__help">Phases (taxi, take-off, approach) are judged against it.</span></div>
    <div class="cl-field"><span class="cl-field__label">Also follow</span><span class="cl-row"><label class="cl-check is-on"><span class="cl-check__box"></span>LFPG</label><label class="cl-check is-on"><span class="cl-check__box"></span>LFPO</label><label class="cl-check"><span class="cl-check__box"></span>LFPB</label></span><span class="cl-field__help">Up to three: each gets a PAR and an arrival sequence.</span></div>
  </section>
  <section class="cl-page__section"><h3>Callsign matching</h3>
    <div class="cl-field"><span class="cl-field__label">Rules</span><div class="cl-seg"><button class="cl-seg__item" aria-pressed="true">Measured</button><button class="cl-seg__item" aria-pressed="false">Cautious</button><button class="cl-seg__item" aria-pressed="false">Experimental</button></div><span class="cl-field__help">How a transmission is tied to an aircraft. The eleven rules behind each preset stay below, folded.</span></div>
    <button class="cl-disclosure" aria-expanded="false">The eleven rules</button>
  </section>
  <section class="cl-page__section"><h3>Display <span class="cl-age">this browser</span></h3>
    <div class="cl-field"><span class="cl-field__label">Last transmission on aircraft labels</span><span class="cl-switch is-on" role="switch" aria-checked="true"></span><span class="cl-field__help">A third line under the label, in quotes, for ten minutes after it was heard.</span></div>
    <div class="cl-field"><span class="cl-field__label">Smooth movement between updates</span><span class="cl-switch is-on" role="switch" aria-checked="true"></span></div>
    <div class="cl-field"><span class="cl-field__label">Frames per second</span><span class="cl-row" style="width: 240px;"><div class="cl-slider" style="flex: 1;"><span class="cl-slider__rail"></span><span class="cl-slider__fill" style="left:0;width:40%"></span><span class="cl-slider__thumb" style="left:40%"></span></div><span class="cl-age">30</span></span></div>
    <div class="cl-field"><span class="cl-field__label">Track points kept per aircraft</span><span class="cl-row" style="width: 240px;"><div class="cl-slider" style="flex: 1;"><span class="cl-slider__rail"></span><span class="cl-slider__fill" style="left:0;width:25%"></span><span class="cl-slider__thumb" style="left:25%"></span></div><span class="cl-age">1000</span></span></div>
    <div class="cl-field"><span class="cl-field__label">Hide ground aircraft at other airports</span><span class="cl-switch" role="switch" aria-checked="false"></span></div>
    <div class="cl-field"><span class="cl-field__label">Times in</span><div class="cl-seg"><button class="cl-seg__item" aria-pressed="true">UTC</button><button class="cl-seg__item" aria-pressed="false">Local</button></div><span class="cl-field__help">The clock in green in the top bar dates everything on screen.</span></div>
  </section>
  <section class="cl-page__section"><h3>Account</h3>
    <div class="cl-field"><span class="cl-field__label">Who may open Clairance</span><span class="cl-row"><label class="cl-check cl-check--radio is-on"><span class="cl-check__box"></span>This machine</label><label class="cl-check cl-check--radio"><span class="cl-check__box"></span>The network, with a password</label></span><span class="cl-field__help">The same choice as at the first start.</span></div>
  </section>
  <section class="cl-page__section"><h3>System</h3>
    <div class="cl-field"><span class="cl-field__label">Log level</span><select class="cl-select" aria-label="Log level"><option>info</option></select></div>
    <div class="cl-field"><span class="cl-field__label">Keep databases up to</span><span class="cl-row"><input class="cl-input" style="width: 72px;" value="20" aria-label="Gigabytes"><span class="cl-age">GB</span></span></div>
    <button class="cl-disclosure" aria-expanded="false">Reports: database, ADS-B source, receiver, decoder</button>
    <button class="cl-disclosure" aria-expanded="true">Performance and debug</button>
    <div class="cl-field"><span class="cl-field__label">Live updates</span><button class="cl-btn">Pause</button><span class="cl-field__help">Freezes the map and the lists to look at a moment; the radio keeps playing.</span></div>
  </section>
</div>
</div></div>'''


# ---------------------------------------------------------------- sign in, first run
SIGN_64 = '<svg viewBox="0 0 64 64" aria-hidden="true"><circle cx="32" cy="32" r="26.5" fill="none" stroke="#e0e0e0" stroke-width="4"/><circle cx="32" cy="32" r="13" fill="none" stroke="#e0e0e0" stroke-width="2" opacity="0.55"/><path d="M32 32 L32 6 A26 26 0 0 1 50.385 13.615 Z" fill="#4caf50" fill-opacity="0.35"/><path d="M32 32 L50.385 13.615" fill="none" stroke="#4caf50" stroke-width="4" stroke-linecap="round"/><circle cx="39.18" cy="16.59" r="3.8" fill="#4caf50"/></svg>'


import importlib.util as _ilu
_cspec = _ilu.spec_from_file_location('connexion', os.path.join(os.path.dirname(__file__), 'connexion.py'))
_conn = _ilu.module_from_spec(_cspec); _cspec.loader.exec_module(_conn)


def vitals():
    """StationVitals: aggregated figures only; served before sign-in by a public route (≈ 1 h server)."""
    ok = '<i class="cl-vitals__ok"></i>'
    return f'''<div class="cl-vitals"><div class="cl-vitals__head"><span>This station, live</span><span>refreshed every 10 s</span></div>
<dl class="cl-vitals__list">
  <dt>ads-b</dt><dd>{ok}107 aircraft <span>·</span> 702 msg/s <span>·</span> farthest 182 NM</dd>
  <dt>airband</dt><dd>{ok}2 channels on air <span>·</span> 412 transmissions today</dd>
  <dt>speech</dt><dd>{ok}Whisper large-v3 ATCO2 <span>·</span> Apple M2 Pro <span>·</span> 1.4 s a transmission</dd>
  <dt>matched</dt><dd>31 % of transmissions to an aircraft today</dd>
  <dt>runways</dt><dd>3 airports followed <span>·</span> 4 runways in use</dd>
  <dt>up</dt><dd>6 d 14 h <span>·</span> Clairance 2026.10 <span>·</span> a7489d5</dd>
</dl></div>'''


def signin(first_run=False):
    if first_run:
        body = '''<h1 class="cl-signin__title">Welcome</h1>
<span class="cl-plain">First start. Who may open Clairance?</span>
<label class="cl-check cl-check--radio is-on"><span class="cl-check__box"></span>Only this machine, no password</label>
<label class="cl-check cl-check--radio"><span class="cl-check__box"></span>Any device on my network, with a password</label>
<span class="cl-plain">You can change this later in Settings › Account.</span>
<button class="cl-btn cl-btn--primary">Continue</button>'''
    else:
        body = '''<h1 class="cl-signin__title">Sign in</h1>
<label class="cl-signin__field"><span class="cl-label">User</span><input class="cl-input" autocomplete="username"></label>
<label class="cl-signin__field"><span class="cl-label">Password</span><input class="cl-input" type="password" autocomplete="current-password"></label>
<button class="cl-btn cl-btn--primary">Sign in</button>
<span class="cl-plain">After 8 failed tries, wait 15 minutes.</span>'''
    return f'''<div class="cl-signin cl-signin--sky">
{_conn.scope_svg('cl-signin__scope')}
<div class="cl-signin__brand">{SIGN_64}Clairance</div>
<div class="cl-signin__console">{vitals()}</div>
<div class="cl-signin__card"><span class="cl-plain">Hear the airband you receive, read it in English and French, and see which aircraft is speaking.</span>
{body}
</div>
<div class="cl-signin__foot">Runs on your own machine: nothing leaves it. Made for the listener; it judges no one.</div>
</div>'''


# ---------------------------------------------------------------- phone
TAB_ICONS = {
    'Radio': '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M3 4h10v7H7l-3 2.5V11H3z"/></svg>',
    'Sky': '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="8" cy="8" r="6"/><circle cx="8" cy="8" r="2.5"/></svg>',
    'Traffic': '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M3 4h10M3 8h10M3 12h10"/></svg>',
    'Right now': '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="8" cy="8" r="6"/><path d="M8 5v3l2 1.5"/></svg>',
}


def phone(current, pane):
    tabs_html = ''.join(f'<button class="cl-tabbar__tab" aria-selected="{str(n == current).lower()}">{TAB_ICONS[n]}{n}</button>' for n in TAB_ICONS)
    return f'''<div class="cl-phone">
<div class="cl-phone__bar">{SIGN_ON}<span class="cl-light cl-light--ok">On air: Approche De Gaulle</span><span class="cl-topbar__utc">13:13Z</span><button class="cl-btn cl-btn--icon" aria-label="Event log">{BELL}</button><button class="cl-btn cl-btn--icon" aria-label="Settings">{GEAR}</button></div>
<div class="cl-phone__pane">{pane}</div>
<nav class="cl-tabbar" aria-label="Panes">{tabs_html}</nav>
</div>'''


# ---------------------------------------------------------------- states
def states_sky():
    banner = '<div class="cl-banner" style="position: absolute; top: 56px; left: 12px; right: 12px; z-index: 2;">Connection lost: reconnecting (attempt 3), next in 5 s. The map shows the last positions.</div>'
    resume = f'<div class="cl-resume" style="position: absolute; bottom: 24px; left: 50%; transform: translateX(-50%); z-index: 2;">{SIGN_ON}Click anywhere to resume listening</div>'
    return sky(extra=banner + resume)


def thread_empty():
    return f'''<section class="cl-app__thread" aria-label="Radio">
<div class="cl-threadhead"><div class="cl-threadhead__row"><h2 class="cl-threadhead__title">Radio</h2></div></div>
<div class="cl-empty"><svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="8" cy="8" r="6.25"/><path d="M8 7v4.5M8 4.5v.5"/></svg><span>Nothing to listen to: the station's receiver is lent to another use since 14:20. The thread comes back when it is on air again; the map and the PAR keep working.</span></div>
</section>'''


def write(name, html):
    os.makedirs(OUT, exist_ok=True)
    with open(os.path.join(OUT, name), 'w') as f:
        f.write(html)
    print(os.path.relpath(os.path.join(OUT, name), ROOT))


if __name__ == '__main__':
    write('a-maintenant.html', page('A · Right now', topbar() + '<div class="cl-app__main">' + thread() + sky() + column_now() + '</div>'))
    write('a-trafic.html', page('A · Traffic', topbar() + '<div class="cl-app__main">' + thread() + sky(note='Showing <b>64</b> of 107 · SFC – FL 195') + column_traffic() + '</div>'))
    write('a-frequence.html', page('A · A frequency\'s menu', topbar() + '<div class="cl-app__main">' + thread(menu=True, newbelow=True) + sky() + column_now() + '</div>' + channel_menu()))
    card = ('<div class="cl-row"><button class="cl-btn">Traffic <span class="cl-tabs__count">110</span></button>'
            '<button class="cl-btn cl-btn--icon cl-mono" aria-label="Unfold the column">«</button></div>'
            '<div class="cl-card cl-card--sky"><span class="cl-label">Right now</span><p class="cl-summary">De Gaulle lands on <b>26L</b>, three on final, next in about 2 min. '
            '<span class="cl-warn">▲</span> A go-around at 13:09.</p><span class="cl-summary__source">From the ADS-B tracks.</span>'
            '<button class="cl-btn">Open Right now</button></div>')
    write('b-replie.html', page('B · Column folded', topbar() + '<div class="cl-app__main">' + thread(wide=True) + sky(card=card, width=800) + '</div>', folded=True))
    write('c-suivre.html', page('C · Following a flight', topbar(following='AFR47KX') + '<div class="cl-app__main">' + thread() + sky_follow() + column_follow('Story') + '</div>'))
    write('c-details.html', page('C · Its details', topbar(following='AFR47KX') + '<div class="cl-app__main">' + thread() + sky_follow() + column_follow('Details') + '</div>'))
    write('a-calques.html', page('A · Layers and an airport', topbar() + '<div class="cl-app__main">' + thread() + sky(extra=airport_popup()) + column_now() + '</div>' + map_controls()))
    write('a-journal.html', page('A · Event log', topbar() + '<div class="cl-app__main">' + thread() + sky() + column_now() + '</div>' + event_log()))
    write('par.html', page('The PAR view', topbar() + '<div class="cl-app__main">' + thread() + sky(extra=par_view(), corner=False, pressed='PAR · LFPG') + column_now() + '</div>'))
    # The settings, in pages (D86, 28/10): tools/design/reglages.py
    _rspec = importlib.util.spec_from_file_location('reglages', os.path.join(os.path.dirname(__file__), 'reglages.py'))
    R = importlib.util.module_from_spec(_rspec); _rspec.loader.exec_module(R)
    app = topbar() + '<div class="cl-app__main">' + thread() + sky() + column_now() + '</div>'
    write('reglages.html', page('Settings · Listening', app + R.settings_page('listening')))
    write('reglages-lecteur.html', page('Settings · Listening, a viewer', app + R.settings_page('listening', role='viewer')))
    write('reglages-usage.html', page('Settings · Usage', app + R.settings_page('usage')))
    write('reglages-serveur.html', page('Settings · Server', app + R.settings_page('server')))
    write('reglages-acces.html', page('Settings · Access', app + R.settings_page('access')))
    write('telephone-reglages.html', page('Settings on a phone: the pages', R.settings_page('listening', state='is-list'), width=390, height=844))
    write('telephone-reglages-page.html', page('Settings on a phone: a page', R.settings_page('listening', state='is-detail'), width=390, height=844))
    write('connexion.html', page('Sign in', signin(), width=1440, height=900))
    write('premier-lancement.html', page('First run', signin(first_run=True), width=1440, height=900))
    write('etats.html', page('States: connection lost, nothing on air, resume listening', topbar() + '<div class="cl-app__main">' + thread_empty() + states_sky() + column_now() + '</div>'))
    narrow_thread = '<div class="cl-threadhead"><div class="cl-threadhead__row">' + channel('124.350', 'Approche', '', on=True) + channel('126.425', 'App. (L)', '') + '</div></div><div class="cl-app__bottom">' + exchanges() + '</div>'
    phones = (phone('Radio', narrow_thread) + phone('Sky', '<section class="cl-app__sky" style="height: 100%;">' + skymap(width=390) + '</section>')
              + phone('Traffic', column_traffic(with_tabs=False).replace('cl-app__column', 'cl-phone__pane')))
    write('telephone.html', page('Phone', '<div class="cl-row" style="padding: 24px; align-items: flex-start; gap: 32px;">' + phones + '</div>', width=1300, height=900))

    SCREENS = [
        ('a-maintenant.html', 'A · Right now', 'The default: the thread, the sky, the context column on Right now.', 1440, 900),
        ('a-trafic.html', 'A · Traffic', 'The flat list and its filters; the map shows only what the filters keep.', 1440, 900),
        ('a-frequence.html', 'A · A frequency', 'The frequency menu: volume, only this frequency in the thread; "3 new exchanges" after scrolling up.', 1440, 900),
        ('b-replie.html', 'B · Column folded', 'The thread widens into a transcript; Right now becomes a card over the map.', 1440, 900),
        ('c-suivre.html', 'C · Following a flight', 'Its story in the column, its track on the map, its approach in the PAR frame.', 1440, 900),
        ('c-details.html', 'C · Its details', 'The Details tab: phases, instructions heard, basic info, folded sections.', 1440, 900),
        ('a-calques.html', 'A · Layers, an airport', 'The map controls (base map, layers, opacity) and an airport\'s card.', 1440, 900),
        ('a-journal.html', 'A · Event log', 'The bell: emergencies, go-arounds, runway changes, aircraft of interest, the station; its sounds.', 1440, 900),
        ('par.html', 'The PAR view', 'The existing PAR, with its header and arrival sequence, in the sky\'s place.', 1440, 900),
        ('reglages.html', 'Settings · Listening', 'One page at a time, grouped: This browser, Station, Administration (D86). Listening holds the official names (AIP).', 1440, 900),
        ('reglages-lecteur.html', 'Settings · a viewer', 'A viewer sees Display, Account and Listening, the values read, no control.', 1440, 900),
        ('reglages-acces.html', 'Settings · Access', 'Who may open Clairance, the accounts in two columns, the live figures of the sign-in page.', 1440, 900),
        ('reglages-usage.html', 'Settings · Usage', 'Who is here now, then each account\'s week in two columns.', 1440, 900),
        ('reglages-serveur.html', 'Settings · Server', 'Log detail, databases and their archive (a page that needs a look carries ▲ in the nav), the reports in two columns.', 1440, 900),
        ('telephone-reglages.html', 'Settings on a phone', 'The list of pages, grouped, in touch rows.', 390, 844),
        ('telephone-reglages-page.html', 'Settings on a phone: a page', 'A page, with ‹ Settings to go back.', 390, 844),
        ('connexion.html', 'Sign in', "The sky: the page is the sign's scope; the station's console; the form at the centre.", 1440, 900),
        ('premier-lancement.html', 'First run', 'Who may open Clairance: this machine, or the network with a password.', 1440, 900),
        ('etats.html', 'States', 'Nothing on air, connection lost, click to resume listening.', 1440, 900),
        ('telephone.html', 'Phone', 'One pane at a time: Radio, Sky, Traffic, Right now; touch sizes.', 1300, 900),
    ]
    cards = ''.join(
        f'<a class="cl-card" href="{f}" style="text-decoration: none; color: inherit; width: 360px;">'
        f'<span style="display: block; width: 336px; height: {int(336 * h / w)}px; overflow: hidden; border: 1px solid var(--color-border-default); border-radius: var(--radius-md);">'
        f'<iframe src="{f}" title="{t}" loading="lazy" style="width: {w}px; height: {h}px; border: 0; transform: scale({336 / w:.4f}); transform-origin: 0 0; pointer-events: none;"></iframe></span>'
        f'<span class="cl-menu__title">{t}</span><span class="cl-plain">{n}</span></a>'
        for f, t, n, w, h in SCREENS)
    write('index.html', f'''<!doctype html>
<html lang="en" data-theme="dark">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Clairance screens</title>
<link rel="stylesheet" href="{SYS}/tokens.css"><link rel="stylesheet" href="{SYS}/components/bundle.css"></head>
<body style="margin: 0; background: var(--color-surface-page);">
<div class="cl-root" style="padding: 24px 16px; min-height: 100vh;">
<h1 class="cl-page__title">Clairance · final screens</h1>
<p class="cl-plain" style="max-width: 720px;">Every screen is assembled from the design system only: its tokens.css and components/bundle.css, no style of its own. Open one to see it at full size. They are the reference for the migration of the interface.</p>
<div class="cl-row" style="align-items: flex-start; gap: 16px; margin-top: 16px;">{cards}</div>
</div>
</body>
</html>
''')
