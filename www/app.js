/**
 * Module: app
 * Why it exists:
 * - Main frontend runtime entry point for Co-ATC.
 * - Wires Alpine store state, API/WebSocket clients, map manager integration,
 *   audio/transcription flows, and operator-facing UI behaviors.
 *
 * Key responsibilities:
 * - Define runtime configuration and environment-derived endpoints.
 * - Initialize shared clients/services and attach browser lifecycle handlers.
 * - Orchestrate realtime aircraft updates, map rendering, and sidebar/detail views.
 *
 * Quirks / contracts:
 * - Assumes same-host API/WebSocket deployment by default (LAN/local first).
 * - Contains deliberate fallbacks for non-secure contexts used in local operations,
 *   including UUID generation and service-worker behavior constraints.
 * - Large by design today: this file remains the integration nexus while map-
 *   specific logic is progressively moved into dedicated modules.
 */

// UUID generation with fallback for non-secure contexts (e.g., HTTP on LAN)
function generateUUID() {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return crypto.randomUUID();
    }
    // Fallback for non-secure contexts
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
        const r = Math.random() * 16 | 0;
        const v = c === 'x' ? r : (r & 0x3 | 0x8);
        return v.toString(16);
    });
}

// The API and the WebSocket on the page's own origin: its scheme, name and port
// as the browser has them, which behind an HTTPS proxy is https and no port.
const API_BASE_URL = `${window.location.origin}/api/v1`;

// Configuration
const CONFIG = {
    // defaultCenter: [43.6777, -79.6248], // Will be fetched from API
    mapEngine: 'openlayers',
    mapAircraftWebGL: false,
    mapOverlays: null,
    aviationChartOverlayUrl: '',
    airspaceOverlayGeoJsonUrl: '',
    defaultZoom: 10,
    dataUrl: `${API_BASE_URL}/aircraft`,
    // wss on an HTTPS page: a browser refuses a plain ws:// connection from one,
    // which behind the owner's proxy left the page without aircraft or feed (26/10).
    wsUrl: `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws`,
    useRealData: true,
    useSampleData: true,
    rangeRings: [5, 10, 25, 50, 100],
    selectedFadeOpacity: 0.4, // Opacity for non-selected items when one is selected
    
    // Refresh intervals (in milliseconds)
    stationRefreshInterval: 30 * 60 * 1000,  // 30 minutes for station data
    weatherRefreshInterval: 30 * 60 * 1000,  // 30 minutes for weather data
};

