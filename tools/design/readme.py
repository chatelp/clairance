#!/usr/bin/env python3
"""The README's images: the reference screens, brought up to the product of
20/10, with made-up flights and transmissions.

    python3 tools/design/readme.py <out-dir>

Writes <out-dir>/{hero,follow,par,map-routes,phone}.html, assembled from the
design system (tokens.css, components/bundle.css) and the screen builders of
ecrans.py. The sky is drawn on a projection of the real airports (runway
thresholds from assets/runways.csv) and of the published routes of the cycle
(data/aip-names.json, public SIA data); the station is a made-up point in
Paris, the flights, registrations and words are invented. Capture them with
tools/design/capture.mjs; tools/design/readme-shots.sh crops and compresses.
"""
import csv
import importlib.util
import json
import math
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '../..'))
OUT = os.path.abspath(sys.argv[1]) if len(sys.argv) > 1 else os.path.join(ROOT, 'runs/design/readme')

_argv, sys.argv = sys.argv, sys.argv[:1]
_spec = importlib.util.spec_from_file_location('ecrans', os.path.join(HERE, 'ecrans.py'))
E = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(E)
sys.argv = _argv
E.SYS = 'file://' + os.path.join(ROOT, 'docs-fr/design/systeme/project')

MONO = 'ui-monospace,SFMono-Regular,Menlo,monospace'
TEXT = 'system-ui,-apple-system,sans-serif'
PH = {'arr': '#4c9ce8', 'dep': '#ec8b34', 'crz': '#b8d3f2', 'gnd': '#dccb9e', 'unk': '#969696'}
BAND = [(1000, '#fff59a'), (3000, '#e68cba'), (12500, '#c2a3ff'), (18000, '#61c9e3'), (99999, '#6e7b8f')]
ROUTE = '#e0e0e0'

# ---------------------------------------------------------------- data
RWY = {}
with open(os.path.join(ROOT, 'assets/runways.csv')) as f:
    for row in csv.DictReader(f):
        if row['airport_ident'] in ('LFPG', 'LFPO', 'LFPZ', 'LFPB', 'LFPV'):
            try:
                RWY.setdefault(row['airport_ident'], []).append((row['le_ident'], float(row['le_latitude_deg']), float(row['le_longitude_deg']),
                                                                 row['he_ident'], float(row['he_latitude_deg']), float(row['he_longitude_deg'])))
            except ValueError:
                pass
THR = {}
for ap, rows in RWY.items():
    for le, la1, lo1, he, la2, lo2 in rows:
        THR[(ap, le)] = (la1, lo1)
        THR[(ap, he)] = (la2, lo2)
def _aip_path():
    """data/ is not tracked: in a worktree, read the main checkout's."""
    import subprocess
    here = os.path.join(ROOT, 'data/aip-names.json')
    if os.path.exists(here):
        return here
    common = subprocess.run(['git', '-C', ROOT, 'rev-parse', '--git-common-dir'], capture_output=True, text=True).stdout.strip()
    return os.path.join(os.path.dirname(os.path.abspath(os.path.join(ROOT, common))), 'data/aip-names.json')


AIP = json.load(open(_aip_path()))
ROUTES = {(r['airport'], r['name']): r for r in AIP['routes']}


def route(ap, name):
    return ROUTES[(ap, name)]


def along(lat, lon, brg, nm):
    """The point nm nautical miles from (lat, lon) on a true bearing."""
    return (lat + nm * math.cos(math.radians(brg)) / 60,
            lon + nm * math.sin(math.radians(brg)) / (60 * math.cos(math.radians(lat))))


def final(ap, rwy, nm, course):
    lat, lon = THR[(ap, rwy)]
    return along(lat, lon, (course + 180) % 360, nm)


def band(ft):
    return next(c for top, c in BAND if ft < top)


def alt_text(ft, ta=5000):
    if ft <= ta:
        return f'{ft:,}'.replace(',', ' ') + ' ft'
    return f'FL {round(ft / 100):03d}'


class Geo:
    def __init__(self, lat0, lon0, width_nm, w=620, h=852):
        self.lat0, self.lon0, self.w, self.h = lat0, lon0, w, h
        self.ppn = w / width_nm
        self.width_nm = width_nm

    def xy(self, lat, lon):
        return (self.w / 2 + (lon - self.lon0) * math.cos(math.radians(self.lat0)) * 60 * self.ppn,
                self.h / 2 - (lat - self.lat0) * 60 * self.ppn)


# ---------------------------------------------------------------- drawing
AC = 'M0-7 1.3-2 6 .5v1.6L1.3 1 .9 4.6 2.6 6v1.2L0 6.4l-2.6.8V6l1.7-1.4-.4-3.6L-6 2.1V.5L-1.3-2Z'
CW = 6.6   # mono 11 px


def esc(s):
    return s.replace('&', '&amp;').replace('<', '&lt;').replace('>', '&gt;')


def rings(g, lat, lon, radii=(5, 10, 25, 50)):
    x, y = g.xy(lat, lon)
    out = ''
    for r in radii:
        R = r * g.ppn
        out += f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{R:.1f}" fill="none" stroke="#3d3d3d"/>'
        out += f'<text x="{x:.1f}" y="{y - R + 13:.1f}" text-anchor="middle" font-family="{MONO}" font-size="10.5" fill="#949494">{r} NM</text>'
    return out


