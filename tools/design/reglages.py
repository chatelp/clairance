"""The settings, in pages (D86, 28/10): the reference screens built by ecrans.py.

A grouped navigation (This browser, Station, Administration), one page at a
time. From 768 px a window over the application, its pages in a row of tabs,
each a glyph over its label (owner, 28/10); fields keep their reading width,
lists of cards go in two columns. On a phone, the list of pages, then a page
with "‹ Settings". Only cl-* classes; inline style only for an illustration
(the usage bars). The fields are the page's own (www/index.html, 28/10), moved
and renamed as D86 decided; the sample values are made up.
"""

CLOSE = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8"/></svg>'

PAGES = [
    ('This browser', [('display', 'Display'), ('account', 'Account')]),
    ('Station', [('listening', 'Listening'), ('matching', 'Matching')]),
    ('Administration', [('access', 'Access'), ('usage', 'Usage'), ('server', 'Server')]),
]
LEDE = {
    'display': 'How this browser draws the screen. Kept in this browser only.',
    'account': 'Who is signed in on this browser.',
    'listening': 'What the station listens to, and the official names it reads.',
    'listening-viewer': 'What the station listens to, and the official names it reads. Set by an admin.',
    'matching': 'How a transmission is tied to the aircraft that spoke.',
    'access': 'Who may open Clairance, and with which account.',
    'usage': 'What each account did these 7 days. Seen by admins only.',
    'server': "The server's log, its databases and their archive, then what it reports.",
    'about': 'The version of this page and of the server, and what changed.',
}
TITLE = {k: t for _, items in PAGES for k, t in items}
GROUP = {k: g for g, items in PAGES for k, _ in items}

# Each page's glyph: the interface glyphs' 16 grid, 1.5 px strokes, round ends, drawn at 20 px
_G = '<svg class="cl-page__glyph" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">{}</svg>'
GLYPH = {
    # a screen on its stand: how this browser draws
    'display': _G.format('<rect x="2" y="2.75" width="12" height="8.5" rx="1"/><path d="M8 11.25v2.5M5.5 13.75h5"/>'),
    # a person: who is signed in
    'account': _G.format('<circle cx="8" cy="5.25" r="2.75"/><path d="M2.75 14c.35-2.9 2.55-4.5 5.25-4.5s4.9 1.6 5.25 4.5"/>'),
    # headphones: what the station listens to
    'listening': _G.format('<path d="M2.75 10.5V8a5.25 5.25 0 0 1 10.5 0v2.5"/><rect x="2" y="9.25" width="3" height="4.75" rx="1"/><rect x="11" y="9.25" width="3" height="4.75" rx="1"/>'),
    # two links: a transmission tied to the aircraft that spoke
    'matching': _G.format('<path d="M7 9.5 9.5 7"/><path d="M8.6 4.4 9.7 3.3a2.55 2.55 0 0 1 3.6 3.6l-1.1 1.1"/><path d="M7.4 11.6 6.3 12.7a2.55 2.55 0 0 1-3.6-3.6l1.1-1.1"/>'),
    # a padlock: who may open Clairance
    'access': _G.format('<rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2M8 9.75v1.5"/>'),
    # a week in bars: what each account did
    'usage': _G.format('<path d="M2.5 13.5h11M4.75 11V8.5M8 11V4M11.25 11V6.5"/>'),
    # two rack units: the server, its log and databases
    'server': _G.format('<rect x="2.5" y="2.5" width="11" height="4.5" rx="1"/><rect x="2.5" y="9" width="11" height="4.5" rx="1"/><path d="M5 4.75h.01M5 11.25h.01"/>'),
    # the information sign: the versions and what is new
    'about': _G.format('<circle cx="8" cy="8" r="6"/><path d="M8 7.5v3.5M8 5h.01"/>'),
}
TITLE['about'] = 'About Clairance'
MARKS = {'server': ('is-warn', '▲', 'Needs a look: the archive disk is missing')}


