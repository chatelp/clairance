// node --test www/websocket-client.test.js
// Every message the server sends reaches the page's listeners. The client
// kept a list of its own, and frequency_activity, sent since 26/10 and
// listened to by the page, was not on it: "someone is speaking" never came
// from the server (27/10).
const test = require('node:test');
const assert = require('node:assert/strict');

class FakeSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    constructor(url) {
        this.url = url;
        this.readyState = FakeSocket.CONNECTING;
        this.handlers = {};
        FakeSocket.last = this;
    }
    addEventListener(type, fn) { (this.handlers[type] ||= []).push(fn); }
    removeEventListener(type, fn) { this.handlers[type] = (this.handlers[type] || []).filter((f) => f !== fn); }
    close() {}
    receive(message) { for (const fn of this.handlers.message || []) fn({ data: JSON.stringify(message) }); }
}

globalThis.window = globalThis;
globalThis.WebSocket = FakeSocket;
require('./websocket-client.js');

function connected() {
    const client = new window.WebSocketClient('ws://station.lan/ws');
    client.connect();
    return { client, socket: FakeSocket.last };
}

// What internal/websocket and internal/transcription send, with a listener
// in www/store/feed.js.
const SERVER_TYPES = [
    'aircraft_added', 'aircraft_update', 'aircraft_predicted_state', 'aircraft_removed',
    'aircraft_bulk_response', 'transcription', 'transcription_update', 'transcription_second',
    'status_update', 'phase_change', 'clearance_issued', 'frequency_status', 'frequency_activity',
    'frequencies_changed',
];

for (const type of SERVER_TYPES) {
    test(`a ${type} message reaches its listener`, () => {
        const { client, socket } = connected();
        const got = [];
        client.addEventListener(type, (data) => got.push(data));
        socket.receive({ type, data: { frequency_id: 'f1', speaking: true } });
        assert.deepEqual(got, [{ frequency_id: 'f1', speaking: true }]);
    });
}

test('a message named after one of the client\'s own events is not taken for it', () => {
    const { client, socket } = connected();
    const got = [];
    for (const type of ['open', 'close', 'error', 'state_change', 'reconnect_scheduled']) {
        client.addEventListener(type, () => got.push(type));
        socket.receive({ type, data: {} });
    }
    assert.deepEqual(got, []);
});

test('an unknown type is dropped, not thrown', () => {
    const { socket } = connected();
    assert.doesNotThrow(() => socket.receive({ type: 'something_new', data: {} }));
});
