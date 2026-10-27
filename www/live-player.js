// The page plays each frequency itself (docs-fr/35-plan.md, D2 ter, decided
// by the owner on 26/10). An <audio> element left the browser in charge of a
// live stream, and every browser did it its own way: Chromium waited 17 s
// before playing the station's MP3, Safari played it past Web Audio (the mute
// button muted nothing and "someone is speaking" never showed), an iPhone
// played nothing. Here the page reads the decoded sound the server sends in
// G.711 mu-law (?format=ulaw, audio/basic, its rate in X-Sample-Rate),
// decodes it with a table, and schedules it through Web Audio with a reserve
// of LEAD seconds: it starts at once, the same in every browser, and what the
// analyser measures is what comes out of the speakers.
//
// LiveStreamPlayer stands where audio-client.js had an <audio> element: src,
// play(), pause(), paused, volume, readyState, the playing, pause and error
// events. Its sound leaves through `output`, which audio-client.js connects
// to the analyser and the mute gain. A stream that ends -- a restart, a
// station switching -- is taken again after a pause, as long as it plays.
(function () {
    'use strict';

    // G.711 mu-law to [-1, 1].
    const ULAW = new Float32Array(256);
    for (let i = 0; i < 256; i++) {
        const u = ~i & 0xff;
        const sign = u & 0x80;
        const exp = (u >> 4) & 0x07;
        const mant = u & 0x0f;
        let s = (((mant << 3) + 0x84) << exp) - 0x84;
        ULAW[i] = (sign ? -s : s) / 32768;
    }

    // The stream's rate brought to the context's, here and continuously. A
    // buffer at 8 kHz is resampled by the browser on its own, edges included:
    // in WebKit each 0.1 s chunk joined the next with a step (0.02 on a tone of
    // 0.5, nothing between the joins, measured offline on 26/10) and Safari
    // clicked ten times a second. A windowed sinc whose history runs from one
    // chunk to the next has no joins.
    const HALF = 16;          // taps on each side of the output sample
    const TABLE = 256;        // kernel samples per input sample
    class Resampler {
        constructor(inRate, outRate) {
            this.inRate = inRate;
            this.step = inRate / outRate;
            this.cut = Math.min(1, outRate / inRate);
            this.kernel = new Float32Array(HALF * TABLE + 2);
            for (let i = 0; i < this.kernel.length; i++) {
                const x = i / TABLE;
                const c = this.cut * x;
                const sinc = c === 0 ? 1 : Math.sin(Math.PI * c) / (Math.PI * c);
                const w = x < HALF ? 0.42 + 0.5 * Math.cos(Math.PI * x / HALF) + 0.08 * Math.cos(2 * Math.PI * x / HALF) : 0;
                this.kernel[i] = this.cut * sinc * w;
            }
            this.buf = new Float32Array(HALF);   // silence before the first sample
            this.t = HALF;                       // where the next output falls, in input samples
        }

        _k(d) {
            const x = Math.abs(d) * TABLE;
            const i = Math.floor(x);
            if (i >= HALF * TABLE) return 0;
            const f = x - i;
            return this.kernel[i] * (1 - f) + this.kernel[i + 1] * f;
        }

        push(samples) {
            const buf = new Float32Array(this.buf.length + samples.length);
            buf.set(this.buf); buf.set(samples, this.buf.length);
            const n = Math.max(0, Math.ceil((buf.length - HALF - this.t) / this.step));
            const out = new Float32Array(n);
            let t = this.t;
            for (let j = 0; j < n; j++, t += this.step) {
                const c = Math.floor(t);
                let y = 0;
                for (let k = c - HALF + 1; k <= c + HALF; k++) {
                    if (k >= 0 && k < buf.length) y += buf[k] * this._k(t - k);
                }
                out[j] = y;
            }
            const drop = Math.max(0, Math.floor(t) - HALF);
            this.buf = buf.slice(drop);
            this.t = t - drop;
            return out;
        }
    }

    const LEAD = 0.3;        // seconds held in reserve against the network's jitter
    const MAX_AHEAD = 2.0;   // more queued than this and a chunk is dropped: latency stays bounded
    const CHUNK = 0.1;       // seconds of sound gathered into one scheduled buffer
    const RETRY_MS = [1000, 2000, 5000, 10000];

    class LiveStreamPlayer {
        constructor(context) {
            this.isLivePlayer = true;
            this.ctx = context;
            this.output = context.createGain();
            this._volume = 1;
            this.muted = false;      // the mute is audio-client.js's gain, after the analyser
            this.paused = true;
            this.src = '';
            this.currentSrc = '';
            this.readyState = 0;
            this.error = null;
            this.crossOrigin = null; // kept for the <audio> element's callers
            this.preload = '';
            this.playsInline = true;
            this._listeners = {};
            this._run = 0;           // the current connection; an older one stops itself
            this._next = 0;
            this._acc = [];
            this._accLength = 0;
            this._retry = 0;
        }

        get volume() { return this._volume; }
        set volume(v) {
            this._volume = Math.max(0, Math.min(1, Number(v) || 0));
            this.output.gain.value = this._volume;
        }

        addEventListener(type, cb) { (this._listeners[type] = this._listeners[type] || []).push(cb); }
        _emit(type) {
            for (const cb of this._listeners[type] || []) {
                try { cb({ type, target: this }); } catch (e) { console.error(e); }
            }
        }

        load() {}

        play() {
            if (!this.paused && this.currentSrc === this.src) return Promise.resolve();
            this.paused = false;
            this._connect();
            return Promise.resolve();
        }

        pause() {
            if (this.paused) return;
            this.paused = true;
            this._stop();
            this._emit('pause');
        }

        _stop() {
            this._run++;
            if (this._abort) { try { this._abort.abort(); } catch (e) { /* gone */ } }
            this._abort = null;
            clearTimeout(this._retryTimer);
            this._acc = []; this._accLength = 0;
            this._resampler = null;
            this.readyState = 0;
        }

        async _connect() {
            this._stop();
            const run = this._run;
            this.currentSrc = this.src;
            const abort = new AbortController();
            this._abort = abort;
            let announced = false;
            try {
                // 'include', not 'same-origin': the streams are spread over the
                // server's additional ports, another origin, and with sign-in on
                // the cookie must go there too -- three frequencies in four were
                // refused for a signed-in owner on the local network (fiche S, 26/10).
                const r = await fetch(this.src, { credentials: 'include', cache: 'no-store', signal: abort.signal });
                if (!r.ok) throw new Error(`HTTP ${r.status}`);
                const type = (r.headers.get('Content-Type') || '').toLowerCase();
                let rate = parseInt(r.headers.get('X-Sample-Rate'), 10) || 8000;
                // A source with no mu-law (raw PCM over UDP) answers WAV: 16-bit
                // PCM after a 44-byte header that says its rate.
                const wav = type.startsWith('audio/wav');
                if (!wav && !type.startsWith('audio/basic')) throw new Error(`unexpected ${type}`);
                const reader = r.body.getReader();
                let header = wav ? new Uint8Array(0) : null;
                let odd = null;
                for (;;) {
                    const { value, done } = await reader.read();
                    if (done || run !== this._run) break;
                    let bytes = value;
                    if (header) {
                        const all = new Uint8Array(header.length + bytes.length);
                        all.set(header); all.set(bytes, header.length);
                        if (all.length < 44) { header = all; continue; }
                        rate = new DataView(all.buffer).getUint32(24, true) || rate;
                        bytes = all.subarray(44);
                        header = null;
                    }
                    let samples;
                    if (wav) {
                        if (odd !== null) {
                            const all = new Uint8Array(bytes.length + 1);
                            all[0] = odd; all.set(bytes, 1); bytes = all; odd = null;
                        }
                        if (bytes.length % 2) { odd = bytes[bytes.length - 1]; bytes = bytes.subarray(0, bytes.length - 1); }
                        const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.length);
                        samples = new Float32Array(bytes.length / 2);
                        for (let i = 0; i < samples.length; i++) samples[i] = view.getInt16(2 * i, true) / 32768;
                    } else {
                        samples = new Float32Array(bytes.length);
                        for (let i = 0; i < bytes.length; i++) samples[i] = ULAW[bytes[i]];
                    }
                    this._queue(samples, rate);
                    if (!announced) {
                        announced = true;
                        this.readyState = 4;
                        this._retry = 0;
                        this._emit('playing');
                    }
                }
                if (run === this._run && !this.paused) this._again('the stream ended');
            } catch (e) {
                if (run === this._run && !this.paused && e.name !== 'AbortError') this._again(e.message);
            }
        }

        // Taken again after a pause that lengthens, as a radio does.
        _again(why) {
            this.error = { message: why };
            this._emit('error');
            const wait = RETRY_MS[Math.min(this._retry, RETRY_MS.length - 1)];
            this._retry++;
            this._retryTimer = setTimeout(() => { if (!this.paused) this._connect(); }, wait);
        }

        _queue(samples, rate) {
            // Nothing is held while the context does not run (before the
            // first click, or a phone asleep): it would all play at once.
            if (this.ctx.state !== 'running') { this._acc = []; this._accLength = 0; return; }
            this._acc.push(samples);
            this._accLength += samples.length;
            if (this._accLength < CHUNK * rate) return;
            const all = new Float32Array(this._accLength);
            let at = 0;
            for (const a of this._acc) { all.set(a, at); at += a.length; }
            this._acc = []; this._accLength = 0;
            this._schedule(all, rate);
        }

        _schedule(samples, rate) {
            const to = this.ctx.sampleRate;
            const now = this.ctx.currentTime;
            if (this._next < now + 0.02) {                              // starting, or the network fell behind
                this._next = now + LEAD;
                this._resampler = null;                                 // a gap anyway: start the filter afresh
            } else if (this._next > now + MAX_AHEAD) {
                this._resampler = null;                                 // too far ahead: this chunk is dropped
                return;
            }
            if (rate !== to) {
                if (!this._resampler || this._resampler.inRate !== rate) this._resampler = new Resampler(rate, to);
                samples = this._resampler.push(samples);
                if (!samples.length) return;
            }
            const buffer = this.ctx.createBuffer(1, samples.length, to);
            buffer.copyToChannel(samples, 0);
            const node = this.ctx.createBufferSource();
            node.buffer = buffer;
            node.connect(this.output);
            node.start(this._next);
            this._next += buffer.duration;
        }
    }

    // Whether this browser can play a stream this way: a readable fetch body
    // and Web Audio. Otherwise audio-client.js keeps an <audio> element.
    LiveStreamPlayer.Resampler = Resampler;

    LiveStreamPlayer.supported = () =>
        typeof ReadableStream !== 'undefined' && typeof Response !== 'undefined' &&
        'body' in Response.prototype && !!(window.AudioContext || window.webkitAudioContext);

    window.LiveStreamPlayer = LiveStreamPlayer;
})();