def airports(g, only=None, names=(('LFPG', 'LFPG', 1.0, '#60a5fa', (6, 24)), ('LFPO', 'LFPO', 1.0, '#60a5fa', (10, 16)), ('LFPZ', 'LFPZ', 0.8, '#a78bfa', (8, 14)),
                       ('LFPB', 'LFPB', 0.8, '#60a5fa', (8, -8)))):
    out = ''
    for ap, label, wscale, col, (dx, dy) in names:
        if only and ap not in only:
            continue
        xs = []
        for le, la1, lo1, he, la2, lo2 in RWY.get(ap, []):
            x1, y1 = g.xy(la1, lo1)
            x2, y2 = g.xy(la2, lo2)
            xs.append((x1, y1, x2, y2))
            out += f'<line x1="{x1:.1f}" y1="{y1:.1f}" x2="{x2:.1f}" y2="{y2:.1f}" stroke="#e4e4e4" stroke-width="{2.6 * wscale:.1f}" stroke-linecap="round"/>'
        if xs:
            mx = max(max(a[0], a[2]) for a in xs)
            my = sum(a[1] + a[3] for a in xs) / (2 * len(xs))
            out += f'<text x="{mx + dx:.1f}" y="{my + dy:.1f}" font-family="{MONO}" font-size="11" fill="{col}">{label}</text>'
    return out


def route_lines(g, r, runway, strong=True, alpha=None):
    """A published route: its line from the runway's threshold, chevrons in the direction flown."""
    a = alpha if alpha is not None else (1 if strong else .38)
    w = 1.6 if strong else 1.1
    step = 46 if strong else 70
    lat, lon = THR[(r['airport'], runway)]
    pts = [g.xy(lat, lon)] + [g.xy(l['lat'], l['lon']) for l in r['legs']]
    out = f'<polyline points="{" ".join(f"{x:.1f},{y:.1f}" for x, y in pts)}" fill="none" stroke="{ROUTE}" stroke-opacity="{a}" stroke-width="{w}"/>'
    carry = step / 2
    for (x1, y1), (x2, y2) in zip(pts, pts[1:]):
        L = math.hypot(x2 - x1, y2 - y1)
        ang = math.degrees(math.atan2(y2 - y1, x2 - x1))
        d = carry
        while d < L:
            t = d / L
            out += (f'<text transform="translate({x1 + (x2 - x1) * t:.1f} {y1 + (y2 - y1) * t:.1f}) rotate({ang:.1f})" text-anchor="middle" dominant-baseline="central" '
                    f'font-family="{MONO}" font-size="{13 if strong else 11}" font-weight="600" fill="{ROUTE}" fill-opacity="{a}">›</text>')
            d += step
        carry = d - L
    return out, pts


def widest_gap(x, y, neighbours):
    if not neighbours:
        return (1, 0)
    angs = sorted(math.atan2(ny - y, nx - x) for nx, ny in neighbours)
    best, mid = -1, 0
    for i, a in enumerate(angs):
        b = angs[(i + 1) % len(angs)] + (2 * math.pi if i == len(angs) - 1 else 0)
        if b - a > best:
            best, mid = b - a, (a + b) / 2
    return (math.cos(mid), math.sin(mid))


def route_points(g, routes, strong_names=(), constraints=True, alpha=.6, names_upto=67):
    """Each named point once, in the widest gap between its lines; the likeliest route's at full strength."""
    pts = {}
    for r, runway in routes:
        lat, lon = THR[(r['airport'], runway)]
        seq = [(None, lat, lon)] + [(l, l['lat'], l['lon']) for l in r['legs']]
        for i, (l, la, lo) in enumerate(seq):
            if not l or not l.get('named'):
                continue
            p = pts.setdefault(l['point'], {'lat': la, 'lon': lo, 'nb': [], 'leg': l})
            for j in (i - 1, i + 1):
                if 0 <= j < len(seq):
                    p['nb'].append(g.xy(seq[j][1], seq[j][2]))
    out = ''
    for name, p in pts.items():
        x, y = g.xy(p['lat'], p['lon'])
        if not (-20 < x < g.w + 20 and -20 < y < g.h + 20):
            continue
        a = 1 if name in strong_names else alpha
        out += f'<path d="M{x:.1f},{y - 5:.1f} L{x + 5:.1f},{y + 4:.1f} L{x - 5:.1f},{y + 4:.1f} Z" fill="none" stroke="{ROUTE}" stroke-opacity="{a}" stroke-width="1.2"/>'
        if g.width_nm > names_upto:
            continue
        sx, sy = widest_gap(x, y, p['nb'])
        lines = [name]
        leg = p['leg']
        if constraints and name in strong_names:
            if leg.get('min_alt'):
                lines.append('≥ ' + leg['min_alt'].replace('FL', 'FL '))
            if leg.get('max_alt'):
                lines.append('≤ ' + leg['max_alt'].replace('FL', 'FL '))
            if leg.get('max_kt'):
                lines.append(f'≤ {leg["max_kt"]} kt')
        anchor = 'start' if sx > .3 else 'end' if sx < -.3 else 'middle'
        up = sy < -.3
        for i, t in enumerate(lines):
            k = i - (len(lines) - 1) if up else i
            fill = ROUTE if i == 0 else '#949494'
            fo = a if i == 0 else 1
            out += (f'<text x="{x + sx * 9:.1f}" y="{y + sy * 9 + k * 11:.1f}" dominant-baseline="central" text-anchor="{anchor}" font-family="{MONO}" '
                    f'font-size="{10 if i == 0 else 9.5}" fill="{fill}" fill-opacity="{fo}">{t}</text>')
    return out


class Plane:
    def __init__(self, cs, typ, fam, ph, lat, lon, ft, trend, kt, hdg, show='cs', tag=None, field=None, spoke=False,
                 selected=False, trail=None, trail_nm=None):
        self.__dict__.update(locals())
        del self.__dict__['self']


def glyph(x, y, hdg, fill, scale=1.35):
    return f'<g transform="translate({x:.1f} {y:.1f})"><path d="{AC}" fill="{fill}" transform="rotate({hdg}) scale({scale})"/></g>'


