// node --test www/map/probable-exits.test.js
// The probable runway and exit (Q57).
const test = require('node:test');
const assert = require('node:assert/strict');
const X = require('./probable-exits.js');

// A made-up airport at 49 N 2.5 E, runway 26L, two exits: one due west
// (WESTA, ~60 NM out) and one west-south-west (SOUWE).
const thr = { runway: '26L', lat: 49.0, lon: 2.6 };
const route = (name, point, lat, lon, extra = {}) => Object.assign({
    airport: 'LFXX', name, kind: 'SID', runways: ['26L', '26R'], thresholds: [thr],
    legs: [{ point: 'PG263', lat: 49.0, lon: 2.4 }, { point, lat, lon, named: true }],
}, extra);
const routes = [
    route('WESTA 1A', 'WESTA', 49.0, 1.07),
    route('WESTA 1B', 'WESTA', 49.0, 1.07),
    route('SOUWE 1A', 'SOUWE', 48.75, 1.2),
    route('ARRIV 1A', 'ARRIV', 49.5, 3.5, { kind: 'STAR' }),
    route('TECHN 1A', 'PG999', 48.9, 2.0),
];
const dep = { airport: 'LFXX', runway: '26L' };

// A track flown west along 49 N from lon0, one fix a second.
function westbound(lon0, n, track = 270, now = 100000) {
    const h = [];
    for (let i = 0; i < n; i++) h.push({ t: now - (n - 1 - i) * 1000, lat: 49.0, lon: lon0 - i * 0.002, track });
    return h;
}

test('a departure\'s exits: its runway\'s SIDs, one per point, no technical point', () => {
    const exits = X.exitsFor(routes, dep);
    assert.deepEqual(exits.map((e) => e.point).sort(), ['SOUWE', 'WESTA']);
    assert.equal(exits.find((e) => e.point === 'WESTA').routes.length, 2);
    assert.deepEqual(X.thresholdOf(routes, dep), { lat: 49.0, lon: 2.6 });
});

test('nothing before 15 NM', () => {
    const exits = X.exitsFor(routes, dep);
    // ~10 NM west of the threshold.
    const p = X.departure({ history: westbound(2.35, 40), exits, threshold: thr, now: 100000 });
    assert.equal(p, null);
});

test('heading straight for an exit: it alone, once its margin passes 10 degrees', () => {
    const exits = X.exitsFor(routes, dep);
    // ~20 NM west, flying 270: WESTA dead ahead, SOUWE some 30 degrees off.
    const p = X.departure({ history: westbound(2.08, 40), exits, threshold: thr, now: 100000 });
    assert.ok(p, 'a prediction');
    assert.equal(p.alone, true);
    assert.deepEqual(p.candidates.map((c) => c.point), ['WESTA']);
    assert.deepEqual(X.labelText(p), { main: '→WESTA', rest: '' });
});

test('between two exits: the likelier first, and the count of the others', () => {
    const exits = X.exitsFor(routes, dep);
    // Flying 258: between WESTA (~270) and SOUWE (~246).
    const p = X.departure({ history: westbound(2.08, 40, 258), exits, threshold: thr, now: 100000 });
    assert.ok(p && !p.alone);
    assert.equal(p.candidates.length, 2);
    assert.ok(p.candidates[0].p >= p.candidates[1].p);
    assert.equal(X.labelText(p).rest, ' +1');
});

test('guided on a heading, or past its exit: nothing', () => {
    const exits = X.exitsFor(routes, dep);
    assert.equal(X.departure({ history: westbound(2.08, 40, 330), exits, threshold: thr, now: 100000 }), null, 'heading north-west');
    assert.equal(X.departure({ history: westbound(1.0, 40), exits, threshold: thr, now: 100000 }), null, 'beyond WESTA');
});

test('an arrival: the pair until 15 NM, the runway after; an airport without a pair at once', () => {
    assert.equal(X.arrivalText('26L', 18), '26L/R');
    assert.equal(X.arrivalText('26L', 12), '26L');
    assert.equal(X.arrivalText('25', 18), '25');
    assert.equal(X.arrivalText('26L', NaN), '');
});
