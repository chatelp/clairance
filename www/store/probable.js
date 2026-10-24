// The probable runway and exit of each aircraft (Q57; MapLabel,
// AircraftStory): read from its own ADS-B track every two seconds, said on
// its label ("→26L/R", "→OKASI +2") and in its story; the selected
// departure's candidate exits drawn on the map, their published routes,
// following the recomputing (owner, 16/10: "to follow the aircraft and see
// whether it really goes there"). The rules are map/probable-exits.js's.
//
// One domain of the Alpine store 'atc' (docs-fr/32, palier 5.1); see
// store/station.js for how the domains are assembled.
(function () {
    const TICK_MS = 2000;
    const KEEP_MS = 60000;
    // Positions kept per departure, outside Alpine's reactivity: a minute of
    // fixes for a hundred aircraft would be proxied for nothing.
    const history = new Map();
    // Departures whose track is being, or has been, asked of the server:
    // once each, so that a page just opened has its exits within seconds
    // instead of after the 45 s of track it would otherwise have to see
    // itself (design review 16/10).
    const seeding = new Set();
    const SEED_POSITIONS = 90;
    let exitsCache = { routes: null, byKey: new Map() };

    window.AtcStore = window.AtcStore || [];
    window.AtcStore.push(() => ({
        // hex -> { main: '→OKASI', rest: ' +2', others: ['DOPAP', 'ODEBU'],
        // toNM, kind: 'SID' | 'arrival' }, replaced as a whole each tick.
        probable: {},
        _probableTimer: null,
        _exitRoutesKey: '',

        startProbable() {
            if (this._probableTimer) return;
            this._probableTimer = setInterval(() => this.updateProbable(), TICK_MS);
        },

        _exitsOf(departure) {
            const X = window.ProbableExits;
            if (exitsCache.routes !== this.procedureRoutes) exitsCache = { routes: this.procedureRoutes, byKey: new Map() };
            const key = departure.airport + '|' + departure.runway;
            if (!exitsCache.byKey.has(key)) {
                exitsCache.byKey.set(key, {
                    exits: X.exitsFor(this.procedureRoutes, departure),
                    threshold: X.thresholdOf(this.procedureRoutes, departure),
                });
            }
            return exitsCache.byKey.get(key);
        },

        updateProbable() {
            const X = window.ProbableExits;
            if (!X) return;
            const now = Date.now();
            const next = {};
            if (this.settings.showProbable !== false) {
                // Departures: from the runway the server judged it took off on.
                for (const a of Object.values(this.aircraft || {})) {
                    const dep = a && a.runway && a.runway.departure;
                    const adsb = a && a.adsb;
                    if (!dep || !adsb || a.status !== 'active' || !Number.isFinite(adsb.lat)) continue;
                    let h = history.get(a.hex);
                    if (!h) { h = []; history.set(a.hex, h); }
                    if (!seeding.has(a.hex)) {
                        seeding.add(a.hex);
                        this._seedTrack(a.hex);
                    }
                    const last = h[h.length - 1];
                    if (!last || last.lat !== adsb.lat || last.lon !== adsb.lon) {
                        h.push({ t: now, lat: adsb.lat, lon: adsb.lon, track: Number.isFinite(adsb.track) ? adsb.track : NaN });
                    }
                    while (h.length && now - h[0].t > KEEP_MS) h.shift();
                    const { exits, threshold } = this._exitsOf(dep);
                    const p = X.departure({ history: h, exits, threshold, now });
                    const words = X.labelText(p);
                    if (words) {
                        next[a.hex] = {
                            kind: 'SID', main: words.main, rest: words.rest, toNM: p.toExitNM,
                            point: p.candidates[0].point, others: p.candidates.slice(1).map((c) => c.point),
                            candidates: p.candidates.map((c) => ({ point: c.point, route: c.routes[0] })),
                            runway: dep.runway,
                        };
                    }
                }
                // Arrivals: the sequence's runway (par/par-arrivals.js), its pair
                // until 15 NM.
                for (const seqs of Object.values(this.arrivals || {})) {
                    for (const seq of seqs || []) {
                        for (const it of seq.items || []) {
                            const text = X.arrivalText(seq.runway, it.distNM);
                            if (text) next[it.hex] = { kind: 'arrival', main: '→' + text, rest: '', runway: text, toNM: it.distNM };
                        }
                    }
                }
            }
            for (const hex of history.keys()) {
                if (!this.aircraft || !this.aircraft[hex]) { history.delete(hex); seeding.delete(hex); }
            }
            this.probable = next;
            this._drawSelectedExits();
        },

        // A departure's last positions from the server, put before the ones
        // this page has seen. Dated relative to the newest, so that a clock
        // that differs from the server's (a phone, a VPN) does not matter.
        async _seedTrack(hex) {
            try {
                const r = await fetch(`${API_BASE_URL}/aircraft/${hex}/tracks?limit=${SEED_POSITIONS}`, { credentials: 'same-origin' });
                if (!r.ok) return;
                const data = await r.json();
                const pts = (Array.isArray(data.history) ? data.history : [])
                    .filter((q) => Number.isFinite(q.lat) && Number.isFinite(q.lon) && q.timestamp)
                    .map((q) => ({ at: Date.parse(q.timestamp), lat: q.lat, lon: q.lon, track: Number.isFinite(q.track) ? q.track : NaN }))
                    .filter((q) => Number.isFinite(q.at))
                    .sort((x, y) => x.at - y.at);
                if (!pts.length) return;
                const now = Date.now();
                const newest = pts[pts.length - 1].at;
                const h = history.get(hex) || [];
                const firstSeen = h.length ? h[0].t : Infinity;
                const before = pts
                    .map((q) => ({ t: now - (newest - q.at), lat: q.lat, lon: q.lon, track: q.track }))
                    .filter((q) => q.t < firstSeen && now - q.t <= KEEP_MS);
                history.set(hex, before.concat(h));
            } catch (e) { /* the page's own track fills in within a minute */ }
        },

        // The selected departure's candidate exits, their published routes on
        // the map: the likeliest at full strength, the others quieter; drawn
        // again only when the list changes.
        _drawSelectedExits() {
            const hex = this.selectedAircraft && this.selectedAircraft.hex;
            const p = hex ? this.probable[hex] : null;
            const list = p && p.kind === 'SID' ? p.candidates.filter((c) => c.route) : [];
            const key = list.map((c) => c.point + ':' + c.route.name).join('|') + '@' + (p ? p.runway : '');
            if (key === this._exitRoutesKey) return;
            this._exitRoutesKey = key;
            this.mapManager?.showExitRoutes?.(list.map((c, i) => ({ route: c.route, runways: [p.runway], strong: i === 0 })));
        },

        // The story's key for an aircraft with a probable runway or exit, or
        // null (store/story.js, storyHead).
        probableCell(a) {
            const p = a && this.probable[a.hex];
            if (!p) return null;
            if (p.kind === 'SID') {
                return {
                    label: `To ${p.point}`, value: String(Math.round(p.toNM)), unit: 'NM',
                    extra: p.others.length ? ` · or ${p.others.join(', ')}` : '', extraDim: true,
                    help: 'The exit it is probably heading for, read from its ADS-B track, not announced by the controllers; ' +
                        'the others it could still take, the most likely first. Measured on 813 departures from Paris-CDG: ' +
                        'an exit alone is right 93 times in 100 at 20 NM out. ' +
                        'Its published route is drawn on the map, with those of the exits it could still take, while it is selected.',
                };
            }
            return null;
        },

        // The setting is on by default: undefined reads as on.
        flipProbable() {
            this.settings.showProbable = this.settings.showProbable === false;
            this.saveSettings();
            this.updateProbable();
        },
    }));
})();
