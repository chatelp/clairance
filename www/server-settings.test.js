// node --test www/server-settings.test.js
// The settings' pages (D86, 28/10): three groups for an admin, and for a
// viewer This browser and Station › Listening only.
const test = require('node:test');
const assert = require('node:assert/strict');
const { SETTINGS_PAGES, settingsNav } = require('./server-settings.js');

const names = (groups) => groups.map((g) => g.name + ': ' + g.pages.map((p) => p.label).join(', '));

test('an admin sees every page, in three groups', () => {
    assert.deepEqual(names(settingsNav(true)), [
        'This browser: Display, Account',
        'Station: Listening, Matching',
        'Administration: Access, Usage, Server',
        ': About Clairance',
    ]);
});

test('a viewer sees This browser and Listening, nothing of Administration', () => {
    assert.deepEqual(names(settingsNav(false)), [
        'This browser: Display, Account',
        'Station: Listening',
        ': About Clairance',
    ]);
});

test('every page has a sentence, and the server pages are marked so', () => {
    for (const p of SETTINGS_PAGES) assert.ok(p.lede && p.lede.endsWith('.'), p.id);
    assert.deepEqual(SETTINGS_PAGES.filter((p) => p.server).map((p) => p.id), ['listening', 'matching', 'access', 'server']);
});

// "Also follow" (fiche T, 28/10): the followed and the nearest first, the rest folded.
const { alsoSplit, alsoLines } = require('./server-settings.js');

const codes = (list) => list.map((c) => c.code);
// 21 aerodromes, LF01 nearest to LF21 farthest, given out of order.
const twentyOne = Array.from({ length: 21 }, (_, i) => ({ code: 'LF' + String(i + 1).padStart(2, '0'), distance_nm: 3 + i * 2 })).reverse();
const nearest = (k) => codes(twentyOne).sort().slice(0, k);

function assertWhole(split, choices) {
    const all = codes(split.shown).concat(codes(split.more));
    assert.equal(new Set(all).size, all.length, 'no aerodrome twice');
    assert.deepEqual(all.slice().sort(), codes(choices).sort(), 'no aerodrome lost');
}

test('Also follow: nothing followed shows the five nearest, the sixteen others folded by distance', () => {
    const s = alsoSplit(twentyOne, [], 5);
    assert.deepEqual(codes(s.shown), nearest(5));
    assert.deepEqual(codes(s.more), nearest(21).slice(5));
    assertWhole(s, twentyOne);
});

test('Also follow: one or three followed among the near ones complete the five, in distance order', () => {
    let s = alsoSplit(twentyOne, ['LF03'], 5);
    assert.deepEqual(codes(s.shown), nearest(5));
    assertWhole(s, twentyOne);
    s = alsoSplit(twentyOne, ['LF02', 'LF04', 'LF07'], 5);
    assert.deepEqual(codes(s.shown), ['LF01', 'LF02', 'LF03', 'LF04', 'LF07']);
    assert.equal(s.more[0].code, 'LF05');
    assertWhole(s, twentyOne);
});

test('Also follow: a far followed aerodrome is shown, the nearest others fill the rest', () => {
    const s = alsoSplit(twentyOne, ['LF21'], 5);
    assert.deepEqual(codes(s.shown), ['LF01', 'LF02', 'LF03', 'LF04', 'LF21']);
    assert.deepEqual(codes(s.more), nearest(20).slice(4));
    assertWhole(s, twentyOne);
    const three = alsoSplit(twentyOne, ['LF19', 'LF20', 'LF21'], 5);
    assert.deepEqual(codes(three.shown), ['LF01', 'LF02', 'LF19', 'LF20', 'LF21']);
    assertWhole(three, twentyOne);
});

test('Also follow: fewer choices than five fold nothing; none gives two empty lists', () => {
    const few = twentyOne.slice(-3);
    const s = alsoSplit(few, ['LF02'], 5);
    assert.deepEqual(codes(s.shown), ['LF01', 'LF02', 'LF03']);
    assert.deepEqual(s.more, []);
    assert.deepEqual(alsoSplit([], [], 5), { shown: [], more: [] });
    assert.deepEqual(alsoSplit(undefined, undefined, 5), { shown: [], more: [] });
});

test('Also follow: equal distances keep the order of the codes', () => {
    const tied = ['LFPZ', 'LFPB', 'LFPN', 'LFPV', 'LFPO', 'LFPG'].map((code) => ({ code, distance_nm: 10 }));
    const s = alsoSplit(tied, ['LFPZ'], 5);
    assert.deepEqual(codes(s.shown), ['LFPB', 'LFPG', 'LFPN', 'LFPO', 'LFPZ']);
    assert.deepEqual(codes(s.more), ['LFPV']);
    assertWhole(s, tied);
});

