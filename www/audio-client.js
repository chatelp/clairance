/**
 * Audio streaming client for Co-ATC.
 *
 * Responsibilities:
 * - Prepare and manage per-frequency HTMLAudioElement streams.
 * - Maintain Web Audio analyser pipelines for UI visualization.
 * - Provide mute/unmute and playback orchestration helpers.
 * - Track time since last significant audio per frequency.
 */
class AudioClient {
    /**
     * @param {Object} store Alpine store instance.
     */
    constructor(store) {
        this.store = store;
        this.audioContext = null;
        this.audioElements = {};
        this.audioAnalysers = {};
        this.audioDataArrays = {};
        this.visualizationFrameIds = {};
        this.sourceNodes = {};
        this.gainNodes = {};
        // Live audio is lowered, not stopped, while an archived clip plays
        // (store.playClip): the clip is heard, the live channels stay followed.
        this.duckFactor = 1;
        this.userSetVolumes = {};
        this.lastSignificantAudioTime = {};
        this.secondsSinceLastAudio = {};
        // What the server says of each frequency (noteServerActivity): the
        // same "someone is speaking" in every browser, listened to or not.
        this.serverActivity = {};

        this.defaultVolume = 1.0;
        this.visualizationTargetFps = 30;
        // The analyser hears a frequency while it plays here; a muted one, and
        // any in Safari, which plays a live stream past Web Audio, take their
        // activity from the server (noteServerActivity).
        this.significantAudioThreshold = 0.10;
        this.visualizerMultiplier = 150;
    }

    /**
     * Initialize and cache the AudioContext.
     * @returns {AudioContext|null}
     */
    initAudioContext() {
        if (!this.audioContext) {
            try {
                this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
                console.log("AudioContext initialized.");
                this._watchContext(this.audioContext);
            } catch (e) {
                console.error("Web Audio API is not supported.", e);
            }
        }
        return this.audioContext;
    }

    /**
     * Ensure a frequency stream is prepared with element, source URL, and analyser pipeline.
     * @param {{id: string|number, stream_port?: number|string, stream_url?: string}} frequency
     */
    prepareFrequency(frequency) {
        const frequencyId = this._normalizeFrequencyId(frequency?.id);
        if (!frequencyId) {
            console.error('prepareFrequency: missing frequency.id');
            return;
        }

        if (this.audioElements[frequencyId]?.element) {
            return;
        }

        console.log(`Preparing frequency: ${frequencyId}`);
        this.initAudioContext();

        // The page plays the stream itself where it can (live-player.js): it
        // starts at once and measures what it plays, in every browser. An
        // <audio> element otherwise.
        // Not in Safari for now: there the page's own player gave no sound on
        // 26/10, cause not found yet, where the <audio> element with the MP3
        // plays.
        const webkitOnly = /^((?!chrome|chromium|crios|android|edg|fxios).)*safari/i.test(navigator.userAgent);
        // ?player=live or ?player=element in the page's address forces one or
        // the other, to try a browser.
        const forced = new URLSearchParams(window.location.search).get('player');
        const live = window.LiveStreamPlayer && window.LiveStreamPlayer.supported() && this.audioContext &&
            (forced === 'live' || (forced !== 'element' && !webkitOnly));
        const audioElement = live ? new window.LiveStreamPlayer(this.audioContext) : document.createElement('audio');
        // Streams are spread over the server's additional ports, a different
        // origin from the page: "anonymous" sends no cookie there, and with
        // sign-in required every stream was refused while the transcriptions,
        // on the page's own port, kept arriving (02/10).
        audioElement.crossOrigin = 'use-credentials';
        audioElement.preload = 'metadata';
        audioElement.playsInline = true;

        if (this.userSetVolumes[frequencyId] === undefined) {
            const kept = typeof this.store.rememberedVolume === 'function' ? this.store.rememberedVolume(frequencyId) : null;
            this.userSetVolumes[frequencyId] = kept === null ? this.defaultVolume : kept;
        }

        const streamUrl = this._buildStreamUrl(frequency, live ? 'ulaw' : 'mp3');
        if (!streamUrl) {
            console.error(`prepareFrequency: invalid stream URL for ${frequencyId}`);
            return;
        }

        audioElement.addEventListener('error', (e) => {
            const message = e.target && e.target.error ? e.target.error.message : 'Unknown error';
            console.error(`Audio error for ${frequencyId}:`, message);
        });
        audioElement.addEventListener('playing', () => {
            if (!this.visualizationFrameIds[frequencyId]) {
                this.startVisualization(frequencyId);
            }
        });
        audioElement.addEventListener('pause', () => {
            this.cleanupVisualization(frequencyId);
        });

        this.audioElements[frequencyId] = {
            element: audioElement,
            intendedSrc: streamUrl,
            isPrepared: true
        };
        
        this.setupVisualization(frequencyId, audioElement);
    }