def trail(g, p):
    """Its track over the last two minutes, in its altitude colour."""
    if p.trail:
        pts = [g.xy(la, lo) for la, lo in p.trail] + [g.xy(p.lat, p.lon)]
    else:
        nm = p.trail_nm if p.trail_nm is not None else p.kt * 2 / 60
        la, lo = along(p.lat, p.lon, (p.hdg + 180) % 360, nm)
        pts = [g.xy(la, lo), g.xy(p.lat, p.lon)]
    return f'<polyline points="{" ".join(f"{x:.1f},{y:.1f}" for x, y in pts)}" fill="none" stroke="{band(p.ft)}" stroke-opacity=".55" stroke-width="1.5" stroke-linecap="round"/>'


def label(g, p):
    """MapLabel: the full label (phase, callsign, type; level, trend, speed, the probable runway or exit) and its message tag,
    or the callsign alone; above the glyph, left edges at the glyph's left."""
    x, y = g.xy(p.lat, p.lon)
    out = ''
    if p.show == 'full':
        l1 = f'[{p.ph}] {p.cs} {p.typ}'
        l2 = f'{alt_text(p.ft)} {p.trend} {p.kt} kt'
        n2 = len(l2) + (2 + len(p.field[0]) + (1 + len(p.field[1]) if p.field[1] else 0) if p.field else 0)
        w = max(len(l1), n2) * CW + 12
        h = 32
        tag_h = 19 if p.tag else 0
        top = y - 14 - (tag_h + 3 if p.tag else 0) - h
        lx = x - 6
        stroke = '#66c56a' if p.selected else '#3a3a3a'
        out += f'<rect x="{lx:.1f}" y="{top:.1f}" width="{w:.0f}" height="{h}" rx="3" fill="#0a0a0a" fill-opacity=".85" stroke="{stroke}" stroke-width="{1.5 if p.selected else 1}"/>'
        out += (f'<text x="{lx + 6:.1f}" y="{top + 13:.1f}" font-family="{MONO}" font-size="11" xml:space="preserve"><tspan fill="{PH[p.fam]}" font-weight="700">[{p.ph}]</tspan>'
                f'<tspan fill="#e0e0e0"> {p.cs} {p.typ}</tspan></text>')
        out += f'<text x="{lx + 6:.1f}" y="{top + 26:.1f}" font-family="{MONO}" font-size="11" xml:space="preserve"><tspan fill="#e0e0e0">{l2}</tspan>'
        if p.field:
            out += f'<tspan fill="#b8b8b8">  {p.field[0]}</tspan>'
            if p.field[1]:
                out += f'<tspan fill="#949494"> {p.field[1]}</tspan>'
        out += '</text>'
        if p.tag:
            words, age, alpha = p.tag
            words = words if len(words) <= 34 else words[:33].rstrip() + '…'
            ty = top + h + 3
            tw = max(len(words) * 6.3 + 46, 120)
            out += f'<g opacity="{alpha}"><rect x="{lx:.1f}" y="{ty:.1f}" width="{tw:.0f}" height="{tag_h}" rx="3" fill="#222222"/>'
            out += f'<rect x="{lx:.1f}" y="{ty:.1f}" width="2.5" height="{tag_h}" fill="#6cb6ff"/>'
            out += f'<text x="{lx + 8:.1f}" y="{ty + 13:.1f}" font-family="{TEXT}" font-size="11" fill="#e0e0e0">“{esc(words)}”</text>'
            out += f'<text x="{lx + tw - 6:.1f}" y="{ty + 13:.1f}" text-anchor="end" font-family="{MONO}" font-size="10" fill="#949494">{age}</text></g>'
    elif p.show == 'cs':
        w = len(p.cs) * CW + 10
        out += f'<rect x="{x - 6:.1f}" y="{y - 32:.1f}" width="{w:.0f}" height="17" rx="3" fill="#0a0a0a" fill-opacity=".85" stroke="#3a3a3a"/>'
        out += f'<text x="{x - 1:.1f}" y="{y - 20:.1f}" font-family="{MONO}" font-size="11" fill="#e0e0e0">{p.cs}</text>'
    return out


def boxes(g, p):
    """The label's box (and its tag), and the glyph's, as label() draws them."""
    x, y = g.xy(p.lat, p.lon)
    glyph_box = (x - 10, y - 10, x + 10, y + 10)
    if p.show == 'full':
        l1 = f'[{p.ph}] {p.cs} {p.typ}'
        l2 = f'{alt_text(p.ft)} {p.trend} {p.kt}\u202fkt'
        n2 = len(l2) + (2 + len(p.field[0]) + (1 + len(p.field[1]) if p.field[1] else 0) if p.field else 0)
        w = max(len(l1), n2) * CW + 12
        if p.tag:
            words = p.tag[0] if len(p.tag[0]) <= 34 else p.tag[0][:33] + '…'
            w = max(w, max(len(words) * 6.3 + 46, 120))
        top = y - 14 - (22 if p.tag else 0) - 32
        return (x - 6, top, x - 6 + w, y - 14), glyph_box
    if p.show == 'cs':
        return (x - 6, y - 32, x - 6 + len(p.cs) * CW + 10, y - 15), glyph_box
    return None, glyph_box


CHECKS = []


