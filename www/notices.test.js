// node --test www/notices.test.js
// The page's notices (notices.js, D87): what is new to an account, once.
const test = require('node:test');
const assert = require('node:assert/strict');
const { newsUnseen, newsDate, listeningSentence } = require('./notices.js');

const news = [
    { id: 'c', date: '2026-10-28', lines: ['c1', 'c2', 'c3'] },
    { id: 'b', date: '2026-10-20', lines: ['b1', 'b2'] },
    { id: 'a', date: '2026-10-16', lines: ['a1'] },
];

test('the lines not seen yet, newest first, four at most', () => {
    assert.deepEqual(newsUnseen(news, 'a'), { date: '2026-10-28', lines: ['c1', 'c2', 'c3', 'b1'], newest: 'c' });
    assert.deepEqual(newsUnseen(news, 'b'), { date: '2026-10-28', lines: ['c1', 'c2', 'c3'], newest: 'c' });
});

test('nothing once the newest is seen; the newest alone with none seen, or an id gone', () => {
    assert.equal(newsUnseen(news, 'c'), null);
    assert.deepEqual(newsUnseen(news, '').lines, ['c1', 'c2', 'c3']);
    assert.deepEqual(newsUnseen(news, 'gone').lines, ['c1', 'c2', 'c3']);
    assert.equal(newsUnseen([], ''), null);
});

test('a version\'s date, and what a playing frequency is told', () => {
    assert.equal(newsDate('2026-10-28'), '28 October');
    assert.equal(newsDate('2026-10-28', true), '28 October 2026');
    assert.equal(listeningSentence([]), '');
    assert.equal(listeningSentence(['124.350']), ' You are listening to 124.350: the sound stops for a moment, and a click starts it again.');
    assert.match(listeningSentence(['124.350', '126.425']), /to 124\.350 and 126\.425:/);
});