    /**
     * Connect and start playback for a prepared frequency stream.
     * @param {{id: string|number}} frequency
     */
    connectToFrequency(frequency) {
        const frequencyId = this._normalizeFrequencyId(frequency?.id);
        if (!frequencyId) {
            console.error('connectToFrequency: missing frequency.id');
            return;
        }

        if (!this.audioElements[frequencyId]?.element) {
            console.warn(`Audio element for ${frequencyId} not found during connect. Preparing now.`);
            this.prepareFrequency(frequency);
        }
        
        const audioInfo = this.audioElements[frequencyId];
        if (!audioInfo || !audioInfo.element || !audioInfo.intendedSrc) {
            console.error(`Audio info incomplete for frequency: ${frequencyId}. Cannot connect.`);
            return;
        }

        const audioElement = audioInfo.element;
        const intendedSrc = audioInfo.intendedSrc;

        this._applyMute(frequencyId);

        if (audioElement.currentSrc !== intendedSrc) {
            audioElement.src = intendedSrc;
        }

        if (!this.visualizationFrameIds[frequencyId] && this.audioAnalysers[frequencyId]) {
            this.startVisualization(frequencyId);
        }

        if (audioElement.readyState === 0 || audioElement.currentSrc !== intendedSrc) {
            audioElement.load();
        }

        if (audioElement.paused) {
            const playPromise = audioElement.play();
            
            if (playPromise !== undefined) {
                playPromise.catch(error => {
                    if (error.name !== 'AbortError') {
                        console.error(`Error invoking play for ${frequencyId}:`, error);
                    }
                });
            }
        }
    }

    /**
     * Start playback attempts across all known radio frequencies.
     */
    startAllRadios() {
        if (this.store.radiosStarted) {
            return;
        }

        this.store.radiosStarted = true; 
        const context = this.initAudioContext();
        if (!context) {
            console.error('startAllRadios: AudioContext unavailable');
            return;
        }

        const resumeContextAndPlay = () => {
            this.store.audioFrequencies.forEach(freq => {
                this.connectToFrequency(freq);
            });
        };

        if (context.state === 'suspended') {
            context.resume().then(() => {
                resumeContextAndPlay();
            }).catch(e => {
                console.error("Error resuming audio context:", e);
                resumeContextAndPlay(); 
            });
        } else {
            resumeContextAndPlay();
        }
    }

    /**
     * Follow whether the page's sound actually runs. The page plays through
     * this context (live-player.js), which a browser suspends until a gesture
     * and an iPhone suspends again when the page goes to the background or a
     * call comes: while frequencies are listened to, a stopped context says
     * so ("Click anywhere to resume listening") and the next gesture resumes
     * it, instead of a silence with no word; the tab's icon lights only while
     * it runs (favicon-state.js, design review of 26/10).
     */
    _watchContext(context) {
        const update = () => {
            const running = context.state === 'running';
            this.store.audioRunning = running;
            if (running || !this.store.radiosStarted || !this.store.anyUnmuted) {
                if (running) this.store.resumeListeningPending = false;
                return;
            }
            this.store.resumeListeningPending = true;
            this._resumeOnGesture(context);
        };
        context.onstatechange = update;
        update();
    }

