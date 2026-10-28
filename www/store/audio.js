// The radios: the frequencies listened to, their streams and levels, what
// was heard last, the archived clips (audio-client.js does the Web Audio work).
//
// One domain of the Alpine store 'atc' (docs-fr/32, palier 5.1): the entries
// below are the store's, moved from app.js as they were, and app.js assembles
// them with the rest. A method's `this` is the store. The entries are built
// when app.js assembles the store, inside alpine:init as they always were: by
// then the module-level names of app.js (generateUUID, API_BASE_URL, CONFIG,
// MAP_STYLES, wsClient, audioClient, mapManager) are defined, and a method
// resolves them when it runs.
(function () {
    window.AtcStore = window.AtcStore || [];
    window.AtcStore.push(() => ({
        audioFrequencies: [],
        clientID: generateUUID(), // Unique client ID for audio streams
        unmutedFrequencies: new Set(), // Set of unmuted frequency IDs
        restoreListeningPending: false, // "Start Monitoring" clicked before the frequency list arrived
        audioElements: {}, // Map of frequency ID to audio element
        audioAnalysers: {}, // Map of frequency ID to analyser node
        audioDataArrays: {}, // Map of frequency ID to data array
        visualizationFrameIds: {}, // Map of frequency ID to animation frame ID
        sourceNodes: {},
        radiosStarted: false, // Initialize radiosStarted to false
        audioApiUrl: `${API_BASE_URL}/frequencies`,
        // Listening to a transmission again (audit 3.2): whether the sidecar's
        // archive of clips is there, and the clip playing, one at a time.
        audioArchive: { available: false },
        clip: { id: null, state: 'idle', failed: {} },
        resumeListeningPending: false, // remembered listening waits for the page's first gesture
        audioRunning: false, // the page's sound runs (audio-client.js, _watchContext)
        userSetVolumes: {}, // Initialize as empty object
        lastSignificantAudioTime: {}, // Stores timestamp for each freqId
        secondsSinceLastAudio: {},  // Stores formatted string for display (e.g., "5s")
        lastAudioUpdateIntervalId: null, // Interval ID for updating secondsSinceLastAudio
        frequencyConnectionStatus: {}, // Tracks connection status per frequency (connecting, connected, failed)

        // Helper method to check if a frequency is unmuted
        isUnmuted(frequencyId) {
            return this.unmutedFrequencies.has(frequencyId);
        },



        updateSecondsSinceLastAudio() {
            if (!audioClient) { console.warn("audioClient not ready in updateSecondsSinceLastAudio (store)"); return; }
            audioClient.updateSecondsSinceLastAudio(this.secondsSinceLastAudio); // Pass the store's object to be updated
        },

        // Plays a transmission's clip, as the receiver heard it; the same
        // button stops it. One clip at a time: another one stops the first.
        playClip(id) {
            if (!id) return;
            const el = this._clipAudio || (this._clipAudio = new Audio());
            // Live audio is lowered while a clip plays, and restored after.
            const duck = (on) => { if (audioClient) audioClient.setDucked(on); };
            if (this.clip.id === id && this.clip.state !== 'idle') {
                el.pause();
                duck(false);
                this.clip = { ...this.clip, id: null, state: 'idle' };
                return;
            }
            el.pause();
            el.onplaying = () => { if (this.clip.id === id) { duck(true); this.clip = { ...this.clip, state: 'playing' }; } };
            el.onended = () => { if (this.clip.id === id) { duck(false); this.clip = { ...this.clip, id: null, state: 'idle' }; } };
            el.onerror = () => {
                if (this.clip.id !== id) return;
                duck(false);
                this.clip = { id: null, state: 'idle', failed: { ...this.clip.failed, [id]: true } };
            };
            this.clip = { ...this.clip, id, state: 'loading' };
            el.src = `${API_BASE_URL}/transcriptions/${id}/audio`;
            el.play().catch(() => { /* onerror says why, or the page has had no click yet */ });
        },

        clipIcon(id) {
            if (this.clip.failed[id]) return 'fa-volume-xmark';
            if (this.clip.id !== id) return 'fa-play';
            return this.clip.state === 'loading' ? 'fa-spinner fa-spin' : 'fa-stop';
        },

        clipHelp(id) {
            if (this.clip.failed[id]) {
                return 'No audio was kept for this transmission: the archive was off, its disk full or unplugged, or the transmission is older than the archive.';
            }
            if (this.clip.id === id) return 'Stop listening.';
            return 'Listen to this transmission again, as the receiver heard it, to check what was transcribed.';
        },
        
        async fetchAudioFrequencies() {
            try {
                const response = await fetch(this.audioApiUrl);
                if (!response.ok) {
                    console.error(`HTTP error fetching frequencies! Status: ${response.status}`);
                    this.audioFrequencies = [];
                    return;
                }
                const data = await response.json();
                if (data && data.frequencies) {
                    this.audioFrequencies = data.frequencies;

                    // Populate connection status from API response
                    data.frequencies.forEach(freq => {
                        if (freq.status && freq.status !== 'available') {
                            this.frequencyConnectionStatus[freq.id] = {
                                status: freq.status,
                                error: freq.last_error || null,
                                lastUpdate: Date.now()
                            };
                            console.log(`Frequency ${freq.id} initial status: ${freq.status}`, freq.last_error || '');
                        }
                    });

                    // Load historical transcriptions per frequency
                    if (data.transcriptions) {
                        for (const [freqId, txns] of Object.entries(data.transcriptions)) {
                            if (txns && txns.length > 0) {
                                // Normalize API field names to match WS format and mark as historical
                                const normalized = txns.map(t => ({
                                    ...t,
                                    timestamp: t.created_at,
                                    text: t.content,
                                    _historical: true
                                }));
                                this.frequencyTranscriptions[freqId] = normalized;
                                this.originalTranscriptions[freqId] = [...normalized];
                            }
                        }
                    }

                    // DO NOT connect to all frequencies immediately here.
                    // Let the user click "Start Radios"
                    // this.connectToAllFrequencies();
                    // Instead, just prepare them (create elements, setup viz graph)
                    this.prepareAllFrequencies();
                    if (this.restoreListeningPending) this.restoreListening();
                } else {
                    this.audioFrequencies = [];
                }
            } catch (error) {
                console.error('Error fetching audio frequencies:', error);
                this.audioFrequencies = [];
            }
        },

        // Renamed from connectToAllFrequencies to reflect its new role
        // Fetch the list again after a source was added or removed: prepare what
        // is new, release what left. Transcriptions already on the page stay.
        async refreshAudioFrequencies() {
            try {
                const response = await fetch(this.audioApiUrl);
                if (!response.ok) return;
                const data = await response.json();
                const next = (data && data.frequencies) || [];
                const kept = new Set(next.map(f => String(f.id)));
                const before = new Set(this.audioFrequencies.map(f => String(f.id)));
                const names = list => list.map(f => f.name || f.id).join(', ') || 'nothing';
                if (before.size && (next.length !== before.size || next.some(f => !before.has(String(f.id))))) {
                    Alpine.store('events')?.add({
                        kind: 'station',
                        text: `Station now on ${names(next)} (was ${names(this.audioFrequencies)})`,
                    });
                }
                this.audioFrequencies.forEach(f => {
                    if (!kept.has(String(f.id)) && audioClient) audioClient.cleanupFrequency(String(f.id));
                });
                this.audioFrequencies = next;
                this.prepareAllFrequencies();
                // Once the radios are started, a frequency that arrives later is
                // connected like those there at the start. Prepared alone it has
                // no source: unmuting its tile played an empty element, silently
                // (02/10, after the station switched channels).
                if (this.radiosStarted) {
                    next.filter(f => !before.has(String(f.id))).forEach(f => this.connectToFrequency(f));
                }
            } catch (e) {
                console.error('Failed to refresh frequencies:', e);
            }
        },

        prepareAllFrequencies() {
            // REMOVED: this.initAudioContext(); // Ensure audio context is ready - This was causing the error.
            // The audioClient.prepareFrequency (called below) handles context initialization.
            this.audioFrequencies.forEach(freq => {
                this.prepareFrequency(freq); // This will now also setup visualization graph
            });
        },

        prepareFrequency(frequency) {
            if (!audioClient) { console.warn("audioClient not ready in prepareFrequency"); return; }
            audioClient.prepareFrequency(frequency);
        },

        connectToFrequency(frequency) {
            if (!audioClient) { console.warn("audioClient not ready in connectToFrequency"); return; }
            audioClient.connectToFrequency(frequency);
        },

        startAllRadios() {
            if (!audioClient) { console.warn("audioClient not ready in startAllRadios"); return; }
            audioClient.startAllRadios();
        },

        toggleMute(frequency) {
            if (!audioClient) { console.warn("audioClient not ready in toggleMute"); return; }
            audioClient.toggleMute(frequency);
            this.saveListening();
        },

        // Any frequency listened to: the "all" button then mutes every one.
        get anyUnmuted() {
            return this.audioFrequencies.some((f) => this.unmutedFrequencies.has(String(f.id)));
        },

        toggleAllListening() {
            if (!audioClient) return;
            audioClient.setAllMuted(this.anyUnmuted);
            this.saveListening();
        },

        // The level of one frequency, 0 to 1, kept by this browser like the
        // choice of what is listened to.
        volumeOf(frequencyId) {
            const id = String(frequencyId);
            if (audioClient && audioClient.userSetVolumes[id] !== undefined) return audioClient.userSetVolumes[id];
            const kept = this.rememberedVolume(id);
            return kept === null ? 1 : kept;
        },

        setVolume(frequencyId, value) {
            if (!audioClient) return;
            const id = String(frequencyId);
            audioClient.setVolume(id, value);
            try {
                const all = JSON.parse(localStorage.getItem('frequencyVolumes') || '{}');
                all[id] = audioClient.userSetVolumes[id];
                localStorage.setItem('frequencyVolumes', JSON.stringify(all));
            } catch (e) { /* lasts the page */ }
        },

        rememberedVolume(frequencyId) {
            try {
                const v = JSON.parse(localStorage.getItem('frequencyVolumes') || '{}')[String(frequencyId)];
                return typeof v === 'number' && v >= 0 && v <= 1 ? v : null;
            } catch (e) {
                return null;
            }
        },

        // A tile is narrow: the name alone, and the frequency under it only when
        // the name does not already start with it (the owner's station names
        // its streams "124.350 Approche De Gaulle").
        tileShowsMhz(freq) {
            if (!(freq.frequency_mhz > 0)) return false;
            const digits = String(freq.frequency_mhz).replace('.', '');
            const name = String(freq.name || '').replace(/[.,\s]/g, '');
            return !name.startsWith(digits);
        },

        tileHelp(freq) {
            const state = this.isFrequencyFailed(freq.id) ? `Connection failed: ${this.getFrequencyError(freq.id) || 'unknown error'}.`
                : this.isFrequencyConnecting(freq.id) ? 'Connecting…'
                : this.isUnmuted(freq.id) ? 'Listening. Click to mute.' : 'Muted. Click to listen.';
            const mhz = freq.frequency_mhz > 0 ? `${freq.frequency_mhz} MHz` : 'a mixed stream of several channels';
            return `${freq.name} (${mhz}). ${state} The bar shows the sound level, the figure how long since the last transmission; the slider under it is this frequency's volume.`;
        },

        // Which frequencies are listened to, remembered by this browser like the
        // display preferences. Upstream kept it for the page's lifetime only, so
        // every reload -- signing in, a server restart, a change of access --
        // opened the page silent while transcriptions kept arriving: "sometimes
        // both, sometimes only the text" was whether the tile had been clicked
        // since the last load.
        saveListening() {
            try {
                localStorage.setItem('listeningFrequencies', JSON.stringify([...this.unmutedFrequencies]));
            } catch (e) { /* private mode: the choice lasts the page, as before */ }
        },

        rememberedListening() {
            try {
                const ids = JSON.parse(localStorage.getItem('listeningFrequencies') || '[]');
                return Array.isArray(ids) ? ids.map(String) : [];
            } catch (e) {
                return [];
            }
        },

        // Browsers let a page play sound only after a click on it. "Start
        // Monitoring" is that click: the frequencies listened to last time start
        // again there, instead of waiting for their tile to be clicked.
        restoreListening() {
            if (!audioClient) return;
            if (!this.audioFrequencies.length) {
                this.restoreListeningPending = true; // the list is still loading
                return;
            }
            this.restoreListeningPending = false;
            const known = new Set(this.audioFrequencies.map(f => String(f.id)));
            const ids = this.rememberedListening().filter(id => known.has(id));
            if (!ids.length) return;
            ids.forEach(id => this.unmutedFrequencies.add(id));
            audioClient.startAllRadios();
        },

        cleanupFrequency(frequencyId) {
            if (!audioClient) { console.warn("audioClient not ready in cleanupFrequency"); return; }
            audioClient.cleanupFrequency(frequencyId);
        },

        // Handle frequency connection status changes from WebSocket
        handleFrequencyStatusChange(data) {
            if (!data || !data.frequency_id) {
                console.warn('Invalid frequency status data:', data);
                return;
            }

            const { frequency_id, status, error } = data;
            console.log(`Frequency ${frequency_id} status changed to: ${status}`, error ? `(${error})` : '');

            // A stream lost, or back after being lost, goes to the event log.
            const was = this.frequencyConnectionStatus[frequency_id]?.status;
            const freq = this.audioFrequencies.find(f => String(f.id) === String(frequency_id));
            const label = (freq && freq.name) || frequency_id;
            if (status === 'failed' && was !== 'failed') {
                // What happened, in words; the technical reason to the console (design, 28/10).
                if (error) console.warn(`${label}: audio stream lost:`, error);
                Alpine.store('events')?.add({ kind: 'stream', text: `${label}: audio stream lost.` });
            } else if (status === 'connected' && was === 'failed') {
                Alpine.store('events')?.add({ kind: 'stream', text: `${label}: audio stream back` });
            }

            // Update the status tracking object
            this.frequencyConnectionStatus[frequency_id] = {
                status: status,
                error: error || null,
                lastUpdate: Date.now()
            };
        },

        // Check if a frequency is in a failed state
        isFrequencyFailed(frequencyId) {
            const statusInfo = this.frequencyConnectionStatus[frequencyId];
            return statusInfo && statusInfo.status === 'failed';
        },

        // Check if a frequency is connecting
        isFrequencyConnecting(frequencyId) {
            const statusInfo = this.frequencyConnectionStatus[frequencyId];
            return statusInfo && statusInfo.status === 'connecting';
        },

        // Get error message for a frequency
        getFrequencyError(frequencyId) {
            const statusInfo = this.frequencyConnectionStatus[frequencyId];
            return statusInfo ? statusInfo.error : null;
        },

        // Browsers let a page play sound only after a gesture on it. The first
        // click or key anywhere resumes what was listened to last time; it
        // replaced upstream's welcome screen, which blocked the map at every load
        // to get that click (10/10). Capture phase: it runs before the click's
        // own handler, so a tile clicked first is toggled after the resume.
        armResumeOnFirstGesture() {
            this.resumeListeningPending = this.rememberedListening().length > 0;
            const onFirstGesture = () => {
                document.removeEventListener('pointerdown', onFirstGesture, true);
                document.removeEventListener('keydown', onFirstGesture, true);
                this.resumeListeningPending = false;
                this.restoreListening();
            };
            document.addEventListener('pointerdown', onFirstGesture, true);
            document.addEventListener('keydown', onFirstGesture, true);
        },
    }));
})();
