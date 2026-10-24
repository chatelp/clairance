// The aircraft story (design system, AircraftStory; migration step 8): what
// the context column shows while a flight is followed. Its head (phase,
// callsign, operator, four numbers), the tabs Story, Details, Track and
// Nearby -- the panel's former Timeline, Details, Tracks and Proximity, whose
// contents and refreshes are unchanged -- and the instructions heard with
// what the aircraft did next ("Then", www/story/then.js, never a verdict).
//
// One domain of the Alpine store 'atc' (docs-fr/32, palier 5.1); see
// store/station.js for how the domains are assembled. Nothing new is fetched:
// the instructions are the `plain` values of the controller's transmissions
// matched to the aircraft (aircraftRadio), the track is the one the panel
// already refreshes every 5 s (aircraftDetailsHistoryData).
(function () {
    // "Show only" this level: the aircraft's ± this, as Pierre chose (15/10).
    const ONLY_LEVEL_FT = 500;

    // "FL 340 ± 500 ft", "2 400 ft ± 500 ft": the slider's words (feet on
    // the QNH under the transition altitude).
    function onlyLevelText(ft) {
        const S = window.AltitudeScale;
        const geo = window.ParGeometry;
        const ta = (geo && geo.DEFAULTS && geo.DEFAULTS.transitionAltitudeFt) || S.DEFAULT_TA;
        return `${S.label(ft, ta)} ± ${ONLY_LEVEL_FT} ft`;
    }

    // A narrow no-break space between thousands, as the design system writes
    // numbers ("4 000 ft").
    const THIN = ' ';
    const group = (n) => String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, THIN);
    // Faster than this, the altitude's arrow says it climbs or descends.
    const TREND_FPM = 300;
    // A track silent this long ends a flight: the next point starts another
    // (the same as the Track tab's "gap" divider).
    const FLIGHT_GAP_MS = 10 * 60 * 1000;
    // An airport is named for a phase only where it is plausible: under this
    // altitude and this close to it, or in its arrival sequence (design
    // review of step 8: a Citation at FL 215, 47 NM out, "Arrival at LFPZ").
    const AIRPORT_MAX_FT = 10000;
    const AIRPORT_MAX_NM = 20;
    // A takeoff or a touchdown judged this soon after the aircraft reappeared
    // was not seen: it is dated by the reappearance.
    const REAPPEAR_MS = 60 * 1000;
    const COMPASS = ['north', 'north-east', 'east', 'south-east', 'south', 'south-west', 'west', 'north-west'];

    function nmBetween(lat1, lon1, lat2, lon2) {
        const r = Math.PI / 180;
        const dLat = (lat2 - lat1) * r;
        const dLon = (lon2 - lon1) * r;
        const h = Math.sin(dLat / 2) ** 2 + Math.cos(lat1 * r) * Math.cos(lat2 * r) * Math.sin(dLon / 2) ** 2;
        return 2 * 3440.065 * Math.asin(Math.sqrt(h));
    }

    function compass(lat1, lon1, lat2, lon2) {
        const r = Math.PI / 180;
        const y = Math.sin((lon2 - lon1) * r) * Math.cos(lat2 * r);
        const x = Math.cos(lat1 * r) * Math.sin(lat2 * r) - Math.sin(lat1 * r) * Math.cos(lat2 * r) * Math.cos((lon2 - lon1) * r);
        const deg = (Math.atan2(y, x) / r + 360) % 360;
        return COMPASS[Math.round(deg / 45) % 8];
    }

    // "25818s" -> "7 h 10 min ago", "95s" -> "1 min ago", "40s" -> "40 s ago".
    function agoText(seconds) {
        const n = parseInt(seconds, 10);
        if (!Number.isFinite(n)) return '';
        if (n < 60) return `${n} s ago`;
        if (n < 3600) return `${Math.floor(n / 60)} min ago`;
        return `${Math.floor(n / 3600)} h ${Math.floor((n % 3600) / 60)} min ago`;
    }

    // "BOEING 737-800" -> "Boeing 737-800": the aircraft base writes some
    // types in capitals; a word with a digit is a model and stays.
    const NAME_PARTICLES = new Set(['DE', 'DU', 'DA', 'LA', 'LE', 'VAN', 'VON', 'DOS']);

    function typeCase(text) {
        const t = String(text || '').trim();
        const title = (w) => w.charAt(0) + w.slice(1).toLowerCase();
        // All in capitals: every word without a digit, but initialisms of
        // three letters or fewer (ATR, BAE) stay, name particles apart
        // ("DE HAVILLAND"; design review, 15/10).
        if (!/[a-z]/.test(t)) {
            return t.split(' ').map((w) => (/\d/.test(w) || (w.length <= 3 && !NAME_PARTICLES.has(w)) ? w : title(w))).join(' ');
        }
        // Mixed, as "CESSNA 560 Citation Ultra": the words wholly in capitals,
        // short initialisms (VOR, DME) apart (design review, 15/10).
        return t.split(' ').map((w) => (/^[A-Z]{4,}$/.test(w) ? title(w) : w)).join(' ');
    }
    // Out of the store, so that reading them in a getter does not make Alpine
    // render again: the last instructions table and the runway frames.
    const memo = { key: null, rows: [] };
    const framesByAirport = {};

    window.AtcStore = window.AtcStore || [];
    window.AtcStore.push(() => ({
        // The height of the PAR frame under the map, measured: the map
        // controls sit above it while it shows.
        parFrameHeight: 0,

        // Which sections of the Details tab are open (c-details): the tables
        // and the basic facts open, the long lists closed.
        storyOpen: {
            earlier: false, earlierPhases: false, phases: true, instructions: true, radio: false, basic: true,
            position: false, speed: false, nav: false, adsb: false,
        },

        // 'story', 'details', 'track' or 'nearby', from the panel's three flags.
        get storyTab() {
            if (this.showTimelineView) return 'story';
            if (this.showProximityView) return 'nearby';
            if (this.aircraftDetailsShowHistoryView) return 'track';
            return 'details';
        },

        // What the panel's four buttons did, each with its refreshes.
        setStoryTab(tab) {
            this.showTimelineView = tab === 'story';
            this.aircraftDetailsShowHistoryView = tab === 'track';
            this.showProximityView = tab === 'nearby';
            if (tab === 'nearby') {
                this.stopPhaseHistoryRefresh();
                this.loadProximityData();
                this.startProximityRefresh();
            } else {
                this.stopProximityRefresh();
                this.clearProximityView();
                if (tab === 'details') this.stopPhaseHistoryRefresh();
            }
        },

        toggleStorySection(key) {
            this.storyOpen[key] = !this.storyOpen[key];
        },

        // "‹ Back to Traffic": back to the column tab the aircraft was chosen
        // from, which is still the selected one.
        get storyBackLabel() {
            return this.columnTab === 'now' ? 'Back to Right now' : 'Back to Traffic';
        },

        // ‹, ✕ and Escape: stop following, every refresh and trail with it.
        stopFollowing() {
            const previousHex = this.selectedAircraft && this.selectedAircraft.hex;
            this.closeAircraftPanel();
            this.aircraftDetailsShowHistoryView = false;
            this.showProximityView = false;
            this.aircraftDetailsHistoryData = [];
            this.aircraftDetailsFutureData = [];
            this.aircraftDetailsHindcastData = [];
            this.aircraftDetailsHistoryCount = 0;
            this.phaseHistoryData = [];
            this.phaseHistoryAircraftHex = null;
            this.aircraftDetailsStopHistoryRefresh();
            this.stopProximityRefresh();
            this.stopPhaseHistoryRefresh();
            this.clearProximityView();
            if (this.mapManager && previousHex) this.mapManager.clearAircraftTrails(previousHex);
        },

        // Where the followed aircraft stands in an arrival sequence of a
        // followed airport: { airport, runway, distNM, etaSec }, or null.
        storyArrival(a) {
            if (!a) return null;
            for (const [code, seqs] of Object.entries(this.arrivals || {})) {
                for (const seq of seqs || []) {
                    const item = (seq.items || []).find((x) => x.hex === a.hex);
                    if (item) return { airport: code, runway: seq.runway, distNM: item.distNM, etaSec: item.etaSec };
                }
            }
            return null;
        },

        // The airport whose runway frame shows under the map: the followed
        // aircraft's arrival, while the map (not the PAR view) is on.
        // Also before it is on a runway's axis: an arriving aircraft (ARR,
        // APP) whose airport is followed and plausible, the frame then being
        // the runway in use (seen by the owner, 14/10: 5 of 21 arriving
        // aircraft were in a sequence, the others still on vectors).
        get storyFrameAirport() {
            const a = this.selectedAircraft;
            if (!a || this.mainView !== 'map') return '';
            const arr = this.storyArrival(a);
            if (arr) return arr.airport;
            const phase = this.getCurrentPhase(a);
            const code = this.getPhaseAirport(a);
            const adsb = a.adsb || {};
            if ((phase !== 'ARR' && phase !== 'APP') || !code) return '';
            if (!(this.followedAirports || []).some((x) => x.code === code)) return '';
            return this.plausibleAirport(code, adsb.alt_baro, adsb.lat, adsb.lon, a) ? code : '';
        },

        // The head of the story: who, and four numbers.
        // "Show only" this aircraft's level or its phase (AircraftStory, owner
        // 15/10): each toggle sets the Traffic filter it names -- the altitude
        // slider to the level ± 500 ft, the phase filters to that code alone --
        // and, pressed again, puts back what that filter was before. On while
        // the filter is still the one it set.
        _onlyLevel: null,   // { min, max, label, before: { min, max } }
        _onlyPhase: null,   // { code, before: phaseFilters }

        get onlyLevelOn() {
            const o = this._onlyLevel;
            return !!o && this.settings.minAltitude === o.min && this.settings.maxAltitude === o.max;
        },

        get onlyPhaseOn() {
            const o = this._onlyPhase;
            const f = this.settings.phaseFilters || {};
            return !!o && Object.entries(f).every(([k, v]) => (k === o.code) === (v !== false));
        },

        // The row's two toggles: the level's text (the one set while on, the
        // aircraft's now otherwise) and the phase code its label shows.
        get storyOnly() {
            const a = this.selectedAircraft;
            if (!a) return null;
            let level = null;
            if (this.onlyLevelOn) level = this._onlyLevel.label;
            else if (!a.on_ground) {
                const ft = this.altitudeFt(a);
                if (Number.isFinite(ft)) level = onlyLevelText(Math.round(ft / 100) * 100);
            }
            const phase = this.onlyPhaseOn ? this._onlyPhase.code : this.getCurrentPhase(a);
            return { level, phase };
        },

        toggleOnlyLevel() {
            const s = this.settings;
            if (this.onlyLevelOn) {
                s.minAltitude = this._onlyLevel.before.min;
                s.maxAltitude = this._onlyLevel.before.max;
                this._onlyLevel = null;
            } else {
                const a = this.selectedAircraft;
                const ft = a && !a.on_ground ? this.altitudeFt(a) : NaN;
                if (!Number.isFinite(ft)) return;
                const centre = Math.round(ft / 100) * 100;
                this._onlyLevel = {
                    min: Math.max(0, centre - ONLY_LEVEL_FT), max: centre + ONLY_LEVEL_FT,
                    label: onlyLevelText(centre), before: { min: s.minAltitude, max: s.maxAltitude },
                };
                s.minAltitude = this._onlyLevel.min;
                s.maxAltitude = this._onlyLevel.max;
            }
            this.refreshFilters();
        },

        toggleOnlyPhase() {
            const s = this.settings;
            if (this.onlyPhaseOn) {
                s.phaseFilters = { ...this._onlyPhase.before };
                this._onlyPhase = null;
            } else {
                const a = this.selectedAircraft;
                if (!a) return;
                const code = this.getCurrentPhase(a);
                const before = { ...(s.phaseFilters || {}) };
                const next = {};
                for (const k of new Set([...Object.keys(before), code])) next[k] = k === code;
                this._onlyPhase = { code, before };
                s.phaseFilters = next;
            }
            this.refreshFilters();
        },

        // The ✕ of the sky's note: everything the note lists, back to all.
        clearFilters() {
            const s = this.settings;
            s.minAltitude = window.AltitudeScale.FLOOR;
            s.maxAltitude = window.AltitudeScale.NO_CEILING;
            const f = {};
            for (const k of Object.keys(s.phaseFilters || {})) f[k] = true;
            s.phaseFilters = f;
            s.showAirAircraft = true;
            s.showGroundAircraft = true;
            s.voiceOnly = false;
            this.searchTerm = '';
            this._onlyLevel = null;
            this._onlyPhase = null;
            this.refreshFilters();
        },

        refreshFilters() {
            this.applyFilters();
            if (this.mapManager) this.mapManager.applyFiltersAndRefreshView();
            this.saveSettings();
        },

        get storyHead() {
            const a = this.selectedAircraft;
            if (!a) return null;
            const adsb = a.adsb || {};
            const bsdb = a.bsdb || {};
            const phase = this.getCurrentPhase(a);
            const who = [a.airline || bsdb.registered_owners, typeCase(bsdb.type || adsb.t), bsdb.registration || adsb.r]
                .map((x) => String(x || '').trim()).filter((x) => x && x !== '-');

            const r = this.altitudeReading(a);
            let alt = { value: '–', unit: '' };
            if (r && r.kind === 'gnd') alt = { value: 'Ground', unit: '' };
            else if (r && r.kind === 'ft') alt = { value: group(Math.round(r.ft / 100) * 100), unit: 'ft' };
            else if (r) alt = { value: window.AltitudeScale.readingText(r), unit: '' };
            const rate = adsb.baro_rate;
            const arrow = typeof rate === 'number' && !(r && r.kind === 'gnd')
                ? (rate > TREND_FPM ? ' ↑' : rate < -TREND_FPM ? ' ↓' : '') : '';

            const arr = this.storyArrival(a);
            let second;
            if (arr && Number.isFinite(arr.distNM)) {
                const eta = Number.isFinite(arr.etaSec)
                    ? ` · ${Math.floor(arr.etaSec / 60)}:${String(Math.round(arr.etaSec % 60)).padStart(2, '0')}` : '';
                // The label's rule (Q57): the pair of runways until 15 NM.
                const said = window.ProbableExits ? window.ProbableExits.arrivalText(arr.runway, arr.distNM) || arr.runway : arr.runway;
                second = {
                    label: `To ${said}`, value: arr.distNM.toFixed(1), unit: 'NM', extra: eta,
                    help: `Distance along the approach to runway ${arr.runway} at ${arr.airport}, and the time to touchdown at its present speed: the same estimate as the PAR.` +
                        (said !== arr.runway ? ' Until 15 NM out the pair of runways is said: the runway itself is right 97 times in 100 from there.' : ''),
                };
            } else if (typeof this.probableCell === 'function' && this.probableCell(a)) {
                // A departure's probable exit (Q57).
                second = this.probableCell(a);
            } else {
                const d = a.distance;
                second = {
                    label: 'Distance', value: d === undefined || d === null ? '–' : String(d), unit: d == null ? '' : 'NM', extra: '',
                    help: 'From the receiving station, in nautical miles (1 NM = 1.852 km).',
                };
            }

            const h = this.getHeadingWithType(a);
            const hType = { magnetic: 'mag', track: 'trk', true: 'true' }[h.type] || '';
            const gs = adsb.gs;
            return {
                callsign: (a.flight || '').trim() || a.hex,
                phase: phase || 'NEW',
                airport: this.plausibleAirport(this.getPhaseAirport(a), adsb.alt_baro, adsb.lat, adsb.lon, a) ? this.getPhaseAirport(a) : '',
                who: who.join(' · '),
                cells: [
                    { label: 'Altitude', value: alt.value, unit: alt.unit, extra: arrow, help: this.altitudeHelp(a) },
                    second,
                    {
                        label: 'Ground speed', value: typeof gs === 'number' ? String(Math.round(gs)) : '–', unit: typeof gs === 'number' ? 'kt' : '', extra: '',
                        help: 'Its speed over the ground, in knots. The wind sits between it and the airspeed a controller gives.',
                    },
                    {
                        label: 'Heading', value: h.value === null ? '–' : `${Math.round(h.value)}°`, unit: h.value === null ? '' : hType, extra: '',
                        help: this.headingHelp(h.type),
                    },
                ],
            };
        },

        // The map's note while a flight is followed: what the map shows (it
        // was centred once, it does not follow).
        get storySkyNote() {
            const a = this.selectedAircraft;
            if (!a) return null;
            const first = this.storyTrack().find((p) => p.t >= this.storyFlight.start);
            return {
                callsign: (a.flight || '').trim() || a.hex,
                since: first ? ` · its track since ${this.storyClock(new Date(first.t))}` : '',
            };
        },

        // The track as points in time order: { t, alt, lat, lon, distance, id }.
        storyTrack() {
            return (this.aircraftDetailsHistoryData || [])
                .map((p) => ({ t: Date.parse(p.timestamp), alt: p.altitude, lat: p.lat, lon: p.lon, distance: p.distance, id: p.id }))
                .filter((p) => Number.isFinite(p.t))
                .sort((x, y) => x.t - y.t);
        },

        // Is naming this airport for a phase plausible: low and near, or the
        // aircraft is in its arrival sequence?
        plausibleAirport(code, alt, lat, lon, aircraft) {
            if (!code) return false;
            const arr = aircraft && this.storyArrival(aircraft);
            if (arr && arr.airport === code) return true;
            const apt = (this.followedAirports || []).find((x) => x.code === code);
            if (!apt || typeof alt !== 'number' || lat == null || lon == null || apt.latitude == null) return false;
            return alt < AIRPORT_MAX_FT && nmBetween(lat, lon, apt.latitude, apt.longitude) < AIRPORT_MAX_NM;
        },

        // The flight in progress: it starts where the aircraft last reappeared
        // (a NEW phase, or a silence of the track); the ones before it today
        // are counted, and folded in the story.
        get storyFlight() {
            const a = this.selectedAircraft;
            const created = a ? Date.parse(a.created_at) : NaN;
            const starts = [];
            const phases = this.phaseHistoryAircraftHex === (a && a.hex) ? (this.phaseHistoryData || []) : [];
            for (const p of phases) {
                const t = Date.parse(p.timestamp);
                if (p.phase === 'NEW' && Number.isFinite(t) && !(Number.isFinite(created) && t - created < REAPPEAR_MS)) starts.push(t);
            }
            const pts = this.storyTrack();
            for (let i = 1; i < pts.length; i++) if (pts[i].t - pts[i - 1].t > FLIGHT_GAP_MS) starts.push(pts[i].t);
            const uniq = [...new Set(starts)].sort((x, y) => x - y)
                .filter((t, i, all) => i === 0 || t - all[i - 1] > REAPPEAR_MS);
            const start = uniq.length ? uniq[uniq.length - 1] - 1000 : -Infinity;
            return { start, earlier: uniq.length, marks: uniq };
        },

        // "↑", "↓" or "↔" from a vertical rate in ft/min (Nearby, step 13).
        rateArrow(rate) {
            return typeof rate !== 'number' ? '' : rate > 100 ? '↑' : rate < -100 ? '↓' : '↔';
        },

        // "↑", "↓" or "↔": a past or predicted position's trend, as text (the
        // Track tab, step 13), from the same reading as before.
        trackTrend(position, index, isFuture) {
            const icon = this.aircraftDetailsGetAltitudeTrend(position, index, isFuture);
            return icon === 'fas fa-arrow-up' ? '↑' : icon === 'fas fa-arrow-down' ? '↓' : '↔';
        },

        // "13:12Z" or "13:12": a timeline's time without its seconds.
        storyClock(at) {
            return this.timelineTime(at).replace(/(\d{1,2}:\d{2}):\d{2}/, '$1');
        },

        // The Story tab: the flight's timeline (www/timeline/), one line per
        // fact. A transmission is one line ("Controller on 124.350: Descend
        // to 3 000 ft"), a hand-off another ("Handed to Approche De Gaulle"):
        // the words stay in the thread, where a click on the line leads. The
        // flight in progress is shown, the earlier ones of the day folded;
        // the expected touchdown ends it when the aircraft is in a sequence.
        get storyEvents() {
            const a = this.selectedAircraft;
            const flight = this.storyFlight;
            const pts = this.storyTrack();
            const byId = new Map(pts.filter((p) => p.id != null).map((p) => [p.id, p]));
            const handed = new Set();
            for (const ev of this.flightTimeline) if (ev.kind === 'transfer' && ev.transmission) handed.add(ev.transmission.id);
            const afterReappearing = (t) => flight.marks.some((m) => t >= m - 1000 && t - m < REAPPEAR_MS);
            const P = (window.FlightTimeline && window.FlightTimeline.PHASES) || {};

            const items = [];
            for (const ev of this.flightTimeline) {
                const t = ev.at.getTime();
                let text = ev.text;
                let exchange = null;
                let radio = false;
                if (ev.kind === 'seen') continue;   // told from the track below
                if (ev.kind === 'phase') {
                    if ((ev.phase === 'T/O' || ev.phase === 'T/D') && afterReappearing(t)) continue;
                    const p = byId.get(ev.adsbId);
                    const name = (P[ev.phase] || [ev.phase])[0];
                    const named = ev.airport && p && this.plausibleAirport(ev.airport, p.alt, p.lat, p.lon, t >= flight.start ? a : null);
                    text = named ? `${name} at ${ev.airport}` : name;
                } else if (ev.kind === 'transfer') {
                    radio = true;
                    exchange = ev.transmission ? ev.transmission.id : null;
                    text = ev.toName ? `Handed to ${this.freqShortName({ name: ev.toName })}` : `Told to contact ${ev.frequency}`;
                } else if (ev.kind === 'transmission') {
                    const tr = ev.transmission;
                    if (handed.has(tr.id)) continue;
                    radio = true;
                    exchange = tr.id;
                    const who = tr.speaker_type === 'ATC' ? 'Controller' : tr.speaker_type === 'PILOT' ? 'Pilot' : 'Heard';
                    const said = (tr.plain || []).map((x) => x.text).join(', ');
                    text = `${who} on ${this.freqLabel(tr.frequency_id)}${said ? ': ' + said : ''}`;
                }
                items.push({ t, text, help: ev.help || '', radio, exchange, quote: ev.quote || '', key: items.length + ev.kind + t });
            }

            // Seen by the receiver: the first point of each flight, told from
            // the track; the day's first sight when the track does not reach it.
            const firsts = [Number.isFinite(Date.parse(a && a.created_at)) ? Date.parse(a.created_at) : null, ...flight.marks].filter((x) => x != null);
            firsts.forEach((from, i) => {
                const p = pts.find((q) => q.t >= from - 1000);
                const close = p && p.t - from < 2 * 60 * 1000;
                // Dated by the first message (it opens the flight), told from the first position.
                items.push(close ? { t: Math.min(p.t, from), text: this.seenText(p), help: 'The first position the station decoded for this flight.', key: 'seen' + i + p.t }
                    : { t: from, text: 'First seen by the receiver', help: 'When the station first decoded this aircraft\'s ADS-B messages in this database day.', key: 'seen' + i + from });
            });
            items.sort((x, y) => (x.t - y.t) || ((y.key.startsWith('seen') ? 1 : 0) - (x.key.startsWith('seen') ? 1 : 0)));

            const shape = (it, now) => ({
                key: it.key, time: this.storyClock(new Date(it.t)), next: false, text: it.text, help: it.help, quote: it.quote,
                exchange: it.exchange, dot: now ? 'is-now' : it.radio ? 'is-radio' : '',
            });
            const earlier = items.filter((it) => it.t < flight.start).map((it) => shape(it, false));
            const current = items.filter((it) => it.t >= flight.start);
            const list = current.map((it, i) => shape(it, i === current.length - 1));
            const arr = this.storyArrival(a);
            if (arr && Number.isFinite(arr.etaSec) && !(a && a.on_ground)) {
                const at = new Date(Date.now() + arr.etaSec * 1000);
                list.push({ key: 'next', time: '≈' + this.storyClock(at), dot: '', next: true, exchange: null, quote: '', help: '',
                    text: `Expected touchdown on ${arr.runway}, from its distance and speed.` });
            }
            return { current: list, earlier, flights: flight.earlier };
        },

        // "Seen by the receiver, FL 210, 61 NM north-east".
        seenText(p) {
            const parts = ['Seen by the receiver'];
            const r = typeof p.alt === 'number' && window.AltitudeScale ? window.AltitudeScale.reading(p.alt, this.altimetry) : null;
            if (r) parts.push(r.kind === 'gnd' ? 'on the ground' : r.kind === 'ft' ? `${group(Math.round(r.ft / 100) * 100)} ft` : window.AltitudeScale.readingText(r));
            if (p.lat != null && this.stationLatitude != null) {
                const nm = typeof p.distance === 'number' ? p.distance : nmBetween(this.stationLatitude, this.stationLongitude, p.lat, p.lon);
                parts.push(`${Math.round(nm)} NM ${compass(this.stationLatitude, this.stationLongitude, p.lat, p.lon)}`);
            }
            return parts.join(', ');
        },

        // The phase history, the flight in progress apart from the earlier ones.
        get storyPhases() {
            const start = this.storyFlight.start;
            const all = this.safePhaseHistoryData;
            return {
                current: all.filter((p) => Date.parse(p.timestamp) >= start),
                earlier: all.filter((p) => Date.parse(p.timestamp) < start),
            };
        },

        // "Show in the thread": the line's exchange, if the thread still holds it.
        showInThread(id) {
            if (id != null) window.dispatchEvent(new CustomEvent('thread-show', { detail: { id } }));
        },

        // "Radio · 6 matched", "Radio · none matched".
        get storyRadioCount() {
            const R = this.aircraftRadio;
            if (R.loading && !R.transcriptions.length) return '· loading…';
            return R.transcriptions.length ? `· ${R.transcriptions.length} matched` : '· none matched';
        },

        // The instructions heard: every value the grammar read in what the
        // controller said to this aircraft, newest first, with what the
        // aircraft did next. Kept until the transmissions or the track change:
        // each row reads the whole track.
        get instructionsHeard() {
            const a = this.selectedAircraft;
            const R = this.aircraftRadio;
            if (!a || !R || R.callsign !== (a.flight || '').trim()) return [];
            const start = this.storyFlight.start;
            const said = (R.transcriptions || []).filter((t) => t.speaker_type === 'ATC' && Array.isArray(t.plain) && t.plain.length && Date.parse(t.created_at) >= start);
            const track = this.aircraftDetailsHistoryData || [];
            const last = track.length ? track[track.length - 1].timestamp : '';
            const frames = this.storyFrames();
            const key = `${a.hex}|${start}|${said.length}|${said.length ? said[0].id : ''}|${track.length}|${last}|${Math.round(this.altimetry.qnh || 0)}|${frames ? frames.length : 0}`;
            if (memo.key === key) return memo.rows;

            const T = window.StoryThen;
            const options = {
                qnh: this.altimetry.qnh,
                ta: this.altimetry.ta,
                frames,
                transmissions: (R.transcriptions || []).map((t) => ({ at: t.created_at, frequency: this.freqMhz(t.frequency_id) })),
            };
            const rows = [];
            for (const t of said) {
                t.plain.forEach((p, i) => {
                    const then = T ? T.describe(Object.assign({}, p, { at: t.created_at }), track, options) : null;
                    rows.push({
                        key: `${t.id}-${i}`,
                        at: Date.parse(t.created_at),
                        time: this.formatDate(t.created_at, true),
                        text: p.text,
                        then: then ? then.text : '—',
                        known: !!then,
                    });
                });
            }
            rows.sort((x, y) => y.at - x.at);
            memo.key = key;
            memo.rows = rows;
            return rows;
        },

        freqMhz(freqId) {
            const f = (this.audioFrequencies || []).find((x) => x.id === freqId);
            return f && Number(f.frequency_mhz) > 0 ? Number(f.frequency_mhz) : null;
        },

        // The runway frames of the followed airports, to name the runway an
        // aircraft landed on (built once per airport, when its runways came).
        storyFrames() {
            const G = window.ParGeometry;
            if (!G) return null;
            const out = [];
            for (const apt of this.followedAirports || []) {
                const frames = framesByAirport[apt.code];
                if (frames) { out.push(...frames); continue; }
                if (frames === null || typeof this.runwaysOf !== 'function') continue;
                framesByAirport[apt.code] = null;
                this.runwaysOf(apt.code).then((runways) => {
                    framesByAirport[apt.code] = runways && runways.length ? G.buildFrames(apt.code, runways, G.DEFAULTS) : [];
                });
            }
            return out.length ? out : null;
        },

        // The Details tab's key-value sections (formerly one HTML string):
        // the same facts, grouped as c-details has them.
        get aircraftDetailGroups() {
            const groups = this.aircraftDetailFields();
            if (!groups) return [];
            const by = Object.fromEntries(groups);
            const merge = (...names) => names.flatMap((n) => by[n] || []);
            const rows = (items) => items.map(([k, v, extra]) => {
                const text = String(v === undefined || v === null ? '-' : v).trim();
                const none = text === '-' || /^-\s*(ft|kts|hPa|°|°C|s|dBm|ft\/min)?$/.test(text);
                const shown = k === 'Type' ? typeCase(text) : text;
                return { k, v: none ? '—' : (extra ? `${shown} · ${agoText(extra)}` : shown), none };
            });
            return [
                { key: 'basic', name: 'Basic info', rows: rows(merge('Basic Info', 'Status')) },
                { key: 'position', name: 'Position', rows: rows(merge('Position')) },
                { key: 'speed', name: 'Speed and direction', rows: rows(merge('Speed & Direction')) },
                { key: 'nav', name: 'Navigation and weather', rows: rows(merge('Navigation', 'Weather', 'ATC Derived')) },
                { key: 'adsb', name: 'ADS-B and signal', rows: rows(merge('ADSB Info', 'Signal')) },
            ];
        },
    }));
})();