// During a visit, the line keeps what it opened with: a box ticked from the
// fold comes up, a box unticked on the line stays (design, 28/10).
test('the line holds still during a visit to Listening', () => {
    const choices = [
        { code: 'LFPN', distance_nm: 3 }, { code: 'LFPV', distance_nm: 5 }, { code: 'LFXU', distance_nm: 8 },
        { code: 'LFPO', distance_nm: 10 }, { code: 'LFPT', distance_nm: 12 }, { code: 'LFPG', distance_nm: 20 },
        { code: 'LFOB', distance_nm: 40 },
    ];
    const codes = (l) => l.map((c) => c.code);
    const line = codes(alsoSplit(choices, ['LFOB'], 5).shown);
    assert.deepEqual(line, ['LFPN', 'LFPV', 'LFXU', 'LFPO', 'LFOB']);
    // LFOB unticked: it stays on the line
    assert.deepEqual(codes(alsoLines(choices, [], line).shown), ['LFPN', 'LFPV', 'LFXU', 'LFPO', 'LFOB']);
    // LFPG ticked from the fold: it comes up, six on the line for the visit
    const now = alsoLines(choices, ['LFPG'], line);
    assert.deepEqual(codes(now.shown), ['LFPN', 'LFPV', 'LFXU', 'LFPO', 'LFPG', 'LFOB']);
    assert.deepEqual(codes(now.more), ['LFPT']);
});

// The folder picker (FolderPicker, 28/10): the path's parts, the place that
// holds it, what choosing it means, and the rows' keys.
const { folderCrumbs, folderPlace, folderFoot, folderStep } = require('./server-settings.js');

test('a path as buttons back up to each of its parts', () => {
    assert.deepEqual(folderCrumbs('/Volumes/Archive/clairance'), [
        { name: '/', path: '/' },
        { name: 'Volumes', path: '/Volumes' },
        { name: 'Archive', path: '/Volumes/Archive' },
        { name: 'clairance', path: '/Volumes/Archive/clairance' },
    ]);
    assert.deepEqual(folderCrumbs('/'), [{ name: '/', path: '/' }]);
});

test('the place that holds a folder is the longest root it starts with', () => {
    const roots = [{ name: 'Archive', path: '/Volumes/Archive' }, { name: 'Macintosh HD', path: '/Volumes/Macintosh HD' }, { name: 'Home', path: '/Users/pierre' }];
    assert.equal(folderPlace(roots, '/Volumes/Archive/clairance').name, 'Archive');
    assert.equal(folderPlace(roots, '/Volumes/Archive').name, 'Archive');
    assert.equal(folderPlace(roots, '/Volumes/Archive2'), null); // a prefix of the name is not a parent
    assert.equal(folderPlace(roots, '/Users/pierre/x').name, 'Home');
    assert.equal(folderPlace(roots, '/Volumes'), null);
});

test('the foot says what choosing the open folder means', () => {
    const tb = (n) => (n / 1024 ** 4).toFixed(1) + ' TB';
    const place = { name: 'Archive', free_bytes: 1.8 * 1024 ** 4 };
    assert.deepEqual(folderFoot({ choosable: true }, place, tb), { warn: false, text: '1.8 TB free on Archive.' });
    assert.deepEqual(folderFoot({ choosable: false, why: 'databases' }, place, tb),
        { warn: true, text: 'The databases are kept here: choose a folder outside, best on another disk.' });
    assert.deepEqual(folderFoot({ choosable: false, why: 'readonly' }, place, tb),
        { warn: true, text: 'The server cannot write in this folder: choose another.' });
    assert.deepEqual(folderFoot({ choosable: true }, null, tb), { warn: false, text: '' });
    assert.equal(folderFoot(null, place, tb), null);
});

test('the rows answer up, down, Home and End, and stop at the ends', () => {
    assert.equal(folderStep('ArrowDown', 0, 3), 1);
    assert.equal(folderStep('ArrowDown', 2, 3), 2);
    assert.equal(folderStep('ArrowUp', 0, 3), 0);
    assert.equal(folderStep('ArrowDown', -1, 3), 0); // from the list itself, the first row
    assert.equal(folderStep('End', 0, 3), 2);
    assert.equal(folderStep('Home', 2, 3), 0);
    assert.equal(folderStep('Enter', 1, 3), -1);
    assert.equal(folderStep('ArrowDown', 0, 0), -1);
});
