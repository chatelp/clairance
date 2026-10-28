// serverSettings(), the x-data of the settings page. Moved from app.js as it
// was (docs-fr/32, palier 5.1): a global function, which index.html calls by
// name. Since 28/10 (D86) the settings are pages, one shown at a time, in
// three groups; it carries the page shown, the reader's role, the folded
// parts, the light in its head, and the list-then-page state on a phone.
// ---------------------------------------------------------------- server panel
//
// The Server section of the settings panel. Everything else in that panel is a
// browser preference kept in localStorage; this one is the server's own state,
// and the two were worth separating -- one follows you between machines, the
// other decides when data is deleted.
//
// Writable while the server runs: the log level, the database retention, the
// airports followed, the callsign matching rules and the station's console on
// the sign-in page. The rest is reported and not editable, because changing it
// means reopening the log file or the audio pipeline. A panel that silently
// fails to apply half of what it shows is worse than one that says which half.
// The three matching presets (D73, 12/10), measured on the 03 and 04/10
// databases against shuffled skies: 'measured' is the default in force since
// 26/09 (95% precision); 'cautious' asks for the exact flight number (97%,
// 27% fewer matches); 'experimental' adds the two rules the owner has yet to
// judge by ear (+25% matches, 95% by the shuffle, which cannot see an airline
// named alone). Sector sizes are not part of a preset.
const MATCHING_PRESETS = (() => {
    const measured = { letters: true, approx_operators: true, min_digits: 3, one_digit_off: false,
        context_letters: true, context_digits: true, context_names: false, prefix_digits: false,
        sectors: true, min_score: 0 };
    return {
        measured,
        cautious: { ...measured, min_score: 0.9 },
        experimental: { ...measured, context_names: true, prefix_digits: true },
    };
})();

// The presets as the page offers them: name, label, and the bubble that says
// what each measured.
const MATCHING_PRESET_CHOICES = [
    ['measured', 'Standard', 'The rules in force since 26/09, each measured against shuffled skies: letters in flight numbers, the airline heard roughly, three digits at least, the last letters and the last two digits of an aircraft just heard, the frequency\'s sector. Measured on 03 and 04/10: 2 557 and 1 699 matches, 95 % right. The default.'],
    ['cautious', 'Cautious', 'Standard, plus the exact flight number required (score 0.9): a number heard in part, or an abbreviation alone, no longer names an aircraft. Measured on 03 and 04/10: 97 % right, for 27 % fewer matches. For when a wrong aircraft costs more than a missing one.'],
    ['experimental', 'Experimental', 'Standard, plus the two rules the shuffled sky cannot judge: an airline named alone when one aircraft of it was just heard, and the first digits after the airline\'s name. Measured on 03 and 04/10: 25 % more matches, 95 % right by the shuffle -- but two aircraft of one airline on the frequency can be confused, which only an ear can tell. To try, and judge.'],
];

// The settings' pages (D86, 28/10; docs-fr/design, SettingsPage): their group,
// their name, the sentence under it, whether a viewer sees it (read only), and
// whether it holds the server's settings (the light and "Reload" in the head).
// Each has its glyph, a tab's over its label from 768 px (owner, 28/10).
const SETTINGS_PAGES = [
    { id: 'display', group: 'This browser', label: 'Display', lede: 'How this browser draws the screen. Kept in this browser only.', viewer: true },
    { id: 'account', group: 'This browser', label: 'Account', lede: 'Who is signed in on this browser.', viewer: true },
    { id: 'listening', group: 'Station', label: 'Listening', lede: 'What the station listens to, and the official names it reads.', viewer: true, server: true },
    { id: 'matching', group: 'Station', label: 'Matching', lede: 'How a transmission is tied to the aircraft that spoke.', server: true },
    { id: 'access', group: 'Administration', label: 'Access', lede: 'Who may open Clairance, and with which account.', server: true },
    { id: 'usage', group: 'Administration', label: 'Usage', lede: 'What each account did these 7 days. Seen by admins only.' },
    { id: 'server', group: 'Administration', label: 'Server', lede: "The server's log, its databases and their archive, then what it reports.", server: true },
    // Below the groups, for everyone (D87).
    { id: 'about', group: '', label: 'About Clairance', lede: 'The version of this page and of the server, and what changed.', viewer: true },
];

// The pages a reader sees, in their groups: all for an admin; This browser and
// Listening for a viewer. Pure, for the tests.
function settingsNav(admin) {
    const groups = [];
    for (const p of SETTINGS_PAGES) {
        if (!admin && !p.viewer) continue;
        let g = groups.find((x) => x.name === p.group);
        if (!g) groups.push(g = { name: p.group, pages: [] });
        g.pages.push(p);
    }
    return groups;
}