// Base maps the map engine offers (map/core/map-engine.js). None needs a key: the
// Carto ones, which now do, are gone (docs-fr/audit-2026-10-09/fonds-de-carte.md).
// "Reduce motion" (macOS, and the other systems' equivalents) is honoured
// (migration step 11): a scroll asked for in code is then instant, as the
// stylesheet makes every transition and animation; the aircraft still move
// between two positions, which is information, not decoration (03-systeme).
window.smoothScroll = () =>
    (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth');

const MAP_STYLES = ['ign-dark', 'ign', 'osm-dark', 'osm', 'none'];

// The base map shown until one is chosen: none, the aircraft and the airport layout on
// black -- the most readable, in the owner's judgement (10/10); the IGN and OSM maps
// are a click away. Also the fallback for a saved style that is no longer offered
// (the FAA charts, Carto dark and light). The station's position no longer matters.
function defaultMapStyle(lat, lon) {
    return 'none';
}

// Initialize WebSocket client
const wsClient = new WebSocketClient(CONFIG.wsUrl);
window.wsClient = wsClient;

// Declare Audio client - will be initialized in alpine:init
let audioClient;
// Declare Map Manager - will be initialized in alpine:init
let mapManager;
// Declare Animation Engine - will be initialized in alpine:init
let animationEngine;

// Map instance and layers
let isAppInitialized = false; // Flag to ensure init() runs only once

// Alpine.js data store
document.addEventListener('alpine:init', () => {
    // The store 'atc', assembled from its domains -- www/store/*.js, each a
    // builder pushed to window.AtcStore before this script runs, called here so
    // its entries are built now, inside alpine:init, as the one literal was --
    // and what stays here: the settings, the map's display, the clock, the
    // keyboard, init and cleanup. Getters travel as descriptors: Object.assign
    // would freeze their value.
    const atcStore = {};
    for (const part of [...(window.AtcStore || []).map((build) => build()), {
        // State
        aircraft: [],
        settingsOpen: false, // the settings page (audit 2.1)
        selectedAircraft: null,
        // coordinates removed as they're not needed
        currentTime: new Date().toLocaleTimeString(),
        zuluTime: new Date().toUTCString().match(/(\d{2}:\d{2}:\d{2})/)[0] + 'Z', // Initial Zulu Time
        showLocalDates: localStorage.getItem('showLocalDates') === 'true' || false, // Default to UTC (false)
        timeUpdateIntervalId: null, // Store ID for the time update interval
        mapPerfUpdateIntervalId: null, // Interval for map performance stats polling
        mapPerformanceStats: {
            heapUsedMB: null,
            heapLimitMB: null,
            heapUsagePct: null,
            heapDeltaMB: null,
            animationFps: 0,
            animationFrameMs: 0,
            animationAnimatedThisFrame: 0,
            animationVisibleTargets: 0,
            animationMarkerUpdatesPerSec: 0,
            refreshPerSec: 0,
            avgRefreshMs: 0,
            markerOpsPerSec: 0,
            fullVisibilityPassesPerSec: 0,
            animationCorrectedPerSec: 0,
            animationPredictedPerSec: 0,
            animationDroppedFrames: 0,
            animationQuality: 0,
            wsRates: {
                total: 0,
                aircraft_update: 0,
                aircraft_predicted_state: 0,
                aircraft_added: 0,
                aircraft_removed: 0,
                phase_change: 0,
                transcription: 0,
                frequency_status: 0,
                parse_errors: 0
            },
            tileCache: { ...tileCacheRuntimeStats }
        },
        tileCacheStats: { ...tileCacheRuntimeStats },
        _previousHeapUsedMB: null,

        // True while no valid base map is saved, so the default one is shown.
        mapStyleIsDefault: !MAP_STYLES.includes(localStorage.getItem('mapStyle')),

        // Settings
        settings: {
            // A browser with no valid saved style gets defaultMapStyle.
            mapStyle: (() => {
                const savedStyle = localStorage.getItem('mapStyle');
                return MAP_STYLES.includes(savedStyle) ? savedStyle : defaultMapStyle();
            })(),
            showLabels: JSON.parse(localStorage.getItem('showLabels')) ?? true,
            voiceBubbles: JSON.parse(localStorage.getItem('voiceBubbles')) ?? true, // last transmission over each aircraft named
            showPaths: JSON.parse(localStorage.getItem('showPaths')) ?? true,
            showRings: JSON.parse(localStorage.getItem('showRings')) ?? true,
            showAirports: JSON.parse(localStorage.getItem('showAirports')) ?? true,
            showHeliports: JSON.parse(localStorage.getItem('showHeliports')) ?? true,
            showNavaids: JSON.parse(localStorage.getItem('showNavaids')) ?? true,
            showAllRunways: JSON.parse(localStorage.getItem('showAllRunways')) ?? true,
            showAirportLayout: JSON.parse(localStorage.getItem('showAirportLayout')) ?? true,
            showAltitudeLegend: JSON.parse(localStorage.getItem('showAltitudeLegend')) ?? true,
            // The published routes layer (Q56): off by default; arrivals first.
            showProcedures: JSON.parse(localStorage.getItem('showProcedures')) ?? false,
            proceduresKind: localStorage.getItem('proceduresKind') === 'SID' ? 'SID' : 'STAR',
            proceduresScope: localStorage.getItem('proceduresScope') === 'all' ? 'all' : 'inUse',
            // The probable runway and exit on the labels (Q57): on by default.
            showProbable: JSON.parse(localStorage.getItem('showProbable')) ?? true,
            showAirspaceBoundaries: JSON.parse(localStorage.getItem('showAirspaceBoundaries')) ?? false,
            airspaceOpacity: (() => {
                const value = parseFloat(localStorage.getItem('airspaceOpacity'));
                return Number.isFinite(value) ? value : 0.5;
            })(),
            ringsOpacity: (() => {
                const value = parseFloat(localStorage.getItem('ringsOpacity'));
                return Number.isFinite(value) ? value : 1;
            })(),
            airportsOpacity: (() => {
                const value = parseFloat(localStorage.getItem('airportsOpacity'));
                return Number.isFinite(value) ? value : 1;
            })(),
            heliportsOpacity: (() => {
                const value = parseFloat(localStorage.getItem('heliportsOpacity'));
                return Number.isFinite(value) ? value : 1;
            })(),
            navaidsOpacity: (() => {
                const value = parseFloat(localStorage.getItem('navaidsOpacity'));
                return Number.isFinite(value) ? value : 1;
            })(),
            allRunwaysOpacity: (() => {
                const value = parseFloat(localStorage.getItem('allRunwaysOpacity'));
                return Number.isFinite(value) ? value : 1;
            })(),
            minAltitude: parseInt(localStorage.getItem('minAltitude')) || 0,
            maxAltitude: parseInt(localStorage.getItem('maxAltitude')) || 60000,
            trailLength: parseInt(localStorage.getItem('trailLength')) || 2,
            tracksLimit: parseInt(localStorage.getItem('tracksLimit')) || 1000,
            lastSeenMinutes: (() => {
                const allowed = ['1', '2', '5', '10', '20'];
                const value = (localStorage.getItem('lastSeenMinutes') || '10').trim();
                return allowed.includes(value) ? value : '10';
            })(), // Compact retention presets for sidebar control
            listSort: localStorage.getItem('listSort') || localStorage.getItem('activeListSort') || localStorage.getItem('lostListSort') || 'callsign_az',
            statusFilters: JSON.parse(localStorage.getItem('statusFilters')) || { active: true, stale: true, signal_lost: true },
            showAirAircraft: JSON.parse(localStorage.getItem('showAirAircraft')) ?? true,
            showGroundAircraft: JSON.parse(localStorage.getItem('showGroundAircraft')) ?? true,
            showLocalDates: JSON.parse(localStorage.getItem('showLocalDates')) ?? false, // Default to UTC (false)
            // Show only the aircraft the radio has named. The matching that fills this
            // lives in internal/transcription/phraseology; see docs-fr/19-appariement-en-ligne.md.
            voiceOnly: localStorage.getItem('voiceOnly') === 'true',
            phaseFilters: JSON.parse(localStorage.getItem('phaseFilters')) || { CRZ: true, CLB: true, DEP: true, APP: true, ARR: true, TAX: true, 'T/O': true, 'T/D': true, NEW: true, UNK: true },
            excludeOtherAirportsGrounded: JSON.parse(localStorage.getItem('excludeOtherAirportsGrounded')) ?? false, // Default to false (show all grounded aircraft)
            // Aircraft animation settings
            aircraftAnimation: {
                enabled: JSON.parse(localStorage.getItem('aircraftAnimationEnabled')) ?? true,
                interpolationFps: parseInt(localStorage.getItem('aircraftAnimationFps')) || 30,
                viewportCulling: JSON.parse(localStorage.getItem('aircraftAnimationViewportCulling')) ?? true
            },
        },

        // Add property to track previous settings for change detection
        previousSettings: {},
        needsFullReload: false,
        _settingsSaveTimeoutId: null,
        _settingsSaveDebounceMs: 300,
        _lastSettingsHash: null,

        // Enhanced settings save with change detection for WebSocket
        saveSettings() {
            const previousSettings = { ...this.previousSettings };
            
            // Save to localStorage
            // A default base map is not a choice: it is not saved, so that it can
            // still follow the station's position.
            if (!this.mapStyleIsDefault) localStorage.setItem('mapStyle', this.settings.mapStyle);
            localStorage.setItem('showLabels', this.settings.showLabels);
            localStorage.setItem('voiceBubbles', this.settings.voiceBubbles);
            localStorage.setItem('showPaths', this.settings.showPaths);
            localStorage.setItem('showRings', this.settings.showRings);
            localStorage.setItem('minAltitude', this.settings.minAltitude);
            localStorage.setItem('maxAltitude', this.settings.maxAltitude);
            localStorage.setItem('trailLength', this.settings.trailLength);
            localStorage.setItem('tracksLimit', this.settings.tracksLimit);
            localStorage.setItem('lastSeenMinutes', this.settings.lastSeenMinutes);
            localStorage.setItem('listSort', this.settings.listSort);
            localStorage.setItem('statusFilters', JSON.stringify(this.settings.statusFilters));
            localStorage.setItem('showAirAircraft', this.settings.showAirAircraft);
            localStorage.setItem('showGroundAircraft', this.settings.showGroundAircraft);
            localStorage.setItem('showLocalDates', this.settings.showLocalDates);
            localStorage.setItem('phaseFilters', JSON.stringify(this.settings.phaseFilters));
            localStorage.setItem('voiceOnly', this.settings.voiceOnly);
            localStorage.setItem('excludeOtherAirportsGrounded', this.settings.excludeOtherAirportsGrounded);
            localStorage.setItem('showAirports', this.settings.showAirports);
            localStorage.setItem('showHeliports', this.settings.showHeliports);
            localStorage.setItem('showNavaids', this.settings.showNavaids);
            localStorage.setItem('showAllRunways', this.settings.showAllRunways);
            localStorage.setItem('showAirportLayout', this.settings.showAirportLayout);
            localStorage.setItem('showAltitudeLegend', this.settings.showAltitudeLegend);
            localStorage.setItem('showProcedures', this.settings.showProcedures);
            localStorage.setItem('proceduresKind', this.settings.proceduresKind);
            localStorage.setItem('proceduresScope', this.settings.proceduresScope);
            localStorage.setItem('showProbable', this.settings.showProbable);
            localStorage.setItem('showAirspaceBoundaries', this.settings.showAirspaceBoundaries);
            localStorage.setItem('airspaceOpacity', this.settings.airspaceOpacity);
            localStorage.setItem('ringsOpacity', this.settings.ringsOpacity);
            localStorage.setItem('airportsOpacity', this.settings.airportsOpacity);
            localStorage.setItem('heliportsOpacity', this.settings.heliportsOpacity);
            localStorage.setItem('navaidsOpacity', this.settings.navaidsOpacity);
            localStorage.setItem('allRunwaysOpacity', this.settings.allRunwaysOpacity);

            // Save aircraft animation settings
            localStorage.setItem('aircraftAnimationEnabled', this.settings.aircraftAnimation.enabled);
            localStorage.setItem('aircraftAnimationFps', this.settings.aircraftAnimation.interpolationFps);
            localStorage.setItem('aircraftAnimationViewportCulling', this.settings.aircraftAnimation.viewportCulling);
            
            // Update animation engine configuration if it exists
            if (this.animationEngine) {
                this.animationEngine.updateConfig(this.settings.aircraftAnimation);
            }
            
            // Detect if server-side filter change occurred
            // The altitude band is applied here, in the page: the bulk request
            // does not send it, so moving it needs no reload.
            const serverSideChanged = (
                previousSettings.lastSeenMinutes !== this.settings.lastSeenMinutes ||
                previousSettings.excludeOtherAirportsGrounded !== this.settings.excludeOtherAirportsGrounded
            );
            
            if (serverSideChanged && Object.keys(previousSettings).length > 0) {
                this.needsFullReload = true;
            }
            
            // Store current settings for next comparison
            this.previousSettings = { ...this.settings };
        },

        // Debounced settings save to reduce frequent writes and reactivity churn
        queueSaveSettings() {
            const settingsHash = JSON.stringify(this.settings);
            if (this._lastSettingsHash === settingsHash) {
                return;
            }
            this._lastSettingsHash = settingsHash;

            if (this._settingsSaveTimeoutId) {
                clearTimeout(this._settingsSaveTimeoutId);
            }

            this._settingsSaveTimeoutId = setTimeout(() => {
                this._settingsSaveTimeoutId = null;
                this.saveSettings();
            }, this._settingsSaveDebounceMs);
        },

        // Enhanced settings change handlers for WebSocket
        onFilterChange() {
            this.queueSaveSettings();
            
            // Check if this is a server-side filter change that requires bulk reload
            if (this.needsFullReload) {
                console.log('Server-side filter change detected, requesting bulk reload via WebSocket...');
                this.requestInitialAircraftData(); // Use WebSocket bulk request
                this.needsFullReload = false;
            } else {
                // Simple filters like search, phase, air/ground can be handled client-side
                this._lastFilterHash = null;
                
                if (this.mapManager) {
                    this.mapManager.applyFiltersAndRefreshView();
                }
            }
        },

        // MapManager instance
        mapManager: null, // Will be set in alpine:init

        // Methods
        async init() {
            if (isAppInitialized) {
                console.warn("Application already initialized. Skipping init() call.");
                return;
            }

            if (!document.getElementById('map')) {
                console.error("Map container #map not found in DOM. Aborting initialization.");
                return;
            }
            
            if (!audioClient || !this.mapManager) { // Ensure mapManager is also ready
                return; 
            }
            console.log("Alpine store init() invoked.");

            try {
                await this.fetchStationData();
                await this.fetchWeatherData();
                this.startRightNow();
                this.startThread();
                await this.fetchADSBSourceStatus();
                // Initialize map using MapManager
                if (this.mapManager && !this.mapManager.map) {
                    this.mapManager.initMap();

                    // Apply persisted map style/overlay visibility after overlay registry is initialized
                    this.applyMapDisplaySettings();
                    
                    // Draw range rings after map is initialized
                    this.updateStationRings();
                    
                    // Draw runways if data is available
                    if (this.runwayData) {
                        this.mapManager.drawRunways(this.runwayData);
                    }
                } else if (this.mapManager && this.mapManager.map) {
                     console.warn("Alpine store init: MapManager's map already initialized.");
                } else {
                    console.error("Alpine store init: mapManager not available for map initialization.");
                }

                this.fetchAudioFrequencies();

                // Fetch reference data (airports, navaids, runways) after map init
                this.fetchReferenceData();
                this.loadProcedureRoutes();
                this.startProbable();

                // CRITICAL FIX: Check server config to determine if WebSocket streaming is enabled
                await this.initAircraftDataSource();

                // Initialize previous settings for change detection
                this.previousSettings = { ...this.settings };

                // Manage the current time and Zulu time update interval
                if (this.timeUpdateIntervalId) {
                    clearInterval(this.timeUpdateIntervalId);
                }
                this.updateCurrentTime(); // Initial call
                this.timeUpdateIntervalId = setInterval(() => {
                    this.updateCurrentTime();
                    this.currentTimeForPhases = new Date(); // Update time for phase history
                    if (this.connected && this.lastUpdate) {
                        this.lastUpdateSeconds = Math.floor((new Date() - this.lastUpdate) / 1000);
                    }
                    // Purge stale aircraft from the store every 30s
                    // Uses the same lastSeenMinutes threshold the server uses on bulk load
                    this._staleCleanupCounter = (this._staleCleanupCounter || 0) + 1;
                    if (this._staleCleanupCounter >= 30) {
                        this._staleCleanupCounter = 0;
                        this._purgeStaleAircraft();
                    }
                }, 1000);

                // New interval for updating "seconds since last audio"
                if (this.lastAudioUpdateIntervalId) {
                    clearInterval(this.lastAudioUpdateIntervalId);
                }
                this.updateSecondsSinceLastAudio(); // Initial call
                this.lastAudioUpdateIntervalId = setInterval(() => {
                    this.updateSecondsSinceLastAudio();
                }, 1000);

                // Poll map performance metrics every 2s (debug only, lightweight)
                if (this.mapPerfUpdateIntervalId) {
                    clearInterval(this.mapPerfUpdateIntervalId);
                }
                this.updateMapPerformanceStats();
                this.mapPerfUpdateIntervalId = setInterval(() => {
                    this.updateMapPerformanceStats();
                }, 2000);

                // A hovered aircraft (in the list or on the map) is drawn as such
                // on the map, and the one hovered before is drawn back.
                let previousHoveredHex = null;
                Alpine.effect(() => {
                    const currentHoveredAircraft = Alpine.store('atc').hoveredAircraft;
                    if (previousHoveredHex && (!currentHoveredAircraft || previousHoveredHex !== currentHoveredAircraft.hex)) {
                        if (this.mapManager) this.mapManager.updateVisualState(previousHoveredHex, true);
                    }
                    if (currentHoveredAircraft) {
                        if (this.mapManager) this.mapManager.updateVisualState(currentHoveredAircraft.hex, true);
                        previousHoveredHex = currentHoveredAircraft.hex;
                    } else {
                        previousHoveredHex = null;
                    }
                });

                let previousSelectedHex = null; // Keep this to know if an aircraft was just selected from null
                Alpine.effect(() => {
                    const currentHex = this.selectedAircraft ? this.selectedAircraft.hex : null;
                    if (currentHex === previousSelectedHex) {
                        return;
                    }

                    this.setupAircraftDetailsPanel();

                    if (this.mapManager) {
                        this.mapManager.applyFiltersAndRefreshView();
                    }

                    previousSelectedHex = currentHex;
                });
                
                // Watch for changes to showLocalDates
                Alpine.effect(() => {
                    const showLocalDates = this.showLocalDates;
                    this.settings.showLocalDates = showLocalDates;
                    this.queueSaveSettings();
                });

                // Watch for changes to settings
                Alpine.effect(() => {
                    // Trigger effect on any setting changes without deep clone churn
                    JSON.stringify(this.settings);
                    this.queueSaveSettings();
                });
                
                isAppInitialized = true;
                console.log("Application initialization successful.");

                // Setup keyboard event listeners
                this.setupKeyboardEvents();
                this.armResumeOnFirstGesture();
                
                audioClient.initAudioContext();

                // Setup cleanup handler for page unload (prevents memory leaks)
                window.addEventListener('beforeunload', () => {
                    this.cleanup();
                });

            } catch (error) {
                console.error("Error during application initialization:", error);
            }
        },

        // Centralized cleanup to prevent memory leaks during long sessions
        cleanup() {
            console.log('App cleanup: Clearing all intervals and resources...');

            // Clear all interval IDs
            const intervalIds = [
                'timeUpdateIntervalId',
                'lastAudioUpdateIntervalId',
                'mapPerfUpdateIntervalId',
                'stationRefreshInterval',
                'weatherRefreshInterval',
                'adsbSourceRefreshInterval',
                'aircraftDetailsHistoryRefreshInterval',
                'proximityRefreshInterval',
                'phaseHistoryRefreshInterval'
            ];

            intervalIds.forEach(id => {
                if (this[id]) {
                    clearInterval(this[id]);
                    this[id] = null;
                }
            });

            // Stop animation engine
            if (this.animationEngine) {
                this.animationEngine.stop();
            }

            // Release map resources/listeners/timers
            if (this.mapManager && this.mapManager.cleanup) {
                this.mapManager.cleanup();
            }

            // Disconnect WebSocket and clear listeners
            if (wsClient) {
                wsClient.clearAllListeners();
                wsClient.disconnect();
            }

            console.log('App cleanup: Complete');
        },

        updateCurrentTime() {
            const now = new Date();
            this.currentTime = now.toLocaleTimeString();
            this.zuluTime = now.toUTCString().match(/(\d{2}:\d{2}:\d{2})/)[0] + 'Z';
        },
        
        // Format a date based on user preference (local or UTC)
        formatDate(dateString, timeOnly = false) {
            if (!dateString) return '-';
            
            const date = new Date(dateString);
            if (isNaN(date.getTime())) return 'Invalid Date';
            
            if (timeOnly) {
                // Only show hours, minutes, and seconds
                if (this.showLocalDates) {
                    return date.toLocaleTimeString();
                } else {
                    return date.toISOString().substring(11, 19) + 'Z';
                }
            } else {
                // Show full date and time
                if (this.showLocalDates) {
                    return date.toLocaleString();
                } else {
                    return date.toISOString().replace('T', ' ').substring(0, 19) + 'Z';
                }
            }
        },

        // Settings methods
        toggleLabels() {
            this.saveSettings();
            if (this.mapManager) this.mapManager.applyFiltersAndRefreshView();
        },

        toggleVoiceBubbles() {
            this.saveSettings();
        },

        togglePaths() {
            this.saveSettings();
            if (this.mapManager) this.mapManager.applyFiltersAndRefreshView();
        },

        toggleRings() {
            this.saveSettings();
            if (this.mapManager) {
                this.mapManager.toggleRings();
            }
        },

        setMapStyle() {
            if (!MAP_STYLES.includes(this.settings.mapStyle)) {
                this.settings.mapStyle = defaultMapStyle(this.stationLatitude, this.stationLongitude);
            }
            this.mapStyleIsDefault = false;
            this.saveSettings();
            if (this.mapManager && typeof this.mapManager.setMapStyle === 'function') {
                this.mapManager.setMapStyle(this.settings.mapStyle);
            }
        },

        // The station's position is known: a base map nobody chose follows it.
        applyDefaultMapStyle() {
            if (!this.mapStyleIsDefault) return;
            const style = defaultMapStyle(this.stationLatitude, this.stationLongitude);
            if (style === this.settings.mapStyle) return;
            this.settings.mapStyle = style;
            if (this.mapManager && typeof this.mapManager.setMapStyle === 'function') {
                this.mapManager.setMapStyle(style);
            }
        },

        toggleAnimation() {
            this.saveSettings();
            if (this.animationEngine) {
                if (this.settings.aircraftAnimation.enabled) {
                    this.animationEngine.start();
                    console.log('Aircraft animation enabled');
                } else {
                    this.animationEngine.stop();
                    console.log('Aircraft animation disabled');
                }
            }
        },

        // A map layer whose visibility and opacity are settings: save them, then
        // apply the one that changed. settingKey names it in this.settings.
        applyLayerVisibility(layerId, settingKey) {
            this.saveSettings();
            if (this.mapManager) {
                this.mapManager.toggleLayerVisibility(layerId, this.settings[settingKey]);
            }
        },

        applyLayerOpacity(layerId, settingKey) {
            this.saveSettings();
            if (this.mapManager && typeof this.mapManager.setLayerOpacity === 'function') {
                this.mapManager.setLayerOpacity(layerId, this.settings[settingKey]);
            }
        },

        toggleAirports() { this.applyLayerVisibility('airports', 'showAirports'); },
        toggleHeliports() { this.applyLayerVisibility('heliports', 'showHeliports'); },
        toggleNavaids() { this.applyLayerVisibility('navaids', 'showNavaids'); },
        toggleAllRunways() { this.applyLayerVisibility('allRunways', 'showAllRunways'); },
        toggleAirportLayout() { this.applyLayerVisibility('airportLayout', 'showAirportLayout'); },
        toggleAirspaceBoundaries() { this.applyLayerVisibility('airspace-polygons', 'showAirspaceBoundaries'); },
        setAirspaceOpacity() { this.applyLayerOpacity('airspace-polygons', 'airspaceOpacity'); },
        setRingsOpacity() { this.applyLayerOpacity('rangeRings', 'ringsOpacity'); },
        setAirportsOpacity() { this.applyLayerOpacity('airports', 'airportsOpacity'); },
        setHeliportsOpacity() { this.applyLayerOpacity('heliports', 'heliportsOpacity'); },
        setNavaidsOpacity() { this.applyLayerOpacity('navaids', 'navaidsOpacity'); },
        setAllRunwaysOpacity() { this.applyLayerOpacity('allRunways', 'allRunwaysOpacity'); },

        applyMapDisplaySettings() {
            if (!this.mapManager) return;

            if (typeof this.mapManager.setMapStyle === 'function') {
                this.mapManager.setMapStyle(this.settings.mapStyle);
            }

            this.mapManager.toggleLayerVisibility('airspace-polygons', this.settings.showAirspaceBoundaries);
            this.mapManager.toggleLayerVisibility('rangeRings', this.settings.showRings);
            this.mapManager.toggleLayerVisibility('airports', this.settings.showAirports);
            this.mapManager.toggleLayerVisibility('heliports', this.settings.showHeliports);
            this.mapManager.toggleLayerVisibility('navaids', this.settings.showNavaids);
            this.mapManager.toggleLayerVisibility('allRunways', this.settings.showAllRunways);
            this.mapManager.toggleLayerVisibility('airportLayout', this.settings.showAirportLayout);
            if (typeof this.mapManager.setLayerOpacity === 'function') {
                this.mapManager.setLayerOpacity('airspace-polygons', this.settings.airspaceOpacity);
                this.mapManager.setLayerOpacity('rangeRings', this.settings.ringsOpacity);
                this.mapManager.setLayerOpacity('airports', this.settings.airportsOpacity);
                this.mapManager.setLayerOpacity('heliports', this.settings.heliportsOpacity);
                this.mapManager.setLayerOpacity('navaids', this.settings.navaidsOpacity);
                this.mapManager.setLayerOpacity('allRunways', this.settings.allRunwaysOpacity);
            }
        },

        // Poll map performance stats from MapManager
        updateMapPerformanceStats() {
            if (!this.mapManager || !this.mapManager.getPerformanceStats) return;
            requestTileCacheStatsFromSW();
            const mapStats = this.mapManager.getPerformanceStats();
            const animationStats = this.animationEngine ? this.animationEngine.getStats() : null;
            const wsStats = wsClient && wsClient.getMessageRateStats
                ? wsClient.getMessageRateStats()
                : null;
            const smoothingEnabled = !!this.settings?.aircraftAnimation?.enabled;
            const measuredFps = animationStats && Number.isFinite(animationStats.measuredFps)
                ? Number(animationStats.measuredFps.toFixed(1))
                : 0;
            const animationFrameMs = smoothingEnabled && animationStats && Number.isFinite(animationStats.averageFrameTime)
                ? Number(animationStats.averageFrameTime.toFixed(2))
                : null;
            const animationVisibleTargets = smoothingEnabled && animationStats && Number.isFinite(animationStats.visibleTargets)
                ? animationStats.visibleTargets
                : null;
            const animationAnimatedThisFrame = smoothingEnabled && animationStats && Number.isFinite(animationStats.animatedThisFrame)
                ? animationStats.animatedThisFrame
                : null;
            const animationPredictedPerSec = smoothingEnabled && animationStats && Number.isFinite(animationStats.predictedPerSec)
                ? Number(animationStats.predictedPerSec.toFixed(1))
                : null;
            const animationCorrectedPerSec = smoothingEnabled && animationStats && Number.isFinite(animationStats.correctedPerSec)
                ? Number(animationStats.correctedPerSec.toFixed(1))
                : null;
            const animationMarkerUpdatesPerSec = smoothingEnabled && animationStats && Number.isFinite(animationStats.markerUpdatesPerSec)
                ? Number(animationStats.markerUpdatesPerSec.toFixed(1))
                : null;
            const animationQuality = smoothingEnabled && animationStats && Number.isFinite(animationStats.qualityLevel)
                ? Number(animationStats.qualityLevel.toFixed(2))
                : null;
            const animationDroppedFrames = smoothingEnabled && animationStats && Number.isFinite(animationStats.droppedFrames)
                ? animationStats.droppedFrames
                : null;

            const wsByType = wsStats?.byTypePerSec || {};
            const wsRates = {
                total: Number.isFinite(wsStats?.totalPerSec) ? wsStats.totalPerSec : 0,
                aircraft_update: Number.isFinite(wsByType.aircraft_update) ? wsByType.aircraft_update : 0,
                aircraft_predicted_state: Number.isFinite(wsByType.aircraft_predicted_state) ? wsByType.aircraft_predicted_state : 0,
                aircraft_added: Number.isFinite(wsByType.aircraft_added) ? wsByType.aircraft_added : 0,
                aircraft_removed: Number.isFinite(wsByType.aircraft_removed) ? wsByType.aircraft_removed : 0,
                phase_change: Number.isFinite(wsByType.phase_change) ? wsByType.phase_change : 0,
                transcription: Number.isFinite(wsByType.transcription) ? wsByType.transcription : 0,
                frequency_status: Number.isFinite(wsByType.frequency_status) ? wsByType.frequency_status : 0,
                parse_errors: Number.isFinite(wsStats?.parseErrorsPerSec) ? wsStats.parseErrorsPerSec : 0
            };

            const effectiveUpdateRate = Number((wsRates.aircraft_update + wsRates.aircraft_predicted_state).toFixed(1));
            const displayRenderRate = smoothingEnabled ? measuredFps : effectiveUpdateRate;

            const fullVisibilityPassesPerSec = Number.isFinite(mapStats?.windowSec) && mapStats.windowSec > 0
                ? Number((mapStats.fullVisibilityPasses / mapStats.windowSec).toFixed(2))
                : 0;

            const memory = performance && performance.memory ? performance.memory : null;
            const heapUsedMB = memory && Number.isFinite(memory.usedJSHeapSize)
                ? Number((memory.usedJSHeapSize / (1024 * 1024)).toFixed(1))
                : null;
            const heapLimitMB = memory && Number.isFinite(memory.jsHeapSizeLimit)
                ? Number((memory.jsHeapSizeLimit / (1024 * 1024)).toFixed(1))
                : null;
            const heapUsagePct = (heapUsedMB !== null && heapLimitMB && heapLimitMB > 0)
                ? Number(((heapUsedMB / heapLimitMB) * 100).toFixed(1))
                : null;
            const heapDeltaMB = (heapUsedMB !== null && this._previousHeapUsedMB !== null)
                ? Number((heapUsedMB - this._previousHeapUsedMB).toFixed(1))
                : null;
            this._previousHeapUsedMB = heapUsedMB;

            this.mapPerformanceStats = {
                ...mapStats,
                smoothingEnabled: smoothingEnabled,
                animationFps: displayRenderRate,
                animationFrameMs: animationFrameMs,
                animationVisibleTargets: animationVisibleTargets,
                animationAnimatedThisFrame: animationAnimatedThisFrame,
                animationPredictedPerSec: animationPredictedPerSec,
                animationCorrectedPerSec: animationCorrectedPerSec,
                animationMarkerUpdatesPerSec: animationMarkerUpdatesPerSec,
                animationQuality: animationQuality,
                animationDroppedFrames: animationDroppedFrames,
                heapUsedMB: heapUsedMB,
                heapLimitMB: heapLimitMB,
                heapUsagePct: heapUsagePct,
                heapDeltaMB: heapDeltaMB,
                fullVisibilityPassesPerSec: fullVisibilityPassesPerSec,
                wsWindowSec: Number.isFinite(wsStats?.windowSec) ? wsStats.windowSec : 0,
                wsRates: wsRates,
                tileCache: { ...this.tileCacheStats }
            };
        },

        // Setup keyboard event listeners
        // Keyboard (migration step 11): Tab is the browser's again, to move
        // through the page; J and K, or the down and up arrows on the map, go
        // from one aircraft to the next (the owner's choice, 13/10). The single
        // letters never fire with Cmd, Ctrl or Alt: Cmd-A no longer toggles the
        // airborne filter. Enter and Space activate what plays the part of a
        // button without being one (role="button", role="checkbox").
        setupKeyboardEvents() {
            document.addEventListener('keydown', (e) => {
                const t = e.target;
                const isInInputField = t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' ||
                    t.tagName === 'SELECT' || t.isContentEditable;

                // Not when the element handled the key itself (its own
                // @keydown.space.prevent): it would toggle twice.
                if ((e.key === 'Enter' || e.key === ' ') && !e.defaultPrevented && !isInInputField && t.tagName !== 'BUTTON' && t.tagName !== 'A' &&
                    t.matches && t.matches('[role="button"], [role="checkbox"], [role="radio"], [role="switch"]')) {
                    e.preventDefault();
                    t.click();
                    return;
                }

                // Escape stops following, unless a page or a menu is open: that
                // one closes first, by its own handler.
                if (e.key === 'Escape' && this.selectedAircraft) {
                    const events = window.Alpine && Alpine.store('events');
                    if (!this.settingsOpen && !this.layersOpen && !(events && events.open) && !this.threadMenuFor) {
                        this.stopFollowing();
                    }
                }

                if (isInInputField || e.metaKey || e.ctrlKey || e.altKey) return;

                // The arrows only where they move nothing else: on the map, or
                // with the focus nowhere.
                const onSky = t === document.body || (t.closest && t.closest('#map, .cl-sky__top, .cl-sky__corner'));
                const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
                if (key === 'j' || (key === 'ArrowDown' && onSky)) {
                    e.preventDefault();
                    this.cycleToNextAircraft();
                    return;
                }
                if (key === 'k' || (key === 'ArrowUp' && onSky)) {
                    e.preventDefault();
                    this.cycleToPreviousAircraft();
                    return;
                }

                if (key === 'a') {
                    e.preventDefault();
                    this.toggleAirAircraft();
                    return;
                }
                if (key === 'g') {
                    e.preventDefault();
                    this.toggleGroundAircraft();
                    return;
                }

                // Flight phase filters (1-9, 0), in the order of the filter bar.
                const phaseKeys = {
                    '1': 'NEW', '2': 'TAX', '3': 'T/O', '4': 'CLB', '5': 'DEP',
                    '6': 'CRZ', '7': 'ARR', '8': 'APP', '9': 'T/D', '0': 'UNK',
                };
                if (phaseKeys[e.key]) {
                    e.preventDefault();
                    this.togglePhaseFilter(phaseKeys[e.key]);
                }
            });
        },
    }]) {
        for (const key of Object.keys(part)) {
            if (key in atcStore) throw new Error(`store 'atc': ${key} is defined twice`);
        }
        Object.defineProperties(atcStore, Object.getOwnPropertyDescriptors(part));
    }
    Alpine.store('atc', atcStore);

    // Initialize audioClient AFTER the store is defined
    audioClient = new AudioClient(Alpine.store('atc'));
    
    // Initialize OpenLayers map manager only
    if (!window.OpenLayersMapManager || typeof window.OpenLayersMapManager.createOpenLayersMapManager !== 'function') {
        throw new Error('OpenLayersMapManager is unavailable; OpenLayers runtime bootstrap failed');
    }
    mapManager = window.OpenLayersMapManager.createOpenLayersMapManager(Alpine.store('atc'), CONFIG);
    Alpine.store('atc').mapManager = mapManager; // Make mapManager accessible in the store

    // Initialize Aircraft Animation Engine
    animationEngine = new AircraftAnimationEngine(mapManager, Alpine.store('atc'));
    Alpine.store('atc').animationEngine = animationEngine; // Make animation engine accessible in the store
    animationEngine.initialize();

    // Now initialize the store's own logic
    Alpine.store('atc').init();

    // The PAR view takes the map's place when chosen (docs-fr/31-vue-par.md)
    Alpine.store('atc').parView = window.ParView.create(document.getElementById('par-view'), Alpine.store('atc'));
    Alpine.store('atc').setMainView(Alpine.store('atc').mainView);

    // Dialogs and popovers take the focus (design review of step 11, AppShell).
    // The settings page is a modal dialog: the rest of the page is inert while
    // it is open, the focus goes into it, and comes back to the gear when it
    // closes. A popover (the event log, a channel's menu, the layers) puts the
    // focus on its first control and gives it back to the button that opened
    // it. Before this, Tab went on behind them: unreachable by keyboard.
    {
        const store = Alpine.store('atc');
        const events = Alpine.store('events');
        const SELECTABLE = 'button:not([disabled]):not([tabindex="-1"]), a[href], input:not([type="hidden"]):not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])';
        const shown = (el) => !!el && el.isConnected && el.getClientRects().length > 0;
        const firstIn = (el) => el && [...el.querySelectorAll(SELECTABLE)].find(shown);
        const madeInert = [];
        const inertAround = (root) => {
            for (let node = root; node && node.parentElement && node !== document.body; node = node.parentElement) {
                for (const sib of node.parentElement.children) {
                    if (sib !== node && !sib.inert && sib.tagName !== 'SCRIPT') { sib.inert = true; madeInert.push(sib); }
                }
            }
        };
        const releaseInert = () => { while (madeInert.length) madeInert.pop().inert = false; };

        const follow = ({ isOpen, root, opener, modal, close }) => {
            let last = false;
            let from = null;
            let watched = null;
            // A popover closes when the focus leaves it (Tab past its last
            // control): left open behind, Escape no longer reached it.
            const onFocusOut = (e) => {
                const el = watched;
                if (!el || !close) return;
                const to = e.relatedTarget;
                if (to && !el.contains(to)) close();
            };
            Alpine.effect(() => {
                const open = !!isOpen();
                if (open === last) return;
                last = open;
                if (open) {
                    from = document.activeElement;
                    const place = () => {
                        const el = root();
                        if (!shown(el)) return false;
                        if (modal) inertAround(el);
                        else if (close) { watched = el; el.addEventListener('focusout', onFocusOut); }
                        const target = firstIn(el);
                        if (target) target.focus();
                        return true;
                    };
                    // Drawn in a frame or two.
                    setTimeout(() => { if (!place()) setTimeout(place, 150); }, 30);
                } else {
                    if (modal) releaseInert();
                    if (watched) { watched.removeEventListener('focusout', onFocusOut); watched = null; }
                    setTimeout(() => {
                        const a = document.activeElement;
                        if (a && a !== document.body && shown(a)) return;
                        const back = (from && shown(from)) ? from : opener();
                        if (back && shown(back)) back.focus();
                    }, 30);
                }
            });
        };

        follow({
            isOpen: () => store.settingsOpen,
            root: () => document.querySelector('.cl-page[role="dialog"]'),
            opener: () => document.querySelector('header [aria-label="Settings"]'),
            modal: true,
        });
        follow({
            isOpen: () => events.open,
            root: () => document.querySelector('[role="dialog"][aria-label="Event log"]'),
            opener: () => document.querySelector('header [aria-label="Event log"]'),
            close: () => { events.open = false; },
        });
        follow({
            isOpen: () => store.layersOpen,
            root: () => document.querySelector('.cl-menu--map'),
            opener: () => document.querySelector('[aria-label="Layers and base map"]'),
            close: () => { store.layersOpen = false; },
        });
        follow({
            isOpen: () => store.threadMenuFor,
            root: () => [...document.querySelectorAll('.cl-threadhead__row--channels .cl-menu')].find(shown),
            opener: () => [...document.querySelectorAll('.cl-channel__caret[aria-expanded="true"]')][0] || null,
            close: () => { store.threadMenuFor = null; },
        });
    }

    // The keyboard's focus follows a flight (step 11), wherever it was
    // started from (a strip, an arrival row, a Nearby row, a thread chip,
    // the map): the element that started it often goes when the column
    // turns to the story. While the focus is on that element or nowhere, it
    // moves to the story's back button; when the flight is let go, it goes
    // back to that element if it is still there, else to the aircraft's
    // strip, else to the column's tab it came from.
    {
        const store = Alpine.store('atc');
        const shown = (el) => !!el && el.isConnected && el.getClientRects().length > 0;
        let previous = null;
        let origin = null;
        // Only for the keyboard: a mouse user's focus is left where it is,
        // and the list not scrolled to a strip they did not ask for.
        let keyboard = false;
        document.addEventListener('keydown', () => { keyboard = true; }, true);
        document.addEventListener('pointerdown', () => { keyboard = false; }, true);
        Alpine.effect(() => {
            const hex = store.selectedAircraft ? store.selectedAircraft.hex : null;
            if (hex === previous) return;
            const left = previous;
            previous = hex;
            const startedFrom = document.activeElement;
            if (hex && !left) origin = startedFrom;
            const lost = (a) => !a || a === document.body || a === startedFrom || !shown(a);
            const place = () => {
                if (!keyboard || !lost(document.activeElement)) return true;
                const column = document.getElementById('region-column');
                let target = null;
                if (hex) {
                    target = column && [...column.querySelectorAll('.cl-tabs--column button')].find(shown);
                } else {
                    target = (shown(origin) && origin)
                        // Its strip or its arrival row, drawn again since.
                        || (left && column && [...column.querySelectorAll(`[data-aircraft-hex="${left}"]`)].find(shown))
                        || (column && [...column.querySelectorAll('[role="tab"][aria-selected="true"]')].find(shown))
                        || column;
                    origin = null;
                }
                if (!target || !shown(target)) return false;
                target.focus();
                return true;
            };
            // The new column is drawn in a frame or two: tried a few times.
            for (const ms of [30, 150, 500]) setTimeout(place, ms);
        });

        // Folding the column takes its fold button away: the focus goes to
        // the unfold button of the folded card, and back (design review of
        // step 11). Only after a key, too.
        let folded = store.columnFolded;
        Alpine.effect(() => {
            const now = store.columnFolded;
            if (now === folded) return;
            folded = now;
            const place = () => {
                const a = document.activeElement;
                if (!keyboard || (a && a !== document.body && shown(a))) return;
                const target = [...document.querySelectorAll(now ? '[aria-label="Unfold the column"]' : '[aria-label="Fold the column"]')].find(shown);
                if (target) target.focus();
            };
            for (const ms of [30, 150, 500]) setTimeout(place, ms);
        });
    }
    // The followed aircraft's approach, in its runway's frame under the map
    // (step 8): shown while it is in an arrival sequence and the map is on.
    {
        const store = Alpine.store('atc');
        const frameEl = document.getElementById('par-frame');
        const frame = window.ParView.create(frameEl, store, { single: true });
        if (typeof ResizeObserver === 'function') {
            new ResizeObserver(() => { store.parFrameHeight = Math.round(frameEl.offsetHeight); }).observe(frameEl);
        }
        let shownFor = '';
        Alpine.effect(() => {
            const code = store.storyFrameAirport;
            if (code === shownFor) return;
            shownFor = code;
            if (code) frame.show(code); else frame.hide();
        });
    }

    // On a phone, selecting an aircraft on the map opens its panel, which lives
    // in the list pane (audit 4.2).
    Alpine.effect(() => {
        const store = Alpine.store('atc');
        // On the phone a flight's story is in the column (PhoneShell): chosen
        // from the thread or the sky, it opens in Traffic; from Traffic or
        // Right now, it stays there and its back button returns there.
        if (store.selectedAircraft && window.matchMedia('(max-width: 767px)').matches && !store.phoneColumn) store.showPane('traffic');
    });

    // Watch for aircraft selection changes to clear pending requests and trails
    Alpine.effect(() => {
        const store = Alpine.store('atc');
        const currentSelectedHex = store.selectedAircraft?.hex || null;
        const previousSelectedHex = store._previousSelectedHex || null;

        if (currentSelectedHex === previousSelectedHex) {
            return;
        }

        // If aircraft selection changed
        if (previousSelectedHex && previousSelectedHex !== currentSelectedHex) {
            // Clear pending requests for the previous aircraft
            store.clearPendingRequestsForAircraft(previousSelectedHex);

            // Clear trails for the previous aircraft when deselecting
            if (store.mapManager) {
                store.mapManager.clearAircraftTrails(previousSelectedHex);
            }
        }

        // Store current selection for next comparison
        store._previousSelectedHex = currentSelectedHex;

        // Refresh map visibility when selected aircraft changes to show/hide aircraft based on filters
        if (store.mapManager) {
            store.mapManager.applyFiltersAndRefreshView();
        }
    });
});
