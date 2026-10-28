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
                if (!this.state && !this.busy) this.load();
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
            this.page = id;
            this.phoneDetail = true;
            if (this.$refs.scroll) this.$refs.scroll.scrollTop = 0;
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
        alsoFollowed(code) {
            return this.draft.also_airports.includes(code);
        },
        toggleAlso(code) {
            const list = this.draft.also_airports.filter(c => c !== code);
            if (list.length === this.draft.also_airports.length) {
                if (list.length >= 3) { this.flash('Three further airports at most.', true); return; }
                list.push(code);
            }
            this.draft.also_airports = list;
            this.save();
        },

        flash(msg, bad) {
            this.note = msg;
            this.noteBad = bad;
            setTimeout(() => { this.note = ''; }, bad ? 8000 : 3000);
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

if (typeof module !== 'undefined') module.exports = { SETTINGS_PAGES, settingsNav };
