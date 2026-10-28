// node --test www/live-player.test.js
// The page's resampler (live-player.js): the stream's 8 kHz brought to the
// context's rate with no join between chunks, which Safari heard as clicks.
const test = require('node:test');
const assert = require('node:assert/strict');

globalThis.window = globalThis;
require('./live-player.js');
const R = window.LiveStreamPlayer.Resampler;

function tone(n, rate) {
    const t = new Float32Array(n);
    for (let i = 0; i < n; i++) t[i] = 0.5 * Math.sin(2 * Math.PI * 440 * i / rate);
    return t;
}

for (const to of [44100, 48000]) {
    test(`8 kHz to ${to} Hz: chunks resampled in turn are the whole resampled at once`, () => {
        const input = tone(8000, 8000);
        const whole = new R(8000, to).push(input);
        const r = new R(8000, to);
        const parts = [];
        for (let k = 0; k < 10; k++) parts.push(...r.push(input.subarray(k * 800, (k + 1) * 800)));
        // The last position can round either way: one sample, no more.
        assert.ok(Math.abs(parts.length - whole.length) <= 1, `${parts.length} against ${whole.length}`);
        let worst = 0;
        for (let i = 0; i < Math.min(parts.length, whole.length); i++) worst = Math.max(worst, Math.abs(parts[i] - whole[i]));
        assert.ok(worst < 1e-6, `largest difference ${worst}`);
    });

    test(`8 kHz to ${to} Hz: a tone comes out as the same tone`, () => {
        const out = new R(8000, to).push(tone(8000, 8000));
        let worst = 0;
        for (let j = Math.ceil(32 * to / 8000); j < out.length - Math.ceil(16 * to / 8000); j++) {
            worst = Math.max(worst, Math.abs(out[j] - 0.5 * Math.sin(2 * Math.PI * 440 * j / to)));
        }
        assert.ok(worst < 1e-3, `largest error ${worst}`);
    });
}

// The owner's iPhone, 28/10: started before its address was set, the player
// fetched '' -- the page itself, in HTML -- and retried it forever.
test('a player with no address fetches nothing', async () => {
    const fetched = [];
    const was = globalThis.fetch;
    globalThis.fetch = (u) => { fetched.push(u); return new Promise(() => {}); };
    try {
        const ctx = { state: 'suspended', sampleRate: 48000, currentTime: 0, createGain: () => ({ gain: { value: 1 }, connect() {} }), destination: {} };
        const p = new window.LiveStreamPlayer(ctx);
        await p.play();
        assert.deepEqual(fetched, []);
        p.src = '/api/v1/stream/124350?format=ulaw';
        await p.play();
        assert.deepEqual(fetched, ['/api/v1/stream/124350?format=ulaw']);
        p.pause();
    } finally {
        globalThis.fetch = was;
    }
});