def check(g, planes, name, avoid=()):
    """Overlaps between labels, labels over another glyph, labels cut by the sky's edges or over a panel."""
    hit = lambda a, b: a[0] < b[2] and b[0] < a[2] and a[1] < b[3] and b[1] < a[3]
    bx = [(p, *boxes(g, p)) for p in planes]
    seen = [(p, l, gl) for p, l, gl in bx if -20 < gl[0] < g.w and -20 < gl[1] < g.h]
    for i, (p, l, gl) in enumerate(seen):
        if not l:
            continue
        if l[0] < 0 or l[2] > g.w or l[1] < 0 or l[3] > g.h:
            CHECKS.append(f'{name}: {p.cs} label cut by the edge {tuple(round(v) for v in l)}')
        for q, l2, gl2 in seen[i + 1:]:
            if l2 and hit(l, l2):
                CHECKS.append(f'{name}: {p.cs} and {q.cs} labels overlap')
        for q, l2, gl2 in seen:
            if q is not p and hit(l, gl2):
                CHECKS.append(f'{name}: {p.cs} label over {q.cs}')
        for a_name, a in avoid:
            if hit(l, a):
                CHECKS.append(f'{name}: {p.cs} label under {a_name}')


def planes_svg(g, planes, trails=True):
    out = ''
    if trails:
        out += ''.join(trail(g, p) for p in planes)
    for p in planes:
        x, y = g.xy(p.lat, p.lon)
        if p.spoke or p.selected:
            out += f'<circle cx="{x:.1f}" cy="{y:.1f}" r="13" fill="none" stroke="#4caf50" stroke-width="2"/>'
        out += glyph(x, y, p.hdg, band(p.ft))
    out += ''.join(label(g, p) for p in planes if p.show == 'cs')
    out += ''.join(label(g, p) for p in planes if p.show == 'full')
    return out


def altlegend(total, counts, histos, left=12, bottom=12):
    names = ('very-low', 'low', 'mid', 'high', 'cruise')
    bands = ''
    for n, c, hs in zip(names, counts, histos):
        bars = ''.join('<i class="is-empty"></i>' if v == 0 else f'<i style="height:{v}%"></i>' for v in hs)
        bands += f'<div class="cl-altlegend__band cl-altlegend__band--{n}"><div class="cl-altlegend__histo">{bars}</div><span class="cl-altlegend__count">{c}</span></div>'
    return (f'<div class="cl-altlegend" style="position: absolute; left: {left}px; bottom: {bottom}px;"><div class="cl-altlegend__head"><span class="cl-altlegend__title">Altitude</span>'
            f'<span class="cl-altlegend__total">{total} airborne</span></div><div class="cl-altlegend__bands">{bands}</div>'
            '<div class="cl-altlegend__bounds"><span style="grid-column: 1">1 000 ft</span><span style="grid-column: 2">3 000 ft</span>'
            '<span style="grid-column: 3">FL 125</span><span style="grid-column: 4">FL 180</span></div></div>')


PANELS = (('the view switch', (8, 8, 300, 44)), ('the altitude legend', (8, 852 - 118, 330, 852)), ('the layers button', (620 - 50, 852 - 50, 620, 852)))
LEGEND = altlegend(107, (4, 9, 22, 11, 61), ((10, 25, 10, 0), (20, 15, 30, 25), (20, 30, 25, 35, 20, 15, 25, 15), (15, 20, 25, 30, 10), (30, 15, 20, 10, 45, 75, 85, 25, 0)))


def sky_svg(g, body, w=620, h=852):
    return f'<svg class="cl-sky__map" viewBox="0 0 {w} {h}" preserveAspectRatio="xMidYMid slice" role="img" aria-label="Map of the traffic">{body}</svg>'


def sky(svg, top='', extra='', corner=True):
    corner_html = f'<div class="cl-sky__corner"><button class="cl-btn cl-btn--icon" aria-label="Layers and base map">{E.LAYERS}</button></div>' if corner else ''
    return f'<section class="cl-app__sky" aria-label="Sky">{svg}{extra}<div class="cl-sky__top">{top}</div>{corner_html}</section>'


def seg(pressed='Map'):
    return '<div class="cl-seg">' + ''.join(f'<button class="cl-seg__item" aria-pressed="{str(x == pressed).lower()}">{x}</button>'
                                            for x in ('Map', 'PAR · LFPG', 'PAR · LFPO', 'PAR · LFPZ')) + '</div>'


# ---------------------------------------------------------------- the scene: west configuration, 13:13Z
STATION = (48.86, 2.36)   # made up: central Paris, not the real station
C26L, C27R, C25 = 265.1, 265.3, 254.0