def nav(current, role):
    groups = PAGES if role == 'admin' else PAGES[:2]
    out = []
    link = lambda key, title, mark='': f'<a href="#" aria-current="{str(key == current).lower()}">{GLYPH[key]}<span>{title}</span>{mark}</a>'
    for i, (name, items) in enumerate(groups):
        links = ''
        for key, title in items:
            if role != 'admin' and key == 'matching':
                continue
            mark = ''
            if role == 'admin' and key in MARKS:
                cls, glyph, why = MARKS[key]
                mark = f'<span class="cl-page__mark {cls}" title="{why}">{glyph}</span>'
            links += link(key, title, mark)
        out.append(f'<div class="cl-page__group" role="group" aria-labelledby="g{i}"><span class="cl-label" id="g{i}">{name}</span>{links}</div>')
    # About, after the groups and for everyone: the versions and what is new (28/10)
    out.append(f'<div class="cl-page__group">{link("about", "About Clairance")}</div>')
    return f'<nav class="cl-page__nav" aria-label="Settings pages">{"".join(out)}</nav>'


def field(label, control, help_='', source='', text=False, tip=''):
    tipattr = f' class="cl-field__label cursor-help" data-help-title="{label}" data-help="{tip}"' if tip else ' class="cl-field__label"'
    return (f'<div class="cl-field{" cl-field--text" if text else ""}"><span{tipattr}>{label}</span>{control}'
            + (f'<span class="cl-field__help">{help_}</span>' if help_ else '')
            + (f'<span class="cl-field__source">{source}</span>' if source else '') + '</div>')


def switch(on):
    return f'<span class="cl-switch{" is-on" if on else ""}" role="switch" aria-checked="{str(on).lower()}"></span>'


def seg(items, pressed):
    return '<div class="cl-seg">' + ''.join(f'<button class="cl-seg__item" aria-pressed="{str(x == pressed).lower()}">{x}</button>' for x in items) + '</div>'


def value(v):
    return f'<span class="cl-field__value">{v}</span>'


def section(title, body, wide=False):
    h = f'<h3>{title}</h3>' if title else ''
    return f'<section class="cl-page__section{" cl-page__section--wide" if wide else ""}">{h}{body}</section>'


# ---------------------------------------------------------------- the pages
def page_display():
    return (section('', field('Last transmission on aircraft labels', switch(True), 'A tag under the label, in quotes, for ten minutes after it was heard: full for two minutes, then paler.')
                    + field('Smooth movement between updates', switch(True))
                    + field('Frames per second', '<span class="cl-row"><div class="cl-slider" style="width: 160px;"><span class="cl-slider__rail"></span><span class="cl-slider__fill" style="left:0;width:40%"></span><span class="cl-slider__thumb" style="left:40%"></span></div><span class="cl-age">30</span></span>')
                    + field('Track points kept per aircraft', '<span class="cl-row"><div class="cl-slider" style="width: 160px;"><span class="cl-slider__rail"></span><span class="cl-slider__fill" style="left:0;width:25%"></span><span class="cl-slider__thumb" style="left:25%"></span></div><span class="cl-age">1000</span></span>')
                    + field('Hide ground aircraft at other airports', switch(False))
                    + field('Times in', seg(('UTC', 'Local'), 'UTC'), 'The clock in green in the top bar dates everything on screen.'))
            + section('', '<button class="cl-disclosure" aria-expanded="false">Diagnostics</button><span class="cl-plain">Live updates, and the counters of this page: to look at a moment, or to report a problem.</span>'))


def page_account():
    return section('', field('Signed in as', '<span class="cl-row"><b class="cl-mono">pierre</b><span class="cl-badge">Admin</span><button class="cl-btn">Sign out</button></span>'))


