// node --test www/accounts-panel.test.js
// The owner's list of accounts (accounts-panel.js): one line per account,
// read at a glance.
const test = require('node:test');
const assert = require('node:assert/strict');
const { accountAgo, accountFacts, ACCOUNT_ROLES } = require('./accounts-panel.js');

const now = Date.parse('2026-10-27T15:00:00Z');

test('how long since, in the units a glance wants', () => {
    assert.equal(accountAgo('2026-10-27T14:59:30Z', now), 'just now');
    assert.equal(accountAgo('2026-10-27T14:57:00Z', now), '3 min ago');
    assert.equal(accountAgo('2026-10-27T12:00:00Z', now), '3 h ago');
    assert.match(accountAgo('2026-10-25T10:05:00Z', now), /^\d\d\/10 \d\d:05$/);
    assert.equal(accountAgo('', now), '');
    assert.equal(accountAgo('not a date', now), '');
});

test('an account signed in: its sessions, when last seen, where it signed in from', () => {
    const a = { sessions: 2, last_seen: '2026-10-27T14:58:00Z', source: 'file',
        last_sign_in: { at: '2026-10-27T13:00:00Z', from: '203.0.113.7' } };
    assert.equal(accountFacts(a, now), '2 sessions · seen 2 min ago · signed in 2 h ago from 203.0.113.7');
});

test('an account not signed in says so, and where it is written when not by the page', () => {
    assert.equal(accountFacts({ sessions: 0, source: 'config' }, now), 'not signed in · written in config.toml');
    assert.equal(accountFacts({ sessions: 0, last_seen: '2026-10-27T14:00:00Z', source: 'file' }, now), 'not signed in');
});

test('an owner is the admin, in the owner\'s words', () => {
    assert.equal(ACCOUNT_ROLES.owner, 'admin');
    assert.equal(ACCOUNT_ROLES.viewer, 'viewer');
});
