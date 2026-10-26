/*
 * Sign-in and first run (design system, SignIn in its "sky" variant and
 * StationVitals; migration step 9d, connexion.html and premier-lancement.html).
 *
 * Kept in a file of its own, loaded before everything else, and written without
 * Alpine on purpose: it has to work before the application boots, and it must not
 * depend on anything the application needs a session to fetch. Its look comes
 * from the design system's stylesheets, which the page loads first; it has no
 * palette of its own any more.
 *
 * When authentication is disabled -- the default of the published configuration --
 * this does nothing at all.
 */
(function () {
    'use strict';

    const ENDPOINT = '/api/v1/auth';
    const SETUP = '/api/v1/setup';
    const VITALS = '/api/v1/vitals';
    const VITALS_EVERY_MS = 10000;

    // The sky behind the card: decoration drawn by tools/design/connexion.py
    // (scope_svg), never positions; and the brand's mark.
    const SCOPE = `<svg class="cl-signin__scope" viewBox="0 0 1440 900" preserveAspectRatio="xMidYMid slice" aria-hidden="true"><path d="M720 450 L720 -150 A600 600 0 0 1 1144.3 25.7 Z" fill="#4caf50" fill-opacity="0.10"/><circle cx="720" cy="450" r="150" fill="none" stroke="#333333"/><circle cx="720" cy="450" r="300" fill="none" stroke="#333333"/><circle cx="720" cy="450" r="450" fill="none" stroke="#333333"/><circle cx="720" cy="450" r="600" fill="none" stroke="#333333"/><line x1="720" y1="450" x2="1144.3" y2="25.7" stroke="#4caf50" stroke-opacity="0.6" stroke-width="2"/><circle cx="817.2" cy="241.5" r="6" fill="#4caf50"/><text x="720" y="294" text-anchor="middle" font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#6b6b6b">5 NM</text><text x="720" y="144" text-anchor="middle" font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#6b6b6b">10 NM</text><text x="720" y="-6" text-anchor="middle" font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#6b6b6b">15 NM</text><g transform="translate(330 210) rotate(78) translate(-13.0 -13.0) scale(0.40625)"><g transform="translate(32 32) scale(1.543) translate(0 -18.8)" fill="#c2a3ff" stroke="#c2a3ff" stroke-width="0.778" stroke-linejoin="round"><path d="M0 0 C1.225 0 1.975 1.748 1.975 4.6 L1.975 28.6 C1.975 34 1.223 37.6 0.972 37.6 L-0.972 37.6 C-1.223 37.6 -1.975 34 -1.975 28.6 L-1.975 4.6 C-1.975 1.748 -1.225 0 0 0Z"/><path d="M1.9 13.6 L17.9 22.107 L17.9 23.707 L1.9 20.2ZM-1.9 13.6 L-17.9 22.107 L-17.9 23.707 L-1.9 20.2Z"/><path d="M0.4 31.6 L6.2 35.224 L6.2 36.624 L0.4 35.2ZM-0.4 31.6 L-6.2 35.224 L-6.2 36.624 L-0.4 35.2Z"/><path d="M-5.9 12.947 L-5.6 12.947 Q-4.7 12.947 -4.7 13.847 L-4.7 16.447 Q-4.7 17.347 -5.6 17.347 L-5.9 17.347 Q-6.8 17.347 -6.8 16.447 L-6.8 13.847 Q-6.8 12.947 -5.9 12.947Z"/><path d="M5.6 12.947 L5.9 12.947 Q6.8 12.947 6.8 13.847 L6.8 16.447 Q6.8 17.347 5.9 17.347 L5.6 17.347 Q4.7 17.347 4.7 16.447 L4.7 13.847 Q4.7 12.947 5.6 12.947Z"/></g></g><g transform="translate(1150 170) rotate(250) translate(-15.0 -15.0) scale(0.46875)"><g transform="translate(32 32) scale(0.868) translate(0 -33.4)" fill="#6e7b8f" stroke="#6e7b8f" stroke-width="1.382" stroke-linejoin="round"><path d="M0 0 C1.848 0 2.98 2.85 2.98 7.5 L2.98 51.8 C2.98 60.8 2.041 66.8 1.728 66.8 L-1.728 66.8 C-2.041 66.8 -2.98 60.8 -2.98 51.8 L-2.98 7.5 C-2.98 2.85 -1.848 0 0 0Z"/><path d="M3 23.5 L32.4 41.165 L32.4 43.565 L3 35ZM-3 23.5 L-32.4 41.165 L-32.4 43.565 L-3 35Z"/><path d="M0.6 57.8 L9.6 63.645 L9.6 65.645 L0.6 63.8ZM-0.6 57.8 L-9.6 63.645 L-9.6 65.645 L-0.6 63.8Z"/><path d="M-10.928 23.727 L-10.472 23.727 Q-8.972 23.727 -8.972 25.227 L-8.972 29.427 Q-8.972 30.927 -10.472 30.927 L-10.928 30.927 Q-12.428 30.927 -12.428 29.427 L-12.428 25.227 Q-12.428 23.727 -10.928 23.727Z"/><path d="M10.472 23.727 L10.928 23.727 Q12.428 23.727 12.428 25.227 L12.428 29.427 Q12.428 30.927 10.928 30.927 L10.472 30.927 Q8.972 30.927 8.972 29.427 L8.972 25.227 Q8.972 23.727 10.472 23.727Z"/></g></g><g transform="translate(560 640) rotate(-30) translate(-10.0 -10.0) scale(0.3125)"><g transform="translate(32 32) scale(5.8) translate(0 -3.6)" fill="#fff59a" stroke="#fff59a" stroke-width="0.207" stroke-linejoin="round"><path d="M0 0 C0.357 0 0.575 0.494 0.575 1.3 L0.575 4 C0.575 6.16 0.338 7.6 0.259 7.6 L-0.259 7.6 C-0.338 7.6 -0.575 6.16 -0.575 4 L-0.575 1.3 C-0.575 0.494 -0.356 0 0 0Z"/><path d="M0.55 2.1 L5 2.1 L5 3.35 L0.55 3.65ZM-0.55 2.1 L-5 2.1 L-5 3.35 L-0.55 3.65Z"/><path d="M0.2 6.5 L1.7 6.631 L1.7 7.381 L0.2 7.45ZM-0.2 6.5 L-1.7 6.631 L-1.7 7.381 L-0.2 7.45Z"/><path d="M-0.691 -0.309 L0.691 -0.309 Q0.95 -0.309 0.95 -0.05 L0.95 -0.05 Q0.95 0.209 0.691 0.209 L-0.691 0.209 Q-0.95 0.209 -0.95 -0.05 L-0.95 -0.05 Q-0.95 -0.309 -0.691 -0.309Z"/></g></g><g transform="translate(1080 690) rotate(120) translate(-12.0 -12.0) scale(0.375)"><g transform="translate(32 32) scale(2.086) translate(0 -13.3)" fill="#e68cba" stroke="#e68cba" stroke-width="0.575" stroke-linejoin="round"><path d="M0 0 C0.884 0 1.425 1.292 1.425 3.4 L1.425 18.7 C1.425 23.8 0.895 27.2 0.719 27.2 L-0.719 27.2 C-0.895 27.2 -1.425 23.8 -1.425 18.7 L-1.425 3.4 C-1.425 1.292 -0.884 0 0 0Z"/><path d="M1.4 10.8 L13.55 11.224 L13.55 12.774 L1.4 13.4ZM-1.4 10.8 L-13.55 11.224 L-13.55 12.774 L-1.4 13.4Z"/><path d="M0.3 24.6 L3.65 25.312 L3.65 26.612 L0.3 26.6ZM-0.3 24.6 L-3.65 25.312 L-3.65 26.612 L-0.3 26.6Z"/><path d="M-4.2 8.494 L-4 8.494 Q-3.2 8.494 -3.2 9.294 L-3.2 12.894 Q-3.2 13.694 -4 13.694 L-4.2 13.694 Q-5 13.694 -5 12.894 L-5 9.294 Q-5 8.494 -4.2 8.494Z"/><path d="M-5.331 7.675 L-2.869 7.675 Q-2.15 7.675 -2.15 8.394 L-2.15 8.394 Q-2.15 9.113 -2.869 9.113 L-5.331 9.113 Q-6.05 9.113 -6.05 8.394 L-6.05 8.394 Q-6.05 7.675 -5.331 7.675Z"/><path d="M4 8.494 L4.2 8.494 Q5 8.494 5 9.294 L5 12.894 Q5 13.694 4.2 13.694 L4 13.694 Q3.2 13.694 3.2 12.894 L3.2 9.294 Q3.2 8.494 4 8.494Z"/><path d="M2.869 7.675 L5.331 7.675 Q6.05 7.675 6.05 8.394 L6.05 8.394 Q6.05 9.113 5.331 9.113 L2.869 9.113 Q2.15 9.113 2.15 8.394 L2.15 8.394 Q2.15 7.675 2.869 7.675Z"/></g></g><g transform="translate(1260 470) rotate(200) translate(-13.0 -13.0) scale(0.40625)"><g transform="translate(32 32) scale(1.543) translate(0 -18.8)" fill="#61c9e3" stroke="#61c9e3" stroke-width="0.778" stroke-linejoin="round"><path d="M0 0 C1.225 0 1.975 1.748 1.975 4.6 L1.975 28.6 C1.975 34 1.223 37.6 0.972 37.6 L-0.972 37.6 C-1.223 37.6 -1.975 34 -1.975 28.6 L-1.975 4.6 C-1.975 1.748 -1.225 0 0 0Z"/><path d="M1.9 13.6 L17.9 22.107 L17.9 23.707 L1.9 20.2ZM-1.9 13.6 L-17.9 22.107 L-17.9 23.707 L-1.9 20.2Z"/><path d="M0.4 31.6 L6.2 35.224 L6.2 36.624 L0.4 35.2ZM-0.4 31.6 L-6.2 35.224 L-6.2 36.624 L-0.4 35.2Z"/><path d="M-5.9 12.947 L-5.6 12.947 Q-4.7 12.947 -4.7 13.847 L-4.7 16.447 Q-4.7 17.347 -5.6 17.347 L-5.9 17.347 Q-6.8 17.347 -6.8 16.447 L-6.8 13.847 Q-6.8 12.947 -5.9 12.947Z"/><path d="M5.6 12.947 L5.9 12.947 Q6.8 12.947 6.8 13.847 L6.8 16.447 Q6.8 17.347 5.9 17.347 L5.6 17.347 Q4.7 17.347 4.7 16.447 L4.7 13.847 Q4.7 12.947 5.6 12.947Z"/></g></g><g transform="translate(200 560) rotate(40) translate(-11.0 -11.0) scale(0.34375)"><g transform="translate(32 32) scale(2.871) translate(0 -10.1)" fill="#61c9e3" stroke="#61c9e3" stroke-width="0.418" stroke-linejoin="round"><path d="M0 0 C0.744 0 1.2 1.216 1.2 3.2 L1.2 14.2 C1.2 17.8 0.692 20.2 0.522 20.2 L-0.522 20.2 C-0.692 20.2 -1.2 17.8 -1.2 14.2 L-1.2 3.2 C-1.2 1.216 -0.744 0 0 0Z"/><path d="M1.1 8 L9.65 12.936 L9.65 14.286 L1.1 12ZM-1.1 8 L-9.65 12.936 L-9.65 14.286 L-1.1 12Z"/><path d="M0.15 17.6 L3.6 19.756 L3.6 20.756 L0.15 19.5ZM-0.15 17.6 L-3.6 19.756 L-3.6 20.756 L-0.15 19.5Z"/><path d="M-2.125 12.7 L-1.975 12.7 Q-1.425 12.7 -1.425 13.25 L-1.425 15.55 Q-1.425 16.1 -1.975 16.1 L-2.125 16.1 Q-2.675 16.1 -2.675 15.55 L-2.675 13.25 Q-2.675 12.7 -2.125 12.7Z"/><path d="M1.975 12.7 L2.125 12.7 Q2.675 12.7 2.675 13.25 L2.675 15.55 Q2.675 16.1 2.125 16.1 L1.975 16.1 Q1.425 16.1 1.425 15.55 L1.425 13.25 Q1.425 12.7 1.975 12.7Z"/></g></g><g transform="translate(880 760) rotate(0) translate(-11.0 -11.0) scale(0.34375)"><g transform="translate(32 32) scale(4.28) translate(0 -4.725)" fill="#fff59a" stroke="#fff59a" stroke-width="0.28" stroke-linejoin="round"><path d="M0 0.5 C1.05 0.5 1.1 2.0 1.1 3.3 C1.1 4.9 0.7 5.8 0.4 6.2 L-0.4 6.2 C-0.7 5.8 -1.1 4.9 -1.1 3.3 C-1.1 2.0 -1.05 0.5 0 0.5Z"/><path d="M0 5.9 L0 5.9 Q0.35 5.9 0.35 6.25 L0.35 10.95 Q0.35 11.3 0 11.3 L0 11.3 Q-0.35 11.3 -0.35 10.95 L-0.35 6.25 Q-0.35 5.9 0 5.9Z"/><path d="M-0.701 9.75 L-0.701 9.75 Q-0.35 9.75 -0.35 10.1 L-0.35 11.3 Q-0.35 11.65 -0.701 11.65 L-0.701 11.65 Q-1.051 11.65 -1.051 11.3 L-1.051 10.1 Q-1.051 9.75 -0.701 9.75Z"/><path d="M-3.535 -0.731 L4.031 6.835 L3.535 7.331 L-4.031 -0.235Z"/><path d="M-4.031 6.835 L3.535 -0.731 L4.031 -0.235 L-3.535 7.331Z"/></g></g></svg>`;
    const MARK = `<svg viewBox="0 0 64 64" aria-hidden="true"><circle cx="32" cy="32" r="26.5" fill="none" stroke="#e0e0e0" stroke-width="4"/><circle cx="32" cy="32" r="13" fill="none" stroke="#e0e0e0" stroke-width="2" opacity="0.55"/><path d="M32 32 L32 6 A26 26 0 0 1 50.385 13.615 Z" fill="#4caf50" fill-opacity="0.35"/><path d="M32 32 L50.385 13.615" fill="none" stroke="#4caf50" stroke-width="4" stroke-linecap="round"/><circle cx="39.18" cy="16.59" r="3.8" fill="#4caf50"/></svg>`;

    function esc(value) {
        return String(value ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
    }

    // The page around a card: the sky, the brand, the station's console, the foot.
    function page(cardHtml) {
        const el = document.createElement('div');
        el.id = 'coatc-login';
        el.className = 'cl-root';
        el.style.cssText = 'position: fixed; inset: 0; z-index: 99999; overflow-y: auto;';
        el.innerHTML = `
          <div class="cl-signin cl-signin--sky">
            ${SCOPE}
            <div class="cl-signin__brand">${MARK}Clairance</div>
            <div class="cl-signin__console" hidden></div>
            <form class="cl-signin__card" autocomplete="on">
              <span class="cl-plain">Hear the airband you receive, read it in English and French, and see which aircraft is speaking.</span>
              ${cardHtml}
            </form>
            <div class="cl-signin__foot">Runs on your own machine: nothing leaves it. Made for the listener; it judges no one.</div>
          </div>`;
        document.body.appendChild(el);
        startVitals(el.querySelector('.cl-signin__console'));
        return el;
    }

    // ---------------------------------------------------------- the console

    // A line's figures; a key that is absent is unknown and drops its figure,
    // a line with nothing left is not drawn (StationVitals).
    function line(key, figures, ok, failWord) {
        const shown = figures.filter(Boolean);
        if (!shown.length) return '';
        const mark = ok === true ? '<i class="cl-vitals__ok"></i>'
            : ok === false ? `<span class="cl-light cl-light--bad">${esc(failWord)}</span>` : '';
        return `<dt>${esc(key)}</dt><dd>${mark}${shown.map(esc).join(' <span>·</span> ')}</dd>`;
    }

    const has = (v) => v !== undefined && v !== null;
    const group = (n) => String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');

    function uptime(sec) {
        if (!Number.isFinite(sec)) return '';
        const d = Math.floor(sec / 86400);
        const h = Math.floor((sec % 86400) / 3600);
        const m = Math.floor((sec % 3600) / 60);
        return d ? `${d} d ${h} h` : h ? `${h} h ${m} min` : `${m} min`;
    }

    function vitalsHtml(v) {
        const rows = [
            line('ads-b', [
                has(v.aircraft) && `${group(v.aircraft)} aircraft`,
                has(v.messages_per_second) && `${group(v.messages_per_second)} msg/s`,
                has(v.farthest_nm) && `farthest ${group(v.farthest_nm)} NM`,
            ], has(v.messages_per_second) ? v.messages_per_second > 0 : undefined, 'no messages'),
            line('airband', [
                has(v.channels_on_air) && `${v.channels_on_air} ${v.channels_on_air === 1 ? 'channel' : 'channels'} on air`,
                has(v.transmissions_today) && `${group(v.transmissions_today)} transmissions today`,
            ], has(v.channels_on_air) ? v.channels_on_air > 0 : undefined, 'off air'),
            line('speech', [
                v.transcription_model,
                v.transcription_backend,
                has(v.seconds_per_transmission) && `${Number(v.seconds_per_transmission).toFixed(1)} s a transmission`,
            ], v.transcription_model ? true : undefined, ''),
            line('matched', [
                has(v.matched_share) && v.transmissions_today > 0 && `${v.matched_share} % of transmissions to an aircraft today`,
            ]),
            line('runways', [
                has(v.airports_followed) && `${v.airports_followed} ${v.airports_followed === 1 ? 'airport' : 'airports'} followed`,
                has(v.runways_in_use) && `${v.runways_in_use} ${v.runways_in_use === 1 ? 'runway' : 'runways'} in use`,
            ]),
            line('up', [
                has(v.uptime_seconds) && uptime(v.uptime_seconds),
                // A build without a version tag gives its commit as the version:
                // "Clairance 3016b7d", once.
                (v.version || v.commit) && `Clairance ${v.version || v.commit}`,
                v.commit && v.commit !== v.version && v.commit,
            ]),
        ].join('');
        return `<div class="cl-vitals"><div class="cl-vitals__head"><span>This station, live</span><span>refreshed every 10 s</span></div>` +
            `<dl class="cl-vitals__list">${rows}</dl></div>`;
    }

    // Public and cached 10 s by the server; a 404 means it is turned off in
    // Settings > Account, and the console is not drawn.
    function startVitals(box) {
        let timer = null;
        const tick = async () => {
            if (!document.body.contains(box)) { clearInterval(timer); return; }
            try {
                const res = await fetch(VITALS, { credentials: 'same-origin' });
                if (!res.ok) { box.hidden = true; return; }
                box.innerHTML = vitalsHtml(await res.json());
                box.hidden = false;
            } catch (e) {
                box.hidden = true;
            }
        };
        tick();
        timer = setInterval(tick, VITALS_EVERY_MS);
    }

    // ---------------------------------------------------------- sign-in

    function show() {
        const el = page(`
              <h1 class="cl-signin__title">Sign in</h1>
              <label class="cl-signin__field"><span class="cl-label">User</span>
                <input class="cl-input" id="coatc-user" name="username" autocomplete="username" autocapitalize="none" autofocus required></label>
              <label class="cl-signin__field"><span class="cl-label">Password</span>
                <input class="cl-input" id="coatc-pass" name="password" type="password" autocomplete="current-password" required></label>
              <button class="cl-btn cl-btn--primary" type="submit">Sign in</button>
              <span class="cl-plain" data-note>After 8 failed tries, wait 15 minutes.</span>`);
        const form = el.querySelector('form');
        const note = el.querySelector('[data-note]');
        const button = el.querySelector('button[type=submit]');

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            button.disabled = true;
            try {
                const res = await fetch(`${ENDPOINT}/login`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        name: el.querySelector('#coatc-user').value,
                        password: el.querySelector('#coatc-pass').value,
                    }),
                });
                if (res.ok) {
                    // Reload rather than tear the page down: everything the page
                    // fetched while signed out was a 401, so the cleanest state is a
                    // fresh one.
                    location.reload();
                    return;
                }
                // The server does not say which of the name or the password was
                // wrong, and neither does this. Blocked, it says for how long.
                fail(note, res.status === 429
                    ? blocked(res.headers.get('Retry-After'))
                    : 'Sign-in failed. After 8 failed tries, wait 15 minutes.');
            } catch (e) {
                fail(note, 'Server unreachable.');
            }
            button.disabled = false;
            el.querySelector('#coatc-pass').select();
        });
    }

    // Too many attempts from this address: how long to wait, from the
    // server's Retry-After (seconds), in whole minutes.
    function blocked(retryAfter) {
        const seconds = parseInt(retryAfter, 10);
        if (!Number.isFinite(seconds) || seconds <= 0) return 'Too many attempts. Wait 15 minutes.';
        const minutes = Math.ceil(seconds / 60);
        return `Too many attempts. Try again in ${minutes} minute${minutes > 1 ? 's' : ''}.`;
    }

    function fail(note, text) {
        note.textContent = text;
        note.setAttribute('role', 'alert');
        note.style.color = 'var(--color-status-urgent)';
    }

    // ---------------------------------------------------------- first run

    /*
     * First run. Upstream ships with no authentication and a README saying never
     * to expose it; this asks the question instead of leaving it to whoever reads
     * the TOML.
     *
     * No setup token: the server refuses to start unconfigured unless it is bound
     * to loopback, so reaching this page already means being on the machine --
     * the same trust boundary as the terminal where you would run -add-user.
     */
    function setupOverlay(state) {
        // Behind a declared proxy the server is reachable from elsewhere even on
        // loopback, and refuses to run open: say so rather than offer it.
        const local = state.local_allowed !== false;
        const el = page(`
              <h1 class="cl-signin__title">Welcome</h1>
              <span class="cl-plain">First start. Who may open Clairance?</span>
              <label class="cl-check cl-check--radio${local ? ' is-on' : ''}" data-mode="local"
                     ${local ? '' : 'aria-disabled="true" style="opacity: .5; cursor: default;"'}
                     title="${local ? `No sign-in: the server stays on ${esc(state.host)}, reachable from nowhere else.` : 'Not available: a proxy is declared in front of this server, so other machines can reach it.'}">
                <span class="cl-check__box"></span>Only this machine, no password</label>
              <label class="cl-check cl-check--radio${local ? '' : ' is-on'}" data-mode="account"
                     title="Creates a sign-in. Use TLS, or a trusted proxy declared in the configuration, before a password crosses a network you do not control.">
                <span class="cl-check__box"></span>Any device on my network, with a password</label>
              <div data-account ${local ? 'hidden' : ''} style="display: flex; flex-direction: column; gap: var(--space-4);">
                <label class="cl-signin__field"><span class="cl-label">User name</span>
                  <input class="cl-input" id="coatc-user" autocomplete="username" autocapitalize="none"></label>
                <label class="cl-signin__field"><span class="cl-label">Password, 10 characters or more</span>
                  <input class="cl-input" id="coatc-pass" type="password" autocomplete="new-password"></label>
              </div>
              <span class="cl-plain" data-note>You can change this later in Settings › Account.</span>
              <button class="cl-btn cl-btn--primary" type="submit">Continue</button>`);

        const form = el.querySelector('form');
        const note = el.querySelector('[data-note]');
        const button = el.querySelector('button[type=submit]');
        const account = el.querySelector('[data-account]');
        let mode = local ? 'local' : 'account';

        el.querySelectorAll('[data-mode]').forEach((choice) => choice.addEventListener('click', (e) => {
            e.preventDefault();
            if (choice.getAttribute('aria-disabled') === 'true') return;
            mode = choice.dataset.mode;
            el.querySelectorAll('[data-mode]').forEach((c) => c.classList.toggle('is-on', c === choice));
            account.hidden = mode !== 'account';
        }));

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            button.disabled = true;
            try {
                const res = await fetch(SETUP, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        mode,
                        username: el.querySelector('#coatc-user').value,
                        password: el.querySelector('#coatc-pass').value,
                    }),
                });
                if (res.ok) { location.reload(); return; }
                fail(note, (await res.text()).trim() || 'Setup failed.');
            } catch (e) {
                fail(note, 'Server unreachable.');
            }
            button.disabled = false;
        });
    }

    async function check() {
        try {
            const setup = await fetch(`${SETUP}/status`);
            if (setup.ok) {
                const st = await setup.json();
                if (st.needed) { setupOverlay(st); return; }
            }
            const res = await fetch(`${ENDPOINT}/status`);
            if (!res.ok) return;
            const s = await res.json();
            if (s.enabled && !s.authenticated) show();
        } catch (e) {
            // Unreachable server: let the application show its own failure rather
            // than covering it with a sign-in that cannot work either.
        }
    }

    // A preview of either card without a server in that state, for the design
    // review: ?signin=1 or ?signin=setup, on a test instance on this machine only.
    const onThisMachine = ['127.0.0.1', 'localhost', '[::1]'].includes(location.hostname);
    const preview = onThisMachine ? new URLSearchParams(location.search).get('signin') : null;

    function start() {
        if (preview === 'setup') setupOverlay({ local_allowed: true, host: '127.0.0.1' });
        else if (preview) show();
        else check();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', start);
    } else {
        start();
    }
})();
