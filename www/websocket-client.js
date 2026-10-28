/**
 * WebSocket client for Co-ATC realtime streams.
 *
 * Responsibilities:
 * - Maintain a single WebSocket connection.
 * - Dispatch typed message events to local listeners.
 * - Reconnect indefinitely with exponential backoff + jitter.
 * - Surface connection state and reconnect telemetry for UI.
 */
// The client's own events, never a server message.
const CONNECTION_EVENTS = new Set(['open', 'close', 'error', 'state_change', 'reconnect_scheduled']);

class WebSocketClient {
    /**
     * @param {string} url WebSocket endpoint URL.
     */
    constructor(url) {
        this.url = url;
        this.connection = null;
        this.reconnectTimeout = null;
        this.isReconnecting = false;
        this.autoReconnect = false;
        this.reconnectAttempts = 0;
        this.baseReconnectDelay = 1000;
        // 5 s, not 30: the wait grows while the server is away, so a restart
        // was seen up to the last wait late -- the map frozen and the banner up
        // 17 s after a 15 s restart, 29 s after a 30 s one, the sound already
        // back (fiche S, 26/10). One refused connection every 5 s costs nothing.
        this.maxReconnectDelay = 5000;
        this.reconnectJitterFactor = 0.25;
        this.connectTimeoutMs = 10000;
        // A connection that says nothing for this long is taken for dead and
        // replaced. The server sends a heartbeat every 15 s (internal/websocket,
        // heartbeatPeriod); a page only learns of a close it receives, and one
        // made behind the proxy may never arrive: on 27/10 a page lost its
        // connection at 21:02, played its sound all night, and never reconnected.
        this.silenceLimitMs = 45000;
        this._lastMessageAt = 0;
        this._silenceWatch = null;
        this.intentionalClose = false;
        this.connectionState = 'idle'; // idle | connecting | connected | reconnecting | closed
        this.nextReconnectDelayMs = null;
        this._connectTimeoutHandle = null;
        this.listeners = {
            transcription: [],
            transcription_update: [],
            transcription_second: [],
            aircraft: [],
            aircraft_added: [],
            aircraft_update: [],
            aircraft_predicted_state: [],
            aircraft_removed: [],
            aircraft_bulk_response: [],
            status_update: [],
            phase_change: [],
            clearance_issued: [],
            frequency_status: [],
            frequency_activity: [],
            frequencies_changed: [],
            state_change: [],
            reconnect_scheduled: [],
            open: [],
            close: [],
            error: []
        };

        this._boundOpenHandler = null;
        this._boundCloseHandler = null;
        this._boundErrorHandler = null;
        this._boundMessageHandler = null;

        this._messageCounters = {
            total: 0,
            parseErrors: 0,
            byType: {}
        };
        this._messageSnapshot = {
            timestamp: performance.now(),
            counters: {
                total: 0,
                parseErrors: 0,
                byType: {}
            }
        };
    }

    /**
     * Connect (or reconnect) the socket.
     *
     * Behavior:
     * - Clears pending reconnect timers.
     * - Prevents concurrent connect attempts.
     * - Replaces any existing socket instance.
     * - Starts a watchdog timeout for stuck CONNECTING states.
     */
    connect() {
        this.intentionalClose = false;

        if (this.reconnectTimeout) {
            clearTimeout(this.reconnectTimeout);
            this.reconnectTimeout = null;
        }
        this.nextReconnectDelayMs = null;

        if (this.isReconnecting) {
            console.log('WebSocket: Connection attempt already in progress');
            return;
        }

        if (this.connection) {
            this._removeConnectionHandlers();
            this.connection.close();
            this.connection = null;
        }

        this.isReconnecting = true;
        this._setConnectionState('connecting');

        this.connection = new WebSocket(this.url);

        this._clearConnectTimeout();
        this._connectTimeoutHandle = setTimeout(() => {
            if (this.connection && this.connection.readyState === WebSocket.CONNECTING) {
                console.warn('WebSocket: connect timeout reached, forcing reconnect');
                try {
                    this.connection.close();
                } catch (error) {
                    console.error('WebSocket: failed to close timed-out socket', error);
                }
            }
        }, this.connectTimeoutMs);

        this._boundOpenHandler = (event) => {
            console.log('WebSocket connection established');
            this.isReconnecting = false;
            this.reconnectAttempts = 0;
            this.nextReconnectDelayMs = null;
            this._clearConnectTimeout();
            this._watchSilence();
            this._setConnectionState('connected');
            this._notifyListeners('open', event);
        };

        this._boundCloseHandler = (event) => {
            console.log('WebSocket connection closed');
            this.isReconnecting = false;
            this._clearConnectTimeout();
            this._stopSilenceWatch();
            this._notifyListeners('close', event);

            if (this.autoReconnect && !this.intentionalClose) {
                this.reconnectAttempts++;
                const delayMs = this._getReconnectDelayMs();
                this.nextReconnectDelayMs = delayMs;
                this._setConnectionState('reconnecting');
                this._notifyListeners('reconnect_scheduled', {
                    attempt: this.reconnectAttempts,
                    delayMs: delayMs,
                });
                console.log(`WebSocket: Attempting reconnect #${this.reconnectAttempts} in ${delayMs}ms`);

                this.reconnectTimeout = setTimeout(() => {
                    this.reconnectTimeout = null;
                    this.connect();
                }, delayMs);
            } else {
                this._setConnectionState('closed');
            }
        };

        this._boundErrorHandler = (event) => {
            console.error('WebSocket error:', event);
            this._notifyListeners('error', event);
        };

        this._boundMessageHandler = (event) => {
            this._lastMessageAt = Date.now();
            try {
                const message = JSON.parse(event.data);
                this._recordInboundMessage(message?.type || 'unknown');

                if (message.type === 'status_update' && window.Alpine && Alpine.store('atc')) {
                    Alpine.store('atc').handleStatusUpdateMessage(message.data);
                }
                // Every message type a listener can register for, not a list of
                // its own: frequency_activity was registered and sent, but missing
                // here, and dropped -- "someone is speaking" never came from the
                // server, and a page loaded with every frequency muted showed
                // none (27/10).
                if (!CONNECTION_EVENTS.has(message.type) && this.listeners[message.type]) {
                    this._notifyListeners(message.type, message.data);
                }
            } catch (error) {
                this._recordParseError();
                console.error('Error parsing WebSocket message:', error);
            }
        };

        this.connection.addEventListener('open', this._boundOpenHandler);
        this.connection.addEventListener('close', this._boundCloseHandler);
        this.connection.addEventListener('error', this._boundErrorHandler);
        this.connection.addEventListener('message', this._boundMessageHandler);
    }