def traffic(selected=None):
    """The same made-up sky for every image: west configuration at 13:13Z."""
    P = []
    add = lambda *a, **k: P.append(Plane(*a, **k))
    # Charles de Gaulle, landing 26L and 27R
    la, lo = final('LFPG', '27R', 1.3, C27R)
    add('AFR85ZQ', 'A321', 'arr', 'APP', la, lo, 500, '↓', 138, 265, show='full', field=('→27R', ''))
    la, lo = final('LFPG', '26L', 9.5, C26L)
    add('AFR47KX', 'A320', 'arr', 'APP', la, lo, 3100, '↓', 168, 265, show='full', field=('→26L', ''), spoke=True,
        tag=('Air France quatre sept Kilo X-ray, autorisé ILS piste deux six gauche', '49 s', 1))
    add('EZY82QT', 'A21N', 'arr', 'ARR', 48.87, 2.83, 7400, '↓', 232, 270, show='full',
        tag=('Easy eight two Quebec Tango, descend flight level seven zero', '1 min', 1))
    add('BAW31QX', 'A320', 'arr', 'ARR', 49.135, 2.86, 5200, '↓', 214, 205)
    add('EZY14LM', 'A320', 'dep', 'CLB', 49.07, 2.36, 3000, '↑', 190, 270)
    # Orly, landing 25
    la, lo = final('LFPO', '25', 4.8, C25)
    add('TVF55ME', 'B738', 'arr', 'APP', la, lo, 1600, '↓', 146, 254, show='full', field=('→25', ''))
    la, lo = final('LFPO', '25', 16.0, C25)
    add('AFR59MY', 'A319', 'arr', 'APP', la, lo, 4300, '↓', 196, 254, show='full', field=('→25', ''))
    # departures
    add('TVF61WB', 'B738', 'dep', 'CLB', 49.205, 2.326, 9800, '↑', 288, 357, show='full', field=('→ATREX', ''),
        trail=[(49.13, 2.321)],
        )
    add('AFR71KE', 'A320', 'dep', 'DEP', 48.79, 2.198, 9000, '↑', 305, 152, show='full' if selected == 'AFR71KE' else 'cs',
        field=('→OKASI', '+1'), selected=selected == 'AFR71KE',
        trail=None if selected == 'AFR71KE' else [(48.971, 2.112), (48.8185, 2.1867)])
    add('AFR19ME', 'A21N', 'dep', 'T/O', 48.9975, 2.44, 1300, '↑', 168, 265)
    add('EZY27XH', 'A320', 'dep', 'CLB', 48.745, 2.285, 3900, '↑', 220, 240)
    # passing through, from low to cruise
    for cs, typ, la, lo, ft, tr, kt, hdg in (
            ('AFR23QR', 'A320', 49.24, 2.70, 11500, '↓', 280, 210), ('TVF30KP', 'B738', 48.97, 3.02, 13800, '↓', 300, 260),
            ('DLH4AN', 'A20N', 49.20, 3.10, 16500, '↓', 310, 230), ('KLM73VZ', 'E190', 48.66, 2.86, 24000, '↑', 380, 160),
            ('RYR4TQ', 'B38M', 48.76, 3.05, 37000, '↔', 452, 200), ('SWR8KJ', 'A20N', 49.29, 3.00, 35000, '↔', 448, 135),
            ('BAW8RT', 'A320', 48.62, 2.47, 33000, '↔', 441, 320), ('UAE7WK', 'A388', 49.13, 3.04, 38000, '↔', 486, 245),
            ('IBE36CZ', 'A321', 48.55, 2.85, 30000, '↓', 430, 10), ('EJU52XB', 'A320', 48.60, 3.00, 21000, '↓', 360, 330),
            ('TAP4NR', 'A21N', 49.12, 2.50, 36000, '↔', 455, 60), ('AUA61QE', 'A320', 48.66, 2.66, 28000, '↑', 400, 120),
            ('VLG8MW', 'A320', 48.45, 3.00, 34000, '↔', 446, 200), ('SAS5RB', 'A20N', 49.31, 2.86, 31000, '↑', 420, 20),
            ('AFR1092', 'A318', 48.84, 2.50, 8000, '↓', 250, 60)):
        add(cs, typ, 'crz' if ft > 24000 else ('arr' if tr == '↓' else 'dep'), 'CRZ' if ft > 24000 else ('ARR' if tr == '↓' else 'CLB'),
            la, lo, ft, tr, kt, hdg)
    return P


def thread_html(selected=None):
    sel = lambda cs: ' is-selected' if cs == selected else ''
    chip = E.chip
    W = E.WHEN
    return f'''<section class="cl-app__thread" aria-label="Radio">
{E.threadhead()}
<div class="cl-app__bottom">
<article class="cl-exchange{sel('AFR47KX')}">
  <div class="cl-exchange__head">{W('12:58:31Z · 126.425')}{chip('ARR', 'arr', 'AFR47KX', sel('AFR47KX'))}<span class="cl-exchange__to">→</span><span class="cl-chip cl-chip--atc">ATC</span>{E.STR3}{E.PLAYB}</div>
  <p class="cl-exchange__text" lang="fr">De Gaulle bonjour, Air France quatre sept Kilo X-ray, niveau un huit zéro en descente vers un cinq zéro.</p>
  <div class="cl-exchange__explain">First call on this frequency.</div>
</article>
<article class="cl-exchange{sel('TVF61WB')}">
  <div class="cl-exchange__head">{W('13:01:20Z · 124.350')}{chip('CLB', 'dep', 'TVF61WB', sel('TVF61WB'))}<span class="cl-exchange__to">→</span><span class="cl-chip cl-chip--atc">ATC</span>{E.STR3}{E.PLAYB}</div>
  <p class="cl-exchange__text">Approach, good afternoon, Transavia six one Whiskey Bravo, passing three thousand feet climbing flight level six zero, <span class="is-place">ATREX seven Alpha</span> departure.</p>
  <div class="cl-exchange__instr"><button class="cl-chip cl-chip--place">ATREX 7A</button></div>
  <div class="cl-exchange__explain">First call on this frequency.</div>
</article>
<article class="cl-exchange{sel('TVF61WB')}">
  <div class="cl-exchange__head">{W('13:01:31Z · 124.350')}<span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('CLB', 'dep', 'TVF61WB', sel('TVF61WB'))}{E.STR3}{E.PLAYB}</div>
  <p class="cl-exchange__text">Transavia six one Whiskey Bravo, identified, climb flight level one two zero.</p>
  <div class="cl-exchange__instr"><span class="cl-chip cl-chip--instr">Climb to FL 120</span></div>
  <div class="cl-exchange__reply"><span>↩</span><span>Pilot replied, 3 s later</span></div>
</article>
<article class="cl-exchange{sel('EZY82QT')}">
  <div class="cl-exchange__head">{W('13:11:52Z · 124.350')}<span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('APP', 'arr', 'EZY82QT', sel('EZY82QT'))}{E.STR3}{E.PLAYB}</div>
  <p class="cl-exchange__text">Easy eight two Quebec Tango, descend flight level seven zero, turn left heading two seven zero.</p>
  <div class="cl-exchange__instr"><span class="cl-chip cl-chip--instr">Descend to FL 070</span><span class="cl-chip cl-chip--instr">Turn left heading 270</span></div>
  <div class="cl-exchange__reply"><span>↩</span><span>Pilot replied, 12 s later</span></div>
</article>
<article class="cl-exchange is-unmatched">
  <div class="cl-exchange__head">{W('13:12:04Z · 126.425')}<span class="cl-chip cl-chip--callsign is-away">?</span><span class="cl-plain">no aircraft in range was named</span>{E.PLAYB}</div>
  <p class="cl-exchange__text">… roger, two seven zero, maintaining.</p>
</article>
<article class="cl-exchange{sel('AFR47KX')}" lang="fr">
  <div class="cl-exchange__head" lang="en">{W('13:12:31Z · 124.350')}<span class="cl-chip cl-chip--atc">ATC</span><span class="cl-exchange__to">→</span>{chip('APP', 'arr', 'AFR47KX', sel('AFR47KX'))}{E.STR2}{E.PLAYB}</div>
  <p class="cl-exchange__text">Air France quatre sept Kilo X-ray, autorisé ILS piste deux six gauche, contactez la tour un deux zéro décimal six cinq.</p>
  <div class="cl-exchange__instr" lang="en"><span class="cl-chip cl-chip--instr cl-chip--clearance">Cleared ILS runway 26L</span><span class="cl-chip cl-chip--instr">Contact 120.650</span></div>
  <div class="cl-exchange__explain cl-exchange__explain--boxed">Autorisé à l'approche ILS, piste 26L. Contactez 120.650.</div>
  <div class="cl-exchange__reply" lang="en"><span>↩</span><span>Pilot replied, 4 s later</span></div>
</article>
<article class="cl-exchange is-live">
  <div class="cl-exchange__head">{W('now · 124.350')}<span class="cl-level"><i style="height:6px"></i><i style="height:12px"></i><i style="height:9px"></i><i style="height:14px"></i><i style="height:7px"></i></span><span class="cl-plain">someone is speaking</span></div>
</article>
</div>
</section>'''