    /**
     * Resume the context at each gesture until it runs. Safari lets a context
     * resume from click, touchend or keydown, not from pointerdown (26/10: the
     * banner stayed after a click, and the page's own player never sounded
     * there); and a gesture whose resume is refused must not be the last one
     * tried.
     */
    _resumeOnGesture(context) {
        if (this._gestureResume) return;
        const events = ['pointerdown', 'click', 'touchend', 'keydown'];
        const onGesture = () => {
            if (context.state === 'running') {
                events.forEach((e) => document.removeEventListener(e, onGesture, true));
                this._gestureResume = null;
                return;
            }
            context.resume().catch(() => {});
        };
        this._gestureResume = onGesture;
        events.forEach((e) => document.addEventListener(e, onGesture, true));
    }

    /**
     * Create analyser pipeline for a given frequency.
     * @param {string} frequencyId
     * @param {HTMLAudioElement} audioElement
     */
    setupVisualization(frequencyId, audioElement) {
        if (!this.audioContext) {
            return;
        }

        this._safeDisconnectNode(this.sourceNodes[frequencyId]);
        this._safeDisconnectNode(this.audioAnalysers[frequencyId]);
        this._safeDisconnectNode(this.gainNodes[frequencyId]);
        delete this.sourceNodes[frequencyId];
        delete this.audioAnalysers[frequencyId];
        delete this.gainNodes[frequencyId];

        try {
            const sourceNode = audioElement.isLivePlayer ? audioElement.output : this.audioContext.createMediaElementSource(audioElement);
            this.sourceNodes[frequencyId] = sourceNode;

            const analyserNode = this.audioContext.createAnalyser();
            analyserNode.fftSize = 256;
            analyserNode.smoothingTimeConstant = 0.5;
            this.audioAnalysers[frequencyId] = analyserNode;

            this.audioDataArrays[frequencyId] = new Uint8Array(analyserNode.frequencyBinCount);

            const gainNode = this.audioContext.createGain();
            this.gainNodes[frequencyId] = gainNode;

            sourceNode.connect(analyserNode);
            analyserNode.connect(gainNode);
            gainNode.connect(this.audioContext.destination);
            this._applyMute(frequencyId);
        } catch (e) {
            console.error(`Error setting up visualization for ${frequencyId}:`, e);
            this._safeDisconnectNode(this.sourceNodes[frequencyId]);
            this._safeDisconnectNode(this.audioAnalysers[frequencyId]);
            this._safeDisconnectNode(this.gainNodes[frequencyId]);
            delete this.sourceNodes[frequencyId];
            delete this.audioAnalysers[frequencyId];
            delete this.gainNodes[frequencyId];
        }
    }

    /**
     * Silence or restore a frequency's output.
     *
     * The mute is the gain after the analyser and the element's own mute:
     * Safari plays a live stream past Web Audio, where the gain alone muted
     * nothing (26/10). The level of a muted channel comes from the server.
     * Upstream lowered the element's volume to 0.01 instead, which left a
     * muted channel audible at -40 dB under the others (04/10).
     * @param {string} frequencyId
     */
    /**
     * The level of one frequency, 0 to 1, kept by the browser (audit 4.1).
     * Applied through the element's own volume, which the Web Audio graph takes
     * as its input; the gain node after it only mutes.
     */
    setVolume(frequencyId, volume) {
        const id = this._normalizeFrequencyId(frequencyId);
        if (!id) return;
        this.userSetVolumes[id] = Math.max(0, Math.min(1, Number(volume) || 0));
        this._applyMute(id);
    }

