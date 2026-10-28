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
    ]);
});

test('a viewer sees This browser and Listening, nothing of Administration', () => {
    assert.deepEqual(names(settingsNav(false)), [
        'This browser: Display, Account',
        'Station: Listening',
    ]);
});

test('every page has a sentence, and the server pages are marked so', () => {
    for (const p of SETTINGS_PAGES) assert.ok(p.lede && p.lede.endsWith('.'), p.id);
    assert.deepEqual(SETTINGS_PAGES.filter((p) => p.server).map((p) => p.id), ['listening', 'matching', 'access', 'server']);
});