def seq_table(rows):
    body = ''.join(f'<tr><td class="is-dim">{i + 1}</td><td><b>{cs}</b> <span class="is-dim">{t}</span></td><td class="is-num">{d}</td><td class="is-num">{tm}</td>'
                   f'<td class="is-num{" is-dim" if b == "first" else (" is-tight" if b.startswith("+2") else "")}">{b}</td></tr>' for i, (cs, t, d, tm, b) in enumerate(rows))
    return ('<table class="cl-table"><thead><tr><th></th><th>Aircraft</th><th class="is-num">To thr.</th><th class="is-num">Time</th><th class="is-num">Behind</th></tr></thead>'
            f'<tbody>{body}</tbody></table>')


def column_now():
    return f'''<aside class="cl-app__column" aria-label="Context">
{E.tabs('Right now')}
<div class="cl-app__scroll">
<div class="cl-section"><p class="cl-summary">Charles de Gaulle has been using runways <b>26L</b> and <b>27R</b> since 11:40Z. Paris-Orly uses runway <b>25</b>. Four aircraft on final; the next, AFR85ZQ, lands on <b>27R</b> at Charles de Gaulle in about 1 min.</p><p class="cl-summary__source">Read from the ADS-B tracks, not announced by the controllers.</p></div>
<div class="cl-section"><span class="cl-label">Arrivals to LFPG 26L</span>{seq_table([('AFR47KX', 'A320', '9.5 NM', '3:24', 'first')])}</div>
<div class="cl-section"><span class="cl-label">Arrivals to LFPG 27R</span>{seq_table([('AFR85ZQ', 'A321', '1.3 NM', '0:34', 'first')])}</div>
<div class="cl-section"><span class="cl-label">Arrivals to LFPO 25</span>{seq_table([('TVF55ME', 'B738', '4.8 NM', '1:58', 'first'), ('AFR59MY', 'A319', '16.0 NM', '4:49', '+11.2 NM')])}</div>
<div class="cl-section"><span class="cl-label">Worth knowing · last 30 min</span><ul class="cl-events"><li><time>13:09Z</time><span><span class="cl-warn">▲ Go-around</span> EZY14LM on 27R at LFPG, now climbing back.</span></li><li><time>12:57Z</time><span><span class="is-info">●</span> UAE7WK, an A388, is in range, 34 NM north-east.</span></li></ul></div>
<div class="cl-section"><span class="cl-label">Weather at LFPG</span><span class="cl-metar">LFPG 201300Z 25012KT 9999 FEW030 17/09 Q1018</span><span class="cl-metar__plain">Wind from the west, 12 kt; visibility over 10 km; a few clouds at 3 000 ft; QNH 1018.</span><span class="cl-row"><button class="cl-btn">TAF</button><button class="cl-btn">NOTAM <span class="cl-tabs__count">4</span></button></span></div>
<div class="cl-section"><span class="cl-label">Traffic</span><div class="cl-stats"><span><b>107</b> airborne</span><span><b>12</b> heard</span><span><b>3</b> on the ground</span></div></div>
</div>
</aside>'''