    /**
     * Watch the open connection for silence: past silenceLimitMs without a
     * message, it is dropped unheard and a new one opened, as for a close.
     */
    _watchSilence() {
        this._stopSilenceWatch();
        this._lastMessageAt = Date.now();
        this._silenceWatch = setInterval(() => {
            if (this.connectionState !== 'connected') return;
            if (Date.now() - this._lastMessageAt <= this.silenceLimitMs) return;
            console.warn(`WebSocket: nothing heard for ${Math.round((Date.now() - this._lastMessageAt) / 1000)} s, reconnecting`);
            this._stopSilenceWatch();
            this._notifyListeners('close', { silent: true });
            this.connect(); // drops the old socket unheard, then opens a new one
        }, Math.min(5000, this.silenceLimitMs));
    }

    _stopSilenceWatch() {
        if (this._silenceWatch) {
            clearInterval(this._silenceWatch);
            this._silenceWatch = null;
        }
    }

    /**
     * Compute the next reconnect delay using capped exponential backoff with jitter.
     * @returns {number} Delay in milliseconds.
     */
    _getReconnectDelayMs() {
        const exponent = Math.max(0, this.reconnectAttempts-1);
        const exponentialDelay = this.baseReconnectDelay * Math.pow(2, exponent);
        const cappedDelay = Math.min(this.maxReconnectDelay, exponentialDelay);
        const jitterRange = cappedDelay * this.reconnectJitterFactor;
        const jitter = (Math.random() * 2 - 1) * jitterRange;
        return Math.max(250, Math.round(cappedDelay + jitter));
    }

    /**
     * Update connection state and emit a `state_change` event when state changes.
     * @param {'idle'|'connecting'|'connected'|'reconnecting'|'closed'} state
     */
    _setConnectionState(state) {
        if (this.connectionState === state) {
            return;
        }
        this.connectionState = state;
        this._notifyListeners('state_change', this.getConnectionStatus());
    }

    /**
     * Clear the connect watchdog timer.
     */
    _clearConnectTimeout() {
        if (this._connectTimeoutHandle) {
            clearTimeout(this._connectTimeoutHandle);
            this._connectTimeoutHandle = null;
        }
    }

    /**
     * Snapshot current connection/reconnect status for UI and diagnostics.
     * @returns {{state: string, reconnectAttempts: number, nextRetryDelayMs: number|null, autoReconnect: boolean}}
     */
    getConnectionStatus() {
        return {
            state: this.connectionState,
            reconnectAttempts: this.reconnectAttempts,
            nextRetryDelayMs: this.nextReconnectDelayMs,
            autoReconnect: this.autoReconnect,
        };
    }

