"""The settings, in pages (D86, 28/10): the reference screens built by ecrans.py.

A grouped navigation (This browser, Station, Administration), one page at a
time, a page widened to 1 080 px whose fields keep their reading width, lists
of cards in two columns; on a phone, the list of pages, then a page with
"‹ Settings". Only cl-* classes; inline style only for an illustration (the
usage bars). The fields are the page's own (www/index.html, 28/10), moved and
renamed as D86 decided; the sample values are made up.
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
}
TITLE = {k: t for _, items in PAGES for k, t in items}
MARKS = {'server': ('is-warn', '▲', 'Needs a look: the archive disk is missing')}


def nav(current, role):
    groups = PAGES if role == 'admin' else PAGES[:2]
    out = []
    for name, items in groups:
        links = ''
        for key, title in items:
            if role != 'admin' and key == 'matching':
                continue
            mark = ''
            if role == 'admin' and key in MARKS:
                cls, glyph, why = MARKS[key]
                mark = f'<span class="cl-page__mark {cls}" title="{why}">{glyph}</span>'
            links += f'<a href="#" aria-current="{str(key == current).lower()}">{title}{mark}</a>'
        out.append(f'<div class="cl-page__group"><span class="cl-label">{name}</span>{links}</div>')
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


def page_server():
    log = field('Log detail', '<select class="cl-select" aria-label="Log detail"><option>info</option></select>', 'How much the server writes to its log.')
    keep = field('Keep databases up to', '<span class="cl-row"><input class="cl-input" style="width: 72px;" value="6" aria-label="Gigabytes"><span class="cl-age">GB</span></span>',
                 'The day’s database and the days before; a day past this size goes to the archive below.')
    arch = field('Archive old databases to', '<input class="cl-input" style="width: 300px;" value="/Volumes/Archive/clairance/bases" aria-label="Archive folder">',
                 '<span class="cl-light cl-light--warn">Not found: the disk is not mounted. Nothing is deleted until it is back.</span>', text=True)
    kvcard = lambda title, rows: (f'<div class="cl-card"><span class="cl-label">{title}</span><dl class="cl-kv">'
                                  + ''.join(f'<dt>{k}</dt><dd>{v}</dd>' for k, v in rows) + '</dl></div>')
    reports = (kvcard('Storage', [('Database', 'co-atc-2026-10-28.db · 412 MB'), ('Growing', '33 MB/h'), ('Disk free', '41 GB'), ('Disk full in', '52 d')])
               + kvcard('Server', [('Uptime', '1 d 3 h'), ('Transcription', 'ok'), ('Log', '100 MB × 5 files')])
               + kvcard('ADS-B source', [('Mode', 'tar1090'), ('Status', '● connected'), ('Aircraft feed', 'available'), ('Receiver', 'available'), ('Stats', 'available'), ('Updated', '1 s ago')])
               + kvcard('Receiver and decoder', [('Receiver', 'range 210 NM · 107 aircraft'), ('Decoder', 'readsb · 700 msg/s')]))
    return section('', log + keep + arch) + section('Reports', f'<div class="cl-grid2">{reports}</div>', wide=True)


BODY = {'display': page_display, 'account': page_account, 'matching': page_matching, 'access': page_access, 'usage': page_usage, 'server': page_server}


def settings_page(current='listening', role='admin', state=''):
    """One markup for the computer and both phone states: state '' (computer), 'is-list' or 'is-detail' (phone)."""
    body = page_listening(role) if current == 'listening' else BODY[current]()
    lede = LEDE['listening-viewer'] if (current == 'listening' and role != 'admin') else LEDE[current]
    server_page = current in ('listening', 'matching', 'access', 'server') and role == 'admin'
    reload_ = '<button class="cl-btn cl-btn--quiet">Reload from the server</button>' if server_page else ''
    saved = '<span class="cl-light cl-light--ok">Saved</span>' if (role == 'admin' or current == 'display') else ''
    head = (f'<div class="cl-page__head"><h1 class="cl-page__title">Settings</h1>{saved}'
            f'<span class="cl-row" style="margin-left: auto;">{reload_}<button class="cl-btn cl-btn--icon" aria-label="Close">{CLOSE}</button></span></div>')
    back = '<div class="cl-page__back"><button class="cl-btn cl-btn--icon cl-mono" aria-label="Back to Settings">‹</button><span class="cl-plain">Settings</span></div>'
    cls = f' {state}' if state else ''
    return (f'<div class="cl-page" role="dialog" aria-label="Settings"><div class="cl-page__inner{cls}">'
            f'{nav(current if state != "is-list" else None, role)}<div class="cl-page__body">{head}{back}<div class="cl-page__scroll">'
            f'<div class="cl-page__pagehead"><h2 class="cl-page__pagetitle">{TITLE[current]}</h2><p class="cl-page__lede">{lede}</p></div>'
            f'{body}</div></div></div></div>')