def column_follow():
    head = f'''<div class="cl-tabs cl-tabs--column"><button class="cl-btn cl-btn--icon cl-mono" aria-label="Back to Right now">‹</button><span class="cl-plain">Back to Right now</span><span class="cl-tabs__end"><button class="cl-btn cl-btn--icon cl-mono" aria-label="Fold the column">»</button></span></div>
<div class="cl-story__head">
  <div class="cl-story__id"><span class="cl-phase cl-phase--dep">DEP</span><span class="cl-story__callsign">AFR71KE</span><span class="cl-badge" title="Followed airport">LFPG</span></div>
  <span class="cl-plain">Air France · Airbus A320 · F-HQXZ</span>
  <div class="cl-story__grid">
    <div><div class="cl-label">Altitude</div><div class="cl-story__val">FL 090 ↑</div></div>
    <div><div class="cl-label">To OKASI</div><div class="cl-story__val">45<span class="cl-unit">NM</span><span style="color: var(--color-text-tertiary);"> · or PILUL</span></div></div>
    <div><div class="cl-label">Ground speed</div><div class="cl-story__val">305<span class="cl-unit">kt</span></div></div>
    <div><div class="cl-label">Heading</div><div class="cl-story__val">152°<span class="cl-unit">mag</span></div></div>
  </div>
  <div class="cl-row"><span class="cl-label">Show only</span><button class="cl-toggle">FL 090 ± 500 ft</button><button class="cl-toggle">DEP</button></div>
</div>
<div class="cl-tabs cl-tabs--inset" role="tablist"><button class="cl-tabs__tab" role="tab" aria-selected="true">Story</button><button class="cl-tabs__tab" role="tab" aria-selected="false">Details</button><button class="cl-tabs__tab" role="tab" aria-selected="false">Track <span class="cl-tabs__count">96</span></button><button class="cl-tabs__tab" role="tab" aria-selected="false">Nearby</button></div>'''
    body = '''<ol class="cl-story__events">
  <li><time>13:01Z</time><i></i><span>Seen by the receiver on the ground at LFPG.</span></li>
  <li><time>13:04Z</time><i></i><span>Departed runway 26R at LFPG.</span></li>
  <li><time>13:07Z</time><i class="is-radio"></i><span>First call to Approche De Gaulle, passing 4 000 ft.</span></li>
  <li><time>13:07Z</time><i class="is-radio"></i><span>Controller on 124.350: Climb to FL 120.</span></li>
  <li><time>13:13Z</time><i class="is-now"></i><span>Climbing through FL 090, 21 NM south-west of LFPG.</span></li>
</ol>'''
    return f'<aside class="cl-app__column" aria-label="AFR71KE">{head}<div class="cl-app__scroll">{body}</div></aside>'


def topbar(following=None):
    return E.topbar(following).replace('13:13:20Z', '13:13:20Z')


def page(title, body, width=1440, height=900, zoom=2):
    html = E.page(title, body, width, height)
    return html.replace('</head>', f'<style>html {{ zoom: {zoom}; }}</style>\n</head>')


def hero():
    g = Geo(48.90, 2.755, 40)
    P = traffic()
    for p in P:
        if p.cs == 'AFR71KE':
            p.show = None
    check(g, P, 'hero', PANELS)
    body = rings(g, *STATION) + airports(g) + planes_svg(g, P)
    return page('Clairance', topbar() + '<div class="cl-app__main">' + thread_html() + sky(sky_svg(g, body), seg(), LEGEND) + column_now() + '</div>')


def follow():
    g = Geo(48.58, 2.33, 56)
    ok, pil = route('LFPG', 'OKASI 7B'), route('LFPG', 'PILUL 7B')
    lines_q, _ = route_lines(g, pil, '26R', strong=False)
    lines_s, _ = route_lines(g, ok, '26R', strong=True)
    pts = route_points(g, [(ok, '26R'), (pil, '26R')], strong_names={'DOPAP', 'ODEBU', 'OKASI'})
    keep = {'AFR71KE', 'EZY27XH', 'BAW8RT', 'AUA61QE', 'KLM73VZ', 'IBE36CZ', 'TVF55ME', 'AFR59MY'}
    P = [p for p in traffic(selected='AFR71KE') if p.cs in keep]
    for p in P:
        if p.cs != 'AFR71KE':
            p.show = 'cs'
    # its track since the runway, in the followed colour, with its levels a few minutes back
    trk = [THR[('LFPG', '26R')], (48.991, 2.465), (48.984, 2.346), (48.971, 2.112), (48.8185, 2.1867), (48.79, 2.198)]
    tp = [g.xy(*q) for q in trk]
    track = f'<polyline points="{" ".join(f"{x:.1f},{y:.1f}" for x, y in tp)}" fill="none" stroke="#66c56a" stroke-width="2" stroke-dasharray="5 4"/>'
    for (x, y), t, dx, dy in zip(tp[2:4], ('−6m 4800', '−4m 6900'), (0, -8), (-8, -8)):
        anchor = 'start' if dx >= 0 else 'end'
        track += f'<circle cx="{x:.1f}" cy="{y:.1f}" r="2.2" fill="#66c56a"/><text x="{x + dx:.1f}" y="{y + dy:.1f}" text-anchor="{anchor}" font-family="{MONO}" font-size="10.5" fill="#949494">{t}</text>'
    x1, y1 = tp[4]
    track += f'<circle cx="{x1:.1f}" cy="{y1:.1f}" r="2.2" fill="#66c56a"/>'
    check(g, P, 'follow', PANELS + (('the note', (300, 8, 560, 44)),))
    body = rings(g, *STATION, radii=(25, 50)) + airports(g, ('LFPG', 'LFPO', 'LFPZ')) + lines_q + lines_s + pts + track + planes_svg(g, [p for p in P if p.cs != 'AFR71KE'], trails=True)
    body += planes_svg(g, [p for p in P if p.cs == 'AFR71KE'], trails=False)
    note = '<div class="cl-sky__note"><b>AFR71KE</b> · its track since 13:01Z</div>'
    return page('Following a flight', topbar('AFR71KE') + '<div class="cl-app__main">' + thread_html('AFR71KE') + sky(sky_svg(g, body), seg() + note, LEGEND) + column_follow() + '</div>')