    /**
     * Detach all event handlers from the current socket and clear handler references.
     */
    _removeConnectionHandlers() {
        if (this.connection) {
            if (this._boundOpenHandler) {
                this.connection.removeEventListener('open', this._boundOpenHandler);
            }
            if (this._boundCloseHandler) {
                this.connection.removeEventListener('close', this._boundCloseHandler);
            }
            if (this._boundErrorHandler) {
                this.connection.removeEventListener('error', this._boundErrorHandler);
            }
            if (this._boundMessageHandler) {
                this.connection.removeEventListener('message', this._boundMessageHandler);
            }
        }

        this._boundOpenHandler = null;
        this._boundCloseHandler = null;
        this._boundErrorHandler = null;
        this._boundMessageHandler = null;

        this._clearConnectTimeout();
    }

    /**
     * Intentionally close the connection and disable automatic reconnect.
     */
    disconnect() {
        this.autoReconnect = false;
        this.intentionalClose = true;
        this.nextReconnectDelayMs = null;
        this._stopSilenceWatch();

        if (this.reconnectTimeout) {
            clearTimeout(this.reconnectTimeout);
            this.reconnectTimeout = null;
        }

        this._removeConnectionHandlers();

        if (this.connection) {
            this.connection.close();
            this.connection = null;
        }

        this.isReconnecting = false;
        this._setConnectionState('idle');
    }

    /**
     * Remove all registered listeners for every event type.
     */
    clearAllListeners() {
        Object.keys(this.listeners).forEach(type => {
            this.listeners[type] = [];
        });
    }

    /**
     * Enable automatic reconnect after non-intentional disconnects.
     */
    enableAutoReconnect() {
        this.autoReconnect = true;
        this.intentionalClose = false;
    }

    /**
     * Disable automatic reconnect.
     */
    disableAutoReconnect() {
        this.autoReconnect = false;
    }

    /**
     * Reset reconnect telemetry counters.
     */
    resetReconnectAttempts() {
        this.reconnectAttempts = 0;
        this.nextReconnectDelayMs = null;
    }

    /**
     * Register an event listener for a supported message/event type.
     * @param {string} type
     * @param {(data: any) => void} callback
     */
    addEventListener(type, callback) {
        if (this.listeners[type]) {
            this.listeners[type].push(callback);
        }
    }

    /**
     * Remove a previously registered listener callback.
     * @param {string} type
     * @param {(data: any) => void} callback
     */
    removeEventListener(type, callback) {
        if (this.listeners[type]) {
            this.listeners[type] = this.listeners[type].filter(cb => cb !== callback);
        }
    }

    /**
     * Request a bulk aircraft payload from the server.
     * @param {Object} [filters={}] Optional server-side filter object.
     */
    requestBulkAircraftData(filters = {}) {
        if (this.connection && this.connection.readyState === WebSocket.OPEN) {
            const message = {
                type: 'aircraft_bulk_request',
                data: {
                    filters: filters
                }
            };
            
            console.log('Requesting bulk aircraft data via WebSocket...', filters);
            this.connection.send(JSON.stringify(message));
        } else {
            console.error('WebSocket not connected, cannot request bulk data');
        }
    }

    /**
     * Emit an internal event to all subscribers.
     * @param {string} type
     * @param {any} data
     */
    _notifyListeners(type, data) {
        if (this.listeners[type]) {
            this.listeners[type].forEach(callback => {
                try {
                    callback(data);
                } catch (error) {
                    console.error(`Error in ${type} listener:`, error);
                }
            });
        }
    }

    _recordInboundMessage(type) {
        this._messageCounters.total += 1;
        const key = type || 'unknown';
        this._messageCounters.byType[key] = (this._messageCounters.byType[key] || 0) + 1;
    }

    _recordParseError() {
        this._messageCounters.parseErrors += 1;
    }

    getMessageRateStats() {
        const now = performance.now();
        const elapsedSeconds = Math.max(0.001, (now - this._messageSnapshot.timestamp) / 1000);

        const current = this._messageCounters;
        const previous = this._messageSnapshot.counters;

        const deltaTotal = current.total - (previous.total || 0);
        const deltaParseErrors = current.parseErrors - (previous.parseErrors || 0);

        const allTypes = new Set([
            ...Object.keys(current.byType || {}),
            ...Object.keys(previous.byType || {})
        ]);

        const byTypePerSec = {};
        allTypes.forEach((type) => {
            const currentCount = current.byType[type] || 0;
            const previousCount = previous.byType[type] || 0;
            const delta = currentCount - previousCount;
            byTypePerSec[type] = Number((delta / elapsedSeconds).toFixed(2));
        });

        this._messageSnapshot = {
            timestamp: now,
            counters: {
                total: current.total,
                parseErrors: current.parseErrors,
                byType: { ...current.byType }
            }
        };

        return {
            windowSec: Number(elapsedSeconds.toFixed(1)),
            totalPerSec: Number((deltaTotal / elapsedSeconds).toFixed(2)),
            parseErrorsPerSec: Number((deltaParseErrors / elapsedSeconds).toFixed(2)),
            byTypePerSec
        };
    }
}

// Export the WebSocketClient class
window.WebSocketClient = WebSocketClient;