def aip_block(role):
    lights = '<span class="cl-light cl-light--ok">Up to date · AIRAC 2026-10-01</span>'
    kv = ('<dl class="cl-kv"><dt>Reporting points</dt><dd>452</dd><dt>Control positions</dt><dd>38</dd><dt>Procedures</dt><dd>445</dd>'
          '<dt>Area</dt><dd>120 NM around the station</dd><dt>Aerodromes</dt><dd>LFPG · LFPO · LFPZ (the ones followed)</dd></dl>')
    src = 'Source: SIA (DGAC), eAIP France, AIRAC cycle of 2026-10-01: ENR 4.4, ENR 2.1, AD 2 LFPG, LFPO, LFPZ. Made on 15/10 at 14:20Z. Next cycle: 2026-10-29.'
    body = field('Names from the AIP', lights, 'The official names around the station, from the French eAIP: reporting points, control positions, procedures.',
                 tip='They name the procedures heard in the thread, from the sound of the words. They do not decide which aircraft is speaking.') + kv
    if role == 'admin':
        body += '<div class="cl-row"><button class="cl-btn">Update from the SIA</button><span class="cl-plain">Five public pages of the eAIP, about 9 MB, a few seconds.</span></div>'
    body += f'<span class="cl-field__source">{src}</span>'
    if role == 'admin':
        body += field('Update at each new AIRAC cycle', switch(False), 'Every 28 days, when a new cycle comes into force, the server downloads the pages again.')
        body += field('Name the procedures heard in the thread', switch(True), 'A dashed chip beside the words gives the official name they probably were: “ELCOB 7G” for “elcope seven golf”.')
        body += field('How sure', seg(('Surer', 'Standard', 'More'), 'Standard'), '18 names, all right.', "Measured on the owner's listening of Paris approaches, 77 cases, October 2026.")
    else:
        body += field('Update at each new AIRAC cycle', value('Off'))
        body += field('Name the procedures heard in the thread', value('On · Standard'), 'A dashed chip beside the words gives the official name they probably were.')
    return section('Official names (AIP)', body)


def page_listening(role):
    onair = field('On air', '<span class="cl-field__value">Approche De Gaulle 124.350 · De Gaulle App. (L) 126.425</span>', 'Chosen at the station; this page plays what it receives.', text=True)
    if role == 'admin':
        ref = field('Reference airport', '<select class="cl-select" aria-label="Reference airport"><option>LFPZ · Saint-Cyr-l’École · 2 NM</option></select>', 'Phases (taxi, take-off, approach) are judged against it.')
        follow = field('Also follow', '<span class="cl-row"><label class="cl-check is-on"><span class="cl-check__box"></span>LFPG</label><label class="cl-check is-on"><span class="cl-check__box"></span>LFPO</label><label class="cl-check"><span class="cl-check__box"></span>LFPB</label></span>', 'Up to three: each gets a PAR and an arrival sequence.')
    else:
        ref = field('Reference airport', value('LFPZ · Saint-Cyr-l’École · 2 NM'), 'Phases (taxi, take-off, approach) are judged against it.')
        follow = field('Also follow', value('LFPG · LFPO'), 'Each gets a PAR and an arrival sequence.')
    return section('', onair + ref + follow) + aip_block(role)


def page_matching():
    return (section('', field('Rules', seg(('Standard', 'Cautious', 'Experimental'), 'Standard'), 'How a transmission is tied to an aircraft.', 'Measured on Paris approaches (CDG, ORY, LBG), October 2026.',
                              tip='Standard: 95 % right on a day of the station. Cautious: fewer matches, 97 % right. Experimental: more matches, less sure.'))
            + section('', '<button class="cl-disclosure" aria-expanded="false">Advanced</button><span class="cl-plain">The eleven rules behind the presets, each with what it gave on the station; “Accept one digit off” and “First two digits” among them.</span>'))


