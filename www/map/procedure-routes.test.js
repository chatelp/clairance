// node --test www/map/procedure-routes.test.js
// The published routes on the map (ProcedureRoute, Q56).
const test = require('node:test');
const assert = require('node:assert/strict');
const R = require('./procedure-routes.js');

const okasi = {
    airport: 'LFPG', name: 'OKASI 7B', kind: 'SID', runways: ['26L', '26R'],
    thresholds: [{ runway: '26L', lat: 48.99488, lon: 2.60244 }, { runway: '26R', lat: 48.9987, lon: 2.60916 }],
    legs: [
        { point: 'PG263', lat: 48.99, lon: 2.46, path: 'CF' },
        { point: 'DOPAP', lat: 48.70, lon: 2.26, named: true, path: 'TF', min_alt: 'FL060' },
        { point: 'OKASI', lat: 48.08, lon: 2.78, named: true, path: 'TF', max_alt: '4000', max_kt: 250 },
    ],
    complete: true,
};
const okasiEast = Object.assign({}, okasi, { name: 'OKASI 6G', runways: ['09L', '09R'], thresholds: [] });
const enori = { airport: 'LFPG', name: 'ENORI 9E', kind: 'STAR', runways: ['26L', '26R', '27L', '27R'], legs: [{ point: 'ENORI' }, { point: 'LORNI', named: true }], complete: true };
const lfpg = (end, p = 1) => ({ code: 'LFPG', runway_in_use: [{ runway_end: '08R-26L/' + end, probability: p }] });

test('the runways in use, from /station', () => {
    assert.deepEqual(R.inUse(lfpg('26L')), ['26L']);
    assert.deepEqual(R.inUse(lfpg('26L', 0.1)), [], 'below 15 % of the evidence, not in use');
});

test('a departure from the doublet: the runway in use when it says which, both otherwise', () => {
    assert.deepEqual(R.runwaysFor(okasi, ['26L']), ['26L']);
    assert.deepEqual(R.runwaysFor(okasi, ['27R']), ['26L', '26R']);
    assert.equal(R.note(okasi, ['26L']), 'OKASI 7B · published departure from 26L');
    assert.equal(R.note(okasi, ['26L', '26R']), 'OKASI 7B · published departure from 26L or 26R');
    assert.match(R.help(okasi, ['26R']), /^The published route of OKASI 7B, a departure from 26R\. The controller often clears/);
});

test('two starts meet at the first point', () => {
    const ls = R.lines(okasi, ['26L', '26R']);
    assert.equal(ls.length, 2);
    assert.equal(ls[0][0].name, '26L');
    assert.equal(ls[0].length, 4, 'the first start carries the route');
    assert.deepEqual(ls[1].map((p) => p.name), ['26R', 'PG263']);
    assert.equal(R.lines(okasi, ['26L']).length, 1);
});

test('constraints in the brand\'s numbers', () => {
    assert.deepEqual(R.constraints({ min_alt: 'FL060' }), ['≥ FL 060']);
    assert.deepEqual(R.constraints({ max_alt: '4000', max_kt: 250 }), ['≤ 4 000 ft', '≤ 250 kt']);
    assert.deepEqual(R.constraints({ min_alt: 'FL070', max_alt: 'FL070' }), ['FL 070']);
    assert.deepEqual(R.constraints({}), []);
});

test('the route a chip asks for: the one serving the runways in use', () => {
    const west = Object.assign({}, okasi, { name: 'X 1A' });
    const east = Object.assign({}, okasiEast, { name: 'X 1A' });
    assert.equal(R.find([east, west], 'X 1A', [lfpg('26L')]), west);
    assert.equal(R.find([east, west], 'X 1A', [lfpg('09L')]), east);
    assert.equal(R.find([okasi], 'ATREX 7K', [lfpg('26L')]), null);
});

test('the layer: the configuration in use, said; nothing while it is not known; nothing outside the eAIP', () => {
    const all = [okasi, okasiEast, enori];
    const west = R.layer(all, [lfpg('26L')], 'SID');
    assert.equal(west.state, 'shown');
    assert.deepEqual(west.routes, [okasi]);
    assert.equal(west.text, 'LFPG west (26L), from the runways in use');
    assert.deepEqual(R.layer(all, [lfpg('26L')], 'STAR').routes, [enori]);
    assert.equal(R.layer(all, [{ code: 'LFPG', runway_in_use: [] }], 'SID').state, 'unknown');
    assert.equal(R.layer(all, [{ code: 'EGLL' }], 'SID').text, R.OUTSIDE);
});

test('the layer, every runway: all the routes of the kind, whatever is in use, even before it is known', () => {
    const all = [okasi, okasiEast, enori];
    const every = R.layer(all, [lfpg('26L')], 'SID', 'all');
    assert.equal(every.state, 'shown');
    assert.deepEqual(every.routes, [okasi, okasiEast]);
    assert.equal(every.text, 'LFPG, every runway');
    assert.deepEqual(R.layer(all, [{ code: 'LFPG', runway_in_use: [] }], 'SID', 'all').routes, [okasi, okasiEast]);
    assert.equal(R.layer(all, [{ code: 'EGLL' }], 'SID', 'all').text, R.OUTSIDE);
});

test('a name goes away from the route', () => {
    // A corner: the route comes from the west and leaves to the south; the
    // name goes north-east (screen y down).
    const s = R.labelSide([-10, 0], [0, 0], [0, 10]);
    assert.ok(s[0] > 0 && s[1] < 0, `north-east: ${s}`);
    const straight = R.labelSide([-10, 0], [0, 0], [10, 0]);
    assert.ok(Math.abs(straight[0]) < 1e-9 && Math.abs(Math.abs(straight[1]) - 1) < 1e-9, 'a straight line: across it');
});

test('an arrival: its note, and its help says where the approach begins', () => {
    assert.equal(R.note(enori, ['26L', '26R']), 'ENORI 9E · published arrival to 26L or 26R');
    assert.match(R.help(enori, ['26L']), /It ends at LORNI, where the approach begins: from there the controller guides the aircraft to the runway\./);
});

test('a point several routes pass is named once, in the widest gap between them', () => {
    // Lines leave east, north and west (screen y down): the widest gap is south.
    const s = R.widestGap([0, 0], [[10, 0], [0, -10], [-10, 0]]);
    assert.ok(Math.abs(s[0]) < 1e-9 && Math.abs(s[1] - 1) < 1e-9, `south: ${s}`);
    const pts = R.layerPoints([okasi, Object.assign({}, okasi, { name: 'OKASI 7E' })], () => ['26L']);
    assert.deepEqual(pts.map((p) => p.name), ['DOPAP', 'OKASI']);
    assert.equal(pts[0].ends, false);
    assert.equal(pts[1].ends, true, 'a departure ends at its exit');
    assert.equal(pts[0].neighbours.length, 4, 'two routes, a point before and after on each');
});