    /**
     * Listen to every frequency, or mute every one. Starting the radios needs
     * the click this is called from, like toggleMute.
     */
    setAllMuted(muted) {
        if (!this.store.radiosStarted) this.startAllRadios();
        for (const freq of this.store.audioFrequencies) {
            const id = this._normalizeFrequencyId(freq.id);
            if (!id) continue;
            const unmuted = this.store.unmutedFrequencies.has(id);
            if (muted === unmuted) this.toggleMute(freq);
        }
    }

    setDucked(on) {
        this.duckFactor = on ? 0.2 : 1;
        Object.keys(this.gainNodes).forEach((id) => this._applyMute(id));
    }

    _applyMute(frequencyId) {
        const unmuted = this.store.unmutedFrequencies.has(frequencyId);
        const gainNode = this.gainNodes[frequencyId];
        if (gainNode) {
            gainNode.gain.value = unmuted ? this.duckFactor : 0;
        }
        const audioElement = this.audioElements[frequencyId]?.element;
        if (audioElement) {
            const volume = this.userSetVolumes[frequencyId] === undefined ? this.defaultVolume : this.userSetVolumes[frequencyId];
            audioElement.volume = (gainNode || unmuted) ? volume : 0;
            // And the element itself: Safari plays a live stream straight out,
            // past the gain, which then muted nothing (26/10). "someone is
            // speaking" no longer needs the muted channels' sound: the server
            // says it (noteServerActivity).
            audioElement.muted = !unmuted;
        }
    }

    /**
     * Start bar visualization animation loop for a frequency.
     * @param {string} frequencyId
     */
    startVisualization(frequencyId) {
        if (this.visualizationFrameIds[frequencyId]) return;

        let lastFrameTime = 0;
        const frameInterval = 1000 / this.visualizationTargetFps;
        
        const renderFrame = (currentTime) => {
            if (currentTime - lastFrameTime < frameInterval) {
                this.visualizationFrameIds[frequencyId] = requestAnimationFrame(renderFrame);
                return;
            }
            lastFrameTime = currentTime;
            
            const analyser = this.audioAnalysers[frequencyId];
            const dataArray = this.audioDataArrays[frequencyId];
            
            if (!analyser || !dataArray) {
                this.cleanupVisualization(frequencyId);
                return;
            }
            
            try {
                analyser.getByteFrequencyData(dataArray);
            } catch (e) {
                for (let i = 0; i < dataArray.length; i++) {
                    dataArray[i] = 0;
                }
            }
            
            let totalSum = 0;
            let totalPoints = 0;
            const maxBin = Math.min(dataArray.length, 40);
            for (let j = 1; j < maxBin; j++) { 
                const weight = 1 - (j / maxBin * 0.5);
                totalSum += dataArray[j] * weight;
                totalPoints += weight;
            }
            
            const audioLevel = totalPoints > 0 ? (totalSum / totalPoints) / 255 : 0;

            if (audioLevel >= this.significantAudioThreshold) {
                this.lastSignificantAudioTime[frequencyId] = Date.now();
            } else if (!this.lastSignificantAudioTime[frequencyId]) {
                this.lastSignificantAudioTime[frequencyId] = Date.now();
            }

            // The thread's level meter, from Web Audio when it hears something;
            // otherwise the server's level draws it (noteServerActivity), which
            // is what Safari gets.
            if (audioLevel > 0) {
                this._drawMeter(frequencyId, Math.min(1, audioLevel * this.visualizerMultiplier / 100));
            }

            this.visualizationFrameIds[frequencyId] = requestAnimationFrame(renderFrame);
        };
        
        this.visualizationFrameIds[frequencyId] = requestAnimationFrame(renderFrame);
    }

    /**
     * The thread's level meter (LevelMeter, migration step 6): five bars, 2 to
     * 14 px; drawn here, at the meter's own pace, so the store is not touched
     * thirty times a second.
     * @param {string} frequencyId
     * @param {number} level 0 to 1
     */
    _drawMeter(frequencyId, level) {
        const meter = document.querySelector(`[data-level-for="${frequencyId}"]`);
        if (!meter) return;
        const shape = [0.55, 0.9, 0.7, 1, 0.6];
        for (let i = 0; i < meter.children.length && i < shape.length; i++) {
            const bar = meter.children[i];
            const was = parseFloat(bar.style.height) || 2;
            const target = 2 + 12 * level * shape[i];
            bar.style.height = (was * 0.4 + target * 0.6).toFixed(1) + 'px';
        }
    }

