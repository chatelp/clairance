// node --test www/usage-panel.test.js
// The owner's view of what each account did (usage-panel.js, D84): lines read
// at a glance, and a bar a day.
const test = require('node:test');
const assert = require('node:assert/strict');
const { usageDuration, usageFreq, usageNowLine, usageWeekLine, usageInterruptionLine, usageBars } = require('./usage-panel.js');

test('a duration in the units a glance wants', () => {
    assert.equal(usageDuration(0), '0 s');
    assert.equal(usageDuration(40), '40 s');
    assert.equal(usageDuration(720), '12 min');
    assert.equal(usageDuration(3600 + 5 * 60), '1 h 05');
    assert.equal(usageDuration(null), '0 s');
});

test('a frequency as the radio says it', () => {
    assert.equal(usageFreq('124350'), '124.350');
    assert.equal(usageFreq('station'), 'station');
});

test('someone here now: the page, and what they hear or not', () => {
    const now = Date.parse('2026-10-27T17:30:00Z');
    assert.equal(usageNowLine({ pages: 1, since: '2026-10-27T17:18:00Z', listening: [{ freq: '124350' }, { freq: '126425' }] }, now),
        'page open 12 min · listening to 124.350, 126.425');
    assert.equal(usageNowLine({ pages: 2, since: '2026-10-27T17:29:20Z', listening: [] }, now), '2 pages open 40 s · sound off');
    assert.equal(usageNowLine({ pages: 0, listening: [{ freq: '124350' }] }, now), 'listening to 124.350');
});

test('a week: connected, listened by frequency, clips, visits', () => {
    assert.equal(usageWeekLine({ week: { connected_s: 11400, listening_s: 7500, by_freq_s: { '124350': 6000, '126425': 4800 }, clips: 4, visits: 3 } }),
        'connected 3 h 10 · listened 2 h 05 (124.350 1 h 40, 126.425 1 h 20) · 4 clips played again · 3 visits');
    assert.equal(usageWeekLine({ week: { connected_s: 600, listening_s: 300, by_freq_s: { '124350': 300 }, clips: 1, visits: 1 } }),
        'connected 10 min · listened 5 min (124.350) · 1 clip played again · 1 visit');
    assert.equal(usageWeekLine({ week: { connected_s: 30, listening_s: 0, by_freq_s: {}, clips: 0, visits: 1 } }),
        'connected 30 s · no sound · 1 visit');
});

test('an interruption: when, what, why, and whether it came back', () => {
    const line = usageInterruptionLine({ at: '2026-10-27T15:59:09Z', kind: 'sound', freq: '124350', why: 'stopped', back_after_s: 15 });
    assert.match(line, /^27\/10 \d\d:59 · sound 124\.350 · server stopped · back in 15 s$/);
    assert.match(usageInterruptionLine({ at: '2026-10-27T15:59:09Z', kind: 'page', why: 'source', back_after_s: null }),
        / · page · station stream ended · not back within 10 min$/);
});

test('a bar a day, against the busiest day', () => {
    const bars = usageBars([
        { date: '2026-10-26', connected_s: 600, listening_s: 300, clips: 0 },
        { date: '2026-10-27', connected_s: 1200, listening_s: 1200, clips: 2 },
    ]);
    assert.deepEqual(bars.map((b) => [b.label, b.connected, b.listening]), [['Mo', 50, 25], ['Tu', 100, 100]]);
    assert.equal(bars[1].title, '27/10: connected 20 min, listened 20 min, 2 clips');
    assert.equal(usageBars([{ date: '2026-10-25', connected_s: 0, listening_s: 0 }])[0].connected, 0);
});
