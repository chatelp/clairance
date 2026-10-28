// accountsPanel(), the x-data of the Accounts part of the settings page (D83,
// 27/10): the owner sees who may sign in and what each has open, adds an
// account, gives one a new password, cuts one off, removes one -- while the
// server runs. Shown only when the server answers the list, which it does for
// the owner alone (a viewer gets 403, and the part stays hidden).
//
// The password typed here goes to the server once, in the request body, and is
// cleared from the form as soon as the answer comes.

// Roles in the owner's words, one word each everywhere on screen (D86): an
// owner is the Admin of this server.
const ACCOUNT_ROLES = { owner: 'Admin', viewer: 'Viewer' };

// "3 min ago", "2 h ago", "26/10 18:21": how long since, for a list read at a
// glance. Pure, for the tests.
function accountAgo(iso, now = Date.now()) {
    if (!iso) return '';
    const t = Date.parse(iso);
    if (!Number.isFinite(t)) return '';
    const s = Math.max(0, Math.round((now - t) / 1000));
    if (s < 60) return 'just now';
    if (s < 3600) return Math.round(s / 60) + ' min ago';
    if (s < 86400) return Math.round(s / 3600) + ' h ago';
    const d = new Date(t);
    const two = (n) => String(n).padStart(2, '0');
    return `${two(d.getDate())}/${two(d.getMonth() + 1)} ${two(d.getHours())}:${two(d.getMinutes())}`;
}

// One line on an account: its sessions, its last request, its last sign-in.
function accountFacts(a, now = Date.now()) {
    const parts = [];
    parts.push(a.sessions === 0 ? 'not signed in' : a.sessions === 1 ? '1 session' : a.sessions + ' sessions');
    if (a.sessions > 0 && a.last_seen) parts.push('seen ' + accountAgo(a.last_seen, now));
    if (a.last_sign_in) {
        let s = 'signed in ' + accountAgo(a.last_sign_in.at, now);
        if (a.last_sign_in.from) s += ' from ' + a.last_sign_in.from;
        parts.push(s);
    }
    if (a.source === 'config') parts.push('written in config.toml');
    return parts.join(' · ');
}

function accountsPanel() {
    return {
        loaded: false,
        accounts: [],
        you: '',
        minPassword: 10,
        busy: false,
        error: '',
        note: '',
        adding: false,     // the form to add an account is open
        // Its fields, kept apart from adding: closing the form while Alpine
        // still reads them must find them there.
        form: { name: '', password: '', role: 'viewer' },
        passwordFor: '',   // the account whose new password is being typed
        newPassword: '',

        roleLabel(role) { return ACCOUNT_ROLES[role] || role; },
        facts(a) { return accountFacts(a); },

        async load() {
            try {
                const r = await fetch('/api/v1/accounts', { credentials: 'same-origin', cache: 'no-store' });
                if (!r.ok) return; // not the owner: the part stays hidden
                this.take(await r.json());
                this.loaded = true;
            } catch (e) { /* the rest of the page does not depend on it */ }
        },

        take(body) {
            if (Array.isArray(body.accounts)) this.accounts = body.accounts;
            if (typeof body.you === 'string') this.you = body.you;
            if (body.min_password) this.minPassword = body.min_password;
        },

        // send makes one change and reads back the list the server answers
        // with; what went wrong is said under the list.
        async send(method, path, body, done) {
            this.busy = true;
            this.error = '';
            this.note = '';
            try {
                const r = await fetch(path, {
                    method, credentials: 'same-origin', cache: 'no-store',
                    headers: body ? { 'Content-Type': 'application/json' } : {},
                    body: body ? JSON.stringify(body) : undefined,
                });
                const out = await r.json().catch(() => ({}));
                if (!r.ok) {
                    this.error = out.error || ('the server answered ' + r.status);
                    return false;
                }
                this.take(out);
                if (done) done(out);
                return true;
            } catch (e) {
                this.error = 'the server could not be reached';
                return false;
            } finally {
                this.busy = false;
            }
        },

        startAdding() {
            this.form = { name: '', password: '', role: 'viewer' };
            this.adding = true;
            this.error = '';
        },

        stopAdding() {
            this.adding = false;
            this.form.password = '';
        },

        async add() {
            const name = this.form.name.trim();
            const ok = await this.send('POST', '/api/v1/accounts', { name, password: this.form.password, role: this.form.role });
            this.form.password = '';
            if (ok) {
                this.note = `${name} can sign in now.`;
                this.stopAdding();
            }
        },

        startPassword(name) {
            this.passwordFor = name;
            this.newPassword = '';
            this.error = '';
        },

        async setPassword() {
            const name = this.passwordFor;
            const ok = await this.send('PUT', '/api/v1/accounts/' + encodeURIComponent(name) + '/password', { password: this.newPassword });
            this.newPassword = '';
            if (ok) {
                this.note = name === this.you
                    ? 'Password changed. Your other browsers are signed out.'
                    : `Password changed. ${name} is signed out everywhere.`;
                this.passwordFor = '';
            }
        },

        async cut(name) {
            if (!confirm(`Cut ${name}'s access now? Every page ${name} has open is signed out, its sound stops. ${name} can sign in again with the same password.`)) return;
            await this.send('POST', '/api/v1/accounts/' + encodeURIComponent(name) + '/cut', null, (out) => {
                const e = out.ended || {};
                this.note = `${name} is signed out: ${e.sessions || 0} session(s) ended, ${e.sockets || 0} page(s) and ${e.streams || 0} sound stream(s) closed.`;
            });
        },

        async remove(name) {
            if (!confirm(`Remove the account ${name}? Its pages are signed out at once, and it can no longer sign in.`)) return;
            await this.send('DELETE', '/api/v1/accounts/' + encodeURIComponent(name), null, () => {
                this.note = `${name} is removed.`;
            });
        },
    };
}

if (typeof module !== 'undefined') module.exports = { accountAgo, accountFacts, ACCOUNT_ROLES };