def page_access():
    acct = lambda name, role, facts, own=False: (
        f'<div class="cl-card"><div class="cl-row"><b class="cl-mono">{name}</b><span class="cl-badge">{role}</span></div>'
        f'<span class="cl-plain">{facts}</span><div class="cl-row"><button class="cl-btn cl-btn--quiet">New password</button>'
        + ('' if own else '<button class="cl-btn cl-btn--quiet">Cut access</button><button class="cl-btn cl-btn--quiet">Remove</button>') + '</div></div>')
    cards = (acct('pierre', 'Admin', '2 sessions · seen just now · written in config.toml', own=True)
             + acct('famille', 'Viewer', '1 session · seen 3 h ago')
             + acct('club', 'Viewer', 'not signed in'))
    who = field('Who may open Clairance', '<span class="cl-row"><label class="cl-check cl-check--radio"><span class="cl-check__box"></span>This machine</label><label class="cl-check cl-check--radio is-on"><span class="cl-check__box"></span>The network, with an account</label></span>', 'The same choice as at the first start.')
    live = field('Live figures on the sign-in page', switch(True), 'Aircraft, channels, transmissions and uptime of this station, aggregated, before signing in.')
    return (section('', who)
            + section('Accounts', '<span class="cl-plain">An admin sees and changes everything, accounts included; a viewer reads and listens.</span><div class="cl-row"><button class="cl-btn">Add an account</button></div>'
                      f'<div class="cl-grid2">{cards}</div>', wide=True)
            + section('', live))


def bars(days):
    out = ''
    for d, c, l in days:
        out += (f'<div style="display:flex;flex-direction:column;align-items:center;gap:2px;width:1.5rem;">'
                f'<div style="position:relative;width:100%;height:2rem;overflow:hidden;">'
                f'<div style="position:absolute;bottom:0;left:0;right:0;border-radius:2px;height:{c}%;background:var(--color-border-default);"></div>'
                f'<div style="position:absolute;bottom:0;left:0;right:0;border-radius:2px;height:{l}%;background:var(--color-accent-fill);"></div></div>'
                f'<span class="cl-age">{d}</span></div>')
    return f'<div class="cl-row" role="img" aria-label="Each day of the week" style="align-items:flex-end;gap:4px;">{out}</div>'


def page_usage():
    now = ('<div class="cl-field"><span class="cl-field__label">Now</span><span></span>'
           '<span class="cl-field__help"><b class="cl-mono">pierre</b> · page open 42 min · listening to 124.350</span>'
           '<span class="cl-field__help"><b class="cl-mono">famille</b> · page open 5 min · sound off</span></div>')
    card = lambda name, seen, week, days, inter: (
        f'<div class="cl-card"><div class="cl-row"><b class="cl-mono">{name}</b><span class="cl-age">last seen {seen}</span></div>'
        f'<span class="cl-plain">{week}</span>{bars(days)}' + ''.join(f'<span class="cl-age">{i}</span>' for i in inter) + '</div>')
    week = [('We', 30, 20), ('Th', 55, 40), ('Fr', 20, 0), ('Sa', 80, 70), ('Su', 100, 85), ('Mo', 45, 30), ('Tu', 60, 35)]
    week2 = [('We', 0, 0), ('Th', 25, 0), ('Fr', 40, 30), ('Sa', 0, 0), ('Su', 60, 55), ('Mo', 10, 0), ('Tu', 15, 0)]
    cards = (card('pierre', 'just now', 'connected 21 h · listened 14 h (124.350 9 h, 126.425 5 h) · 6 clips played again · 23 visits', week,
                  ['Interruptions', '27/10 11:29 · page · the server restarted · back in 8 s'])
             + card('famille', '3 h ago', 'connected 5 h 10 min · listened 2 h 40 min (124.350) · 4 visits', week2, []))
    return (section('', '<div class="cl-row"><span class="cl-plain">The last 7 days; 30 days are kept.</span><button class="cl-btn cl-btn--quiet">Refresh</button></div>' + now)
            + section('Accounts', f'<div class="cl-grid2">{cards}</div>', wide=True))


# The folder picker (owner, 28/10): the server machine's folders, browsed from the page
_I = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">{}</svg>'
FOLDER = _I.format('<path d="M2 4.25c0-.55.45-1 1-1h3.25l1.5 1.5H13c.55 0 1 .45 1 1v6c0 .55-.45 1-1 1H3c-.55 0-1-.45-1-1z"/>')
DISK = _I.format('<rect x="1.75" y="5" width="12.5" height="6" rx="1.5"/><path d="M11.25 8h.01"/>')
HOME = _I.format('<path d="M2.5 7.5 8 3l5.5 4.5M4 6.5V13h8V6.5"/>')
PLACES = [('Archive', DISK, '1.8 TB free'), ('Macintosh HD', DISK, '210 GB free'), ('Home', HOME, '')]


