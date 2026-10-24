/**
 * Module: map/renderers/aircraft-webgl
 * Why it exists:
 * - Implements high-volume aircraft rendering and styling for OpenLayers maps.
 * - Encapsulates label composition, visual state styling, and icon colorization.
 *
 * Key responsibilities:
 * - Upsert aircraft features with geometry/style updates.
 * - Build callsign/type/detail labels and phase-aware visual prefixes.
 * - Manage WebGL-capable rendering paths with vector fallback behavior.
 *
 * Quirks / contracts:
 * - Runtime adapts between WebGL and vector rendering based on capability/errors.
 * - Icon colorization is cached by `iconSrc|colorHex` and notifies listeners on completion.
 * - Label density/readability settings are expected to be driven by external store state.
 */
(function () {
    const colorizedAircraftIconCache = new Map();
    const pendingAircraftIconColorizations = new Set();
    const aircraftIconColorizationListeners = new Set();

    function addAircraftIconColorizationListener(listener) {
        if (typeof listener === 'function') {
            aircraftIconColorizationListeners.add(listener);
        }
    }

    function removeAircraftIconColorizationListener(listener) {
        if (typeof listener === 'function') {
            aircraftIconColorizationListeners.delete(listener);
        }
    }

    function notifyAircraftIconColorized() {
        aircraftIconColorizationListeners.forEach((listener) => {
            try {
                listener();
            } catch (error) {
                // ignore listener failures
            }
        });
    }

    function createColorizedAircraftIcon(iconSrc, colorHex) {
        if (!iconSrc || !colorHex) return;

        const cacheKey = `${iconSrc}|${colorHex}`;
        if (colorizedAircraftIconCache.has(cacheKey) || pendingAircraftIconColorizations.has(cacheKey)) {
            return;
        }

        pendingAircraftIconColorizations.add(cacheKey);

        const image = new Image();
        image.decoding = 'async';
        image.onload = () => {
            try {
                const width = image.naturalWidth || image.width || 64;
                const height = image.naturalHeight || image.height || 64;
                const canvas = document.createElement('canvas');
                canvas.width = width;
                canvas.height = height;

                const context = canvas.getContext('2d');
                if (!context) return;

                context.clearRect(0, 0, width, height);
                context.drawImage(image, 0, 0, width, height);
                context.globalCompositeOperation = 'source-atop';
                context.fillStyle = colorHex;
                context.fillRect(0, 0, width, height);
                context.globalCompositeOperation = 'source-over';

                colorizedAircraftIconCache.set(cacheKey, canvas.toDataURL('image/png'));
                notifyAircraftIconColorized();
            } finally {
                pendingAircraftIconColorizations.delete(cacheKey);
            }
        };

        image.onerror = () => {
            pendingAircraftIconColorizations.delete(cacheKey);
        };

        image.src = iconSrc;
    }

    // A lost aircraft's glyph as an outline (MapLabel, signal lost): the
    // silhouette widened by about 1.5 px at the drawn size, its inside cut out.
    function createOutlinedAircraftIcon(iconSrc, colorHex, cacheKey) {
        pendingAircraftIconColorizations.add(cacheKey);
        const image = new Image();
        image.decoding = 'async';
        image.onload = () => {
            try {
                const width = image.naturalWidth || image.width || 64;
                const height = image.naturalHeight || image.height || 64;
                const solid = document.createElement('canvas');
                solid.width = width;
                solid.height = height;
                const s = solid.getContext('2d');
                if (!s) return;
                s.drawImage(image, 0, 0, width, height);
                s.globalCompositeOperation = 'source-in';
                s.fillStyle = colorHex;
                s.fillRect(0, 0, width, height);

                const canvas = document.createElement('canvas');
                canvas.width = width;
                canvas.height = height;
                const c = canvas.getContext('2d');
                if (!c) return;
                // The glyphs are drawn at about 24 px.
                const r = Math.max(1, 1.5 * width / 24);
                for (let a = 0; a < 16; a++) {
                    c.drawImage(solid, r * Math.cos(a * Math.PI / 8), r * Math.sin(a * Math.PI / 8));
                }
                c.globalCompositeOperation = 'destination-out';
                c.drawImage(image, 0, 0, width, height);
                colorizedAircraftIconCache.set(cacheKey, canvas.toDataURL('image/png'));
                notifyAircraftIconColorized();
            } finally {
                pendingAircraftIconColorizations.delete(cacheKey);
            }
        };
        image.onerror = () => pendingAircraftIconColorizations.delete(cacheKey);
        image.src = iconSrc;
    }

    function getOutlinedAircraftIconSrc(iconSrc, colorHex) {
        const cacheKey = `${iconSrc}|${colorHex}|outline`;
        const cached = colorizedAircraftIconCache.get(cacheKey);
        if (cached) return cached;
        if (!pendingAircraftIconColorizations.has(cacheKey)) createOutlinedAircraftIcon(iconSrc, colorHex, cacheKey);
        return getColorizedAircraftIconSrc(iconSrc, colorHex);
    }

    function getColorizedAircraftIconSrc(iconSrc, colorHex) {
        if (!iconSrc || !colorHex) return iconSrc || 'assets/a0.svg';

        const cacheKey = `${iconSrc}|${colorHex}`;
        const cached = colorizedAircraftIconCache.get(cacheKey);
        if (cached) return cached;

        createColorizedAircraftIcon(iconSrc, colorHex);
        return iconSrc;
    }

    function hasWebGLPointsSupport() {
        return !!(window.ol && window.ol.layer && window.ol.layer.WebGLPoints);
    }

    function getAircraftHeading(aircraft, store) {
        if (store && typeof store.getHeadingWithFallback === 'function') {
            const heading = store.getHeadingWithFallback(aircraft);
            if (Number.isFinite(heading)) return heading;
        }

        const adsb = aircraft?.adsb || {};
        if (Number.isFinite(adsb.true_heading)) return adsb.true_heading;
        if (Number.isFinite(adsb.mag_heading)) return adsb.mag_heading;
        if (Number.isFinite(adsb.track)) return adsb.track;
        // None: the receiver lets track and heading expire when the messages
        // stop, so a lost aircraft has none. The caller keeps the glyph's
        // last orientation; north only for an aircraft never oriented.
        return null;
    }

    function getAircraftPhase(aircraft) {
        return window.MapVisibilityRules && window.MapVisibilityRules.getCurrentPhase
            ? window.MapVisibilityRules.getCurrentPhase(aircraft)
            : ((aircraft && aircraft.phase && aircraft.phase.current && aircraft.phase.current.length > 0)
                ? aircraft.phase.current[0].phase
                : 'NEW');
    }

    function getAircraftWakeCategory(aircraft) {
        const category = (aircraft?.adsb?.category || '').toString().toUpperCase().trim();
        return category;
    }

    function normalizeAircraftTypeText(value) {
        const text = (value || '').toString().toUpperCase();
        return text
            .replace(/[\/_.,-]+/g, ' ')
            .replace(/\s+/g, ' ')
            .trim();
    }

    function compactAircraftTypeText(value) {
        return normalizeAircraftTypeText(value).replace(/[^A-Z0-9]/g, '');
    }

    function getAircraftTypeCandidates(aircraft) {
        const rawCandidates = [
            aircraft?.bsdb?.type,
            aircraft?.adsb?.type,
            aircraft?.adsb?.t,
            aircraft?.type,
            aircraft?.aircraft_type,
            aircraft?.model,
        ];

        if (aircraft?.bsdb?.manufacturer && aircraft?.bsdb?.type) {
            rawCandidates.push(`${aircraft.bsdb.manufacturer} ${aircraft.bsdb.type}`);
        }

        const unique = new Set();
        const candidates = [];

        for (const value of rawCandidates) {
            const normalized = normalizeAircraftTypeText(value);
            if (!normalized || unique.has(normalized)) continue;

            unique.add(normalized);
            candidates.push({
                normalized,
                compact: compactAircraftTypeText(normalized),
            });
        }

        return candidates;
    }

    function matchesAircraftTypePattern(candidates, patterns) {
        return candidates.some((candidate) => {
            return patterns.some((pattern) => {
                if (pattern.normalized && pattern.normalized.test(candidate.normalized)) return true;
                if (pattern.compact && pattern.compact.test(candidate.compact)) return true;
                return false;
            });
        });
    }

    function getSpecificAircraftIconClass(aircraft) {
        const candidates = getAircraftTypeCandidates(aircraft);
        if (!candidates.length) return null;
        const match = (patterns) => matchesAircraftTypePattern(candidates, patterns);

        if (match([{ normalized: /\bA\s*3\s*8\s*0\b|\bAIRBUS\s+A\s*380\b/, compact: /A380/ }])) return 'a380';
        if (match([{ normalized: /\bA\s*3\s*4\s*0\b|\bAIRBUS\s+A\s*340\b/, compact: /A340/ }])) return 'a340';
        if (match([{ normalized: /\bA\s*3\s*3\s*0\b|\bAIRBUS\s+A\s*330\b/, compact: /A330/ }])) return 'a330';
        if (match([{ normalized: /\bA\s*3\s*(1[89]|2[01])\b|\bAIRBUS\s+A\s*3\s*(1[89]|2[01])\b|\bA20N\b|\bA21N\b/, compact: /A31[89]|A320|A321|A20N|A21N/ }])) return 'a320';
        if (match([{ normalized: /\bB\s*7\s*8\s*7\b|\bBOEING\s+787\b|\b78[89]\b/, compact: /B787|78[89]|78X/ }])) return 'b787';
        if (match([{ normalized: /\bB\s*7\s*7\s*7\b|\bBOEING\s+777\b|\b77[0-9W]\b/, compact: /B777|77[0-9W]/ }])) return 'b777';
        if (match([{ normalized: /\bB\s*7\s*6\s*7\b|\bBOEING\s+767\b|\b76[0-9]\b/, compact: /B767|76[0-9]/ }])) return 'b767';
        if (match([{ normalized: /\bB\s*7\s*4\s*7\b|\bBOEING\s+747\b|\b74[0-9]\b/, compact: /B747|74[0-9]/ }])) return 'b747';
        if (match([{ normalized: /\bB\s*7\s*3\s*7\b|\bBOEING\s+737\b|\b73[0-9]\b/, compact: /B737|73[0-9]/ }])) return 'b737';
        if (match([{ normalized: /\bC\s*1\s*3\s*0\b|\bHERCULES\b/, compact: /C130|HERCULES/ }])) return 'c130';
        if (match([{ normalized: /\bCRJ\b|\bCANADAIR\s+REGIONAL\s+JET\b|\bCL\s*[- ]?65\b/, compact: /CRJ|CL65/ }])) return 'crjx';
        if (match([{ normalized: /\bDH\s*8\s*A\b|\bDHC\s*8\b|\bDASH\s*8\b|\bQ\s*400\b/, compact: /DH8A|DHC8|DASH8|Q400/ }])) return 'dh8a';
        if (match([{ normalized: /\bE\s*1\s*9\s*5\b/, compact: /E195/ }])) return 'e195';
        if (match([{ normalized: /\bERJ\b|\bEMBRAER\s+REGIONAL\s+JET\b|\bE\s*1\s*7\s*[05]\b/, compact: /ERJ|E170|E175/ }])) return 'erj';
        if (match([{ normalized: /\bF\s*1\s*0\s*0\b|\bFOKKER\s*100\b/, compact: /F100|FOKKER100/ }])) return 'f100';
        if (match([{ normalized: /\bFA\s*7\s*X\b|\bFALCON\s*7\s*X\b/, compact: /FA7X|FALCON7X/ }])) return 'fa7x';
        if (match([{ normalized: /\bGLF\s*5\b|\bGULFSTREAM\s*(V|5|G\s*5\s*0\s*0)\b/, compact: /GLF5|GULFSTREAMV|GULFSTREAM5|G500|GV/ }])) return 'glf5';
        if (match([{ normalized: /\bLEAR\s*JET\b|\bLEARJET\b|\bLJ\s*\d{2}\b/, compact: /LEARJET|LJ\d{2}/ }])) return 'learjet';
        if (match([{ normalized: /\bMD\s*1\s*1\b|\bMCDONNELL\s+DOUGLAS\s+11\b/, compact: /MD11/ }])) return 'md11';
        if (match([{ normalized: /\bCESSNA\b|\bC\s*1\s*5\s*2\b|\bC\s*1\s*7\s*2\b|\bC\s*2\s*0\s*8\b/, compact: /CESSNA|C152|C172|C208/ }])) return 'cessna';

        return null;
    }

    function getAircraftIconClass(aircraft) {
        const specificTypeClass = getSpecificAircraftIconClass(aircraft);
        if (specificTypeClass) return specificTypeClass;

        const category = getAircraftWakeCategory(aircraft);
        if (category === 'A1') return 'a1';
        if (category === 'A2') return 'a2';
        if (category === 'A3') return 'a3';
        if (category === 'A4') return 'a4';
        if (category === 'A5') return 'a5';
        if (category === 'A6') return 'a6';
        if (category === 'A7') return 'a7';
        return 'unknown';
    }

    function getAircraftAssetPath(iconClass) {
        switch (iconClass) {
            case 'a1': return 'assets/a1.svg';
            case 'a2': return 'assets/a2.svg';
            case 'a3': return 'assets/a3.svg';
            case 'a4': return 'assets/a4.svg';
            case 'a5': return 'assets/a5.svg';
            case 'a6': return 'assets/a6.svg';
            case 'a7': return 'assets/a7.svg';
            case 'a320': return 'assets/aircraft/a320.svg';
            case 'a330': return 'assets/aircraft/a330.svg';
            case 'a340': return 'assets/aircraft/a340.svg';
            case 'a380': return 'assets/aircraft/a380.svg';
            case 'b737': return 'assets/aircraft/b737.svg';
            case 'b747': return 'assets/aircraft/b747.svg';
            case 'b767': return 'assets/aircraft/b767.svg';
            case 'b777': return 'assets/aircraft/b777.svg';
            case 'b787': return 'assets/aircraft/b787.svg';
            case 'c130': return 'assets/aircraft/c130.svg';
            case 'cessna': return 'assets/aircraft/cessna.svg';
            case 'crjx': return 'assets/aircraft/crjx.svg';
            case 'dh8a': return 'assets/aircraft/dh8a.svg';
            case 'e195': return 'assets/aircraft/e195.svg';
            case 'erj': return 'assets/aircraft/erj.svg';
            case 'f100': return 'assets/aircraft/f100.svg';
            case 'fa7x': return 'assets/aircraft/fa7x.svg';
            case 'glf5': return 'assets/aircraft/glf5.svg';
            case 'learjet': return 'assets/aircraft/learjet.svg';
            case 'md11': return 'assets/aircraft/md11.svg';
            case 'unknown':
            default:
                return 'assets/a0.svg';
        }
    }

    function getAircraftIconSize(iconClass) {
        const scale = 0.805;
        const scaled = (size) => Math.max(18, Math.round(size * scale));
        switch (iconClass) {
            case 'a1': return scaled(26);
            case 'a2': return scaled(30);
            case 'a3': return scaled(33);
            case 'a4': return scaled(36);
            case 'a5': return scaled(38);
            case 'a6':
            case 'a7': return scaled(31);
            case 'unknown': return scaled(30);
            default: return scaled(34);
        }
    }

    function getAltitudeColorBand(aircraft, store) {
        const altitude = Number(aircraft?.adsb?.alt_baro);
        if (!Number.isFinite(altitude)) return 'cruise';
        // The bands the altitude legend and the Traffic slider draw (altitude-bands.js).
        if (window.AltitudeBands) return window.AltitudeBands.bandOf(altitude, store?.stationCruiseAltitudeFt);

        const configuredCruise = Number(store?.stationCruiseAltitudeFt);
        const cruiseAltitudeFt = Number.isFinite(configuredCruise) && configuredCruise > 0
            ? configuredCruise
            : 18000;

        if (altitude >= cruiseAltitudeFt) return 'cruise';
        if (altitude > 12500) return 'high';
        if (altitude >= 3000) return 'mid';
        if (altitude >= 1000) return 'low';
        return 'very-low';
    }

    // The bright base maps are ign and osm; ign-dark, osm-dark and none (the aircraft
    // on plain black, the default since 10/10) are dark. Testing "ends with dark" put
    // the black background in the bright palette: cruise aircraft at 3.1:1 on black.
    const BRIGHT_MAP_STYLES = ['ign', 'osm'];

    function isBrightMapStyle(store) {
        return BRIGHT_MAP_STYLES.includes((store?.settings?.mapStyle || 'none').toLowerCase());
    }

    // The altitude of an aircraft on the map: the design system's palette F,
    // warm to cold from the lowest to cruise, chosen by the owner on 13/10 on
    // a scene of 34 aircraft against the palette of before, a grey ramp and
    // another variant (every pair of bands at least 13.6 apart under the worst
    // colour blindness, far from the urgency red and from the phase families).
    // Tokens color.altitude.dark-* on a dark map, light-* on a bright one, read
    // once from the page, the same hex as fallbacks for a canvas.
    const ALTITUDE_PALETTE = {
        dark: { 'very-low': '#fff59a', low: '#e68cba', mid: '#c2a3ff', high: '#61c9e3', cruise: '#6e7b8f' },
        light: { 'very-low': '#33311f', low: '#452a38', mid: '#746299', high: '#27505b', cruise: '#6e7b8f' },
    };
    let altitudePaletteCache = null;

    function altitudePalette() {
        if (altitudePaletteCache) return altitudePaletteCache;
        const palette = { dark: { ...ALTITUDE_PALETTE.dark }, light: { ...ALTITUDE_PALETTE.light } };
        if (typeof document !== 'undefined' && document.documentElement) {
            const cs = getComputedStyle(document.documentElement);
            for (const theme of ['dark', 'light']) {
                for (const band of Object.keys(palette[theme])) {
                    const v = cs.getPropertyValue(`--color-altitude-${theme}-${band}`).trim();
                    if (v) palette[theme][band] = v;
                }
            }
        }
        altitudePaletteCache = palette;
        return palette;
    }

    function getAltitudeBandColor(aircraft, store) {
        const band = getAltitudeColorBand(aircraft, store);
        const palette = altitudePalette()[isBrightMapStyle(store) ? 'light' : 'dark'];
        return palette[band] || palette.cruise;
    }

    // The text colours of the design system (color.text.primary / tertiary),
    // read once from the page's variables, the tokens' hex as fallbacks.
    const TEXT_COLORS = { primary: '#e0e0e0', secondary: '#b8b8b8', tertiary: '#949494' };
    let textColorCache = null;

    function accentColor() {
        if (typeof document !== 'undefined' && document.documentElement) {
            const v = getComputedStyle(document.documentElement).getPropertyValue('--color-accent-default').trim();
            if (v) return v;
        }
        return '#4CAF50';
    }

    function labelTextColor(level) {
        if (!textColorCache) {
            textColorCache = { ...TEXT_COLORS };
            if (typeof document !== 'undefined' && document.documentElement) {
                const cs = getComputedStyle(document.documentElement);
                for (const k of Object.keys(textColorCache)) {
                    const v = cs.getPropertyValue(`--color-text-${k}`).trim();
                    if (v) textColorCache[k] = v;
                }
            }
        }
        return textColorCache[level] || textColorCache.primary;
    }

    // A lost aircraft (no message for a minute) is drawn in the tertiary text
    // colour where the receiver last placed it, not on the altitude ramp: it is
    // no longer at that altitude for sure (owner, 14/10; AircraftStrip is-lost).
    function getAircraftSymbolColor(aircraft, visualState, store) {
        if (aircraft && aircraft.status === 'signal_lost') return labelTextColor('tertiary');
        return getAltitudeBandColor(aircraft, store);
    }

    function hexToRgba(hexColor, alpha) {
        if (typeof hexColor !== 'string' || !hexColor.startsWith('#')) {
            return `rgba(76, 175, 80, ${alpha})`;
        }

        const normalized = hexColor.length === 4
            ? `#${hexColor[1]}${hexColor[1]}${hexColor[2]}${hexColor[2]}${hexColor[3]}${hexColor[3]}`
            : hexColor;

        const r = parseInt(normalized.slice(1, 3), 16);
        const g = parseInt(normalized.slice(3, 5), 16);
        const b = parseInt(normalized.slice(5, 7), 16);
        if ([r, g, b].some((value) => Number.isNaN(value))) {
            return `rgba(76, 175, 80, ${alpha})`;
        }
        return `rgba(${r}, ${g}, ${b}, ${alpha})`;
    }

    function getAircraftShortIcaoType(aircraft) {
        const candidates = [
            aircraft?.bsdb?.icao_type_code,
            aircraft?.adsb?.t,
        ];

        for (const candidate of candidates) {
            const raw = (candidate || '').toString().toUpperCase().trim();
            if (!raw) continue;

            const compact = raw.replace(/[^A-Z0-9]/g, '');
            if (!compact) continue;

            if (compact.length <= 6) return compact;
            return compact.slice(0, 6);
        }

        return '';
    }

    function getAltitudeTrendArrow(aircraft) {
        const baroRate = Number(aircraft?.adsb?.baro_rate);
        if (!Number.isFinite(baroRate)) return '↔';
        if (baroRate > 100) return '↑';
        if (baroRate < -100) return '↓';
        return '↔';
    }

    // The five families of the design system (www/phase/phase-families.js,
    // migration step 2): one colour per family, the code written on the label.
    function getPhaseLabelColor(phase) {
        const P = window.PhaseFamilies;
        return P ? P.colorOf(phase) : '#969696';
    }

    function getAircraftLabelParts(aircraft, store) {
        if (!aircraft) {
            return {
                phaseText: '',
                phaseColor: labelTextColor('tertiary'),
                callsignText: '',
                typeText: '',
                detailsText: '',
                ageText: '',
            };
        }

        const callsign = (aircraft.flight || aircraft.hex || '').trim();
        const phase = (aircraft?.phase?.current?.[0]?.phase || '').toString().trim().toUpperCase();
        const phaseText = phase ? `[${phase}] ` : '';
        // Lost, it has no current phase: the code stays written, in the
        // label's tertiary (MapLabel, signal lost).
        const phaseColor = aircraft.status === 'signal_lost' ? labelTextColor('tertiary') : getPhaseLabelColor(phase || 'NEW');
        const shortType = getAircraftShortIcaoType(aircraft);
        // As the list says it: feet on the QNH below the transition altitude,
        // flight level above (store.altitudeReading, www/altitude/).
        const reading = store && typeof store.altitudeReading === 'function' ? store.altitudeReading(aircraft) : null;
        const altitude = !Number.isFinite(aircraft?.adsb?.alt_baro)
            ? '-'
            : aircraft.adsb.alt_baro === 0 ? 'GND'
            : reading && reading.kind === 'fl' ? 'FL ' + String(reading.fl).padStart(3, '0')
            // The brand's numbers (MapLabel), as the story and the list
            // write them: "3 100 ft", "168 kt", a thin space in thousands.
            : `${groupThousands(Math.round((reading ? reading.ft : aircraft.adsb.alt_baro) / 10) * 10)} ft`;
        const trendArrow = getAltitudeTrendArrow(aircraft);
        const speed = Number.isFinite(aircraft?.adsb?.gs)
            ? `${Math.round(aircraft.adsb.gs)} kt`
            : '-- kt';

        let ageSeconds = null;
        if (store && typeof store.getSecondsSinceLastSeen === 'function') {
            const fromStore = store.getSecondsSinceLastSeen(aircraft);
            if (Number.isFinite(fromStore)) {
                ageSeconds = fromStore;
            }
        }

        if (ageSeconds === null && aircraft.last_seen) {
            const lastSeenMs = new Date(aircraft.last_seen).getTime();
            if (Number.isFinite(lastSeenMs) && lastSeenMs > 0) {
                ageSeconds = Math.floor((Date.now() - lastSeenMs) / 1000);
            }
        }

        return {
            phaseText,
            phaseColor,
            callsignText: callsign,
            typeText: shortType ? ` ${shortType}` : '',
            // Lost: since when, and the last altitude it reported.
            detailsText: aircraft.status === 'signal_lost'
                ? `signal lost ${lostAgeText(ageSeconds)}· last ${lostAltitudeText(aircraft, reading)}`
                : `${altitude} ${trendArrow} ${speed}`.trim(),
            ageText: (aircraft.status === 'signal_lost' || ageSeconds === null || !Number.isFinite(ageSeconds) || ageSeconds < 5)
                ? ''
                : `-${Math.max(0, ageSeconds)}s`,
            // Where it is probably going (Q57, MapLabel): "→26L/R", "→OKASI"
            // and the count of the others, read by store/probable.js.
            probable: aircraft.status === 'signal_lost' || store?.settings?.showProbable === false
                ? null
                : (store?.probable?.[aircraft.hex] || null),
        };
    }

    // The width of a label's text in its own font, for what is drawn over it
    // in another colour (the probable runway or exit).
    let labelMeasure = null;
    function labelTextWidth(text) {
        if (!labelMeasure) {
            labelMeasure = document.createElement('canvas').getContext('2d');
            labelMeasure.font = '11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';
        }
        return labelMeasure.measureText(text).width;
    }

    function groupThousands(n) {
        return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, '\u202f');
    }

    // "1 620 ft", "FL 110", "ground": the brand's numbers (thin space).
    function lostAltitudeText(aircraft, reading) {
        const alt = aircraft?.adsb?.alt_baro;
        if (alt === 0 || alt === 'ground') return 'ground';
        if (!Number.isFinite(alt)) return '—';
        if (reading && reading.kind === 'fl') return 'FL ' + String(reading.fl).padStart(3, '0');
        const ft = Math.round((reading && Number.isFinite(reading.ft) ? reading.ft : alt) / 10) * 10;
        return String(ft).replace(/\B(?=(\d{3})+(?!\d))/g, '\u202f') + ' ft';
    }

    // "40 s ", "3 min ": how long ago a lost aircraft was last heard.
    function lostAgeText(seconds) {
        if (!Number.isFinite(seconds)) return '';
        return seconds < 60 ? `${Math.max(0, Math.round(seconds))} s ` : `${Math.floor(seconds / 60)} min `;
    }

    // The last transmission matched to the aircraft, as a message tag hung
    // under its label (MapLabel, owner 15/10): the third line of 13/10 widened
    // the label to 40 mono characters and read as data. Full for two minutes,
    // at 55 % to ten, gone at ten; off with the "last transmission on aircraft
    // labels" setting.
    const VOICE_FULL_MS = 2 * 60 * 1000;
    const VOICE_WINDOW_MS = 10 * 60 * 1000;
    const VOICE_MAX_CHARS = 34;
    // One line of the 11 px label, at OpenLayers' default line height of 1.2,
    // and the label's padding above and below.
    const LABEL_LINE_PX = 13;
    const LABEL_PAD_Y = 2;
    const LABEL_PAD_X = 6;
    const TAG_GAP_PX = 3;
    const TAG_HEIGHT_PX = 18;

    function getVoiceTag(aircraft, store) {
        if (store?.settings?.voiceBubbles === false) return null;
        const v = aircraft?.voice;
        if (!v || !v.last_text || !v.last_heard) return null;
        const heard = Date.parse(v.last_heard);
        const ageMs = Date.now() - heard;
        if (!Number.isFinite(heard) || ageMs > VOICE_WINDOW_MS) return null;
        const t = String(v.last_text).replace(/\s+/g, ' ').trim();
        if (!t) return null;
        const seconds = Math.max(0, Math.round(ageMs / 1000));
        return {
            text: '“' + (t.length > VOICE_MAX_CHARS ? t.slice(0, VOICE_MAX_CHARS - 1).trimEnd() + '…' : t) + '”',
            age: seconds < 60 ? `${seconds} s` : `${Math.floor(seconds / 60)} min`,
            alpha: ageMs <= VOICE_FULL_MS ? 1 : 0.55,
        };
    }

    // The tag drawn once into a canvas (a bar, the words, the age), an icon
    // for OpenLayers, whose text style has no left bar nor two fonts.
    const tagCanvasCache = new Map();
    function tagCanvas(tag) {
        const key = `${tag.text}|${tag.age}`;
        let entry = tagCanvasCache.get(key);
        if (entry) return entry;
        if (tagCanvasCache.size > 400) tagCanvasCache.clear();
        const P = window.MapPalette;
        const ratio = Math.max(1, Math.round(window.devicePixelRatio || 1));
        const textFont = `11px ${cssVar('--font-family-text', 'system-ui, sans-serif')}`;
        const monoFont = `10px ${cssVar('--font-family-mono', 'ui-monospace, monospace')}`;
        const measure = document.createElement('canvas').getContext('2d');
        measure.font = textFont;
        const textW = Math.ceil(measure.measureText(tag.text).width);
        measure.font = monoFont;
        const ageW = Math.ceil(measure.measureText(tag.age).width);
        const bar = 2.5;
        const width = Math.ceil(bar + 6 + textW + 8 + ageW + 6);
        const canvas = document.createElement('canvas');
        canvas.width = width * ratio;
        canvas.height = TAG_HEIGHT_PX * ratio;
        const ctx = canvas.getContext('2d');
        ctx.scale(ratio, ratio);
        ctx.fillStyle = cssVar('--color-surface-raised', '#222222');
        ctx.fillRect(0, 0, width, TAG_HEIGHT_PX);
        ctx.fillStyle = cssVar('--color-status-info', '#6cb6ff');
        ctx.fillRect(0, 0, bar, TAG_HEIGHT_PX);
        ctx.textBaseline = 'middle';
        ctx.font = textFont;
        ctx.fillStyle = labelTextColor('primary');
        ctx.fillText(tag.text, bar + 6, TAG_HEIGHT_PX / 2 + 0.5);
        ctx.font = monoFont;
        ctx.fillStyle = labelTextColor('tertiary');
        ctx.textAlign = 'right';
        ctx.fillText(tag.age, width - 6, TAG_HEIGHT_PX / 2 + 0.5);
        entry = { canvas, width, ratio };
        tagCanvasCache.set(key, entry);
        return entry;
    }

    function cssVar(name, fallback) {
        if (typeof document === 'undefined' || !document.documentElement) return fallback;
        return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
    }

    // Zoom tiers (MapLabel, owner 15/10), by the width of the sky in nautical
    // miles on the ground, not the zoom level: folding the column widens the
    // sky. Measured on about 110 aircraft: at 34 NM, 62 % of full labels
    // overlapped another; at 67 NM, 87 %; callsigns alone 47 to 87 % beyond.
    const FULL_LABELS_UP_TO_NM = 24;
    const CALLSIGNS_UP_TO_NM = 67;
    const EMERGENCY_SQUAWKS = new Set(['7500', '7600', '7700']);

    function skyWidthNm(map, resolution) {
        if (!map || !Number.isFinite(resolution)) return 0;
        const size = map.getSize ? map.getSize() : null;
        const center = map.getView ? map.getView().getCenter() : null;
        if (!size || !center) return 0;
        const lat = window.ol.proj.toLonLat(center)[1];
        return resolution * size[0] * Math.cos(lat * Math.PI / 180) / 1852;
    }

    // The aircraft that matter keep their label further out: heard in the
    // last ten minutes, in an arrival sequence, chosen or followed, or
    // squawking an emergency.
    let sequenceHexes = null;
    let sequenceHexesAt = 0;
    function inArrivalSequence(hex, store) {
        const now = Date.now();
        if (!sequenceHexes || now - sequenceHexesAt > 1000) {
            sequenceHexes = new Set();
            for (const seqs of Object.values(store?.arrivals || {})) {
                for (const seq of (Array.isArray(seqs) ? seqs : [])) {
                    for (const item of (seq?.items || [])) if (item?.hex) sequenceHexes.add(item.hex);
                }
            }
            sequenceHexesAt = now;
        }
        return sequenceHexes.has(hex);
    }

    function aircraftMatters(aircraft, properties, store) {
        if (!aircraft) return false;
        if (properties.selected) return true;
        if (EMERGENCY_SQUAWKS.has(String(aircraft?.adsb?.squawk || ''))) return true;
        const heard = Date.parse(aircraft?.voice?.last_heard || '');
        if (Number.isFinite(heard) && Date.now() - heard <= VOICE_WINDOW_MS) return true;
        return inArrivalSequence(aircraft.hex, store);
    }

    function createAircraftWebGLRenderer(map, options) {
        const zIndex = Number.isFinite(options?.zIndex) ? options.zIndex : 400;
        const store = options?.store || null;
        const WEBGL_DISABLE_SESSION_KEY = 'coatc.aircraftRenderer.webglDisabled';
        const preferWebGL = options?.preferWebGL === true;

        const source = new window.ol.source.Vector();
        const featureIndex = new Map();
        const styleCache = new Map();

        let layer = null;
        let backend = 'vector';
        let runtimeErrorHandler = null;
        let runtimeRejectionHandler = null;
        let iconColorizationListener = null;

        const stats = {
            upserts: 0,
            removes: 0,
            bulkSyncs: 0,
            updateDurationMs: 0,
            backend: 'vector',
        };

        function getStyleKey(properties) {
            const selected = properties.selected ? 1 : 0;
            const hovered = properties.hovered ? 1 : 0;
            const color = properties.color || '#4CAF50';
            const rotationRad = Number.isFinite(properties.rotationRad) ? properties.rotationRad : 0;
            const iconSrc = properties.iconSrc || 'assets/a0.svg';
            const iconSize = Number.isFinite(properties.iconSize) ? properties.iconSize : 24;
            const labelColor = properties.labelColor || labelTextColor('tertiary');
            const deemphasized = properties.deemphasized ? 1 : 0;
            const fade = Number.isFinite(properties.fade) ? properties.fade : 1; // relocation (aircraft-animation.js)
            return `${selected}_${hovered}_${deemphasized}_${color}_${labelColor}_${iconSrc}_${iconSize}_${rotationRad.toFixed(3)}_${fade}`;
        }

        function getVectorStyle(feature, resolution) {
            const properties = feature.getProperties();
            const aircraft = store?.aircraft?.[properties.hex] || null;
            const labelParts = (store?.settings?.showLabels === false)
                ? { callsignText: '', typeText: '', detailsText: '', ageText: '' }
                : getAircraftLabelParts(aircraft, store);
            // Full label, callsign alone, or the glyph alone (zoom tiers);
            // hovering shows the full label always.
            const widthNm = skyWidthNm(map, resolution);
            const labelMode = (properties.hovered || widthNm <= FULL_LABELS_UP_TO_NM)
                ? 'full'
                : aircraftMatters(aircraft, properties, store)
                    ? (widthNm <= CALLSIGNS_UP_TO_NM ? 'full' : 'callsign')
                    : (widthNm <= CALLSIGNS_UP_TO_NM ? 'callsign' : 'none');
            const voiceTag = labelMode === 'full' && labelParts.callsignText ? getVoiceTag(aircraft, store) : null;

            // The label's text reads in the page's primary text colour, tertiary
            // once the signal is lost (design system, migration step 2): green is
            // the tool's own voice, chosen or just heard, never "active". The phase
            // prefix alone carries its family's colour; the rings carry the green.
            const isActiveAircraft = aircraft?.status === 'active';
            let labelColor = labelTextColor(isActiveAircraft ? 'primary' : 'tertiary');
            let staleAgeSeconds = 0;
            let staleAgeFromStore = false;
            if (store && typeof store.getSecondsSinceLastSeen === 'function') {
                const fromStore = store.getSecondsSinceLastSeen(aircraft);
                if (Number.isFinite(fromStore)) {
                    staleAgeSeconds = Math.max(0, fromStore);
                    staleAgeFromStore = true;
                }
            }
            if (!staleAgeFromStore) {
                staleAgeSeconds = aircraft?.last_seen
                    ? Math.max(0, Math.floor((Date.now() - new Date(aircraft.last_seen).getTime()) / 1000))
                    : 0;
            }

            const labelAgeFade = (() => {
                if (!Number.isFinite(staleAgeSeconds) || staleAgeSeconds <= 10) return 1;
                if (staleAgeSeconds >= 60) return 0.5;
                const progress = (staleAgeSeconds - 10) / 50;
                return 1 - (progress * 0.5);
            })();

            const key = getStyleKey(properties);
            const isSelected = !!properties.selected;
            const isHovered = !!properties.hovered;
            const deemphasized = !!properties.deemphasized;
            const brightMap = isBrightMapStyle(store);
            const baseTextOpacity = deemphasized
                ? (brightMap ? 0.72 : 0.5)
                : (brightMap ? 1 : 0.9);
            const fade = Number.isFinite(properties.fade) ? properties.fade : 1; // relocation (aircraft-animation.js)
            const textOpacity = baseTextOpacity * labelAgeFade * fade;
            const labelLineOne = `${labelParts.phaseText || ''}${labelParts.callsignText || ''}${labelParts.typeText || ''}`;
            // The probable runway or exit ends the second line, two spaces after
            // the speed; the line keeps their room in no-break spaces, and
            // they are drawn over it in their own colours.
            const probable = labelMode === 'full' ? labelParts.probable : null;
            const probableWords = probable ? `  ${probable.main}${probable.rest || ''}` : '';
            if (probableWords) labelParts.detailsText = `${labelParts.detailsText || ''}${'\u00a0'.repeat(probableWords.length)}`;
            const labelLineTwo = `${labelParts.detailsText || ''}`;
            const longestLabelLineLength = Math.max(labelLineOne.length, labelLineTwo.length);
            const estimatedLabelWidthPx = Math.max(72, Math.round(longestLabelLineLength * 6.4) + 12);
            const labelBorderColor = brightMap
                ? hexToRgba('#111827', (deemphasized ? 0.7 : 0.85) * labelAgeFade)
                : hexToRgba(labelTextColor('tertiary'), 0.45 * labelAgeFade);
            const labelBackgroundColor = brightMap
                ? hexToRgba('#111827', (deemphasized ? 0.72 : 0.82) * labelAgeFade)
                : hexToRgba('#000000', 0.45 * labelAgeFade);

            const iconSize = Number.isFinite(properties.iconSize) ? properties.iconSize : 24;
            const rotationRad = Number.isFinite(properties.rotationRad) ? properties.rotationRad : 0;
            const markerAlpha = (deemphasized ? 0.5 : 1) * fade;
            const iconColor = properties.color || '#E8ECEF';
            const iconSrc = properties.status === 'signal_lost'
                ? getOutlinedAircraftIconSrc(properties.iconSrc || 'assets/a0.svg', iconColor)
                : getColorizedAircraftIconSrc(properties.iconSrc || 'assets/a0.svg', iconColor);
            const showHalo = isSelected || isHovered;
            // The chosen aircraft's ring in the accent, 2 px; the hovered one
            // in the primary text colour, 1.2 px (palette C, 15/10).
            const P = window.MapPalette;
            const haloColor = P ? P.color(isSelected ? 'selected' : 'hovered') : labelTextColor('primary');
            const haloFillAlpha = isSelected ? 0.12 : 0;
            const haloStrokeAlpha = deemphasized ? 0.4 : 1;
            const haloRadius = Math.max(12, Math.round(iconSize * (isSelected ? 1.08 : 0.96)));

            let symbolStyles = styleCache.get(key);
            if (!symbolStyles) {
                const iconStyle = new window.ol.style.Style({
                    image: new window.ol.style.Icon({
                        src: iconSrc,
                        width: iconSize,
                        height: iconSize,
                        anchor: [0.5, 0.5],
                        anchorXUnits: 'fraction',
                        anchorYUnits: 'fraction',
                        rotation: rotationRad,
                        rotateWithView: false,
                        opacity: markerAlpha,
                    }),
                });

                const haloStyle = showHalo
                    ? new window.ol.style.Style({
                        image: new window.ol.style.Circle({
                            radius: haloRadius,
                            fill: new window.ol.style.Fill({ color: hexToRgba(haloColor, haloFillAlpha) }),
                            stroke: new window.ol.style.Stroke({
                                color: hexToRgba(haloColor, haloStrokeAlpha),
                                width: isSelected ? 2 : 1.2,
                                lineDash: null,
                            }),
                        }),
                    })
                    : null;

                symbolStyles = haloStyle
                    ? [haloStyle, iconStyle]
                    : [iconStyle];
                styleCache.set(key, symbolStyles);
            }

            // The two-line label's centre sits above the glyph, its bottom
            // a few pixels into the glyph's box, which its translucent ground
            // lets through. The message tag is opaque: the block with it ends
            // 2 px above the glyph's box, and the label moves up with it.
            const labelCentreY = -(iconSize / 2 + 8);
            const labelBottomY = labelCentreY + LABEL_LINE_PX + LABEL_PAD_Y;
            const blockBottomY = Math.min(labelBottomY, -(iconSize / 2 + 2));
            const tagTop = blockBottomY - TAG_HEIGHT_PX;
            const lift = voiceTag ? labelBottomY - (tagTop - TAG_GAP_PX) : 0;
            const labelChrome = {
                textAlign: 'left',
                backgroundFill: new window.ol.style.Fill({ color: labelBackgroundColor }),
                backgroundStroke: new window.ol.style.Stroke({ color: labelBorderColor, width: 1 }),
                padding: [LABEL_PAD_Y, LABEL_PAD_X, LABEL_PAD_Y, LABEL_PAD_X],
            };

            const labelMainStyle = labelMode === 'none' || !labelParts.callsignText
                ? null
                : labelMode === 'callsign'
                    // One line, just above the glyph: no phase, no age.
                    ? new window.ol.style.Style({
                        text: new window.ol.style.Text(Object.assign({
                            text: labelParts.callsignText,
                            font: '11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                            offsetY: -(iconSize / 2 + 2 + (LABEL_LINE_PX / 2 + LABEL_PAD_Y)),
                            fill: new window.ol.style.Fill({ color: hexToRgba(labelColor, textOpacity) }),
                        }, labelChrome)),
                    })
                    : new window.ol.style.Style({
                        text: new window.ol.style.Text(Object.assign({
                            text: `${labelParts.phaseText || ''}${labelParts.callsignText}${labelParts.typeText}\n${labelParts.detailsText}`,
                            font: '11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                            offsetY: labelCentreY - lift,
                            fill: new window.ol.style.Fill({ color: hexToRgba(labelColor, textOpacity) }),
                        }, labelChrome)),
                    });

            const phasePrefixStyle = labelMode === 'full' && labelParts.phaseText && labelParts.callsignText
                ? new window.ol.style.Style({
                    text: new window.ol.style.Text({
                        text: labelParts.phaseText,
                        font: '700 11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                        // Drawn over the first line.
                        offsetY: -(iconSize / 2 + 14) - lift,
                        textAlign: 'left',
                        fill: new window.ol.style.Fill({ color: hexToRgba(labelParts.phaseColor || labelTextColor('tertiary'), textOpacity) }),
                    }),
                })
                : null;

            // The message tag, 3 px under the label, left edges aligned; it
            // fades with the label (the relocation fade of the motion engine).
            let voiceTagStyle = null;
            if (voiceTag) {
                const t = tagCanvas(voiceTag);
                voiceTagStyle = new window.ol.style.Style({
                    image: new window.ol.style.Icon({
                        img: t.canvas,
                        scale: 1 / t.ratio,
                        anchor: [LABEL_PAD_X * t.ratio, -tagTop * t.ratio],
                        anchorXUnits: 'pixels',
                        anchorYUnits: 'pixels',
                        opacity: voiceTag.alpha * (deemphasized ? 0.5 : 1) * fade,
                    }),
                });
            }

            const ageTextOffsetX = Math.max(36, estimatedLabelWidthPx - 2);
            const ageTextOffsetY = Math.max(3, Math.round(iconSize * 0.18));

            const labelAgeStyle = labelMode === 'full' && labelParts.ageText
                ? new window.ol.style.Style({
                    text: new window.ol.style.Text({
                        text: labelParts.ageText,
                        font: '700 10px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                        offsetY: ageTextOffsetY,
                        offsetX: ageTextOffsetX,
                        textAlign: 'right',
                        fill: new window.ol.style.Fill({ color: hexToRgba(labelTextColor('tertiary'), (deemphasized ? 0.68 : 0.9) * labelAgeFade) }),
                    }),
                })
                : null;

            // Over the second line: the runway or exit in the secondary text
            // colour, the count of the others in the tertiary (MapLabel, Q57).
            const probableStyles = [];
            if (probable && labelMainStyle && labelMode === 'full') {
                const details = labelLineTwo.slice(0, labelLineTwo.length - probableWords.length);
                const x0 = labelTextWidth(details + '  ');
                const lineTwoY = -(iconSize / 2 + 1) - lift;
                const over = (text, x, role) => new window.ol.style.Style({
                    text: new window.ol.style.Text({
                        text, offsetX: x, offsetY: lineTwoY, textAlign: 'left',
                        font: '11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                        fill: new window.ol.style.Fill({ color: hexToRgba(labelTextColor(role), textOpacity) }),
                    }),
                });
                probableStyles.push(over(probable.main, x0, 'secondary'));
                if (probable.rest) probableStyles.push(over(probable.rest, x0 + labelTextWidth(probable.main), 'tertiary'));
            }

            const textStyles = [];
            if (labelMainStyle) textStyles.push(labelMainStyle);
            if (phasePrefixStyle) textStyles.push(phasePrefixStyle);
            textStyles.push(...probableStyles);
            if (voiceTagStyle) textStyles.push(voiceTagStyle);

            return labelAgeStyle
                ? [...symbolStyles, ...textStyles, labelAgeStyle]
                : [...symbolStyles, ...textStyles];
        }

        function createWebGLLayer() {
            return new window.ol.layer.WebGLPoints({
                source,
                style: {
                    symbol: {
                        symbolType: 'circle',
                        size: 12,
                        // The accent token (the tool's own green), read from the
                        // page; the WebGLPoints path itself is judged at step 13.
                        color: accentColor(),
                        rotateWithView: false,
                        rotation: 0,
                        opacity: 1,
                    },
                },
                disableHitDetection: false,
            });
        }

        function createVectorLayer() {
            return new window.ol.layer.Vector({
                source,
                style: getVectorStyle,
                declutter: false,
                updateWhileAnimating: true,
                updateWhileInteracting: true,
            });
        }

        function isShaderFailureMessage(message) {
            if (!message || typeof message !== 'string') return false;
            const lower = message.toLowerCase();
            return lower.includes('fragment shader compilation failed') ||
                lower.includes('vertex shader compilation failed') ||
                lower.includes('shader compilation failed');
        }

        function isWebGLRuntimeFailureMessage(message) {
            if (!message || typeof message !== 'string') return false;
            const lower = message.toLowerCase();
            return isShaderFailureMessage(message) ||
                lower.includes("cannot read properties of undefined (reading 'ol_uid')") ||
                lower.includes("cannot read properties of undefined (reading 'readpixel')") ||
                lower.includes('pointslayer.js') ||
                lower.includes('getuniformlocation') ||
                lower.includes('prepareframeinternal');
        }

        function markWebGLDisabledForSession() {
            try {
                if (window.sessionStorage) {
                    window.sessionStorage.setItem(WEBGL_DISABLE_SESSION_KEY, '1');
                }
            } catch (error) {
                // ignore storage failures
            }
        }

        function isWebGLDisabledForSession() {
            try {
                return !!(window.sessionStorage && window.sessionStorage.getItem(WEBGL_DISABLE_SESSION_KEY) === '1');
            } catch (error) {
                return false;
            }
        }

        function fallbackToVector(reason) {
            if (!map || backend !== 'webgl') return;

            console.warn('[MapAircraftWebGLRenderer] Falling back to vector renderer:', reason || 'unknown reason');

            if (layer) {
                map.removeLayer(layer);
            }

            layer = createVectorLayer();
            backend = 'vector';
            stats.backend = backend;
            markWebGLDisabledForSession();
            layer.setZIndex(zIndex);
            map.addLayer(layer);
            layer.changed();
        }

        function attachRuntimeWebGLErrorGuard() {
            runtimeErrorHandler = (event) => {
                const message = event && event.message ? event.message : '';
                if (isWebGLRuntimeFailureMessage(message)) {
                    fallbackToVector(message);
                }
            };

            runtimeRejectionHandler = (event) => {
                const reason = event && event.reason ? event.reason : null;
                const message = typeof reason === 'string'
                    ? reason
                    : (reason && reason.message ? reason.message : '');
                if (isWebGLRuntimeFailureMessage(message)) {
                    fallbackToVector(message);
                }
            };

            window.addEventListener('error', runtimeErrorHandler);
            window.addEventListener('unhandledrejection', runtimeRejectionHandler);
        }

        function detachRuntimeWebGLErrorGuard() {
            if (runtimeErrorHandler) {
                window.removeEventListener('error', runtimeErrorHandler);
                runtimeErrorHandler = null;
            }
            if (runtimeRejectionHandler) {
                window.removeEventListener('unhandledrejection', runtimeRejectionHandler);
                runtimeRejectionHandler = null;
            }
        }

        function init() {
            if (!map) return;

            iconColorizationListener = () => {
                styleCache.clear();
                if (layer) {
                    layer.changed();
                }
            };
            addAircraftIconColorizationListener(iconColorizationListener);

            if (isWebGLDisabledForSession()) {
                layer = createVectorLayer();
                backend = 'vector';
            } else if (preferWebGL && hasWebGLPointsSupport()) {
                try {
                    layer = createWebGLLayer();
                    backend = 'webgl';
                    attachRuntimeWebGLErrorGuard();
                } catch (error) {
                    console.warn('[MapAircraftWebGLRenderer] WebGLPoints init failed, falling back to vector layer:', error);
                    layer = createVectorLayer();
                    backend = 'vector';
                    markWebGLDisabledForSession();
                }
            } else {
                layer = createVectorLayer();
                backend = 'vector';
            }

            stats.backend = backend;
            layer.setZIndex(zIndex);
            map.addLayer(layer);
        }

        function dispose() {
            if (!map || !layer) return;
            if (iconColorizationListener) {
                removeAircraftIconColorizationListener(iconColorizationListener);
                iconColorizationListener = null;
            }
            detachRuntimeWebGLErrorGuard();
            map.removeLayer(layer);
            layer = null;
            featureIndex.clear();
            styleCache.clear();
            source.clear();
        }

        function createFeatureProps(aircraft, visualState) {
            const headingDeg = getAircraftHeading(aircraft, store);
            const headingRad = Number.isFinite(headingDeg) ? (headingDeg * Math.PI) / 180 : null;
            const phase = getAircraftPhase(aircraft);
            const color = getAircraftSymbolColor(aircraft, visualState, store);
            const iconClass = getAircraftIconClass(aircraft);
            const iconSrc = getAircraftAssetPath(iconClass);
            const iconSize = getAircraftIconSize(iconClass);

            return {
                hex: aircraft.hex,
                lat: aircraft.adsb.lat,
                lon: aircraft.adsb.lon,
                heading: headingDeg,
                rotationRad: headingRad,
                on_ground: !!aircraft.on_ground,
                status: aircraft.status || 'active',
                phase,
                selected: !!visualState.selected,
                hovered: !!visualState.hovered,
                proximity: !!visualState.proximity,
                deemphasized: !!visualState.deemphasized,
                visible: true,
                color,
                iconClass,
                iconSrc,
                iconSize,
            };
        }

        function upsertAircraftFeature(aircraft, visualState, options = {}) {
            if (!aircraft || !aircraft.hex || !aircraft.adsb) return;
            const lat = aircraft.adsb.lat;
            const lon = aircraft.adsb.lon;
            const hasPosition = typeof lat === 'number' && typeof lon === 'number' && (lat !== 0 || lon !== 0);
            if (!hasPosition) return;

            const startedAt = performance.now();

            let feature = featureIndex.get(aircraft.hex);
            const geometry = new window.ol.geom.Point(window.ol.proj.fromLonLat([lon, lat]));
            const props = createFeatureProps(aircraft, visualState || { selected: false, hovered: false });
            const preservePose = options?.preservePose === true;

            // No heading in this update (a lost aircraft's expired track): the
            // glyph keeps the way it faced, which drew it at 90° to its own
            // track once it pointed north instead (owner, 15/10).
            if (!Number.isFinite(props.heading)) {
                const keptHeading = feature ? Number(feature.get('heading')) : NaN;
                const keptRotation = feature ? Number(feature.get('rotationRad')) : NaN;
                props.heading = Number.isFinite(keptHeading) ? keptHeading : 0;
                props.rotationRad = Number.isFinite(keptRotation) ? keptRotation : 0;
            }

            if (!feature) {
                feature = new window.ol.Feature({ geometry });
                feature.setProperties(props);
                source.addFeature(feature);
                featureIndex.set(aircraft.hex, feature);
            } else {
                if (!preservePose) {
                    feature.setGeometry(geometry);
                } else {
                    const currentHeading = Number(feature.get('heading'));
                    const currentRotationRad = Number(feature.get('rotationRad'));
                    if (Number.isFinite(currentHeading)) {
                        props.heading = currentHeading;
                    }
                    if (Number.isFinite(currentRotationRad)) {
                        props.rotationRad = currentRotationRad;
                    }
                }
                feature.setProperties(props);
                feature.changed();
            }

            stats.upserts++;
            stats.updateDurationMs += (performance.now() - startedAt);
        }

        function removeAircraftFeature(hex) {
            if (!hex) return;
            const feature = featureIndex.get(hex);
            if (!feature) return;
            source.removeFeature(feature);
            featureIndex.delete(hex);
            stats.removes++;
        }

        // options.preservePose: an aircraft already drawn keeps the pose the
        // animation engine gave it. Set from the raw position, an aircraft
        // whose position the receiver lost 10 to 40 s ago was drawn back
        // there at each full sync -- the transcriptions trigger them -- and
        // forward again the next frame (owner, 16/10: "the third family of
        // jumps", 580 visible jumps in 5 min with the audio streams on).
        function bulkSyncAircraft(aircraftMap, shouldRender, visualResolver, options = {}) {
            const activeHexes = new Set();
            const collection = aircraftMap || {};

            Object.keys(collection).forEach((hex) => {
                const aircraft = collection[hex];
                if (!aircraft || !aircraft.hex) return;

                const visible = typeof shouldRender === 'function' ? !!shouldRender(aircraft) : true;
                if (!visible) {
                    removeAircraftFeature(aircraft.hex);
                    return;
                }

                const visualState = typeof visualResolver === 'function'
                    ? (visualResolver(aircraft.hex) || { selected: false, hovered: false })
                    : { selected: false, hovered: false };

                upsertAircraftFeature(aircraft, visualState, { preservePose: options.preservePose === true });
                activeHexes.add(aircraft.hex);
            });

            Array.from(featureIndex.keys()).forEach((hex) => {
                if (!activeHexes.has(hex)) {
                    removeAircraftFeature(hex);
                }
            });

            stats.bulkSyncs++;
        }

        function setVisualState(selectedHex, hoveredHex) {
            featureIndex.forEach((feature, hex) => {
                const selected = !!selectedHex && selectedHex === hex;
                const hovered = !!hoveredHex && hoveredHex === hex;
                const proximity = !!(store?.proximityHighlightedAircraft && store.proximityHighlightedAircraft.has(hex));
                const deemphasized = !!selectedHex && !selected;
                feature.set('selected', selected);
                feature.set('hovered', hovered);
                feature.set('proximity', proximity);
                feature.set('deemphasized', deemphasized);

                const aircraft = store?.aircraft?.[hex] || null;
                feature.set('color', getAircraftSymbolColor(aircraft, { selected, hovered, proximity }, store));
                if (aircraft) feature.set('status', aircraft.status || 'active');
                feature.changed();
            });

            if (layer) {
                layer.changed();
            }
        }

        function refreshAircraftStyle(hex, selectedHex, hoveredHex) {
            const feature = featureIndex.get(hex);
            if (!feature) return;

            const selected = !!selectedHex && selectedHex === hex;
            const hovered = !!hoveredHex && hoveredHex === hex;
            const proximity = !!(store?.proximityHighlightedAircraft && store.proximityHighlightedAircraft.has(hex));
            const deemphasized = !!selectedHex && !selected;
            feature.set('selected', selected);
            feature.set('hovered', hovered);
            feature.set('proximity', proximity);
            feature.set('deemphasized', deemphasized);

            const aircraft = store?.aircraft?.[hex] || null;
            feature.set('color', getAircraftSymbolColor(aircraft, { selected, hovered, proximity }, store));
            if (aircraft) feature.set('status', aircraft.status || 'active');
            feature.changed();

            if (layer) {
                layer.changed();
            }
        }

        function getFeature(hex) {
            return featureIndex.get(hex) || null;
        }

        function getFeatureHexes() {
            return new Set(featureIndex.keys());
        }

        function getLayer() {
            return layer;
        }

        function getStats() {
            const avgUpdateMs = stats.upserts > 0 ? stats.updateDurationMs / stats.upserts : 0;
            return {
                backend: stats.backend,
                featureCount: featureIndex.size,
                upserts: stats.upserts,
                removes: stats.removes,
                bulkSyncs: stats.bulkSyncs,
                avgUpdateMs: Number(avgUpdateMs.toFixed(3)),
            };
        }

        function requestRedraw() {
            if (layer) {
                layer.changed();
            }
        }

        return {
            init,
            dispose,
            upsertAircraftFeature,
            removeAircraftFeature,
            bulkSyncAircraft,
            setVisualState,
            refreshAircraftStyle,
            requestRedraw,
            getFeature,
            getFeatureHexes,
            getLayer,
            getStats,
        };
    }

    window.MapAircraftWebGLRenderer = {
        createAircraftWebGLRenderer,
        // An aircraft's altitude colour, for its trail (openlayers-map-manager.js).
        altitudeColor: getAltitudeBandColor,
    };
})();