    /**
     * Someone speaking on a frequency, or not, as the server measures it on
     * the audio it decodes, with the level, four times a second while it
     * lasts (internal/audio, activity.go). It keeps "someone is speaking" and
     * the meter alive where Web Audio hears nothing -- Safari plays a live
     * stream without routing it there -- and for a frequency not listened to.
     */
    noteServerActivity(frequencyId, speaking, level) {
        const id = this._normalizeFrequencyId(frequencyId);
        if (!id) return;
        // A frequency the page plays itself is measured on what it plays,
        // in time with the voice; the server's word, seconds ahead of it, is
        // for the others.
        const el = this.audioElements[id]?.element;
        if (el && el.isLivePlayer && !el.paused && el.readyState) return;
        this.serverActivity[id] = { speaking: !!speaking, at: Date.now() };
        if (!speaking) return;
        this.lastSignificantAudioTime[id] = Date.now();
        this._drawMeter(id, Math.max(0, Math.min(1, Number(level) || 0)));
    }

    /**
     * Stop visualization animation for a frequency.
     * @param {string} frequencyId
     */
    cleanupVisualization(frequencyId) {
        if (this.visualizationFrameIds[frequencyId]) {
            cancelAnimationFrame(this.visualizationFrameIds[frequencyId]);
            delete this.visualizationFrameIds[frequencyId];
        }
        if (this.secondsSinceLastAudio[frequencyId] !== '--s') {
            this.secondsSinceLastAudio[frequencyId] = '--s'; 
        }
    }

    /**
     * Toggle mute state for a frequency while preserving user-set volume.
     * @param {{id: string|number}} frequency
     */
    toggleMute(frequency) {
        if (!this.store.radiosStarted) {
            this.startAllRadios();
        }

        const frequencyId = this._normalizeFrequencyId(frequency?.id);
        if (!frequencyId) {
            console.error('toggleMute: missing frequency.id');
            return;
        }

        let audioInfo = this.audioElements[frequencyId];
        if (!audioInfo || !audioInfo.element) {
            this.prepareFrequency(frequency);
            audioInfo = this.audioElements[frequencyId];
            if (!audioInfo || !audioInfo.element) {
                console.error(`No audio element found for frequency: ${frequencyId} in toggleMute.`);
                return;
            }
        }
        const audioElement = audioInfo.element;

        const isCurrentlyUnmuted = this.store.unmutedFrequencies.has(frequencyId);

        if (isCurrentlyUnmuted) {
            this.store.unmutedFrequencies.delete(frequencyId);
        } else {
            this.store.unmutedFrequencies.add(frequencyId);
        }
        this._applyMute(frequencyId);

        if (audioElement.paused && this.store.unmutedFrequencies.has(frequencyId) && this.store.radiosStarted) {
            setTimeout(() => {
                if (audioElement.paused && this.store.unmutedFrequencies.has(frequencyId)) {
                    audioElement.play().catch(err => {
                        if (err.name !== 'AbortError') {
                            console.error(`Error playing audio for ${frequencyId} after unmute:`, err);
                        }
                    });
                }
            }, 100);
        }
    }