// The aerodromes offered under "Also follow", in two lists: shown, the
// followed ones (always, however far) then the nearest others, n in all,
// by distance; more, the rest, folded, by distance. Equal distances keep
// the order of the codes. Pure, for the tests.
function alsoSplit(choices, followed, n) {
    const on = new Set(followed || []);
    const sorted = (choices || []).slice().sort((a, b) =>
        (a.distance_nm || 0) - (b.distance_nm || 0) || (a.code < b.code ? -1 : a.code > b.code ? 1 : 0));
    let room = n - sorted.filter((c) => on.has(c.code)).length;
    const shown = [];
    const more = [];
    for (const c of sorted) {
        if (on.has(c.code)) shown.push(c);
        else if (room-- > 0) shown.push(c);
        else more.push(c);
    }
    return { shown, more };
}

// The line of "Also follow" during a visit to Listening: the codes fixed at
// its opening (alsoSplit's shown then), plus any ticked since. An aerodrome
// unticked stays where it is until the next opening -- it does not leave
// under the pointer (design, 28/10); the line may hold more than five
// meanwhile. Pure, for the tests.
function alsoLines(choices, followed, line) {
    const keep = new Set([...(line || []), ...(followed || [])]);
    const sorted = (choices || []).slice().sort((a, b) =>
        (a.distance_nm || 0) - (b.distance_nm || 0) || (a.code < b.code ? -1 : a.code > b.code ? 1 : 0));
    return { shown: sorted.filter((c) => keep.has(c.code)), more: sorted.filter((c) => !keep.has(c.code)) };
}

// The folder picker (FolderPicker, 28/10): the server's folders, browsed in
// the page for the archive -- a page cannot open the system's folder window
// for another machine. Pure, for the tests.

// Each part of a path, as a button back up to it: "/" first.
function folderCrumbs(path) {
    const parts = String(path || '').split('/').filter(Boolean);
    return [{ name: '/', path: '/' }].concat(parts.map((name, i) => ({ name, path: '/' + parts.slice(0, i + 1).join('/') })));
}

// The place that holds a folder: the longest root its path starts with.
function folderPlace(roots, path) {
    let best = null;
    for (const r of roots || []) {
        if ((path === r.path || path.startsWith(r.path.replace(/\/$/, '') + '/')) && (!best || r.path.length > best.path.length)) best = r;
    }
    return best;
}

// What choosing the open folder means, from the server's word on it.
function folderFoot(here, place, fmtBytes) {
    if (!here) return null;
    if (here.why === 'databases') return { warn: true, text: 'The databases are kept here: choose a folder outside, best on another disk.' };
    if (!here.choosable) return { warn: true, text: 'The server cannot write in this folder: choose another.' };
    if (place && place.free_bytes) return { warn: false, text: fmtBytes(place.free_bytes) + ' free on ' + place.name + '.' };
    return { warn: false, text: '' };
}

// The row a key moves to, from row i of n; -1 for a key that does not move.
function folderStep(key, i, n) {
    if (!n) return -1;
    if (key === 'ArrowDown') return Math.min(n - 1, i + 1);
    if (key === 'ArrowUp') return Math.max(0, i - 1);
    if (key === 'Home') return 0;
    if (key === 'End') return n - 1;
    return -1;
}

