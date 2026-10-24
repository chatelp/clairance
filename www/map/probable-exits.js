/**
 * The probable runway of an arrival and the probable exit of a departure,
 * read from the aircraft's own ADS-B track (Q57; MapLabel, AircraftStory).
 * Pure functions, no DOM -- loaded as a script in the page
 * (window.ProbableExits) and as a module by the tests.
 *
 * Measured on the stations's databases of 02-04 and 15-16/10
 * (docs-fr/05-decisions.md, Q57):
 * - an arrival's runway, from the arrival sequence's cone (20 NM): every
 *   error was the other runway of the pair, so the pair is said from 20 NM
 *   ("26L/R") and the runway from 15 NM (96.6 % right at LFPG);
 * - a departure's exit, from 15 NM: the angle between the track and the
 *   bearing to each exit of its runway pair, median over 45 s; a
 *   probability per exit, exp(-angle / T), T fitted per distance; the exits
 *   under 10 % dropped; one exit alone once its margin over the second
 *   reaches 10 degrees (93 % right at 20 NM, for half the departures).
 */
(function (root, factory) {
    const api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    } else {
        root.ProbableExits = api;
    }
})(typeof self !== 'undefined' ? self : this, function () {
    const FROM_NM = 15;          // nothing before: the true exit is in the list 52 times in 100 at 10 NM
    const PAIR_UNTIL_NM = 15;    // an arrival's pair of runways until then, the runway after
    const WINDOW_MS = 45000;     // the median's window
    const MIN_SAMPLES = 5;
    const KEEP_P = 0.10;         // an exit under this is dropped from the list
    const ALONE_MARGIN_DEG = 10; // one exit alone past this margin over the second
    // An aircraft whose best exit is further off its track than this aims at
    // none: guided on a heading, it shows nothing.
    const GUIDED_DEG = 15;
    // Passed: within this of its exit, or further from the airport than it.
    const PASSED_NM = 2.5;

    // T in degrees by distance from the runway, fitted on two halves of the
    // days (Q57: 15-20 at 15 NM, 8 at 20, 6-8 at 25, 6 at 30).
    const T_BY_NM = [[15, 17], [20, 8], [25, 7], [30, 6]];

    function temperature(nm) {
        if (nm <= T_BY_NM[0][0]) return T_BY_NM[0][1];
        for (let i = 1; i < T_BY_NM.length; i++) {
            const [n0, t0] = T_BY_NM[i - 1], [n1, t1] = T_BY_NM[i];
            if (nm <= n1) return t0 + (t1 - t0) * (nm - n0) / (n1 - n0);
        }
        return T_BY_NM[T_BY_NM.length - 1][1];
    }

    function nm(a, b) {
        const x = (b.lon - a.lon) * 60 * Math.cos(((a.lat + b.lat) / 2) * Math.PI / 180);
        const y = (b.lat - a.lat) * 60;
        return Math.hypot(x, y);
    }

    function bearing(a, b) {
        const r = Math.PI / 180;
        const y = Math.sin((b.lon - a.lon) * r) * Math.cos(b.lat * r);
        const x = Math.cos(a.lat * r) * Math.sin(b.lat * r) - Math.sin(a.lat * r) * Math.cos(b.lat * r) * Math.cos((b.lon - a.lon) * r);
        return (Math.atan2(y, x) / r + 360) % 360;
    }

    function angleOff(a, b) {
        const d = Math.abs(a - b) % 360;
        return d > 180 ? 360 - d : d;
    }

    function median(xs) {
        const s = xs.slice().sort((a, b) => a - b);
        const m = s.length >> 1;
        return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
    }

    // A technical point (PG271, PO772) is not an exit a listener knows.
    function technical(name) {
        return /^P[GO]\d{3}$/.test(name);
    }

    // The exits of a departure: the last named point of each published SID
    // of its airport serving its runway, with the routes ending there.
    function exitsFor(routes, departure) {
        if (!departure || !departure.airport || !departure.runway) return [];
        const out = new Map();
        for (const r of routes || []) {
            if (r.kind !== 'SID' || r.airport !== departure.airport || !(r.runways || []).includes(departure.runway)) continue;
            const legs = r.legs || [];
            const end = legs[legs.length - 1];
            if (!end || !end.named || technical(end.point)) continue;
            let e = out.get(end.point);
            if (!e) { e = { point: end.point, lat: end.lat, lon: end.lon, routes: [] }; out.set(end.point, e); }
            e.routes.push(r);
        }
        return [...out.values()];
    }

    // Where a departure's runway starts, from its routes' thresholds.
    function thresholdOf(routes, departure) {
        for (const r of routes || []) {
            if (r.airport !== departure.airport) continue;
            const t = (r.thresholds || []).find((x) => x.runway === departure.runway);
            if (t) return { lat: t.lat, lon: t.lon };
        }
        return null;
    }

    // The probable exit of a departure now: null, or { candidates: [{ point,
    // p, angle, route }], alone, distNM }. history: [{ t, lat, lon, track }],
    // oldest first; now: ms.
    function departure({ history, exits, threshold, now }) {
        if (!exits || exits.length < 1 || !threshold || !history || !history.length) return null;
        const last = history[history.length - 1];
        const d = nm(threshold, last);
        if (d < FROM_NM) return null;
        const recent = history.filter((s) => now - s.t <= WINDOW_MS && Number.isFinite(s.track));
        if (recent.length < MIN_SAMPLES) return null;
        const scored = exits.map((e) => ({
            point: e.point, lat: e.lat, lon: e.lon, routes: e.routes,
            angle: median(recent.map((s) => angleOff(bearing(s, e), s.track))),
        })).sort((a, b) => a.angle - b.angle);
        const best = scored[0];
        if (best.angle > GUIDED_DEG) return null;
        if (nm(last, best) < PASSED_NM || d > nm(threshold, best) + PASSED_NM) return null;
        const T = temperature(d);
        const w = scored.map((s) => Math.exp(-s.angle / T));
        const sum = w.reduce((a, b) => a + b, 0);
        scored.forEach((s, i) => { s.p = w[i] / sum; });
        const margin = scored.length > 1 ? scored[1].angle - best.angle : Infinity;
        const alone = margin >= ALONE_MARGIN_DEG;
        const candidates = alone ? [best] : scored.filter((s) => s.p >= KEEP_P);
        return { candidates, alone, distNM: d, toExitNM: nm(last, best) };
    }

    // An arrival's runway as the label says it: nothing beyond 20 NM (the
    // sequence's cone), the pair until 15 NM where the airport has one.
    function arrivalText(runway, distNM) {
        if (!runway || !Number.isFinite(distNM)) return '';
        const m = /^(\d{2})([LRC])$/.exec(runway);
        if (m && distNM > PAIR_UNTIL_NM) return `${m[1]}L/R`;
        return runway;
    }

    // The label's words: "→OKASI" and the count of the others ("+2"), or
    // the runway.
    function labelText(prediction) {
        if (!prediction || !prediction.candidates.length) return null;
        const others = prediction.candidates.length - 1;
        return { main: '→' + prediction.candidates[0].point, rest: others > 0 ? ` +${others}` : '' };
    }

    return {
        exitsFor, thresholdOf, departure, arrivalText, labelText, temperature,
        FROM_NM, PAIR_UNTIL_NM, WINDOW_MS, KEEP_P, ALONE_MARGIN_DEG,
    };
});