def map_routes():
    """A route asked from a procedure chip (Q56), and the probable runway and exit on the labels (Q57)."""
    g = Geo(49.30, 2.67, 60)
    at = route('LFPG', 'ATREX 7A')
    lines, _ = route_lines(g, at, '27L', strong=True)
    pts = route_points(g, [(at, '27L')], strong_names={'ATREX'})
    x0, y0 = g.xy(*THR[('LFPG', '27L')])
    lines += f'<text x="{x0 + 8:.1f}" y="{y0 + 14:.1f}" font-family="{MONO}" font-size="10" fill="{ROUTE}">27L</text>'
    P = []
    add = lambda *a, **k: P.append(Plane(*a, **k))
    la, lo = final('LFPG', '27R', 1.3, C27R)
    add('AFR85ZQ', 'A321', 'arr', 'APP', la, lo, 500, '↓', 138, 265, show='full', field=('→27R', ''))
    la, lo = final('LFPG', '26L', 16.0, C26L)
    add('BAW31QX', 'A320', 'arr', 'APP', la, lo, 5200, '↓', 212, 265, show='full', field=('→26L/R', ''))
    add('TVF61WB', 'B738', 'dep', 'CLB', 49.205, 2.326, 9800, '↑', 288, 357, show='full', field=('→ATREX', ''), trail=[(49.13, 2.321)])
    add('AFR19ME', 'A21N', 'dep', 'T/O', 48.9975, 2.44, 1300, '↑', 168, 265)
    add('EZY14LM', 'A320', 'dep', 'CLB', 49.07, 2.40, 3000, '↑', 190, 270)
    add('EZY82QT', 'A21N', 'arr', 'ARR', 48.90, 2.95, 7400, '↓', 232, 270)
    for cs, typ, la, lo, ft, tr, kt, hdg in (
            ('SAS5RB', 'A20N', 49.55, 2.90, 31000, '↑', 420, 20), ('SWR8KJ', 'A20N', 49.62, 2.45, 35000, '↔', 448, 135),
            ('DLH4AN', 'A20N', 49.45, 3.05, 16500, '↓', 310, 230), ('UAE7WK', 'A388', 49.70, 3.10, 38000, '↔', 486, 245),
            ('AFR23QR', 'A320', 49.35, 2.20, 11500, '↓', 280, 160), ('TAP4NR', 'A21N', 49.22, 2.98, 36000, '↔', 455, 60),
            ('KLM73VZ', 'E190', 49.85, 2.75, 24000, '↑', 380, 340), ('RYR4TQ', 'B38M', 49.40, 3.20, 37000, '↔', 452, 200)):
        add(cs, typ, 'crz' if ft > 24000 else ('arr' if tr == '↓' else 'dep'), 'CRZ' if ft > 24000 else ('ARR' if tr == '↓' else 'CLB'),
            la, lo, ft, tr, kt, hdg)
    check(g, P, 'map-routes', PANELS + (('the note', (300, 8, 600, 44)),))
    body = airports(g, names=(('LFPG', 'LFPG', 1.0, '#60a5fa', (10, 26)),)) + lines + pts + planes_svg(g, P)
    note = ('<div class="cl-sky__note cl-sky__note--action"><span class="cl-sky__note-text">ATREX 7A · published departure from 27L</span>'
            f'<button class="cl-btn cl-btn--icon" aria-label="Close the route">{E.CLOSE}</button></div>')
    return page('A published route', topbar() + '<div class="cl-app__main">' + thread_html() + sky(sky_svg(g, body), seg() + note, LEGEND) + column_now() + '</div>')


def par():
    return page('The PAR view', topbar() + '<div class="cl-app__main">' + thread_html() + sky('', seg('PAR · LFPG'), E.par_view(), corner=False) + column_now() + '</div>')


def phone():
    g = Geo(48.985, 2.80, 20, w=390, h=700)
    keep = {'AFR85ZQ', 'AFR47KX', 'EZY82QT', 'BAW31QX', 'AFR23QR', 'DLH4AN', 'AFR1092'}
    P = [p for p in traffic() if p.cs in keep]
    for p in P:
        if p.cs not in ('AFR85ZQ', 'AFR47KX'):
            p.show = 'cs'
        if p.cs == 'AFR47KX':
            p.tag = None
    check(g, P, 'phone', ())
    svg = f'<svg class="cl-sky__map" viewBox="0 0 390 700" preserveAspectRatio="xMidYMid slice">{rings(g, *STATION, radii=(10, 25)) + airports(g, ("LFPG", "LFPB")) + planes_svg(g, P)}</svg>'
    narrow = ('<div class="cl-threadhead"><div class="cl-threadhead__row">' + E.channel('124.350', 'Approche', '', on=True) + E.channel('126.425', 'App. (L)', '')
              + '</div></div><div class="cl-app__bottom">' + thread_html().split('<div class="cl-app__bottom">')[1].rsplit('</section>', 1)[0])
    phones = E.phone('Radio', narrow) + E.phone('Sky', f'<section class="cl-app__sky" style="height: 100%;">{svg}</section>')
    return page('Phone', '<div class="cl-row" style="padding: 24px; align-items: flex-start; gap: 32px;">' + phones + '</div>', width=880, height=900)


if __name__ == '__main__':
    os.makedirs(OUT, exist_ok=True)
    for name, fn in (('hero', hero), ('follow', follow), ('map-routes', map_routes), ('par', par), ('phone', phone)):
        with open(os.path.join(OUT, name + '.html'), 'w') as f:
            f.write(fn())
        print(os.path.join(OUT, name + '.html'))
    print('\n'.join(CHECKS) or 'no overlap')