def folder_picker(state='open'):
    """'open': inside a disk, a folder that can be chosen; 'databases': the databases' own folder, refused."""
    if state == 'databases':
        path, here, rows = ['/', 'Users', 'pierre', 'clairance', 'data'], 'Home', []
        foot = ('<span class="cl-light cl-light--warn">The databases are kept here: choose a folder outside, best on another disk.</span>'
                '<span class="cl-row"><button class="cl-btn cl-btn--quiet">Cancel</button><button class="cl-btn cl-btn--primary" disabled>Choose this folder</button></span>')
    else:
        path, here = ['/', 'Volumes', 'Archive', 'clairance'], 'Archive'
        rows = [('bases', ''), ('exports', ''), ('Old Mac', 'read only')]
        foot = ('<span class="cl-plain">1.8 TB free on Archive.</span>'
                '<span class="cl-row"><button class="cl-btn cl-btn--quiet">Cancel</button><button class="cl-btn cl-btn--primary">Choose this folder</button></span>')
    places = ''.join(f'<button class="cl-folder__place" aria-current="{str(n == here).lower()}">{g}{n}' + (f' <span class="cl-folder__free">{f}</span>' if f else '') + '</button>'
                     for n, g, f in PLACES)
    crumbs = []
    for i, part in enumerate(path):
        cur = ' aria-current="location"' if i == len(path) - 1 else ''
        crumbs.append(f'<button{cur}>{part}</button>')
    crumb = '<span class="cl-folder__sep" aria-hidden="true">/</span>'.join(crumbs[1:])
    crumb = crumbs[0] + crumb
    if rows:
        lst = ''.join(f'<li><button class="cl-folder__row">{FOLDER}<span>{n}</span>' + (f'<span class="cl-folder__note">{w}</span>' if w else '') + '</button></li>' for n, w in rows)
    else:
        lst = '<li class="cl-folder__empty">No folder inside.</li>'
    return (f'<div class="cl-folder" role="group" aria-label="Choose the archive folder"><div class="cl-folder__places">{places}</div>'
            f'<nav class="cl-folder__crumbs" aria-label="Open folder">{crumb}</nav>'
            f'<ul class="cl-folder__list" aria-label="Folders in {path[-1]}">{lst}</ul><div class="cl-folder__foot">{foot}</div></div>')


def page_server(picker=None):
    log = field('Log detail', '<select class="cl-select" aria-label="Log detail"><option>info</option></select>', 'How much the server writes to its log.')
    keep = field('Keep databases up to', '<span class="cl-row"><input class="cl-input" style="width: 72px;" value="6" aria-label="Gigabytes"><span class="cl-age">GB</span></span>',
                 'The day’s database and the days before; a day past this size goes to the archive below.')
    if picker:
        arch = field('Archive old databases to', '<span class="cl-pathfield"><input class="cl-input" value="" placeholder="none: deleted past the size" aria-label="Archive folder">'
                     '<button class="cl-btn" aria-expanded="true">Choose…</button></span>',
                     '<span class="cl-light cl-light--warn">No archive: the databases past 6 GB are deleted for good.</span>', text=True) + folder_picker(picker)
    else:
        arch = field('Archive old databases to', '<span class="cl-pathfield"><input class="cl-input" value="/Volumes/Archive/clairance/bases" aria-label="Archive folder">'
                     '<button class="cl-btn" aria-expanded="false">Choose…</button></span>',
                     '<span class="cl-light cl-light--warn">Not found: the disk is not mounted. Nothing is deleted until it is back.</span>', text=True)
    kvcard = lambda title, rows: (f'<div class="cl-card"><span class="cl-label">{title}</span><dl class="cl-kv">'
                                  + ''.join(f'<dt>{k}</dt><dd>{v}</dd>' for k, v in rows) + '</dl></div>')
    reports = (kvcard('Storage', [('Database', 'co-atc-2026-10-28.db · 412 MB'), ('Growing', '33 MB/h'), ('Disk free', '41 GB'), ('Disk full in', '52 d')])
               + kvcard('Server', [('Uptime', '1 d 3 h'), ('Transcription', 'ok'), ('Log', '100 MB × 5 files')])
               + kvcard('ADS-B source', [('Mode', 'tar1090'), ('Status', '● connected'), ('Aircraft feed', 'available'), ('Receiver', 'available'), ('Stats', 'available'), ('Updated', '1 s ago')])
               + kvcard('Receiver and decoder', [('Receiver', 'range 210 NM · 107 aircraft'), ('Decoder', 'readsb · 700 msg/s')]))
    return section('', log + keep + arch) + section('Reports', f'<div class="cl-grid2">{reports}</div>', wide=True)


