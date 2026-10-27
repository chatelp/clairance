// usagePanel(), the x-data of the Usage part of the settings page (D84,
// 27/10): what each account did, for the owner -- who is here now and what
// they hear, and per account over the last 7 days the time connected and
// listening, the clips played again, a bar a day, and the interruptions met.
// Shown only when the server answers, which it does for the owner alone.

// "2 h 05", "12 min", "40 s": a duration read at a glance. Pure, for the tests.
function usageDuration(s) {
    s = Math.max(0, Math.round(s || 0));
    if (s < 60) return s + ' s';
    const m = Math.round(s / 60);
    if (m < 60) return m + ' min';
    const h = Math.floor(m / 60);
    return h + ' h ' + String(m % 60).padStart(2, '0');
}

// "124.350" for the frequency 124350; any other id as it is.
function usageFreq(id) {
    return /^\d{6}$/.test(id) ? id.slice(0, 3) + '.' + id.slice(3) : String(id);
}

// "26/10 16:59": a moment in the browser's time.
function usageWhen(iso) {
    const t = new Date(iso);
    if (Number.isNaN(t.getTime())) return '';
    const two = (n) => String(n).padStart(2, '0');
    return `${two(t.getDate())}/${two(t.getMonth() + 1)} ${two(t.getHours())}:${two(t.getMinutes())}`;
}

// One line on an account here now.
function usageNowLine(p, now = Date.now()) {
    const parts = [];
    if (p.pages > 0 && p.since) {
        parts.push((p.pages > 1 ? p.pages + ' pages open ' : 'page open ') + usageDuration((now - Date.parse(p.since)) / 1000));
    }
    if (p.listening && p.listening.length) {
        parts.push('listening to ' + p.listening.map((l) => usageFreq(l.freq)).join(', '));
    } else if (p.pages > 0) {
        parts.push('sound off');
    }
    return parts.join(' · ');
}

// One line on an account's week.
function usageWeekLine(a) {
    const w = a.week || {};
    const parts = ['connected ' + usageDuration(w.connected_s)];
    if (w.listening_s > 0) {
        // Each frequency's time when there are two, its name alone when one.
        const by = Object.entries(w.by_freq_s || {}).sort((x, y) => y[1] - x[1]);
        const detail = by.length > 1 ? by.map(([f, s]) => usageFreq(f) + ' ' + usageDuration(s)).join(', ')
            : by.length ? usageFreq(by[0][0]) : '';
        parts.push('listened ' + usageDuration(w.listening_s) + (detail ? ' (' + detail + ')' : ''));
    } else {
        parts.push('no sound');
    }
    if (w.clips) parts.push(w.clips + (w.clips > 1 ? ' clips' : ' clip') + ' played again');
    parts.push(w.visits + (w.visits === 1 ? ' visit' : ' visits'));
    return parts.join(' · ');
}

// One line on an interruption an account met.
const USAGE_WHY = { stopped: 'server stopped', source: 'station stream ended', lost: 'connection lost' };
function usageInterruptionLine(i) {
    const what = i.kind === 'sound' ? 'sound ' + usageFreq(i.freq) : 'page';
    const back = i.back_after_s == null ? 'not back within 10 min' : 'back in ' + usageDuration(i.back_after_s);
    return `${usageWhen(i.at)} · ${what} · ${USAGE_WHY[i.why] || i.why} · ${back}`;
}

// The 7 bars of a week: each day's height against the busiest, and what it
// says on hover.
const USAGE_DAY = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];
function usageBars(days) {
    const max = Math.max(1, ...days.map((d) => d.connected_s));
    return days.map((d) => {
        const date = new Date(d.date + 'T12:00:00');
        return {
            date: d.date,
            label: USAGE_DAY[date.getDay()],
            connected: Math.round((d.connected_s / max) * 100),
            listening: Math.round((d.listening_s / max) * 100),
            title: `${d.date.slice(8)}/${d.date.slice(5, 7)}: connected ${usageDuration(d.connected_s)}, listened ${usageDuration(d.listening_s)}` +
                (d.clips ? `, ${d.clips} clip${d.clips > 1 ? 's' : ''}` : ''),
        };
    });
}

function usagePanel() {
    return {
        loaded: false,
        enabled: false,
        days: 0,
        now: [],
        accounts: [],
        busy: false,
        timer: null,

        nowLine(p) { return usageNowLine(p); },
        weekLine(a) { return usageWeekLine(a); },
        interruptionLine(i) { return usageInterruptionLine(i); },
        bars(a) { return usageBars(a.days || []); },
        lastSeen(a) { return typeof accountAgo === 'function' ? accountAgo(a.last_seen) : usageWhen(a.last_seen); },

        async load() {
            this.busy = true;
            try {
                const r = await fetch('/api/v1/usage', { credentials: 'same-origin', cache: 'no-store' });
                if (!r.ok) return; // not the owner: the part stays hidden
                const body = await r.json();
                this.enabled = !!body.enabled;
                this.days = body.days || 0;
                this.now = body.now || [];
                this.accounts = body.accounts || [];
                this.loaded = true;
            } catch (e) { /* the rest of the page does not depend on it */ } finally {
                this.busy = false;
            }
            // Kept fresh while the settings are open, the owner's only.
            if (this.loaded && !this.timer) {
                this.timer = setInterval(() => {
                    if (document.visibilityState === 'visible' && Alpine.store('atc').settingsOpen) this.load();
                }, 15000);
            }
        },
    };
}

if (typeof module !== 'undefined') {
    module.exports = { usageDuration, usageFreq, usageNowLine, usageWeekLine, usageInterruptionLine, usageBars };
}