function serverSettings() {
    return {
        presets: MATCHING_PRESETS,
        presetChoices: MATCHING_PRESET_CHOICES,

        // The page shown; on a phone, whether it is shown (is-detail) or the
        // list of pages (is-list).
        page: 'listening',
        phoneDetail: false,
        phone: false,
        // Who reads: /api/v1/auth/status, read once. The server stays the
        // authority (403); this only spares a viewer the controls it cannot use.
        me: null,
        signingOut: false,

        // How sure a procedure's name must be (Q55): only measured settings,
        // with what each gave on the owner's listening of 77 cases.
        namingChoices: [['surer', 'Surer', '9 names, all right'], ['measured', 'Standard', '18 names, all right'], ['more', 'More', '19 names, 18 right']],
        rulesOpen: false,
        debugOpen: false,
        alsoMoreOpen: false,
        alsoLine: null, // the codes of the line, fixed when Listening opens

        // The role, the phone's width, and the server's state read at the
        // first opening rather than with the page: the settings are closed
        // most of the time.
        init() {
            fetch('/api/v1/auth/status', { credentials: 'same-origin', cache: 'no-store' })
                .then((r) => (r.ok ? r.json() : null))
                .then((s) => { if (s) this.me = s; })
                .catch(() => {});
            const narrow = window.matchMedia('(max-width: 767px)');
            this.phone = narrow.matches;
            narrow.addEventListener('change', (e) => { this.phone = e.matches; });
            const opened = () => {
                this.phoneDetail = false;
                this.alsoLine = null;
                if (!this.state && !this.busy) this.load();
                // The focus on the page's tab.
                // The focus on the page's tab, once the window shows.
                setTimeout(() => this.$root.querySelector('.cl-page__nav a[aria-current="true"]')?.focus(), 60);
            };
            this.$watch('$store.atc.settingsOpen', (open) => { if (open) opened(); });
            if (Alpine.store('atc').settingsOpen) opened();
        },

        // An admin, or a server without sign-in (where everyone is the owner).
        get admin() {
            return !this.me || this.me.role !== 'viewer';
        },

        roleLabel() {
            return this.admin ? 'Admin' : 'Viewer';
        },

        navGroups() {
            return settingsNav(this.admin);
        },

        pageInfo() {
            return SETTINGS_PAGES.find((p) => p.id === this.page) || SETTINGS_PAGES[0];
        },

        // A viewer reads Listening as text: its sentence says who sets it.
        pageLede() {
            const p = this.pageInfo();
            return p.lede + (!this.admin && p.server ? ' Set by an admin.' : '');
        },

        serverPage() {
            return !!this.pageInfo().server;
        },

        open(id) {
            if (id === 'listening') this.alsoLine = null; // fixed again at each opening
            this.picker.open = false;
            this.page = id;
            this.phoneDetail = true;
            if (this.$refs.scroll) this.$refs.scroll.scrollTop = 0;
        },

        glyph(id) {
            return SETTINGS_GLYPHS[id] || '';
        },

        // Closing keeps what was typed: the field being edited is left first,
        // so it saves as with Tab (and a lower size asks first), and the window
        // is hidden, never emptied. Focus goes back to the gear.
        close() {
            const el = document.activeElement;
            if (el && this.$root.contains(el) && el.blur) el.blur();
            Alpine.store('atc').settingsOpen = false;
            this.$nextTick(() => document.querySelector('button[aria-label="Settings"]')?.focus());
        },

        // A field being typed into, in the window.
        typing() {
            const el = document.activeElement;
            return !!(el && this.$root.contains(el) && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT'));
        },

        // The veil closes the window only for a press and a release both on
        // it, and nothing being typed; otherwise the focus goes back to the field.
        // Read at the press: the browser takes the focus away from the field
        // at the press already, and the release would find nothing typed.
        scrimDown: false,
        typedIn: null,
        veilDown(e) {
            this.scrimDown = e.target === e.currentTarget;
            this.typedIn = this.scrimDown && this.typing() ? document.activeElement : null;
            if (this.typedIn) e.preventDefault(); // the field keeps the focus
        },
        veilUp(e) {
            const both = this.scrimDown && e.target === e.currentTarget;
            const field = this.typedIn;
            this.scrimDown = false;
            this.typedIn = null;
            if (!both) return;
            if (field) { field.focus(); return; }
            this.close();
        },

        // Tab stays in the window (aria-modal).
        trap(e) {
            const items = [...this.$root.querySelectorAll('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea, [tabindex="0"]')]
                .filter((el) => el.offsetParent !== null);
            if (!items.length) return;
            const first = items[0], last = items[items.length - 1];
            if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
            else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
        },

        phoneState() {
            if (!this.phone) return '';
            return this.phoneDetail ? 'is-detail' : 'is-list';
        },

        // A page that needs a look, for an admin: its mark in the navigation,
        // the status lights' shape, never a colour alone.
        mark(id) {
            const notices = Alpine.store('notices');
            if (id === 'about' && notices && notices.updateAvailable) {
                return { cls: 'is-warn', glyph: '▲', why: 'Needs a look: this page is older than the server' };
            }
            if (!this.admin || !this.state) return null;
            if (id === 'server' && this.archiveWarn()) {
                const a = this.state.storage && this.state.storage.archive;
                return { cls: 'is-warn', glyph: '▲', why: a && a.set ? 'Needs a look: the archive cannot be found' : 'Needs a look: no archive, the databases past the size are deleted' };
            }
            // The mark follows the page's lights: ▲ for --warn, ■ for --bad,
            // nothing for --off -- a list not made is a choice, not a fault.
            if (id === 'listening' && this.aip) {
                const level = this.aipLight().level;
                if (level === 'warn') return { cls: 'is-warn', glyph: '▲', why: 'Needs a look: the official names are out of date' };
                if (level === 'bad') return { cls: 'is-bad', glyph: '■', why: 'Needs a look: the last update of the official names failed' };
            }
            return null;
        },

        // The reference airport as a viewer reads it.
        referenceText() {
            const c = this.airportChoices().find((x) => x.code === this.draft.reference_airport);
            return c ? c.code + ' · ' + c.name + ' · ' + c.distance_nm + ' NM' : (this.draft.reference_airport || '—');
        },

        // The light in the page's head: what the last load or save did.
        statusLight() {
            if (this.error) return { level: 'cl-light--bad', text: this.error };
            if (this.busy) return { level: 'cl-light--switching', text: this.loading ? 'Loading…' : 'Saving…' };
            if (this.note) return { level: this.noteBad ? 'cl-light--bad' : 'cl-light--ok', text: this.note.replace(/\.$/, '') };
            if (this.state) return { level: 'cl-light--ok', text: 'Saved' };
            return { level: '', text: '' };
        },

        // The channels the station plays, as one sentence: name and frequency.
        onAir() {
            const atc = Alpine.store('atc');
            const list = (atc && atc.audioFrequencies) || [];
            if (!list.length) return 'No frequency connected.';
            return list.map((f) => {
                const mhz = Number(f.frequency_mhz) > 0 ? ' ' + Number(f.frequency_mhz).toFixed(3) : '';
                return atc.freqShortName(f) + mhz;
            }).join(' · ');
        },

        // A rule switch: flip it, then save as its checkbox did.
        flipRule(key) {
            this.draft.matching[key] = !this.draft.matching[key];
            this.save();
        },

        // The preset the rules match, or null when they match none.
        activePreset() {
            const m = this.draft.matching;
            if (!m) return null;
            for (const [name, preset] of Object.entries(MATCHING_PRESETS)) {
                if (Object.keys(preset).every((k) => (m[k] ?? (k === 'min_score' ? 0 : undefined)) === preset[k])) return name;
            }
            return null;
        },

        applyPreset(name) {
            const preset = MATCHING_PRESETS[name];
            if (!preset || !this.draft.matching) return;
            Object.assign(this.draft.matching, preset);
            this.save();
        },

        state: null,
        draft: { log_level: 'info', db_retention_gb: 20, db_archive_dir: '', reference_airport: '', also_airports: [], matching: null, vitals: true },
        newAccount: null, // { username, password } while the first account is being typed
        busy: false,
        loading: false,
        error: '',
        note: '',
        noteBad: false,

        async load() {
            this.busy = true;
            this.loading = true;
            this.error = '';
            try {
                const r = await fetch('/api/v1/server', { credentials: 'same-origin', cache: 'no-store' });
                if (r.status === 401) { this.error = 'Sign in to see the server state.'; return; }
                if (!r.ok) { this.error = 'Server state unavailable (' + r.status + ')'; return; }
                this.state = await r.json();
                this.draft = {
                    log_level: this.state.settings.log_level,
                    db_retention_gb: this.state.settings.db_retention_gb,
                    db_archive_dir: this.archiveDirOf(this.state.settings),
                    reference_airport: this.state.settings.reference_airport,
                    also_airports: (this.state.settings.also_airports || []).slice(),
                    // A deep copy: the sector sizes are edited in place by the inputs.
                    matching: this.state.settings.matching ? JSON.parse(JSON.stringify(this.state.settings.matching)) : null,
                    vitals: this.vitalsOn(this.state.settings),
                };
                this.loadAIP();
            } catch (e) {
                this.error = 'Server state unreachable: ' + e.message;
            } finally {
                this.busy = false;
                this.loading = false;
            }
        },

        async save() {
            // Retention decides when data is deleted, so the panel says what it
            // is about to do rather than doing it silently.
            const gb = Number(this.draft.db_retention_gb);
            if (!Number.isFinite(gb) || gb < 1 || gb > 1000) {
                this.flash('The size kept must be between 1 and 1000 GB.', true);
                return;
            }
            if (this.state && gb < this.state.settings.db_retention_gb) {
                // What the server will do: newest first, today's kept whatever
                // its size, and from the first day that does not fit, it and
                // every older one deleted.
                const files = this.state.storage.daily_files || [];
                const cap = gb * 1024 * 1024 * 1024;
                let total = files.filter(f => f.active).reduce((s, f) => s + f.bytes, 0);
                let doomed = 0, over = false;
                for (const f of files.filter(f => !f.active)) {
                    if (!over && total + f.bytes <= cap) { total += f.bytes; continue; }
                    over = true;
                    doomed++;
                }
                if (doomed > 0 && !confirm(
                    'Keeping ' + gb + ' GB will delete ' + doomed +
                    ' older database file(s) at the next hourly sweep. Continue?')) {
                    this.draft.db_retention_gb = this.state.settings.db_retention_gb;
                    return;
                }
            }

            // Turning the archive off means the days past the size are deleted
            // from then on: said before it is done (D85).
            const archiveDir = (this.draft.db_archive_dir || '').trim();
            if (this.state && archiveDir === '' && this.archiveDirOf(this.state.settings) !== '' &&
                !confirm('Without an archive, the databases past ' + gb + ' GB will be deleted, not archived. Continue?')) {
                this.draft.db_archive_dir = this.archiveDirOf(this.state.settings);
                return;
            }

            this.busy = true;
            try {
                const r = await fetch('/api/v1/server/settings', {
                    method: 'PUT',
                    credentials: 'same-origin',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        log_level: this.draft.log_level,
                        db_retention_gb: gb,
                        db_archive: { dir: archiveDir },
                        reference_airport: this.draft.reference_airport,
                        // The reference airport cannot also be a further one.
                        also_airports: this.draft.also_airports.filter(c => c !== this.draft.reference_airport),
                        matching: this.draft.matching,
                        vitals: { enabled: !!this.draft.vitals },
                    }),
                });
                if (r.status === 401) { this.flash('Sign in to change server settings.', true); return; }
                if (!r.ok) {
                    this.flash((await r.text()).trim() || ('Refused (' + r.status + ')'), true);
                    // A refused airport must not stay selected as if it were in force,
                    // nor a refused matching rule stay ticked.
                    this.draft.reference_airport = this.state.settings.reference_airport;
                    this.draft.also_airports = (this.state.settings.also_airports || []).slice();
                    if (this.state.settings.matching) this.draft.matching = JSON.parse(JSON.stringify(this.state.settings.matching));
                    this.draft.vitals = this.vitalsOn(this.state.settings);
                    this.draft.db_archive_dir = this.archiveDirOf(this.state.settings);
                    return;
                }
                const followed = s => [s.reference_airport].concat(s.also_airports || []).join('+');
                const airportChanged = followed(this.state.settings) !== followed(this.draft);
                this.state.settings = await r.json();
                this.draft.also_airports = (this.state.settings.also_airports || []).slice();
                this.flash('Saved.', false);
                if (airportChanged) {
                    // The map draws the reference airport's runway extensions from
                    // /station; redraw them now. Weather is fetched by the server in
                    // the background after the change, so read it a little later.
                    const atc = Alpine.store('atc');
                    if (atc && atc.fetchStationData) atc.fetchStationData();
                    if (atc && atc.fetchWeatherData) setTimeout(() => atc.fetchWeatherData(), 4000);
                }
                // Reread rather than trust: the figures downstream -- retention,
                // days until full -- move with what was just changed.
                await this.load();
            } catch (e) {
                this.flash('Could not save: ' + e.message, true);
            } finally {
                this.busy = false;
            }
        },

        // The list of official names around the station (Q55; SettingsPage):
        // its state, the button that makes it again from the SIA, and the
        // switch that does so at each new AIRAC cycle.
        aip: null,
        aipBusy: false,

        async loadAIP() {
            try {
                const r = await fetch('/api/v1/aip-names', { credentials: 'same-origin' });
                if (r.ok) this.aip = await r.json();
            } catch (e) { /* the state stays as it was */ }
        },

        // "14:20Z" from an ISO time, on the page's clock.
        aipClock(iso) {
            const atc = Alpine.store('atc');
            return iso && atc && atc.clockText ? atc.clockText(new Date(iso)) : '';
        },

        // The StatusLight's five states.
        aipLight() {
            const a = this.aip;
            if (!a) return { level: 'unknown', text: '' };
            if (a.coverage === 'none') {
                return { level: 'off', text: `Not for this station: the French eAIP covers aerodromes in France (LF…); this station follows ${(a.airports || []).join(', ')}.` };
            }
            if (a.refreshing || this.aipBusy) return { level: 'switching', text: 'Updating from the SIA…' };
            const failedSince = a.last_error && (!a.made || (a.last_tried && a.last_tried > a.made));
            if (failedSince) {
                return { level: 'bad', text: `Last update failed at ${this.aipClock(a.last_tried)}: ${a.last_error}` +
                    (a.present ? ` (the list of ${a.airac} stays in use)` : '') };
            }
            if (!a.present) return { level: 'off', text: 'Not made yet' };
            if (a.airac < a.in_force) return { level: 'warn', text: `AIRAC ${a.in_force} in force; this list is of ${a.airac}` };
            return { level: 'ok', text: `Up to date · AIRAC ${a.airac}` };
        },

        get aipStale() {
            return !!this.aip && (!this.aip.present || this.aip.airac < this.aip.in_force);
        },

        // "15/10 at 14:20Z".
        aipMadeText() {
            const a = this.aip;
            if (!a || !a.made) return '';
            const d = new Date(a.made);
            return `${String(d.getUTCDate()).padStart(2, '0')}/${String(d.getUTCMonth() + 1).padStart(2, '0')} at ${this.aipClock(a.made)}`;
        },

        async refreshAIP() {
            this.aipBusy = true;
            try {
                const r = await fetch('/api/v1/aip-names/refresh', { method: 'POST', credentials: 'same-origin' });
                if (r.status === 401) { this.flash('Sign in to update the names.', true); return; }
                if (!r.ok && r.status !== 409) { this.flash((await r.text()).trim() || ('Refused (' + r.status + ')'), true); return; }
                // A few seconds: the five pages, then the list.
                for (let i = 0; i < 180; i++) {
                    await new Promise((res) => setTimeout(res, 1000));
                    await this.loadAIP();
                    if (this.aip && !this.aip.refreshing) break;
                }
                // The map's published routes come from the same list (Q56).
                window.Alpine?.store('atc')?.loadProcedureRoutes?.();
            } catch (e) {
                this.flash('Could not update: ' + e.message, true);
            } finally {
                this.aipBusy = false;
            }
        },

        // The line the SIA asks to cite, and the aerodromes it does not cover.
        aipSourceLine() {
            const a = this.aip;
            if (!a) return '';
            let line = `Source: ${a.source}. Made on ${this.aipMadeText()}. Next cycle: ${a.next_airac}.`;
            if ((a.outside || []).length) line += ` Not read: ${a.outside.join(', ')}, outside the French eAIP.`;
            if ((a.skipped || []).length) line += ` Not read: ${a.skipped.join('; ')}.`;
            return line;
        },

        // The list's three settings travel together: one sent alone would
        // reset the others.
        async setAIP(change) {
            const a = this.aip || {};
            const next = Object.assign({ auto_update: !!a.auto_update, name_procedures: a.name_procedures !== false, naming: a.naming || 'measured' }, change);
            try {
                const r = await fetch('/api/v1/server/settings', {
                    method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ aip_names: next }),
                });
                if (r.status === 401) { this.flash('Sign in to change server settings.', true); return; }
                if (!r.ok) { this.flash((await r.text()).trim() || ('Refused (' + r.status + ')'), true); return; }
                this.flash('Saved.', false);
            } catch (e) {
                this.flash('Could not save: ' + e.message, true);
            }
            await this.loadAIP();
        },

        // The sign-in page's station console: absent from a settings file
        // saved before it existed, which means on (config.DefaultVitals).
        vitalsOn(settings) {
            return !settings.vitals || settings.vitals.enabled !== false;
        },

        accessLabel() {
            const a = this.state.access;
            if (a.mode === 'account' && !a.local_allowed) {
                return 'Sign-in required · ' + a.accounts.join(', ') + '. It stays on because the server is not bound to 127.0.0.1, or a proxy is declared.';
            }
            if (a.mode === 'account') return 'Sign-in required · ' + a.accounts.join(', ');
            if (a.mode === 'local') return 'This machine only · no sign-in';
            return 'Not chosen yet';
        },

        accessNote() {
            const a = this.state.access;
            if (a.mode === 'account' && !a.local_allowed) {
                return 'This server can be reached from other machines, so sign-in stays on.';
            }
            if (a.mode === 'account') return 'Switching keeps the account, unused, for when sign-in is required again.';
            if (a.accounts.length) return 'Kept, unused: ' + a.accounts.join(', ') + '. Requiring sign-in brings it back with its password.';
            return 'Requiring sign-in creates the first account.';
        },

        // Either way, and as often as wanted. Turning sign-in off asks first:
        // it opens the server to anyone who can reach it.
        async switchAccess(mode) {
            const a = this.state.access;
            if (mode === 'local') {
                if (!confirm('Anyone who can reach this server will use it without signing in. ' +
                    'The account ' + a.accounts.join(', ') + ' is kept, unused, and comes back if you ' +
                    'require sign-in again. Continue?')) return;
                return this.putAccess({ mode: 'local' });
            }
            if (!a.accounts.length) {
                this.newAccount = { username: '', password: '' };
                return;
            }
            if (!confirm('Sign-in will be required, with the account ' + a.accounts.join(', ') +
                ' and its existing password. Continue?')) return;
            return this.putAccess({ mode: 'account' });
        },

        async createAccount() {
            const { username, password } = this.newAccount;
            if (!username.trim()) { this.flash('Choose a user name.', true); return; }
            if (password.length < 10) { this.flash('The password must be at least 10 characters.', true); return; }
            return this.putAccess({ mode: 'account', username: username.trim(), password });
        },

        async putAccess(body) {
            this.busy = true;
            try {
                const r = await fetch('/api/v1/access', {
                    method: 'PUT',
                    credentials: 'same-origin',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                if (r.status === 401) { this.flash('Sign in to change access.', true); return; }
                if (!r.ok) { this.flash((await r.text()).trim() || ('Refused (' + r.status + ')'), true); return; }
                // What this browser may see has just changed. Reloading lets the
                // page start over: with the sign-in form if sign-in is now required.
                location.reload();
            } catch (e) {
                this.flash('Could not change access: ' + e.message, true);
            } finally {
                this.busy = false;
            }
        },

        // The airports offered as reference, nearest first -- plus the one in
        // force if it is not among them (set by hand farther than the range).
        airportChoices() {
            const ref = this.state && this.state.reference;
            if (!ref) return [];
            const list = (ref.candidates || []).slice();
            if (ref.airport && !list.some(c => c.code === ref.airport)) {
                list.unshift({ code: ref.airport, name: ref.name || '', distance_nm: ref.distance_nm || 0 });
            }
            // The short name, as everywhere else ("Paris-Orly", not "Paris-Orly Airport").
            const R = window.RightNow;
            return list.map((c) => (R ? Object.assign({}, c, { name: R.shortName({ name: c.name, code: c.code }) }) : c));
        },

        // Airports followed besides the reference one (Q40): each aircraft is
        // judged against the one whose runway axis it flies, else the nearest.
        alsoChoices() {
            return this.airportChoices().filter(c => c.code !== this.draft.reference_airport);
        },
        // Shown and folded: five on the line at the opening (alsoSplit), the
        // ones ticked since added, none unticked taken away (alsoLines).
        alsoLists() {
            const choices = this.alsoChoices();
            if (!this.alsoLine && choices.length) {
                this.alsoLine = alsoSplit(choices, this.draft.also_airports, 5).shown.map((c) => c.code);
            }
            return alsoLines(choices, this.draft.also_airports, this.alsoLine);
        },
        // Ticked from the fold, a box moves up to the line: the keyboard
        // follows it there. The root is read before the render, which
        // detaches the box it was ticked from.
        keepAlsoFocus(code) {
            const root = this.$root;
            this.$nextTick(() => root.querySelector('[data-also="' + code + '"]')?.focus());
        },
        alsoFollowed(code) {
            return this.draft.also_airports.includes(code);
        },
        toggleAlso(code) {
            const list = this.draft.also_airports.filter(c => c !== code);
            if (list.length === this.draft.also_airports.length) {
                if (list.length >= 3) { this.flash('Three further airports at most.', true); return; }
                list.push(code);
                // Ticked during the visit, it joins the line for good: unticked
                // again, it stays there until Listening opens again.
                if (this.alsoLine && !this.alsoLine.includes(code)) this.alsoLine = [...this.alsoLine, code];
            }
            this.draft.also_airports = list;
            this.save();
        },

        flash(msg, bad) {
            this.note = msg;
            this.noteBad = bad;
            setTimeout(() => { this.note = ''; }, bad ? 8000 : 3000);
        },

        // The folder picker under the archive's field: closed, or what the
        // server said of the open folder. The list is replaced only once the
        // next one is read; "Reading..." shows only if that takes a while.
        picker: { open: false, loading: false, slow: false, error: '', path: '', here: null, folders: [], truncated: false, roots: [] },

        pickerToggle() {
            if (this.picker.open) { this.pickerFold(); return; }
            this.picker.open = true;
            // At the field's folder, or the nearest above it that can be read;
            // with none, where the server starts (the archive in force, the disks).
            const typed = (this.draft.db_archive_dir || '').trim();
            this.pickerGo(typed.startsWith('/') ? typed : '', true);
        },

        async pickerGo(path, nearest, from) {
            const p = this.picker;
            p.loading = true;
            const slow = setTimeout(() => { if (p.loading) p.slow = true; }, 250);
            try {
                const q = new URLSearchParams();
                if (path) q.set('path', path);
                if (nearest) q.set('nearest', '1');
                const r = await fetch('/api/v1/server/folders?' + q, { credentials: 'same-origin', cache: 'no-store' });
                if (!p.open) return;
                if (!r.ok) {
                    // Places and path stay, to go elsewhere.
                    p.error = r.status === 404 ? 'This folder cannot be read: is its disk plugged in?' : 'The server refused (' + r.status + ').';
                    p.path = path || p.path;
                    p.here = null;
                    p.folders = [];
                    p.truncated = false;
                } else {
                    const b = await r.json();
                    Object.assign(p, { error: '', path: b.path, here: b.here, folders: b.folders || [], truncated: !!b.truncated, roots: b.roots || p.roots });
                }
            } catch (e) {
                p.error = 'The server cannot be reached.';
            } finally {
                clearTimeout(slow);
                p.loading = false;
                p.slow = false;
            }
            // The focus on the first row, on the folder just left when going
            // up, or on the path's last part when there is no row.
            this.$nextTick(() => {
                const rows = [...this.$root.querySelectorAll('.cl-folder__row')];
                const back = from && rows.find((el) => el.dataset.path === from);
                (back || rows[0] || this.$root.querySelector('.cl-folder__crumbs [aria-current="location"]'))?.focus();
            });
        },

        pickerCrumbs() { return folderCrumbs(this.picker.path); },
        pickerPlace() { return folderPlace(this.picker.roots, this.picker.path); },
        pickerFoot() { return folderFoot(this.picker.here, this.pickerPlace(), (n) => this.fmtBytes(n)); },
        pickerFree(root) { return root.name !== 'Home' && root.free_bytes ? '· ' + this.fmtBytes(root.free_bytes) + ' free' : ''; },

        // Folded, the focus back on Choose...: Escape, Cancel, or after choosing.
        pickerFold() {
            this.picker.open = false;
            this.picker.error = '';
            this.$nextTick(() => this.$refs.pickerButton?.focus());
        },

        // The open folder becomes the archive, saved as a typed path is: the
        // server checks it again, and the field's light says the result.
        pickerChoose() {
            const h = this.picker.here;
            if (!h || !h.choosable) return;
            this.draft.db_archive_dir = h.path;
            this.pickerFold();
            this.save();
        },

        pickerUp() {
            const crumbs = this.pickerCrumbs();
            if (crumbs.length > 1) this.pickerGo(crumbs[crumbs.length - 2].path, false, this.picker.path);
        },

        // On a row: up and down, Home and End, Enter or right to open (Enter is
        // the button's own), left or Backspace up one folder.
        pickerKey(e) {
            const rows = [...this.$root.querySelectorAll('.cl-folder__row')];
            const i = rows.indexOf(document.activeElement);
            const to = folderStep(e.key, i, rows.length);
            if (to >= 0) { e.preventDefault(); rows[to].focus(); rows[to].scrollIntoView({ block: 'nearest' }); return; }
            if (e.key === 'ArrowRight' && i >= 0) { e.preventDefault(); this.pickerGo(rows[i].dataset.path); return; }
            if (e.key === 'ArrowLeft' || e.key === 'Backspace') { e.preventDefault(); this.pickerUp(); }
        },

        // The database archive in force (D85), "" for none.
        archiveDirOf(settings) {
            return (settings && settings.db_archive && settings.db_archive.dir) || '';
        },

        // What the archive is now: none, not found, or what it holds. The first
        // two are warnings (archiveWarn): with none, what passes the size is
        // deleted for good; not found, this disk grows.
        archiveLine() {
            const a = this.state && this.state.storage && this.state.storage.archive;
            // set, not the path: a guest is told whether there is an archive,
            // never where it is.
            if (!a || !a.set) {
                return 'No archive: the databases past ' + this.state.settings.db_retention_gb + ' GB are deleted for good.';
            }
            if (!a.found) return 'Archive not found: nothing is deleted until it is back, and this disk keeps growing. Is its disk plugged in?';
            return a.files + (a.files === 1 ? ' day' : ' days') + ' archived, ' + this.fmtBytes(a.bytes) +
                ' · ' + this.fmtBytes(a.free_bytes) + ' free on its disk.';
        },

        archiveWarn() {
            const a = this.state && this.state.storage && this.state.storage.archive;
            return !a || !a.set || !a.found;
        },

        fmtBytes(n) {
            if (!n && n !== 0) return '—';
            const u = ['B', 'kB', 'MB', 'GB', 'TB'];
            let i = 0;
            while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
            return (i === 0 ? n : n.toFixed(1)) + ' ' + u[i];
        },

        fmtUptime(s) {
            if (!s && s !== 0) return '—';
            const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
            if (d) return d + ' d ' + h + ' h';
            if (h) return h + ' h ' + m + ' min'; // units, never "05": it reads as a clock
            if (m) return m + ' min';
            return 'under a minute';
        },
    };
}