    /**
     * Release all resources for a frequency.
     * @param {string} frequencyId
     */
    cleanupFrequency(frequencyId) {
        this.cleanupVisualization(frequencyId);

        this._safeDisconnectNode(this.audioAnalysers[frequencyId]);
        this._safeDisconnectNode(this.sourceNodes[frequencyId]);
        this._safeDisconnectNode(this.gainNodes[frequencyId]);

        delete this.audioAnalysers[frequencyId];
        delete this.sourceNodes[frequencyId];
        delete this.gainNodes[frequencyId];

        if (this.audioDataArrays[frequencyId]) {
            delete this.audioDataArrays[frequencyId];
        }

        if (this.audioElements[frequencyId]) {
            const audioElement = this.audioElements[frequencyId].element;
            if (audioElement) {
                audioElement.pause();
                audioElement.src = '';
            }
            delete this.audioElements[frequencyId];
        }

        delete this.lastSignificantAudioTime[frequencyId];
        delete this.secondsSinceLastAudio[frequencyId];
    }

    /**
     * Update `seconds since last audio` values in the provided store-backed object.
     * @param {Object} storeSecondsSinceLastAudio
     */
    updateSecondsSinceLastAudio(storeSecondsSinceLastAudio) {
        if (!storeSecondsSinceLastAudio) {
            console.warn("AudioClient: storeSecondsSinceLastAudio object not provided for update.");
            return;
        }

        // The server's activity too: a frequency not listened to here still
        // says when someone last spoke on it.
        Object.keys(this.serverActivity).forEach(frequencyId => {
            if (!this.audioElements[frequencyId]?.element && this.lastSignificantAudioTime[frequencyId]) {
                const seconds = Math.floor((Date.now() - this.lastSignificantAudioTime[frequencyId]) / 1000);
                storeSecondsSinceLastAudio[frequencyId] = `${seconds}s`;
            }
        });
        Object.keys(this.audioElements).forEach(frequencyId => {
            if (this.audioElements[frequencyId]?.element && this.lastSignificantAudioTime[frequencyId]) {
                const seconds = Math.floor((Date.now() - this.lastSignificantAudioTime[frequencyId]) / 1000);
                storeSecondsSinceLastAudio[frequencyId] = `${seconds}s`;
            } else if (this.audioElements[frequencyId]?.element && !this.lastSignificantAudioTime[frequencyId]) {
                storeSecondsSinceLastAudio[frequencyId] = '--s'; 
            }
        });
    }

    /**
     * Build a frequency stream URL and attach the client id.
     * @param {{stream_port?: number|string, stream_url?: string}} frequency
     * @returns {string|null}
     */
    _buildStreamUrl(frequency, format = 'mp3') {
        const streamPath = typeof frequency?.stream_url === 'string' ? frequency.stream_url : '';
        if (!streamPath) {
            return null;
        }

        // No port from the server (a browser behind a proxy, which reaches
        // only the address it serves): the stream comes from the page's own
        // origin, whose port is empty on 443.
        const streamPort = frequency.stream_port || window.location.port;
        const base = streamPort
            ? `${window.location.protocol}//${window.location.hostname}:${streamPort}`
            : window.location.origin;

        let url;
        try {
            url = new URL(streamPath, base);
        } catch {
            return null;
        }

        // The decoded sound in mu-law for the page's own player, the
        // station's MP3 for an <audio> element; the WAV stream where the
        // server has neither. Safari does not play an endless WAV stream (26/10).
        url.searchParams.set('format', format);

        if (url.href.includes('CLIENT_ID')) {
            return url.href.replace('CLIENT_ID', encodeURIComponent(this.store.clientID));
        }

        if (!url.searchParams.has('id')) {
            url.searchParams.set('id', this.store.clientID);
        }

        return url.toString();
    }

    /**
     * Disconnect a Web Audio node safely.
     * @param {AudioNode|undefined|null} node
     */
    _safeDisconnectNode(node) {
        if (!node) {
            return;
        }
        try {
            node.disconnect();
        } catch {
            // no-op
        }
    }

    /**
     * Normalize frequency id values to string keys.
     * @param {string|number|undefined|null} frequencyId
     * @returns {string|null}
     */
    _normalizeFrequencyId(frequencyId) {
        if (frequencyId === undefined || frequencyId === null) {
            return null;
        }
        return String(frequencyId);
    }
}

// Export the AudioClient class
window.AudioClient = AudioClient; 