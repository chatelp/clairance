// The page's notices (D87, 28/10; docs-fr/design, Notices: Notice): what the
// page says on its own, without being asked and without blocking.
//
// - A new version to load: the server announces the page it serves (its
//   fingerprint, in /api/v1/version at load and in every WebSocket heartbeat);
//   when it is no longer the page this browser loaded, a card says so, with
//   Reload and Later. Never a reload by itself: a sound may be on. After Later,
//   a "New version" button stays in the top bar.
// - What's new: the lines of www/news.json newer than the last ones this
//   account saw in this browser, once.
//
// One notice at a time, none while the connection banner, the settings or
// the sign-in show.

// The lines an account has not seen: news newest first, and the id of the
// newest entry it saw. With none seen yet, the newest entry alone; with the
// newest seen, nothing; with an id the file no longer has, the newest alone.
// Four lines at most, the newest first. Pure, for the tests.
function newsUnseen(entries, seenId) {
    if (!Array.isArray(entries) || !entries.length) return null;
    let fresh;
    if (!seenId) fresh = entries.slice(0, 1);
    else {
        const i = entries.findIndex((e) => e.id === seenId);
        fresh = i < 0 ? entries.slice(0, 1) : entries.slice(0, i);
    }
    const lines = fresh.flatMap((e) => e.lines || []).slice(0, 4);
    return lines.length ? { date: fresh[0].date, lines, newest: entries[0].id } : null;
}

// "28 October", or "28 October 2026" with the year.
function newsDate(date, withYear) {
    const d = new Date(date + 'T12:00:00');
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleDateString('en-GB', withYear ? { day: 'numeric', month: 'long', year: 'numeric' } : { day: 'numeric', month: 'long' });
}

// "You are listening to 124.350 and 126.425: …", when a frequency plays.
function listeningSentence(mhzs) {
    if (!mhzs.length) return '';
    const list = mhzs.length === 1 ? mhzs[0] : mhzs.slice(0, -1).join(', ') + ' and ' + mhzs[mhzs.length - 1];
    return ' You are listening to ' + list + ': the sound stops for a moment, and a click starts it again.';
}

if (typeof document !== 'undefined') document.addEventListener('alpine:init', () => {
    Alpine.store('notices', {
        loaded: null, // what this page was given at load: { page, built, commit }
        server: null, // what the server says now
        laterFor: '', // the page version Later was said to
        news: [],
        unseen: null, // { date, lines, newest } to show once
        user: '',

        async init() {
            try {
                const [v, auth, news] = await Promise.all([
                    fetch('/api/v1/version', { credentials: 'same-origin', cache: 'no-store' }).then((r) => (r.ok ? r.json() : null)),
                    fetch('/api/v1/auth/status', { credentials: 'same-origin', cache: 'no-store' }).then((r) => (r.ok ? r.json() : null)),
                    fetch('news.json', { cache: 'no-store' }).then((r) => (r.ok ? r.json() : [])),
                ]);
                if (!v) return; // not signed in: the sign-in page has none of this
                this.loaded = v;
                this.server = v;
                this.news = Array.isArray(news) ? news : [];
                this.user = (auth && auth.user) || 'this-machine';
                this.unseen = newsUnseen(this.news, this.seen());
            } catch (e) { return; }
            // The heartbeat says which page the server serves now; a
            // reconnection reads it again at once. Checked every 2 s, and put
            // back when missing: the feed clears every listener of the client
            // when it starts it (store/feed.js), after this has run.
            const onBeat = (d) => { if (d && d.page) this.server = { ...this.server, ...d }; };
            const onOpen = () => this.refresh();
            const ensure = () => {
                const c = window.wsClient;
                if (!c || !c.listeners || !c.listeners.heartbeat || c.listeners.heartbeat.includes(onBeat)) return;
                c.addEventListener('heartbeat', onBeat);
                c.addEventListener('open', onOpen);
            };
            ensure();
            setInterval(ensure, 2000);
        },

        async refresh() {
            try {
                const v = await fetch('/api/v1/version', { credentials: 'same-origin', cache: 'no-store' }).then((r) => (r.ok ? r.json() : null));
                if (v) this.server = v;
            } catch (e) { /* the next heartbeat will say */ }
        },

        seenKey() { return 'clairance.news.seen.' + this.user; },
        seen() { try { return localStorage.getItem(this.seenKey()) || ''; } catch (e) { return ''; } },

        get updateAvailable() {
            return !!(this.loaded && this.server && this.server.page && this.server.page !== this.loaded.page);
        },

        // Nothing over the connection banner, the settings or the sign-in.
        free() {
            const atc = Alpine.store('atc');
            if (!atc || atc.settingsOpen) return false;
            if (atc.connected === false && atc.initialDataLoaded) return false;
            return !document.querySelector('.cl-signin');
        },

        get showUpdate() { return this.updateAvailable && this.laterFor !== this.server.page && this.free(); },
        get showNewVersionButton() { return this.updateAvailable && this.laterFor === this.server.page; },
        get showNews() { return !!this.unseen && !(this.updateAvailable && this.laterFor !== this.server.page) && this.free(); },

        updateText() {
            const atc = Alpine.store('atc');
            const on = atc && atc.unmutedFrequencies ? [...atc.unmutedFrequencies] : [];
            const mhzs = ((atc && atc.audioFrequencies) || [])
                .filter((f) => on.includes(String(f.id)) || on.includes(f.id))
                .map((f) => (Number(f.frequency_mhz) > 0 ? Number(f.frequency_mhz).toFixed(3) : atc.freqShortName(f)));
            return 'Reload this page to use the new version.' + (atc && atc.audioRunning ? listeningSentence(mhzs) : '');
        },

        reload() { location.reload(); },
        later() { this.laterFor = this.server.page; },

        newsDone() {
            if (this.unseen) {
                try { localStorage.setItem(this.seenKey(), this.unseen.newest); } catch (e) { /* once per page, then */ }
            }
            this.unseen = null;
        },

        allNews() {
            this.newsDone();
            Alpine.store('atc').settingsOpen = true;
            window.dispatchEvent(new CustomEvent('settings-page', { detail: 'about' }));
        },

        // For Settings › About Clairance.
        when(v, withYear) { return v && v.built ? newsDate(v.built.slice(0, 10), withYear) : '—'; },
        date(d) { return newsDate(d, true); },
    });
});

if (typeof module !== 'undefined') module.exports = { newsUnseen, newsDate, listeningSentence };