NEWS = [
    ('28 October 2026', ['The settings are now in pages, grouped by who they concern.',
                         'The sound starts within a second, and plays on an iPhone.',
                         'After a cut, the page reconnects on its own and says so.']),
    ('16 October 2026', ['A procedure named in the thread can be drawn on the map: click its name.',
                         'Aircraft labels show the runway or the exit they are heading for.']),
]


def page_about(role='admin', outdated=False):
    page_v = '27 October 2026' if outdated else '28 October 2026'
    commit = lambda c: f' <span class="cl-age">{c}</span>' if role == 'admin' else ''
    this = field('This page', value(page_v + commit('9f3c2e1' if outdated else 'b7433bc')))
    if outdated:
        srv = field('The server', '<span class="cl-row"><span class="cl-light cl-light--warn">28 October 2026</span><button class="cl-btn cl-btn--primary">Reload</button></span>',
                    'The server runs a newer version than this page: reload it to use it.')
    else:
        srv = field('The server', value('28 October 2026' + commit('b7433bc')), 'This page and the server are the same version.')
    news = ''.join(f'<div class="cl-field"><span class="cl-field__label">{d}</span><span></span><ul class="cl-notice__list" style="grid-column: 1 / -1;">'
                   + ''.join(f'<li>{i}</li>' for i in items) + '</ul></div>' for d, items in NEWS)
    return section('', this + srv) + section("What's new", news)


BODY = {'about': page_about, 'display': page_display, 'account': page_account, 'matching': page_matching, 'access': page_access, 'usage': page_usage, 'server': page_server}


def settings_page(current='listening', role='admin', state='', picker=None):
    """One markup for the computer and both phone states: state '' (computer), 'is-list' or 'is-detail' (phone)."""
    body = page_listening(role) if current == 'listening' else page_about(role) if current == 'about' else page_server(picker) if current == 'server' else BODY[current]()
    lede = LEDE['listening-viewer'] if (current == 'listening' and role != 'admin') else LEDE[current]
    server_page = current in ('listening', 'matching', 'access', 'server') and role == 'admin'
    reload_ = '<button class="cl-btn cl-btn--quiet">Reload from the server</button>' if server_page else ''
    saved = '<span class="cl-light cl-light--ok">Saved</span>' if (role == 'admin' or current == 'display') else ''
    head = (f'<div class="cl-page__head"><h1 class="cl-page__title">Settings</h1>{saved}'
            f'<span class="cl-row" style="margin-left: auto;">{reload_}<button class="cl-btn cl-btn--icon" aria-label="Close">{CLOSE}</button></span></div>')
    back = '<div class="cl-page__back"><button class="cl-btn cl-btn--icon cl-mono" aria-label="Back to Settings">‹</button><span class="cl-plain">Settings</span></div>'
    cls = f' {state}' if state else ''
    eyebrow = f'<span class="cl-label cl-page__eyebrow">{GROUP[current]}</span>' if current in GROUP else ''
    return (f'<div class="cl-page" role="dialog" aria-label="Settings"><div class="cl-page__inner{cls}">'
            f'{nav(current if state != "is-list" else None, role)}<div class="cl-page__body">{head}{back}<div class="cl-page__scroll">'
            f'<div class="cl-page__pagehead">{eyebrow}<h2 class="cl-page__pagetitle">{TITLE[current]}</h2><p class="cl-page__lede">{lede}</p></div>'
            f'{body}</div></div></div></div>')
