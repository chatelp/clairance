// The aircraft feed: the WebSocket and every message it carries, the deltas
// applied to the aircraft the page holds, the map updates they queue.
//
// One domain of the Alpine store 'atc' (docs-fr/32, palier 5.1): the entries
// below are the store's, moved from app.js as they were, and app.js assembles
// them with the rest. A method's `this` is the store. The entries are built
// when app.js assembles the store, inside alpine:init as they always were: by
// then the module-level names of app.js (generateUUID, API_BASE_URL, CONFIG,
// MAP_STYLES, wsClient, audioClient, mapManager) are defined, and a method
// resolves them when it runs.
(function () {
    // How long a lost connection lasts before it rings and is logged.
    const SERVER_LOST_NOTICE_MS = 10000;

    // "43 s", "8 min 43 s", "1 h 12 min".
    function lostForText(ms) {
        const s = Math.round(ms / 1000);
        if (s < 60) return `${s} s`;
        if (s < 3600) return `${Math.floor(s / 60)} min ${s % 60} s`;
        return `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} min`;
    }

    window.AtcStore = window.AtcStore || [];
    window.AtcStore.push(() => ({
        visibleAircraftOnMap: new Set(), // Track aircraft visible on map for UI indicators
        initialDataLoaded: false,
        connected: null, // null = initial state, true = connected, false = connection lost
        wsReconnectAttempt: 0,
        wsNextRetryDelayMs: null,
        wsConnectionState: 'idle',
        lastUpdate: null,
        lastUpdateSeconds: 0, // For footer status

        // A lost connection to the server (owner, 15/10): the bell's sound
        // once the loss has lasted ten seconds, and on reconnection a line in
        // the event log saying for how long, with the exchanges the server
        // kept meanwhile put back into the thread. A blink rings nothing.
        _serverLostAt: null,
        _serverLostTimer: null,

        markServerLost() {
            if (!this.initialDataLoaded || this._serverLostAt) return;
            this._serverLostAt = new Date();
            this._serverLostTimer = setTimeout(() => {
                this._serverLostTimer = null;
                const events = window.Alpine && Alpine.store('events');
                if (this.connected === false && events) events.ringServerLost();
            }, SERVER_LOST_NOTICE_MS);
        },

        async serverBack() {
            const lostAt = this._serverLostAt;
            this._serverLostAt = null;
            if (this._serverLostTimer) {
                clearTimeout(this._serverLostTimer);
                this._serverLostTimer = null;
            }
            if (!lostAt) return;
            const backAt = new Date();
            if (backAt - lostAt < SERVER_LOST_NOTICE_MS) return;
            const filled = await this.fillBackTranscriptions(lostAt);
            const events = window.Alpine && Alpine.store('events');
            if (!events) return;
            const meanwhile = filled === null
                ? 'exchanges heard meanwhile may be missing from the thread.'
                : filled === 0 ? 'nothing was heard meanwhile.'
                : `${filled} exchange${filled === 1 ? '' : 's'} filled back into the thread.`;
            events.add({
                kind: 'server', silent: true,
                text: `Server connection lost from ${this.clockText(lostAt)} to ${this.clockText(backAt)} (${lostForText(backAt - lostAt)}); ${meanwhile}`,
            });
        },

        // Initialize WebSocket for all aircraft data and alerts. (It sat at column
        // 0 in app.js, a stray indentation; moved with the feed, where it belongs.)
        async initAircraftDataSource() {
            console.log('[AIRCRAFT DATA] Initializing WebSocket for aircraft updates and alerts...');
            this.initWebSocket();
        },

        // Initialize WebSocket connection
        initWebSocket() {
            if (!wsClient) {
                console.error("wsClient not available during initWebSocket. This shouldn't happen.");
                return;
            }

            if (wsClient.enableAutoReconnect) {
                wsClient.enableAutoReconnect();
            }
            if (wsClient.resetReconnectAttempts) {
                wsClient.resetReconnectAttempts();
            }
            if (wsClient.clearAllListeners) {
                wsClient.clearAllListeners();
            }

            const updateWsStatus = (status) => {
                this.wsConnectionState = status?.state || 'idle';
                this.wsReconnectAttempt = Number.isFinite(status?.reconnectAttempts) ? status.reconnectAttempts : 0;
                this.wsNextRetryDelayMs = Number.isFinite(status?.nextRetryDelayMs) ? status.nextRetryDelayMs : null;
            };

            updateWsStatus(wsClient.getConnectionStatus ? wsClient.getConnectionStatus() : null);

            // Add event listeners
            wsClient.addEventListener('transcription', (data) => {
                this.handleTranscriptionMessage(data);
            });
            
            wsClient.addEventListener('transcription_update', (data) => {
                this.handleTranscriptionUpdateMessage(data);
            });

            wsClient.addEventListener('transcription_second', (data) => {
                this.handleTranscriptionSecondMessage(data);
            });

            // Add event listeners for phase changes
            wsClient.addEventListener('phase_change', (data) => {
                this.handlePhaseChangeMessage(data);
            });

            // Add event listener for clearance events
            wsClient.addEventListener('clearance_issued', (data) => {
                this.handleClearanceIssued(data);
            });

            // Add new aircraft streaming handlers
            wsClient.addEventListener('aircraft_added', (data) => {
                this.handleAircraftAdded(data);
            });
            
            wsClient.addEventListener('aircraft_update', (data) => {
                this.handleAircraftUpdate(data);
            });

            wsClient.addEventListener('aircraft_predicted_state', (data) => {
                this.handleAircraftPredictedState(data);
            });
            
            wsClient.addEventListener('aircraft_removed', (data) => {
                this.handleAircraftRemoved(data);
            });
            
            // Add bulk data response handler
            wsClient.addEventListener('aircraft_bulk_response', (data) => {
                this.handleBulkAircraftData(data);
            });

            // Add frequency status change handler
            wsClient.addEventListener('frequency_status', (data) => {
                this.handleFrequencyStatusChange(data);
            });

            // A source added or removed while the server runs -- a station that
            // publishes one stream per channel changes them as it switches.
            wsClient.addEventListener('frequencies_changed', () => {
                this.refreshAudioFrequencies();
            });

            // Aircraft events are now handled as phase changes (T/O and T/D)
            
            wsClient.addEventListener('open', () => {
                console.log('App.js: WebSocket connection now open.');
                this.connected = true;
                this.connectionLostAt = null;
                // From now on a lost connection is shown; never during the first load.
                this.initialDataLoaded = true;
                this.wsReconnectAttempt = 0;
                this.wsNextRetryDelayMs = null;
                this.serverBack();
                
                // Request initial aircraft data via WebSocket (this will send filter update)
                console.log('WebSocket connected, requesting initial aircraft data...');
                this.requestInitialAircraftData();
            });
            
            wsClient.addEventListener('close', (event) => {
                console.log('App.js: WebSocket connection closed.', event);
                
                // Only set connected to false if we previously had a successful connection
                // This prevents the CONNECTION LOST overlay from showing on initial page load
                if (this.connected === true) {
                    console.log('Connection was previously established and is now lost');
                    this.connectionLostAt = new Date();
                    this.connected = false;
                }
                this.markServerLost();
                
                // Note: Reconnection is now handled by WebSocketClient internally
                // No need to manually initiate reconnection here
            });

            wsClient.addEventListener('state_change', (status) => {
                updateWsStatus(status);

                if (status?.state === 'reconnecting' || status?.state === 'closed') {
                    if (this.connected !== false) this.connectionLostAt = new Date();
                    this.connected = false;
                    this.markServerLost();
                }
                if (status?.state === 'connecting' && this.connected === false) {
                    // keep false while recovering to retain clear UX
                }
            });

            wsClient.addEventListener('reconnect_scheduled', (payload) => {
                this.wsReconnectAttempt = Number.isFinite(payload?.attempt) ? payload.attempt : this.wsReconnectAttempt;
                this.wsNextRetryDelayMs = Number.isFinite(payload?.delayMs) ? payload.delayMs : this.wsNextRetryDelayMs;
                if (this.connected !== false) this.connectionLostAt = new Date();
                this.connected = false;
            });
            
            wsClient.addEventListener('error', (event) => {
                console.error('App.js: WebSocket error:', event);
            });
            
            // Connect to the WebSocket server if not already connected or trying
            if (wsClient.connection === null || wsClient.connection.readyState === WebSocket.CLOSED) {
                 console.log("App.js: Attempting to connect WebSocket...");
                wsClient.connect();
            } else if (wsClient.connection.readyState === WebSocket.CONNECTING) {
                console.log("App.js: WebSocket is already connecting.");
                this.connected = null;
            } else if (wsClient.connection.readyState === WebSocket.OPEN) {
                console.log("App.js: WebSocket is already open.");
                this.connected = true;
            }
        },

        // Handle phase change messages
        handlePhaseChangeMessage(data) {
            // Trigger visual effect on the map for takeoff/landing (T/O and T/D phases)
            if ((data.phase === 'T/O' || data.phase === 'T/D') && this.mapManager) {
                const eventType = data.phase === 'T/O' ? 'takeoff' : 'landing';
                this.mapManager.showTakeoffLandingEffect(data.hex, eventType, data.phase);
            }
        },

        // Handle status update message
        handleStatusUpdateMessage(data) {
            if (!data || !data.hex) return;

            // Skip new_aircraft alerts - handled by phase changes
            if (data.new_status === 'new_aircraft') return;

            const aircraft = this.aircraft[data.hex];
            if (!aircraft) return;

            if (data.new_status === 'signal_lost') {
                const secondsSince = this.getSecondsSinceLastSeen(aircraft);
                if (!Number.isFinite(secondsSince) || secondsSince < 60) {
                    return;
                }
            }

            aircraft.status = data.new_status;
            // Its symbol's colour follows (grey once lost).
            if (this.mapManager && typeof this.mapManager.updateVisualState === 'function') this.mapManager.updateVisualState(data.hex);
        },
        
        // Request initial aircraft data via WebSocket
        requestInitialAircraftData() {
            console.log('Requesting initial aircraft data');
            // Request bulk data for initial load
            // Only send last_seen_minutes to server to avoid loading stale aircraft
            // All other filtering is done client-side
            if (wsClient) {
                wsClient.requestBulkAircraftData({
                    last_seen_minutes: Number(this.settings.lastSeenMinutes) || 10
                });
            }
        },

        requestInitialAircraftDataThrottled(reason = 'resync', hex = '') {
            const now = Date.now();
            if (!this._lastUnknownAircraftResyncAt) {
                this._lastUnknownAircraftResyncAt = 0;
            }
            if (!this._unknownAircraftResyncCooldownMs) {
                this._unknownAircraftResyncCooldownMs = 5000;
            }

            if ((now - this._lastUnknownAircraftResyncAt) < this._unknownAircraftResyncCooldownMs) {
                return;
            }

            this._lastUnknownAircraftResyncAt = now;
            console.warn(`Requesting bulk aircraft resync (${reason})`, hex || '');
            this.requestInitialAircraftData();
        },

        // Handle bulk aircraft data response (initial load on WebSocket connect)
        handleBulkAircraftData(data) {
            console.log(`Received bulk aircraft data: ${data.count} aircraft`);

            // Clear current aircraft data
            this.aircraft = {};
            const sentAtMs = Date.parse(data.sent_at || '');
            
            // Process each aircraft
            if (data.aircraft && Array.isArray(data.aircraft)) {
                for (const aircraft of data.aircraft) {
                    // Calculate distance
                    this.calculateAircraftDistance(aircraft);
                    
                    // The engine, dated as the stream's samples are: last_seen
                    // is the server's observed_at (the poll less the whole
                    // seconds of seen). Dated on arrival, as before, each
                    // position was taken for current although decoded up to
                    // two seconds earlier: on a page just loaded, aircraft
                    // were drawn 300 m behind and caught up over 7 s
                    // (browser bench, scenario a).
                    if (this.animationEngine) {
                        const sample = this.animationEngine.sampleFromAircraft(aircraft, Date.parse(aircraft.last_seen || ''));
                        // The server's clock when it answered (sent_at), the
                        // page's evidence of the server's clock; the aircraft's
                        // poll times, before, put it up to 1.4 s late and every
                        // aircraft that much behind for 5 s (browser bench,
                        // scenario a). A server without it: the poll times.
                        if (sample && Number.isFinite(sentAtMs)) sample.sentAt = sentAtMs;
                        this.animationEngine.observe(aircraft.hex, sample);
                    }
                    
                    // Add to aircraft map (all aircraft from bulk response are pre-filtered)
                    this.aircraft[aircraft.hex] = aircraft;
                }
            }

            // Update connection status
            this.connected = true;
            this.lastUpdate = new Date();
            this.lastUpdateSeconds = 0;
            
            // Update map with all aircraft
            if (this.mapManager) {
                // Clear existing markers that are no longer in the current aircraft set
                const currentHexes = new Set(Object.keys(this.aircraft));
                this.mapManager.removeStaleMarkers(currentHexes);
                
                // Update all aircraft markers
                for (const aircraft of Object.values(this.aircraft)) {
                    if (this.mapManager.ensureMapObjects) {
                        this.mapManager.ensureMapObjects(aircraft);
                    }
                }
                
                // Refresh view and apply filters
                this.mapManager.applyFiltersAndRefreshView();
            }
            
            // Invalidate filtered aircraft cache
            this._lastFilterHash = null;
            
            console.log('Bulk aircraft data processing complete');
        },

        // Throttling state for performance optimization
        pendingMapUpdates: new Set(),
        mapUpdateThrottleId: null,
        cacheInvalidationPending: false,
        _lastUnknownAircraftResyncAt: 0,
        _unknownAircraftResyncCooldownMs: 5000,
        
        // Filtering throttling state to prevent main thread blocking
        _lastFilterTime: null,
        // Performance-optimized aircraft handlers (throttling handled by queueMapUpdate/queueCacheInvalidation)
        handleAircraftAdded(data) {
            if (this.wsUpdatesPaused) {
                console.log(`[PAUSED] Skipping aircraft added: ${data.aircraft?.flight || data.hex}`);
                return;
            }

            if (data.aircraft) {
                const observedAtMs = Date.parse(data.observed_at || '');
                if (Number.isFinite(observedAtMs)) {
                    data.aircraft.last_seen = new Date(observedAtMs).toISOString();
                }

                // Update last update timestamp
                this.lastUpdate = new Date();
                this.lastUpdateSeconds = 0;

                // Apply distance calculation
                this.calculateAircraftDistance(data.aircraft);

                // ALWAYS add to aircraft store - filtering is done at display level
                this.aircraft[data.aircraft.hex] = data.aircraft;

                // The engine, dated by the position's time (observed_at, seen, seen_pos)
                if (this.animationEngine) {
                    this.animationEngine.observe(data.aircraft.hex,
                        this.animationEngine.sampleFromAircraft(data.aircraft, observedAtMs));
                }

                // Queue for throttled map update (map will handle visibility based on filters)
                this.queueMapUpdate(data.aircraft.hex);

                // Queue cache invalidation
                this.queueCacheInvalidation();
            }
        },

        handleAircraftUpdate(data) {
            if (this.wsUpdatesPaused) {
                console.log(`[PAUSED] Skipping aircraft update: ${data.hex}`);
                return;
            }

            const hex = data.hex;
            let existing = this.aircraft[hex];
            const observedAtMs = Date.parse(data.observed_at || '');
            const realObservedAtIso = Number.isFinite(observedAtMs)
                ? new Date(observedAtMs).toISOString()
                : null;

            // Update last update timestamp
            this.lastUpdate = new Date();
            this.lastUpdateSeconds = 0;

            // Handle delta update (new efficient format)
            if (data.delta) {
                // If aircraft doesn't exist locally, avoid creating sparse placeholders.
                // Request a bulk sync so details panel fields stay complete.
                if (!existing) {
                    this.requestInitialAircraftDataThrottled('delta_without_local_aircraft', hex);
                    return;
                }

                // Heal previously sparse entries (delta streams may not include all metadata fields).
                if (!existing.created_at) {
                    this.requestInitialAircraftDataThrottled('sparse_local_aircraft', hex);
                }

                // Check if this update contains filter-relevant changes BEFORE applying
                const hasFilterRelevantChanges =
                    data.delta.status !== undefined ||
                    data.delta.phase !== undefined ||
                    data.delta.on_ground !== undefined ||
                    data.delta.flight !== undefined;

                this.applyDelta(existing, data.delta, realObservedAtIso);
                this.calculateAircraftDistance(existing);

                // Update animation engine with delta
                // The engine: the aircraft with the delta applied, dated by the
                // seen and seen_pos the message carries (its timing block; the
                // whole aircraft, from a server before 16/10)
                if (this.animationEngine) {
                    this.animationEngine.observe(hex,
                        this.animationEngine.sampleFromAircraft(existing, observedAtMs, data.timing || data.aircraft?.adsb));
                }

                // Queue map update - map will handle visibility based on filters
                this.queueMapUpdate(hex);

                // Update selected aircraft panel if this is the selected one
                if (this.selectedAircraft && this.selectedAircraft.hex === hex) {
                    this.setupAircraftDetailsPanel();
                }

                // PERFORMANCE: Only invalidate cache if filter-relevant data changed
                // Position/altitude/speed updates don't affect which aircraft are shown
                if (hasFilterRelevantChanges) {
                    this.queueCacheInvalidation();
                }
            }
            // Fallback: full aircraft object (backward compatibility)
            else if (data.aircraft) {
                if (realObservedAtIso) {
                    data.aircraft.last_seen = realObservedAtIso;
                }

                // ALWAYS store aircraft data - filtering is done at display level
                this.aircraft[data.aircraft.hex] = data.aircraft;
                this.calculateAircraftDistance(data.aircraft);

                if (this.animationEngine) {
                    this.animationEngine.observe(data.aircraft.hex,
                        this.animationEngine.sampleFromAircraft(data.aircraft, observedAtMs));
                }

                // Queue map update - map will handle visibility based on filters
                this.queueMapUpdate(hex);

                if (this.selectedAircraft && this.selectedAircraft.hex === data.aircraft.hex) {
                    this.selectedAircraft = data.aircraft;
                    this.setupAircraftDetailsPanel();
                }

                // Full object replacement always needs cache invalidation
                this.queueCacheInvalidation();
            }
        },

        // A predicted state is what the server expects between two polls, not
        // what the station said: it goes to the animation engine alone, and
        // aircraft.adsb keeps only the station's figures, which the distance,
        // the list, the flight card, the trails and the PAR read. (It was
        // written into aircraft.adsb, marked adsb.source = 'predicted', and
        // the next real update that only changed the altitude handed the
        // engine that predicted position as a real fix.)
        handleAircraftPredictedState(data) {
            if (this.wsUpdatesPaused || !data || !data.hex || !data.delta) {
                return;
            }

            const aircraft = this.aircraft[data.hex];
            if (!aircraft || aircraft.status === 'signal_lost' || !this.animationEngine) {
                return;
            }

            const delta = data.delta;
            this.animationEngine.estimate(data.hex, {
                lat: delta.lat,
                lon: delta.lon,
                alt: delta.alt_baro,
                track: delta.track,
                gs: delta.gs,
                confidence: Number(delta.confidence),
                at: Date.parse(data.predicted_at || ''),
                sentAt: Date.parse(data.predicted_at || ''),
                basedOn: Date.parse(data.based_on || ''),
            });
        },

        // Apply delta updates to an existing aircraft object
        // PERFORMANCE: Only set values that have actually changed to minimize Alpine reactivity triggers
        applyDelta(aircraft, delta, realObservedAtIso = null) {
            // Initialize adsb object if needed
            if (!aircraft.adsb) aircraft.adsb = {};

            // Preserve last known position when GPS drops out (ADS-B → mode_s transition).
            // If the delta sends lat=0 and lon=0 but the aircraft previously had a valid position,
            // keep the old lat/lon and flag the position as stale so the map can show
            // the marker at its last known location with a distinct visual style.
            const hadValidPosition = aircraft.adsb.lat && aircraft.adsb.lat !== 0 || aircraft.adsb.lon && aircraft.adsb.lon !== 0;
            const deltaDropsPosition = delta.lat === 0 && delta.lon === 0;

            if (hadValidPosition && deltaDropsPosition) {
                // Don't overwrite — mark position as stale
                aircraft.stale_position = true;
            } else {
                // Apply position normally
                if (delta.lat !== undefined && aircraft.adsb.lat !== delta.lat) aircraft.adsb.lat = delta.lat;
                if (delta.lon !== undefined && aircraft.adsb.lon !== delta.lon) aircraft.adsb.lon = delta.lon;
                // Clear stale flag if we got valid position data back
                if (delta.lat !== undefined && delta.lon !== undefined && (delta.lat !== 0 || delta.lon !== 0)) {
                    aircraft.stale_position = false;
                }
            }
            if (delta.alt_baro !== undefined && aircraft.adsb.alt_baro !== delta.alt_baro) aircraft.adsb.alt_baro = delta.alt_baro;
            if (delta.track !== undefined && aircraft.adsb.track !== delta.track) aircraft.adsb.track = delta.track;
            if (delta.gs !== undefined && aircraft.adsb.gs !== delta.gs) aircraft.adsb.gs = delta.gs;
            if (delta.tas !== undefined && aircraft.adsb.tas !== delta.tas) aircraft.adsb.tas = delta.tas;
            if (delta.baro_rate !== undefined && aircraft.adsb.baro_rate !== delta.baro_rate) aircraft.adsb.baro_rate = delta.baro_rate;
            if (delta.mag_heading !== undefined && aircraft.adsb.mag_heading !== delta.mag_heading) aircraft.adsb.mag_heading = delta.mag_heading;
            if (delta.true_heading !== undefined && aircraft.adsb.true_heading !== delta.true_heading) aircraft.adsb.true_heading = delta.true_heading;
            if (delta.squawk !== undefined) aircraft.adsb.squawk = delta.squawk;
            if (delta.atc_derived !== undefined && aircraft.adsb.atc_derived !== delta.atc_derived) aircraft.adsb.atc_derived = delta.atc_derived;

            // Apply full adsb object if provided — preserve last known position if new object has no GPS
            if (delta.adsb !== undefined) {
                const oldLat = aircraft.adsb?.lat;
                const oldLon = aircraft.adsb?.lon;
                const oldHadPosition = (oldLat && oldLat !== 0) || (oldLon && oldLon !== 0);
                aircraft.adsb = delta.adsb;
                const newHasPosition = (delta.adsb.lat && delta.adsb.lat !== 0) || (delta.adsb.lon && delta.adsb.lon !== 0);
                if (oldHadPosition && !newHasPosition) {
                    aircraft.adsb.lat = oldLat;
                    aircraft.adsb.lon = oldLon;
                    aircraft.stale_position = true;
                } else if (newHasPosition) {
                    aircraft.stale_position = false;
                }
            }

            // Apply top-level aircraft properties - only if value changed
            if (delta.flight !== undefined && aircraft.flight !== delta.flight) aircraft.flight = delta.flight;
            if (delta.status !== undefined && aircraft.status !== delta.status) aircraft.status = delta.status;
            if (delta.on_ground !== undefined && aircraft.on_ground !== delta.on_ground) aircraft.on_ground = delta.on_ground;
            if (delta.phase !== undefined && aircraft.phase !== delta.phase) aircraft.phase = delta.phase;
            // The runway judge's verdict (D69); null when it forgets the aircraft
            if (delta.runway !== undefined) aircraft.runway = delta.runway;
            if (delta.distance !== undefined && aircraft.distance !== delta.distance) aircraft.distance = delta.distance;

            // Apply BSDB (BaseStation.sqb enrichment) data if provided
            if (delta.bsdb !== undefined) aircraft.bsdb = delta.bsdb;

            // Only update last_seen when the delta contains actual ADS-B sensor data
            // (not for status-only changes like signal_lost which would reset the timer)
            const hasAdsbData = delta.lat !== undefined || delta.lon !== undefined ||
                delta.alt_baro !== undefined || delta.gs !== undefined ||
                delta.track !== undefined || delta.baro_rate !== undefined ||
                delta.tas !== undefined || delta.adsb !== undefined;
            if (hasAdsbData && realObservedAtIso) {
                aircraft.last_seen = realObservedAtIso;
            }
        },

        handleAircraftRemoved(data) {
            if (this.wsUpdatesPaused) {
                console.log(`[PAUSED] Skipping aircraft removed: ${data.hex}`);
                return;
            }

            const aircraft = this.aircraft[data.hex];
            if (aircraft) {
                // Don't delete from the store. A "removed" stream event means the target
                // is absent from the current poll, not necessarily past signal-lost timeout.
                // Signal-lost classification is derived strictly from last_seen age.

                // Remove from map and animation (no marker/trail for non-broadcasting aircraft)
                if (this.mapManager) {
                    this.mapManager.removeAircraft(data.hex);
                }
                if (this.animationEngine) {
                    this.animationEngine.removeAircraft(data.hex);
                }

                // Queue cache invalidation
                this.queueCacheInvalidation();
            }
        },

        // Purge aircraft from the store whose last_seen exceeds the lastSeenMinutes setting.
        // This mirrors the server-side filter used on bulk load, keeping client and server in sync.
        _purgeStaleAircraft() {
            const cutoff = Date.now() - ((Number(this.settings.lastSeenMinutes) || 10) * 60 * 1000);
            let purged = 0;
            for (const hex of Object.keys(this.aircraft)) {
                const ac = this.aircraft[hex];
                if (ac.last_seen && new Date(ac.last_seen).getTime() < cutoff) {
                    delete this.aircraft[hex];
                    if (this.mapManager) this.mapManager.removeAircraft(hex);
                    if (this.animationEngine) this.animationEngine.removeAircraft(hex);
                    if (this.selectedAircraft && this.selectedAircraft.hex === hex) {
                        this.selectedAircraft = null;
                    }
                    purged++;
                }
            }
            if (purged > 0) {
                this.queueCacheInvalidation();
            }
        },

        // Queue map updates for throttling
        queueMapUpdate(hex) {
            if (this.wsUpdatesPaused) {
                return;
            }

            this.pendingMapUpdates.add(hex);

            // Apply immediately on each websocket update to avoid perceived buffering/jumps
            this.processPendingMapUpdates();
        },


        // Process all pending map updates in a batch
        processPendingMapUpdates() {
            if (this.pendingMapUpdates.size === 0) return;

            // Process ALL pending updates in one batch - map updates are lightweight
            const aircraftToUpdate = Array.from(this.pendingMapUpdates);
            this.pendingMapUpdates.clear();

            // Update specific aircraft markers instead of full refresh
            aircraftToUpdate.forEach(hex => {
                const aircraft = this.aircraft[hex];
                if (aircraft && this.mapManager) {
                    this.mapManager.updateSingleAircraft(hex, aircraft);
                }
            });

            // TRAILS FIX: Throttled call to update flight paths (draws trail polylines)
            if (this.mapManager && this.settings.showPaths && !this._trailUpdatePending) {
                this._trailUpdatePending = true;
                setTimeout(() => {
                    this._trailUpdatePending = false;
                    if (this.mapManager) {
                        this.mapManager.updateFlightPaths();
                    }
                }, 500); // Update trails every 500ms max
            }
        },

        // Queue cache invalidation to reduce frequency - VERY AGGRESSIVE
        queueCacheInvalidation() {
            if (!this.cacheInvalidationPending) {
                this.cacheInvalidationPending = true;
                
                // Keep sidebar and map state responsive while still batching bursts.
                setTimeout(() => {
                    // Don't null the cache, just invalidate the hash to trigger re-filtering
                    // This maintains the stable array reference to prevent full table re-renders
                    this._lastFilterHash = null;
                    this._performFiltering();
                    this.cacheInvalidationPending = false;
                }, 120);
            }
        },

        // Request new aircraft data when settings change
        // Note: Server-side filtering has been removed. All filtering is done client-side.
        // Settings are just saved locally; server broadcasts all updates.
        requestNewAircraftData() {
            console.log('Settings changed, saving locally');
            this.saveSettings();
        },

        calculateAircraftDistance(aircraft) {
            if (aircraft.adsb && aircraft.adsb.lat && aircraft.adsb.lon &&
                this.stationLatitude && this.stationLongitude) {
                const distanceNM = this.haversineDistance(
                    aircraft.adsb.lat, aircraft.adsb.lon,
                    this.stationLatitude, this.stationLongitude
                );
                aircraft.distance = Math.round(distanceNM * 10) / 10;
            }
        },

        haversineDistance(lat1, lon1, lat2, lon2) {
            const R = 3440.065; // Earth's radius in nautical miles
            const dLat = this.toRadians(lat2 - lat1);
            const dLon = this.toRadians(lon2 - lon1);
            const a = Math.sin(dLat/2) * Math.sin(dLat/2) +
                      Math.cos(this.toRadians(lat1)) * Math.cos(this.toRadians(lat2)) *
                      Math.sin(dLon/2) * Math.sin(dLon/2);
            const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a));
            return R * c;
        },

        toRadians(degrees) {
            return degrees * (Math.PI/180);
        },

        // WebSocket debugging state
        wsUpdatesPaused: false,
        // Removed mapUpdatesDisabled and dataOnlyMode debug parameters

        // Toggle WebSocket updates for troubleshooting
        toggleWebSocketUpdates() {
            this.wsUpdatesPaused = !this.wsUpdatesPaused;
            console.log(`WebSocket updates ${this.wsUpdatesPaused ? 'PAUSED' : 'RESUMED'}`);

            if (this.wsUpdatesPaused) {
                // Clear any pending updates when pausing
                this.cleanupThrottling();
                // PERFORMANCE: Stop animation engine when updates are paused
                if (this.animationEngine) {
                    this.animationEngine.stop();
                    console.log('Animation engine STOPPED');
                }
            } else {
                // Resume animation engine when updates resume
                if (this.animationEngine) {
                    this.animationEngine.start();
                    console.log('Animation engine STARTED');
                }
            }
        },

        // Removed toggleMapUpdates() and toggleDataOnlyMode() debug methods

        // Clean up throttling mechanisms to prevent memory leaks
        cleanupThrottling() {
            if (this.mapUpdateThrottleId) {
                clearTimeout(this.mapUpdateThrottleId);
                this.mapUpdateThrottleId = null;
            }
            this.pendingMapUpdates.clear();
            this.cacheInvalidationPending = false;
        }
    }));
})();