// Each page's glyph: the paths of a 16 grid drawn at 20 px (docs-fr/design, SettingsPage).
const SETTINGS_GLYPHS = {
    display: '<rect x="2" y="2.75" width="12" height="8.5" rx="1"/><path d="M8 11.25v2.5M5.5 13.75h5"/>',
    account: '<circle cx="8" cy="5.25" r="2.75"/><path d="M2.75 14c.35-2.9 2.55-4.5 5.25-4.5s4.9 1.6 5.25 4.5"/>',
    listening: '<path d="M2.75 10.5V8a5.25 5.25 0 0 1 10.5 0v2.5"/><rect x="2" y="9.25" width="3" height="4.75" rx="1"/><rect x="11" y="9.25" width="3" height="4.75" rx="1"/>',
    matching: '<path d="M7 9.5 9.5 7"/><path d="M8.6 4.4 9.7 3.3a2.55 2.55 0 0 1 3.6 3.6l-1.1 1.1"/><path d="M7.4 11.6 6.3 12.7a2.55 2.55 0 0 1-3.6-3.6l1.1-1.1"/>',
    access: '<rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2M8 9.75v1.5"/>',
    usage: '<path d="M2.5 13.5h11M4.75 11V8.5M8 11V4M11.25 11V6.5"/>',
    server: '<rect x="2.5" y="2.5" width="11" height="4.5" rx="1"/><rect x="2.5" y="9" width="11" height="4.5" rx="1"/><path d="M5 4.75h.01M5 11.25h.01"/>',
    about: '<circle cx="8" cy="8" r="6"/><path d="M8 7.5v3.5M8 5h.01"/>',
};

if (typeof module !== 'undefined') module.exports = { SETTINGS_PAGES, SETTINGS_GLYPHS, settingsNav, alsoSplit, alsoLines, folderCrumbs, folderPlace, folderFoot, folderStep };
