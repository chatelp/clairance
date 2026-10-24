/**
 * Module: map/openlayers-map-manager
 * Why it exists:
 * - Serves as the central orchestration layer for all OpenLayers-backed map behavior.
 * - Coordinates data-driven rendering, interaction wiring, and map-specific UI state.
 *
 * Key responsibilities:
 * - Own aircraft, trail, overlay, reference, and effect feature lifecycle.
 * - Integrate map engine, renderers, interactions, and feature modules.
 * - Apply filtering/visibility decisions and incremental visual updates.
 * - Manage special map workflows (proximity selection, mini-map, popup overlays).
 *
 * Quirks / contracts:
 * - This class intentionally holds broad cross-cutting state to minimize transient
 *   object churn in hot update paths.
 * - Uses defensive checks around optional feature modules to preserve graceful
 *   degradation when subsets are unavailable.
 */
(function () {
    // ------------------------------------------------------------ airport layout
    //
    // Runways, taxiways, aprons and terminals around the station, from
    // OpenStreetMap, written per station by tools/airport-layout/fetch.py. Drawn
    // to read on a dark base map as well as a light one. Runways and taxiways
    // take the map's tokens (map-palette.js); aprons and terminals stay faint
    // neutral fills.
    const AIRPORT_LAYOUT_URL = '/data/airport-layout.geojson';
    const AIRPORT_LAYOUT_COLORS = {
        apron: 'rgba(190, 190, 190, 0.16)',
        terminal: 'rgba(170, 182, 214, 0.30)',
    };
    const airportLayoutStyleCache = new Map();
    const featureCosLat = new WeakMap();

    function zoomForResolution(resolution) {
        return Math.log2(156543.03392804097 / resolution);
    }

    // A Web Mercator pixel spans resolution * cos(latitude) metres on the ground.
    function groundMetresPerPixel(feature, resolution) {
        let cosLat = featureCosLat.get(feature);
        if (cosLat === undefined) {
            const extent = feature.getGeometry().getExtent();
            const lat = window.ol.proj.toLonLat([0, (extent[1] + extent[3]) / 2])[1];
            cosLat = Math.cos(lat * Math.PI / 180);
            featureCosLat.set(feature, cosLat);
        }
        return resolution * cosLat;
    }

    function cachedStyle(key, create) {
        let style = airportLayoutStyleCache.get(key);
        if (!style) {
            style = create();
            airportLayoutStyleCache.set(key, style);
        }
        return style;
    }

    // Runways at their real width (the width tag, 45 m without it), 2 px at least,
    // with their designation from z13; taxiways from z12; aprons and terminals as
    // faint fills; nothing but runways below z9.
    function airportLayoutStyle(feature, resolution) {
        const ol = window.ol;
        const kind = feature.get('aeroway');
        const zoom = zoomForResolution(resolution);
        if (kind !== 'runway' && zoom < 9) return null;
        const type = feature.getGeometry().getType();
        const isArea = type === 'Polygon' || type === 'MultiPolygon';
        const P = window.MapPalette;
        const base = P.isBright() ? 'light' : 'dark';

        if (kind === 'runway') {
            const label = zoom >= 13 ? (feature.get('ref') || '') : '';
            const text = label ? new ol.style.Text({
                text: label,
                font: 'bold 11px sans-serif',
                fill: new ol.style.Fill({ color: P.color('text-primary') }),
                stroke: new ol.style.Stroke({ color: '#000000', width: 3 }),
                overflow: true,
            }) : undefined;
            if (isArea) {
                return cachedStyle(`runway-area|${base}|${label}`, () => new ol.style.Style({
                    fill: new ol.style.Fill({ color: P.color('runway') }),
                    text,
                }));
            }
            const widthMetres = parseFloat(feature.get('width')) || 45;
            const widthPx = Math.max(2, widthMetres / groundMetresPerPixel(feature, resolution));
            return cachedStyle(`runway|${base}|${widthPx.toFixed(1)}|${label}`, () => new ol.style.Style({
                stroke: new ol.style.Stroke({ color: P.color('runway'), width: widthPx, lineCap: 'butt' }),
                text,
            }));
        }

        if (kind === 'taxiway') {
            if (zoom < 12) return null;
            return cachedStyle(`taxiway|${base}|${isArea}`, () => new ol.style.Style(isArea
                ? { fill: new ol.style.Fill({ color: P.withAlpha(P.color('taxiway'), 0.25) }) }
                : { stroke: new ol.style.Stroke({ color: P.color('taxiway'), width: 1 }) }));
        }

        const color = kind === 'terminal' ? AIRPORT_LAYOUT_COLORS.terminal : AIRPORT_LAYOUT_COLORS.apron;
        return cachedStyle(`${kind}|${isArea}`, () => new ol.style.Style(isArea
            ? { fill: new ol.style.Fill({ color }) }
            : { stroke: new ol.style.Stroke({ color, width: 1 }) }));
    }

    class OpenLayersMapManager {
        constructor(store, CONFIG) {
            this.store = store;
            this.CONFIG = CONFIG;

            this.engine = null;
            this.map = null;
            this.layers = {};
            this.markers = {};
            this.trails = {};

            this.proximityCircle = null;
            this.proximityHexSet = null;
            this.proximityRefHex = null;
            this.proximityDistanceNM = null;

            this._warnings = new Set();
            this._stats = {
                initCalls: 0,
                refreshCalls: 0,
                singleAircraftUpdates: 0,
                singleAircraftUpdateDurationMs: 0,
            };

            this._olMap = null;
            this._aircraftRenderer = null;
            this._trailSource = null;
            this._trailLayer = null;
            this._trailFeaturesByHex = new Map();
            this._overlaySource = null;
            this._overlayLayer = null;
            this._positionHighlightFeature = null;
            this._proximityCircleFeature = null;
            this._effectFeatures = new Set();
            this._labelRefreshInterval = null;
            this._referenceLayerState = {};
            this._rangeRingLayerState = null;
            this._referenceRenderScheduled = false;
            this.runwayData = null;
            this.airportsData = null;
            this.heliportsData = null;
            this.navaidsData = null;
            this.allRunwaysData = null;
            this.tracksMiniMap = null;
            this.tracksMiniMapLayers = null;
            this._overlayRegistry = null;
            this._overlayStatuses = [];
            this._referencePopupOverlay = null;
            this._referencePopupElement = null;

            this._interactionCleanup = null;
        }

        _toMapCoordinate(lat, lon) {
            return window.ol.proj.fromLonLat([lon, lat]);
        }

        _ensureTrailLayer() {
            if (this._trailLayer || !this._olMap) return;

            this._trailSource = new window.ol.source.Vector();
            this._trailLayer = new window.ol.layer.Vector({
                source: this._trailSource,
                updateWhileAnimating: true,
                updateWhileInteracting: true,
            });
            this._trailLayer.setZIndex(300);
            this._olMap.addLayer(this._trailLayer);
        }

        _ensureOverlayLayer() {
            if (this._overlayLayer || !this._olMap) return;

            this._overlaySource = new window.ol.source.Vector();
            this._overlayLayer = new window.ol.layer.Vector({
                source: this._overlaySource,
                updateWhileAnimating: true,
                updateWhileInteracting: true,
            });
            this._overlayLayer.setZIndex(500);
            this._olMap.addLayer(this._overlayLayer);
        }

        _ensureReferenceLayers() {
            if (!this._olMap) return;
            if (Object.keys(this._referenceLayerState).length > 0 && this._rangeRingLayerState) return;

            const createLayerState = (zIndex, defaultVisible = true) => {
                const source = new window.ol.source.Vector();
                const layer = new window.ol.layer.Vector({ source, updateWhileAnimating: false, updateWhileInteracting: false });
                layer.setZIndex(zIndex);
                layer.setVisible(defaultVisible);
                this._olMap.addLayer(layer);
                return { source, layer };
            };

            this._referenceLayerState = {
                runways: createLayerState(120, true),
                airports: createLayerState(125, this.store?.settings?.showAirports !== false),
                heliports: createLayerState(126, this.store?.settings?.showHeliports !== false),
                navaids: createLayerState(127, this.store?.settings?.showNavaids !== false),
                allRunways: createLayerState(115, this.store?.settings?.showAllRunways === true),
                // Above the base map, under everything else (trails 300, aircraft 400).
                airportLayout: createLayerState(100, this.store?.settings?.showAirportLayout !== false),
            };
            this._referenceLayerState.airportLayout.layer.setStyle(airportLayoutStyle);
            this._referenceLayerState.airportLayout.source.setAttributions('&copy; OpenStreetMap contributors (ODbL)');

            this._rangeRingLayerState = createLayerState(110, this.store?.settings?.showRings !== false);
        }

        // The file is generated per station and may be absent: the layer then
        // stays empty, and nothing is reported.
        _loadAirportLayout() {
            const state = this._referenceLayerState.airportLayout;
            if (!state || this._airportLayoutRequested) return;
            this._airportLayoutRequested = true;
            fetch(AIRPORT_LAYOUT_URL, { credentials: 'same-origin' })
                .then((response) => (response.ok ? response.json() : null))
                .then((geojson) => {
                    if (!geojson || !Array.isArray(geojson.features)) return;
                    const features = new window.ol.format.GeoJSON().readFeatures(geojson, { featureProjection: 'EPSG:3857' });
                    state.source.addFeatures(features);
                })
                .catch(() => {});
        }

        _clearReferenceLayer(layerName) {
            const state = this._referenceLayerState[layerName];
            if (!state || !state.source) return;
            state.source.clear();
        }

        _isLayerVisible(layerName) {
            const state = this._referenceLayerState[layerName];
            if (!state || !state.layer) return false;
            return state.layer.getVisible();
        }

        _isRangeRingVisible() {
            if (!this._rangeRingLayerState || !this._rangeRingLayerState.layer) return false;
            return this._rangeRingLayerState.layer.getVisible();
        }

        _scheduleReferenceRender() {
            if (this._referenceRenderScheduled) return;
            this._referenceRenderScheduled = true;
            window.requestAnimationFrame(() => {
                this._referenceRenderScheduled = false;
                this._renderReferenceLayersForCurrentView();
            });
        }

        _getDefaultOverlayConfigs() {
            return [
                {
                    id: 'aviation-chart',
                    type: 'overlay',
                    sourceType: 'xyz',
                    url: this.CONFIG?.aviationChartOverlayUrl || '',
                    zIndex: 640,
                    defaultVisible: false,
                    minZoom: 6,
                    maxZoom: 18,
                    opacity: 0.65,
                    refreshPolicy: { type: 'none' },
                    attribution: this.CONFIG?.aviationChartOverlayAttribution || '',
                    licenseNotes: 'Disabled by default; configure approved source URL before use.',
                    failureIsolation: { disableOnError: true, maxErrors: 2, retryMaxAttempts: 3, retryBaseMs: 2000 },
                },
                {
                    id: 'airspace-polygons',
                    type: 'overlay',
                    sourceType: 'xyz',
                    url: this.CONFIG?.airspaceOverlayTmsUrl || 'https://map.adsbexchange.com/mapproxy/tiles/1.0.0/openaip/ul_grid/{z}/{x}/{y}.png',
                    zIndex: 630,
                    defaultVisible: false,
                    minZoom: 0,
                    maxZoom: 13,
                    opacity: 0.5,
                    refreshPolicy: { type: 'none' },
                    attribution: this.CONFIG?.airspaceOverlayAttribution || 'openAIP.net',
                    licenseNotes: 'Disabled by default; configure approved OpenAIP TMS source before use.',
                    failureIsolation: { disableOnError: true, maxErrors: 2, retryMaxAttempts: 2, retryBaseMs: 3000 },
                },
            ];
        }

        _initOverlayRegistry() {
            if (!this._olMap || !window.MapLayerRegistry || typeof window.MapLayerRegistry.createOpenLayersOverlayRegistry !== 'function') {
                return;
            }

            const configured = Array.isArray(this.CONFIG?.mapOverlays) && this.CONFIG.mapOverlays.length > 0
                ? this.CONFIG.mapOverlays
                : this._getDefaultOverlayConfigs();

            const validConfigs = configured.filter((config) => {
                if (!config || !config.id || !config.sourceType) return false;
                const sourceType = String(config.sourceType).toLowerCase();
                if ((sourceType === 'xyz' || sourceType === 'wms' || sourceType === 'image-wms' || sourceType === 'geojson') && !config.url) {
                    return false;
                }
                return true;
            });

            this._overlayRegistry = window.MapLayerRegistry.createOpenLayersOverlayRegistry(this._olMap, validConfigs);
            if (this._overlayRegistry && typeof this._overlayRegistry.registerAll === 'function') {
                this._overlayStatuses = this._overlayRegistry.registerAll();
                this._overlayRegistry.updateZoomVisibility(this.getZoom());
            }
        }

        _renderReferenceLayersForCurrentView() {
            if (!this._olMap) return;
            this._renderRunways();
            this._renderAirports();
            this._renderHeliports();
            this._renderNavaids();
            this._renderAllRunways();
            this._renderRangeRings();
        }

        // The sheet of an airport, a heliport or a navaid (design system, a
        // KeyValues card; migration step 9b, a-calques): a cl-card over the map,
        // closed by its ×, by Escape or by a click beside it.
        _ensureReferencePopup() {
            if (!this._olMap || this._referencePopupOverlay) return;

            const container = document.createElement('div');
            container.className = 'cl-root cl-card';
            container.style.cssText = 'display: none; width: 280px; max-height: 60vh; overflow-y: auto;';
            container.setAttribute('role', 'dialog');
            container.addEventListener('click', (event) => {
                if (event.target.closest('[data-ref-close]')) this._closeReferencePopup();
            });
            document.addEventListener('keydown', (event) => {
                if (event.key === 'Escape' && container.style.display !== 'none') this._closeReferencePopup();
            });

            const overlay = new window.ol.Overlay({
                element: container,
                positioning: 'bottom-center',
                offset: [0, -12],
                autoPan: { animation: { duration: 150 } },
                stopEvent: true,
            });

            this._referencePopupOverlay = overlay;
            this._referencePopupElement = container;
            this._olMap.addOverlay(overlay);
        }

        _closeReferencePopup() {
            if (!this._referencePopupOverlay) return;
            this._referencePopupOverlay.setPosition(undefined);
            const element = this._referencePopupOverlay.getElement();
            // The focus inside goes back to the map (step 11, AppShell).
            const hadFocus = element && element.contains(document.activeElement);
            if (element) {
                element.style.display = 'none';
            }
            if (hadFocus) {
                const map = document.getElementById('map');
                if (map) map.focus();
            }
            if (this._referencePopupElement) {
                this._referencePopupElement.innerHTML = '';
            }
        }

        _openReferencePopup(coordinate, html) {
            this._ensureReferencePopup();
            if (!this._referencePopupOverlay || !this._referencePopupElement) return;

            this._referencePopupElement.innerHTML = html || '';
            const element = this._referencePopupOverlay.getElement();
            if (element) {
                element.style.display = 'flex';
                const title = element.querySelector('.cl-menu__title');
                element.setAttribute('aria-label', title ? title.textContent : 'Sheet');
                // A popover takes the focus on its first control (step 11).
                const close = element.querySelector('[data-ref-close]');
                if (close) setTimeout(() => close.focus(), 0);
            }
            this._referencePopupOverlay.setPosition(coordinate);
        }

        // The sheet's head: the ident in mono, the name, and ×.
        _sheetHead(ident, name) {
            const e = this._escapeSheet;
            return `<div class="cl-row"><span class="cl-menu__title cl-mono">${e(ident)}</span>` +
                (name ? `<span class="cl-menu__title" style="min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${e(name)}</span>` : '') +
                `<button class="cl-btn cl-btn--icon" type="button" aria-label="Close" data-ref-close style="margin-left: auto;">×</button></div>`;
        }

        _escapeSheet(value) {
            return String(value ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
        }

        // KeyValues: the rows with a value, "—" for none.
        _sheetKv(rows) {
            const e = this._escapeSheet;
            return '<dl class="cl-kv">' + rows.map(([k, v]) => {
                const none = v === undefined || v === null || v === '';
                return `<dt>${e(k)}</dt><dd${none ? ' class="is-none"' : ''}>${none ? '—' : e(v)}</dd>`;
            }).join('') + '</dl>';
        }

        // "371 ft", "1 220 ft": the brand's numbers.
        _sheetFeet(ft) {
            const n = Number(ft);
            return Number.isFinite(n) ? String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ' ') + ' ft' : '';
        }

        // The frequencies; the Desc column only when one of them has one.
        _sheetFrequencies(list) {
            const e = this._escapeSheet;
            const desc = (list || []).some((f) => String(f?.description || '').trim());
            const rows = (list || []).map((f) => {
                const mhz = Number(f?.frequency_mhz);
                return `<tr><td>${e(f?.type || '')}</td>` + (desc ? `<td style="white-space: normal;">${e(f?.description || '')}</td>` : '') +
                    `<td class="is-num">${Number.isFinite(mhz) && mhz > 0 ? mhz.toFixed(3) : e(f?.frequency_mhz || '')}</td></tr>`;
            }).join('');
            return rows
                ? `<table class="cl-table"><thead><tr><th>Type</th>${desc ? '<th>Desc</th>' : ''}<th class="is-num">MHz</th></tr></thead><tbody>${rows}</tbody></table>`
                : '';
        }

        // The name as the column says it ("Saint-Cyr-l'École", not "… Airport").
        _sheetName(place) {
            const R = window.RightNow;
            return R && typeof R.shortName === 'function' ? R.shortName({ name: place?.name, code: place?.ident }) : (place?.name || '');
        }

        // "BOTH" -> "Both": the base's capitals, for a word that is not an acronym.
        _sheetWord(value) {
            const t = String(value || '').trim();
            return t && t === t.toUpperCase() ? t.charAt(0) + t.slice(1).toLowerCase() : t;
        }

        _airportPopupHtml(airport) {
            const town = [airport?.municipality, airport?.iso_country].filter(Boolean).join(', ');
            return this._sheetHead(airport?.ident || '', this._sheetName(airport)) +
                this._sheetKv([
                    ['Type', (airport?.type || '').replace(/_/g, ' ')],
                    ['Elevation', this._sheetFeet(airport?.elevation_ft)],
                    ...(town ? [['Town', town]] : []),
                    ...(airport?.iata_code ? [['IATA', airport.iata_code]] : []),
                ]) +
                this._sheetFrequencies(airport?.frequencies);
        }

        _navaidPopupHtml(navaid) {
            const khz = Number(navaid?.frequency_khz);
            let frequency = '';
            if (Number.isFinite(khz) && khz > 0) {
                frequency = khz >= 100000 ? `${(khz / 1000).toFixed(2)} MHz` : `${khz} kHz`;
            }
            return this._sheetHead(navaid?.ident || '', navaid?.name || '') +
                this._sheetKv([
                    ['Type', navaid?.type || ''],
                    ['Frequency', frequency],
                    ...(navaid?.elevation_ft ? [['Elevation', this._sheetFeet(navaid.elevation_ft)]] : []),
                    ...(navaid?.associated_airport ? [['Airport', navaid.associated_airport]] : []),
                    ...(navaid?.usage_type ? [['Usage', this._sheetWord(navaid.usage_type)]] : []),
                    ...(navaid?.power ? [['Power', this._sheetWord(navaid.power)]] : []),
                ]);
        }

        _heliportPopupHtml(heliport) {
            const town = [heliport?.municipality, heliport?.iso_country].filter(Boolean).join(', ');
            return this._sheetHead(heliport?.ident || '', this._sheetName(heliport)) +
                this._sheetKv([
                    ['Type', 'heliport'],
                    ['Elevation', this._sheetFeet(heliport?.elevation_ft)],
                    ...(town ? [['Town', town]] : []),
                ]) +
                this._sheetFrequencies(heliport?.frequencies);
        }

        _navaidSvgMarkup(type, size = 18) {
            const s = size;
            const h = s / 2;
            const color = this._navaidColor(type);

            const hex = (radius) => {
                const points = [];
                for (let i = 0; i < 6; i++) {
                    const angle = Math.PI / 6 + i * Math.PI / 3;
                    points.push(`${(h + radius * Math.cos(angle)).toFixed(1)},${(h - radius * Math.sin(angle)).toFixed(1)}`);
                }
                return points.join(' ');
            };

            switch (type) {
                case 'VOR':
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}"><polygon points="${hex(h * 0.8)}" fill="none" stroke="${color}" stroke-width="1.5"/></svg>`;
                case 'VOR-DME':
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}"><polygon points="${hex(h * 0.8)}" fill="none" stroke="${color}" stroke-width="1.5"/><circle cx="${h}" cy="${h}" r="2" fill="${color}"/></svg>`;
                case 'VORTAC': {
                    const spokes = [90, 210, 330].map((angle) => {
                        const radians = angle * Math.PI / 180;
                        return `<line x1="${(h + h * 0.72 * Math.cos(radians)).toFixed(1)}" y1="${(h - h * 0.72 * Math.sin(radians)).toFixed(1)}" x2="${(h + h * 0.95 * Math.cos(radians)).toFixed(1)}" y2="${(h - h * 0.95 * Math.sin(radians)).toFixed(1)}" stroke="${color}" stroke-width="2"/>`;
                    }).join('');
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}"><polygon points="${hex(h * 0.7)}" fill="none" stroke="${color}" stroke-width="1.5"/>${spokes}</svg>`;
                }
                case 'NDB': {
                    const dots = [0, 60, 120, 180, 240, 300].map((angle) => {
                        const radians = angle * Math.PI / 180;
                        return `<circle cx="${(h + h * 0.7 * Math.cos(radians)).toFixed(1)}" cy="${(h - h * 0.7 * Math.sin(radians)).toFixed(1)}" r="1" fill="${color}" opacity="0.6"/>`;
                    }).join('');
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}"><circle cx="${h}" cy="${h}" r="3" fill="${color}"/>${dots}</svg>`;
                }
                case 'NDB-DME': {
                    const dots = [0, 60, 120, 180, 240, 300].map((angle) => {
                        const radians = angle * Math.PI / 180;
                        return `<circle cx="${(h + h * 0.7 * Math.cos(radians)).toFixed(1)}" cy="${(h - h * 0.7 * Math.sin(radians)).toFixed(1)}" r="1" fill="${color}" opacity="0.6"/>`;
                    }).join('');
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}"><circle cx="${h}" cy="${h}" r="3" fill="${color}"/>${dots}<rect x="${h - 2}" y="${h - 2}" width="4" height="4" fill="none" stroke="${color}" stroke-width="0.8"/></svg>`;
                }
                case 'DME':
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}"><rect x="${h - h * 0.5}" y="${h - h * 0.5}" width="${h}" height="${h}" fill="none" stroke="${color}" stroke-width="1.5"/></svg>`;
                case 'TACAN': {
                    const spokes = [90, 210, 330].map((angle) => {
                        const radians = angle * Math.PI / 180;
                        return `<line x1="${(h + h * 0.2 * Math.cos(radians)).toFixed(1)}" y1="${(h - h * 0.2 * Math.sin(radians)).toFixed(1)}" x2="${(h + h * 0.8 * Math.cos(radians)).toFixed(1)}" y2="${(h - h * 0.8 * Math.sin(radians)).toFixed(1)}" stroke="${color}" stroke-width="2"/>`;
                    }).join('');
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}">${spokes}<circle cx="${h}" cy="${h}" r="2" fill="${color}"/></svg>`;
                }
                default:
                    return `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}" viewBox="0 0 ${s} ${s}"><circle cx="${h}" cy="${h}" r="4" fill="none" stroke="${color}" stroke-width="1.5"/></svg>`;
            }
        }

        _navaidIconStyle(type, size = 18) {
            const svg = this._navaidSvgMarkup(type, size);
            const url = `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
            return new window.ol.style.Icon({
                src: url,
                imgSize: [size, size],
                anchor: [size / 2, size / 2],
                anchorXUnits: 'pixels',
                anchorYUnits: 'pixels',
                rotateWithView: false,
            });
        }

        _handleReferenceFeatureClick(event) {
            if (!this._olMap || !event) return false;

            let clickedFeature = null;
            this._olMap.forEachFeatureAtPixel(event.pixel, (feature, candidateLayer) => {
                const refType = feature?.get ? feature.get('refType') : null;
                if (!refType) return false;

                const isReferenceLayer = Object.values(this._referenceLayerState).some((entry) => entry?.layer === candidateLayer)
                    || candidateLayer === this._rangeRingLayerState?.layer;
                if (!isReferenceLayer) return false;

                clickedFeature = feature;
                return true;
            }, { hitTolerance: 6 });

            if (!clickedFeature) {
                this._closeReferencePopup();
                return false;
            }

            const refType = clickedFeature.get('refType');
            const refData = clickedFeature.get('refData');
            if (!refType || !refData) {
                this._closeReferencePopup();
                return false;
            }

            let html = '';
            if (refType === 'airport') {
                html = this._airportPopupHtml(refData);
            } else if (refType === 'navaid') {
                html = this._navaidPopupHtml(refData);
            } else if (refType === 'heliport') {
                html = this._heliportPopupHtml(refData);
            }

            if (!html) {
                this._closeReferencePopup();
                return false;
            }

            this._openReferencePopup(event.coordinate, html);
            return true;
        }

        _getReferenceLabelStyle(text, color = window.MapPalette.color('text-tertiary')) {
            return new window.ol.style.Text({
                text: text || '',
                font: '10px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                offsetY: -10,
                fill: new window.ol.style.Fill({ color }),
                backgroundFill: new window.ol.style.Fill({ color: 'rgba(0,0,0,0.45)' }),
                padding: [1, 3, 1, 3],
            });
        }

        _renderRunways() {
            this._clearReferenceLayer('runways');
            const state = this._referenceLayerState.runways;
            if (!state || !state.source || !this.runwayData || !this._isLayerVisible('runways')) return;

            const zoom = this.getZoom();
            if (zoom < 9) return;
            const showLabels = zoom >= 10;
            const showExtensions = zoom >= 11;
            const lineWeight = Math.max(4, Math.min(8, zoom - 6));

            // One airport, or every airport followed (docs-fr/05-decisions.md, Q40).
            const sets = Array.isArray(this.runwayData) ? this.runwayData : [this.runwayData];
            sets.forEach((runwayData) => {
            const thresholdsMap = (runwayData && runwayData.runway_thresholds) || {};
            const extensionsMap = (runwayData && runwayData.runway_extensions) || {};

            Object.keys(thresholdsMap).forEach((runwayId) => {
                const thresholds = thresholdsMap[runwayId];
                const ends = Object.keys(thresholds || {});
                if (ends.length !== 2) return;

                const endA = thresholds[ends[0]];
                const endB = thresholds[ends[1]];
                if (!endA || !endB) return;

                const line = new window.ol.Feature({
                    geometry: new window.ol.geom.LineString([
                        this._toMapCoordinate(endA.latitude, endA.longitude),
                        this._toMapCoordinate(endB.latitude, endB.longitude),
                    ]),
                });
                line.setStyle(new window.ol.style.Style({
                    stroke: new window.ol.style.Stroke({ color: window.MapPalette.color('runway'), width: lineWeight }),
                }));
                state.source.addFeature(line);

                if (showLabels) {
                    [ends[0], ends[1]].forEach((endName) => {
                        const end = thresholds[endName];
                        if (!end) return;
                        const label = new window.ol.Feature({
                            geometry: new window.ol.geom.Point(this._toMapCoordinate(end.latitude, end.longitude)),
                        });
                        label.setStyle(new window.ol.style.Style({ text: this._getReferenceLabelStyle(endName, window.MapPalette.color('text-primary')) }));
                        state.source.addFeature(label);
                    });
                }

                if (showExtensions) {
                    const extensionSet = extensionsMap[runwayId] || {};
                    Object.keys(extensionSet).forEach((endName) => {
                        const points = extensionSet[endName];
                        if (!Array.isArray(points) || points.length < 2) return;
                        const lineExt = new window.ol.Feature({
                            geometry: new window.ol.geom.LineString(
                                points
                                    .filter((point) => typeof point?.latitude === 'number' && typeof point?.longitude === 'number')
                                    .map((point) => this._toMapCoordinate(point.latitude, point.longitude))
                            ),
                        });
                        lineExt.setStyle(new window.ol.style.Style({
                            stroke: new window.ol.style.Stroke({ color: window.MapPalette.color('centerline'), width: 1.5, lineDash: [8, 12] }),
                        }));
                        state.source.addFeature(lineExt);
                    });
                }
            });
            });
        }

        _renderAirports() {
            this._clearReferenceLayer('airports');
            const state = this._referenceLayerState.airports;
            if (!state || !state.source || !Array.isArray(this.airportsData) || !this._isLayerVisible('airports')) return;

            const zoom = this.getZoom();
            if (zoom < 8) return;
            const showLabels = zoom >= 12;
            const typeCfg = {
                large_airport: { color: window.MapPalette.color('airport'), radius: 6, minZoom: 8 },
                medium_airport: { color: window.MapPalette.color('airport'), radius: 5, minZoom: 9 },
                small_airport: { color: window.MapPalette.color('airport-small'), radius: 4, minZoom: 10 },
                seaplane_base: { color: window.MapPalette.color('airport-small'), radius: 3, minZoom: 11 },
            };

            this.airportsData.forEach((airport) => {
                const cfg = typeCfg[airport?.type] || typeCfg.small_airport;
                if (zoom < cfg.minZoom) return;
                if (typeof airport?.latitude !== 'number' || typeof airport?.longitude !== 'number') return;

                const feature = new window.ol.Feature({
                    geometry: new window.ol.geom.Point(this._toMapCoordinate(airport.latitude, airport.longitude)),
                });
                feature.set('refType', 'airport');
                feature.set('refData', airport);
                feature.setStyle(new window.ol.style.Style({
                    image: new window.ol.style.Circle({
                        radius: cfg.radius,
                        fill: new window.ol.style.Fill({ color: cfg.color }),
                        stroke: new window.ol.style.Stroke({ color: cfg.color, width: 1 }),
                    }),
                    text: showLabels ? this._getReferenceLabelStyle(airport.ident || '', cfg.color) : undefined,
                }));
                state.source.addFeature(feature);
            });
        }

        _renderHeliports() {
            this._clearReferenceLayer('heliports');
            const state = this._referenceLayerState.heliports;
            if (!state || !state.source || !Array.isArray(this.heliportsData) || !this._isLayerVisible('heliports')) return;

            const zoom = this.getZoom();
            if (zoom < 11) return;
            const showLabels = zoom >= 12;

            this.heliportsData.forEach((heliport) => {
                if (typeof heliport?.latitude !== 'number' || typeof heliport?.longitude !== 'number') return;
                const feature = new window.ol.Feature({
                    geometry: new window.ol.geom.Point(this._toMapCoordinate(heliport.latitude, heliport.longitude)),
                });
                feature.set('refType', 'heliport');
                feature.set('refData', heliport);
                feature.setStyle(new window.ol.style.Style({
                    image: new window.ol.style.Circle({
                        radius: 4,
                        fill: new window.ol.style.Fill({ color: window.MapPalette.color('heliport') }),
                        stroke: new window.ol.style.Stroke({ color: window.MapPalette.color('heliport'), width: 1.5 }),
                    }),
                    text: showLabels ? this._getReferenceLabelStyle(heliport.ident || '', window.MapPalette.color('heliport')) : undefined,
                }));
                state.source.addFeature(feature);
            });
        }

        _navaidColor(type) {
            const P = window.MapPalette;
            const role = {
                VOR: 'navaid-vor', 'VOR-DME': 'navaid-vor',
                VORTAC: 'navaid-vortac', TACAN: 'navaid-vortac',
                NDB: 'navaid-ndb', 'NDB-DME': 'navaid-ndb',
                DME: 'navaid-dme',
            }[type];
            return P.color(role || 'text-tertiary');
        }

        _renderNavaids() {
            this._clearReferenceLayer('navaids');
            const state = this._referenceLayerState.navaids;
            if (!state || !state.source || !Array.isArray(this.navaidsData) || !this._isLayerVisible('navaids')) return;

            const zoom = this.getZoom();
            if (zoom < 9) return;
            const showLabels = zoom >= 12;
            const useSymbolIcons = zoom >= 11;
            const radius = useSymbolIcons ? 1 : 3;

            this.navaidsData.forEach((navaid) => {
                if (typeof navaid?.latitude !== 'number' || typeof navaid?.longitude !== 'number') return;
                const color = this._navaidColor(navaid.type);
                const feature = new window.ol.Feature({
                    geometry: new window.ol.geom.Point(this._toMapCoordinate(navaid.latitude, navaid.longitude)),
                });
                feature.set('refType', 'navaid');
                feature.set('refData', navaid);
                feature.setStyle(new window.ol.style.Style({
                    image: useSymbolIcons
                        ? this._navaidIconStyle(navaid.type, 18)
                        : new window.ol.style.Circle({
                            radius,
                            fill: new window.ol.style.Fill({ color }),
                            stroke: new window.ol.style.Stroke({ color, width: 1 }),
                        }),
                    text: showLabels ? this._getReferenceLabelStyle(navaid.ident || '', color) : undefined,
                }));
                state.source.addFeature(feature);
            });
        }

        _renderAllRunways() {
            this._clearReferenceLayer('allRunways');
            const state = this._referenceLayerState.allRunways;
            if (!state || !state.source || !Array.isArray(this.allRunwaysData) || !this._isLayerVisible('allRunways')) return;

            const zoom = this.getZoom();
            if (zoom < 10) return;
            const showLabels = zoom >= 12;
            const weight = Math.max(2, Math.min(5, zoom - 8));

            this.allRunwaysData.forEach((runway) => {
                if (
                    typeof runway?.le_latitude !== 'number' || typeof runway?.le_longitude !== 'number' ||
                    typeof runway?.he_latitude !== 'number' || typeof runway?.he_longitude !== 'number'
                ) return;

                const line = new window.ol.Feature({
                    geometry: new window.ol.geom.LineString([
                        this._toMapCoordinate(runway.le_latitude, runway.le_longitude),
                        this._toMapCoordinate(runway.he_latitude, runway.he_longitude),
                    ]),
                });
                line.setStyle(new window.ol.style.Style({
                    stroke: new window.ol.style.Stroke({ color: window.MapPalette.color('text-tertiary'), width: weight }),
                }));
                state.source.addFeature(line);

                if (showLabels) {
                    if (runway.le_ident) {
                        const leLabel = new window.ol.Feature({
                            geometry: new window.ol.geom.Point(this._toMapCoordinate(runway.le_latitude, runway.le_longitude)),
                        });
                        leLabel.setStyle(new window.ol.style.Style({ text: this._getReferenceLabelStyle(runway.le_ident) }));
                        state.source.addFeature(leLabel);
                    }
                    if (runway.he_ident) {
                        const heLabel = new window.ol.Feature({
                            geometry: new window.ol.geom.Point(this._toMapCoordinate(runway.he_latitude, runway.he_longitude)),
                        });
                        heLabel.setStyle(new window.ol.style.Style({ text: this._getReferenceLabelStyle(runway.he_ident) }));
                        state.source.addFeature(heLabel);
                    }
                }
            });
        }

        _renderRangeRings() {
            if (!this._rangeRingLayerState || !this._rangeRingLayerState.source) return;
            this._rangeRingLayerState.source.clear();
            if (!this._isRangeRingVisible()) return;

            const centerLat = this.store?.stationLatitude || 0;
            const centerLon = this.store?.stationLongitude || 0;
            if (!Number.isFinite(centerLat) || !Number.isFinite(centerLon)) return;

            const center = this._toMapCoordinate(centerLat, centerLon);
            const rings = Array.isArray(this.CONFIG?.rangeRings) ? this.CONFIG.rangeRings : [];
            rings.forEach((radiusNm) => {
                const radiusMeters = Number(radiusNm) * 1852;
                if (!Number.isFinite(radiusMeters) || radiusMeters <= 0) return;

                const ring = new window.ol.Feature({
                    geometry: new window.ol.geom.Circle(center, radiusMeters),
                });
                ring.setStyle(new window.ol.style.Style({
                    stroke: new window.ol.style.Stroke({ color: window.MapPalette.color('ring'), width: 1 }),
                }));
                this._rangeRingLayerState.source.addFeature(ring);

                const labelOffsetMeters = radiusMeters;
                const labelPoint = new window.ol.Feature({
                    geometry: new window.ol.geom.Point([center[0], center[1] + labelOffsetMeters]),
                });
                labelPoint.setStyle(new window.ol.style.Style({
                    text: this._getReferenceLabelStyle(`${radiusNm} NM`, window.MapPalette.color('ring-label')),
                }));
                this._rangeRingLayerState.source.addFeature(labelPoint);
            });
        }

        // A trail is its aircraft's altitude colour at half strength, on the
        // ground too; an old or lost one is grey, as the lost glyph; the
        // followed flight's is the accent, dotted (palette C, 15/10).
        _getTrailStyleForAircraft(aircraft, isSelected) {
            const P = window.MapPalette;
            if (aircraft && (aircraft.status === 'signal_lost' || aircraft.status === 'stale')) {
                return { color: P.color('trail-stale'), lineDash: undefined };
            }
            if (isSelected) return { color: P.color('trail-followed'), lineDash: [4, 4] };
            const altitude = window.MapAircraftWebGLRenderer?.altitudeColor?.(aircraft, this.store);
            return { color: P.withAlpha(altitude || P.color('text-tertiary'), 0.5), lineDash: undefined };
        }

        _updateLiveTrailForAircraft(hex, aircraft) {
            if (!hex || !aircraft || !aircraft.adsb) return;

            const lat = aircraft.adsb.lat;
            const lon = aircraft.adsb.lon;
            const hasPosition = typeof lat === 'number' && typeof lon === 'number' && (lat !== 0 || lon !== 0);
            if (!hasPosition) return;

            const isStalePosition = aircraft.stale_position === true;
            const now = new Date();

            if (!isStalePosition) {
                if (!Array.isArray(this.trails[hex])) {
                    this.trails[hex] = [];
                }
                this.trails[hex].push({
                    lat,
                    lon,
                    alt_baro: (typeof aircraft.adsb?.alt_baro === 'number' && Number.isFinite(aircraft.adsb.alt_baro))
                        ? aircraft.adsb.alt_baro
                        : null,
                    time: now,
                    isHistorical: false,
                });
            }

            if (Array.isArray(this.trails[hex])) {
                const selectedTrailLength = Number(this.store?.settings?.trailLength);
                const retentionMinutes = Number.isFinite(selectedTrailLength) ? selectedTrailLength : 2;
                const cutoffMs = now.getTime() - (retentionMinutes * 60 * 1000);

                const trail = this.trails[hex];
                let pruneIdx = 0;
                while (pruneIdx < trail.length) {
                    const point = trail[pruneIdx];
                    const pointMs = point?.time instanceof Date
                        ? point.time.getTime()
                        : (point?.time ? new Date(point.time).getTime() : NaN);
                    if (!Number.isFinite(pointMs) || pointMs >= cutoffMs) break;
                    pruneIdx++;
                }
                if (pruneIdx > 0) {
                    trail.splice(0, pruneIdx);
                }

                if (trail.length > 500) {
                    trail.splice(0, trail.length - 500);
                }
            }
        }

        _createTrailFeature(hex, latLonPoints, styleOptions) {
            if (!Array.isArray(latLonPoints) || latLonPoints.length < 2) return null;

            const projected = latLonPoints
                .filter((point) => Array.isArray(point) && point.length >= 2)
                .map((point) => this._toMapCoordinate(point[0], point[1]));

            if (projected.length < 2) return null;

            const geometry = new window.ol.geom.LineString(projected);
            const feature = new window.ol.Feature({ geometry });
            feature.set('hex', hex);
            feature.set('trailType', styleOptions?.trailType || 'current');

            feature.setStyle(new window.ol.style.Style({
                stroke: new window.ol.style.Stroke({
                    color: styleOptions?.color || window.MapPalette.color('text-tertiary'),
                    width: Number.isFinite(styleOptions?.width) ? styleOptions.width : 2,
                    lineDash: Array.isArray(styleOptions?.lineDash) ? styleOptions.lineDash : undefined,
                }),
            }));

            return feature;
        }

        _createTrailMetadataFeature(hex, latitude, longitude, styleOptions = {}) {
            if (typeof latitude !== 'number' || typeof longitude !== 'number') return [];

            const coordinate = this._toMapCoordinate(latitude, longitude);
            const features = [];

            const pointFeature = new window.ol.Feature({
                geometry: new window.ol.geom.Point(coordinate),
            });
            pointFeature.set('hex', hex);
            pointFeature.set('trailType', styleOptions?.trailType || 'metadata');
            pointFeature.setStyle(new window.ol.style.Style({
                image: new window.ol.style.Circle({
                    radius: Number.isFinite(styleOptions?.radius) ? styleOptions.radius : 3,
                    fill: new window.ol.style.Fill({ color: styleOptions?.fillColor || window.MapPalette.color('text-tertiary') }),
                    stroke: new window.ol.style.Stroke({
                        color: styleOptions?.strokeColor || styleOptions?.fillColor || window.MapPalette.color('text-tertiary'),
                        width: Number.isFinite(styleOptions?.strokeWidth) ? styleOptions.strokeWidth : 1,
                    }),
                }),
            }));
            features.push(pointFeature);

            const text = (styleOptions?.labelText || '').toString().trim();
            if (text) {
                const labelFeature = new window.ol.Feature({
                    geometry: new window.ol.geom.Point(coordinate),
                });
                labelFeature.set('hex', hex);
                labelFeature.set('trailType', styleOptions?.trailType || 'metadata');
                labelFeature.setStyle(new window.ol.style.Style({
                    text: new window.ol.style.Text({
                        text,
                        font: '10px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                        offsetX: Number.isFinite(styleOptions?.labelOffsetX) ? styleOptions.labelOffsetX : 6,
                        offsetY: Number.isFinite(styleOptions?.labelOffsetY) ? styleOptions.labelOffsetY : 0,
                        textAlign: 'left',
                        fill: new window.ol.style.Fill({ color: styleOptions?.labelColor || window.MapPalette.color('text-tertiary') }),
                    }),
                }));
                features.push(labelFeature);
            }

            return features;
        }

        _replaceTrailFeatures(hex, features) {
            const existing = this._trailFeaturesByHex.get(hex);
            if (existing && this._trailSource) {
                existing.forEach((feature) => this._trailSource.removeFeature(feature));
            }

            if (!Array.isArray(features) || features.length === 0) {
                this._trailFeaturesByHex.delete(hex);
                return;
            }

            this._trailFeaturesByHex.set(hex, features);
            if (this._trailSource) {
                features.forEach((feature) => this._trailSource.addFeature(feature));
            }
        }

        _getAircraftFeature(hex) {
            if (!this._aircraftRenderer || typeof this._aircraftRenderer.getFeature !== 'function') {
                return null;
            }
            return this._aircraftRenderer.getFeature(hex);
        }

        _getAircraftHitLayer() {
            if (!this._aircraftRenderer || typeof this._aircraftRenderer.getLayer !== 'function') {
                return null;
            }
            return this._aircraftRenderer.getLayer();
        }

        _setFeatureVisible(hex, aircraft, visible) {
            if (!this._aircraftRenderer) return;
            if (!aircraft || !aircraft.adsb) return;
            const lat = aircraft.adsb.lat;
            const lon = aircraft.adsb.lon;
            const hasPosition = typeof lat === 'number' && typeof lon === 'number' && (lat !== 0 || lon !== 0);
            if (!hasPosition) return;

            if (!visible) {
                this._aircraftRenderer.removeAircraftFeature(hex);
                return;
            }

            const selectedHex = this.store?.selectedAircraft?.hex || null;
            const hoveredHex = this.store?.hoveredAircraft?.hex || null;
            const smoothingEnabled = !!this.store?.settings?.aircraftAnimation?.enabled;
            this._aircraftRenderer.upsertAircraftFeature(aircraft, {
                selected: selectedHex === hex,
                hovered: hoveredHex === hex,
                deemphasized: !!selectedHex && selectedHex !== hex,
            }, {
                preservePose: smoothingEnabled,
            });
        }

        _aircraftMatchesFilters(aircraft) {
            if (!aircraft) return false;
            if (!this.store || !this.store.settings) return true;

            if (window.MapVisibilityRules && window.MapVisibilityRules.shouldShowAircraftOnMap) {
                const now = new Date();
                const lastSeenCutoff = new Date(now.getTime() - ((this.store.settings?.lastSeenMinutes || 10) * 60 * 1000));
                return window.MapVisibilityRules.shouldShowAircraftOnMap(aircraft, this.store, {
                    searchTerm: this.store.searchTerm || '',
                    lastSeenCutoff,
                    isInViewport: true,
                });
            }

            return true;
        }

        _warnOnce(key, message) {
            if (this._warnings.has(key)) return;
            this._warnings.add(key);
            console.warn(message);
        }

        _getInitialCenter() {
            const lat = this.store?.stationLatitude || 43.6777;
            const lon = this.store?.stationLongitude || -79.6248;
            return { lat, lon };
        }

        _createMapShim() {
            return {
                setView: (position, zoom) => {
                    if (!Array.isArray(position) || position.length < 2) return;
                    this.setView(position[0], position[1], zoom);
                },
                getZoom: () => this.getZoom(),
            };
        }

        initMap() {
            this._stats.initCalls++;
            if (this.engine) return;

            if (!window.MapEngine || !window.MapEngine.createOpenLayersEngine) {
                this._warnOnce('engine-missing', 'MapEngine.createOpenLayersEngine is unavailable');
                return;
            }

            const center = this._getInitialCenter();
            const zoom = Number.isFinite(this.CONFIG?.defaultZoom) ? this.CONFIG.defaultZoom : 10;

            this.engine = window.MapEngine.createOpenLayersEngine({
                targetId: 'map',
                center,
                zoom,
                baseMapStyle: this.store?.settings?.mapStyle || 'none',
            });
            this.engine.init();
            this._olMap = this.engine.getMap();
            console.info('[OpenLayersMapManager] OpenLayers map initialized');

            if (!window.MapAircraftWebGLRenderer || !window.MapAircraftWebGLRenderer.createAircraftWebGLRenderer) {
                this._warnOnce('renderer-missing', 'MapAircraftWebGLRenderer is unavailable; aircraft rendering cannot start');
            } else {
                this._aircraftRenderer = window.MapAircraftWebGLRenderer.createAircraftWebGLRenderer(this._olMap, {
                    zIndex: 400,
                    store: this.store,
                    preferWebGL: !!this.CONFIG?.mapAircraftWebGL,
                });
                this._aircraftRenderer.init();
            }

            this._ensureTrailLayer();
            this._ensureOverlayLayer();
            this._ensureReferenceLayers();
            this._loadAirportLayout();
            this._initOverlayRegistry();
            this._ensureReferencePopup();

            this.map = this._createMapShim();

            if (window.MapInteractionsFeature?.registerOpenLayersInteractions) {
                this._interactionCleanup = window.MapInteractionsFeature.registerOpenLayersInteractions(this);
            }

            this.engine.on('moveend', () => {
                this.updateVisibleAircraftList();
                this._scheduleReferenceRender();
                if (this._overlayRegistry && typeof this._overlayRegistry.updateZoomVisibility === 'function') {
                    this._overlayRegistry.updateZoomVisibility(this.getZoom());
                }
            });
            this.startLabelRefreshTimer();
            this._scheduleReferenceRender();
        }

        startLabelRefreshTimer() {
            this.stopLabelRefreshTimer();
            this._labelRefreshInterval = window.setInterval(() => {
                this.refreshStaleLabels();
            }, 5000);
        }

        stopLabelRefreshTimer() {
            if (this._labelRefreshInterval) {
                clearInterval(this._labelRefreshInterval);
                this._labelRefreshInterval = null;
            }
        }

        refreshStaleLabels() {
            if (!this._aircraftRenderer || typeof this._aircraftRenderer.requestRedraw !== 'function') return;
            this._aircraftRenderer.requestRedraw();
        }

        setView(lat, lon, zoom) {
            if (!this.engine) return;
            this.engine.setView(lat, lon, zoom);
        }

        fitBounds(points) {
            if (!this.engine) return;
            this.engine.fitBounds(points);
        }

        getZoom() {
            if (!this.engine) return 0;
            return this.engine.getZoom();
        }

        setMapStyle(styleId) {
            if (!this.engine || typeof this.engine.setBaseMapStyle !== 'function') return;
            this.engine.setBaseMapStyle(styleId);
            // The rings and the airport layout have their own colours on a
            // bright base (color-map-light-*): drawn again.
            this._renderRangeRings();
            this._referenceLayerState?.airportLayout?.layer?.changed();
            this._procedures?.refresh();
            // Aircraft icon colors depend on map style (dark vs light), so force a full re-render
            if (this._aircraftRenderer) {
                this.applyFiltersAndRefreshView();
            }
        }

        centerOnStation() {
            const center = this._getInitialCenter();
            this.setView(center.lat, center.lon, this.getZoom() || this.CONFIG.defaultZoom || 10);
        }

        centerOnAircraft(aircraft) {
            const lat = aircraft?.adsb?.lat;
            const lon = aircraft?.adsb?.lon;
            if (typeof lat !== 'number' || typeof lon !== 'number') return;
            this.setView(aircraft.adsb.lat, aircraft.adsb.lon, this.getZoom() || this.CONFIG.defaultZoom || 10);
        }

        applyFiltersAndRefreshView() {
            this._stats.refreshCalls++;
            const aircraftByHex = this.store?.aircraft || {};

            if (this._aircraftRenderer && typeof this._aircraftRenderer.bulkSyncAircraft === 'function') {
                this._aircraftRenderer.bulkSyncAircraft(
                    aircraftByHex,
                    (aircraft) => this._aircraftMatchesFilters(aircraft),
                    (hex) => ({
                        selected: this.store?.selectedAircraft?.hex === hex,
                        hovered: this.store?.hoveredAircraft?.hex === hex,
                        deemphasized: !!this.store?.selectedAircraft?.hex && this.store.selectedAircraft.hex !== hex,
                    }),
                    // As a single update does: the animated pose kept.
                    { preservePose: !!this.store?.settings?.aircraftAnimation?.enabled }
                );
            } else {
                Object.keys(aircraftByHex).forEach((hex) => {
                    const aircraft = aircraftByHex[hex];
                    const visible = this._aircraftMatchesFilters(aircraft);
                    this._setFeatureVisible(hex, aircraft, visible);
                });
            }

            this.updateVisualState();
            this.updateVisibleAircraftList();
            this.updateFlightPaths();
        }

        updateVisibleAircraftList() {
            const nextSet = this._aircraftRenderer && typeof this._aircraftRenderer.getFeatureHexes === 'function'
                ? this._aircraftRenderer.getFeatureHexes()
                : new Set();
            const currentSet = this.store.visibleAircraftOnMap;
            if (currentSet && currentSet.size === nextSet.size) {
                let setsEqual = true;
                for (const hex of nextSet) {
                    if (!currentSet.has(hex)) {
                        setsEqual = false;
                        break;
                    }
                }
                if (setsEqual) return;
            }
            this.store.visibleAircraftOnMap = nextSet;
        }

        updateSingleAircraft(hex, aircraft) {
            this._stats.singleAircraftUpdates++;
            if (!hex || !aircraft) return;
            const startedAt = performance.now();

            this._updateLiveTrailForAircraft(hex, aircraft);

            const visible = this._aircraftMatchesFilters(aircraft);
            const hadFeature = !!this._getAircraftFeature(hex);
            this._setFeatureVisible(hex, aircraft, visible);
            // The animation engine moves the aircraft of visibleAircraftOnMap
            // only, and the renderer keeps a symbol's pose while smoothing is
            // on: an aircraft that appeared between two full refreshes (a pan,
            // a filter) stood where it first appeared until the next one
            // (browser bench, 13/10/2026: 6 aircraft of 60, 4 to 6 km behind,
            // for as long as nobody touched the map).
            if (hadFeature !== !!this._getAircraftFeature(hex)) {
                this.updateVisibleAircraftList();
                // A new symbol stands at the position the station gave; the
                // engine draws the aircraft where that position, dated, puts it
                // now: on a page just loaded, 450 m further on its next frame
                // (browser bench, scenario a). It starts there.
                const pose = !hadFeature && this.store?.animationEngine?.currentPose?.(hex);
                if (pose) this.updateAnimatedAircraftPose(hex, pose);
            }
            this.updateProximityCircle();
            this._stats.singleAircraftUpdateDurationMs += (performance.now() - startedAt);
        }

        updateAnimatedAircraftPose(hex, pose) {
            if (!hex || !pose) return false;

            const lat = Number(pose.lat);
            const lon = Number(pose.lon);
            if (!Number.isFinite(lat) || !Number.isFinite(lon) || (lat === 0 && lon === 0)) {
                return false;
            }

            const feature = this._getAircraftFeature(hex);
            if (!feature) return false;

            const geometry = feature.getGeometry();
            if (!geometry || typeof geometry.setCoordinates !== 'function') {
                return false;
            }

            const coordinate = this._toMapCoordinate(lat, lon);
            geometry.setCoordinates(coordinate);

            const headingDeg = Number(pose.heading);
            if (Number.isFinite(headingDeg)) {
                feature.set('heading', headingDeg);
                feature.set('rotationRad', (headingDeg * Math.PI) / 180);
            }
            // The symbol's opacity while the engine relocates it (a gap too
            // wide to glide over), in tenths: the renderer caches a style per
            // value.
            if (Number.isFinite(pose.fade)) {
                feature.set('fade', Math.round(pose.fade * 10) / 10);
            }

            feature.changed();
            return true;
        }

        toggleRings() {
            this._ensureReferenceLayers();
            if (!this._rangeRingLayerState || !this._rangeRingLayerState.layer) return;
            const currentlyVisible = this._rangeRingLayerState.layer.getVisible();
            this._rangeRingLayerState.layer.setVisible(!currentlyVisible);
            this._renderRangeRings();
        }

        toggleLayerVisibility(layerName, visible) {
            this._ensureReferenceLayers();

            if (layerName === 'rangeRings') {
                if (this._rangeRingLayerState?.layer) {
                    const shouldShow = typeof visible === 'boolean'
                        ? visible
                        : !this._rangeRingLayerState.layer.getVisible();
                    this._rangeRingLayerState.layer.setVisible(shouldShow);
                    this._renderRangeRings();
                }
                return;
            }

            const state = this._referenceLayerState[layerName];
            if (!state || !state.layer) {
                if (this._overlayRegistry && typeof this._overlayRegistry.setVisible === 'function') {
                    const shouldShowOverlay = typeof visible === 'boolean' ? visible : true;
                    this._overlayRegistry.setVisible(layerName, shouldShowOverlay);
                    this._overlayStatuses = this._overlayRegistry.getStatuses();
                    if (this._overlayRegistry.updateZoomVisibility) {
                        this._overlayRegistry.updateZoomVisibility(this.getZoom());
                    }
                    return;
                }

                this._warnOnce(`toggle-layer-${layerName || 'unknown'}`, `toggleLayerVisibility(${layerName || 'unknown'}) is unsupported in OpenLayers manager`);
                return;
            }

            const shouldShow = typeof visible === 'boolean' ? visible : !state.layer.getVisible();
            state.layer.setVisible(shouldShow);
            this._scheduleReferenceRender();
        }
        addRangeRings() {
            this._ensureReferenceLayers();
            this._renderRangeRings();
        }
        updateVisualState(hex) {
            if (!this._aircraftRenderer) return;

            const selectedHex = this.store?.selectedAircraft?.hex || null;
            const hoveredHex = this.store?.hoveredAircraft?.hex || null;

            if (!hex) {
                this._aircraftRenderer.setVisualState(selectedHex, hoveredHex);
                return;
            }

            if (typeof this._aircraftRenderer.refreshAircraftStyle === 'function') {
                this._aircraftRenderer.refreshAircraftStyle(hex, selectedHex, hoveredHex);
            } else {
                this._aircraftRenderer.setVisualState(selectedHex, hoveredHex);
            }
        }

        handleReferenceFeatureClick(event) {
            return this._handleReferenceFeatureClick(event);
        }
        updateFlightPaths() {
            this._ensureTrailLayer();

            if (!this._trailSource) return;

            if (!this.store?.settings?.showPaths) {
                this._trailSource.clear();
                this._trailFeaturesByHex.clear();
                return;
            }

            const aircraftByHex = this.store?.aircraft || {};
            const selectedHex = this.store?.selectedAircraft?.hex || null;
            const currentHexes = new Set(Object.keys(aircraftByHex));

            this._trailFeaturesByHex.forEach((_, hex) => {
                if (!currentHexes.has(hex)) {
                    this._replaceTrailFeatures(hex, []);
                }
            });

            Object.keys(aircraftByHex).forEach((hex) => {
                const aircraft = aircraftByHex[hex];
                if (!aircraft) return;

                const markerVisible = this.store?.visibleAircraftOnMap instanceof Set
                    ? this.store.visibleAircraftOnMap.has(hex)
                    : true;
                const isSelected = selectedHex === hex;
                if (!markerVisible && !isSelected) {
                    this._replaceTrailFeatures(hex, []);
                    return;
                }

                const trailStyle = this._getTrailStyleForAircraft(aircraft, isSelected);
                const estimate = window.MapPalette.color('estimate');
                const features = [];

                const trailPoints = (this.trails && Array.isArray(this.trails[hex]))
                    ? this.trails[hex]
                        .filter((point) => {
                            if (typeof point?.lat !== 'number' || typeof point?.lon !== 'number') return false;
                            const trailLengthMinutes = Number(this.store?.settings?.trailLength);
                            const safeTrailMinutes = Number.isFinite(trailLengthMinutes) ? trailLengthMinutes : 2;
                            const cutoffMs = Date.now() - (safeTrailMinutes * 60 * 1000);
                            const pointTime = point?.time instanceof Date
                                ? point.time.getTime()
                                : (point?.time ? new Date(point.time).getTime() : NaN);
                            return !Number.isFinite(pointTime) || pointTime >= cutoffMs;
                        })
                        .map((point) => [point.lat, point.lon])
                    : [];

                const currentTrailFeature = this._createTrailFeature(hex, trailPoints, {
                    trailType: 'current',
                    color: trailStyle.color,
                    width: 2,
                    lineDash: trailStyle.lineDash,
                });
                if (currentTrailFeature) {
                    features.push(currentTrailFeature);
                }

                if (isSelected) {
                    const nowMs = Date.now();
                    const historyPoints = (this.store.aircraftDetailsHistoryData || [])
                        .filter((position) => typeof position?.lat === 'number' && typeof position?.lon === 'number')
                        .map((position) => [position.lat, position.lon]);
                    const historyFeature = this._createTrailFeature(hex, historyPoints, {
                        trailType: 'history',
                        color: trailStyle.color,
                        width: 2,
                        lineDash: [4, 4],
                    });
                    if (historyFeature) {
                        features.push(historyFeature);
                    }

                    const historyPositions = (this.store.aircraftDetailsHistoryData || [])
                        .filter((position) => typeof position?.lat === 'number' && typeof position?.lon === 'number');
                    if (historyPositions.length > 0) {
                        const shownMinutes = new Set();
                        let historyMetadataCount = 0;
                        const maxHistoryMetadata = 12;

                        for (let index = 0; index < historyPositions.length; index++) {
                            if (historyMetadataCount >= maxHistoryMetadata) break;
                            const position = historyPositions[index];
                            const timestampRaw = position?.timestamp || position?.time;
                            if (!timestampRaw) continue;

                            const timestampMs = new Date(timestampRaw).getTime();
                            if (!Number.isFinite(timestampMs)) continue;

                            const minutesAgo = Math.floor(Math.max(0, nowMs - timestampMs) / 60000);
                            const isEdgePoint = index === 0 || index === historyPositions.length - 1;
                            const shouldShow = isEdgePoint || (minutesAgo > 0 && !shownMinutes.has(minutesAgo));
                            if (!shouldShow) continue;

                            shownMinutes.add(minutesAgo);
                            const altitudeRaw = Number(position?.altitude ?? position?.alt_baro);
                            const altitudeText = Number.isFinite(altitudeRaw)
                                ? (altitudeRaw === 0 ? 'GND' : `${Math.round(altitudeRaw / 100) * 100}`)
                                : '';
                            const labelText = altitudeText ? `-${minutesAgo}m ${altitudeText}` : `-${minutesAgo}m`;
                            const metadataFeatures = this._createTrailMetadataFeature(hex, position.lat, position.lon, {
                                trailType: 'history-metadata',
                                radius: isEdgePoint ? 4 : 3,
                                fillColor: window.MapPalette.color('text-tertiary'),
                                strokeColor: window.MapPalette.color('text-tertiary'),
                                labelText,
                                labelColor: window.MapPalette.color('text-tertiary'),
                                labelOffsetX: 8,
                                labelOffsetY: 0,
                            });
                            features.push(...metadataFeatures);
                            historyMetadataCount++;
                        }
                    }

                    const hindcastPoints = (this.store.aircraftDetailsHindcastData || [])
                        .filter((position) => typeof position?.lat === 'number' && typeof position?.lon === 'number')
                        .map((position) => [position.lat, position.lon]);
                    const hindcastFeature = this._createTrailFeature(hex, hindcastPoints, {
                        trailType: 'hindcast',
                        color: estimate,
                        width: 1.5,
                        lineDash: [2, 4],
                    });
                    if (hindcastFeature) {
                        features.push(hindcastFeature);
                    }

                    const futurePoints = (this.store.aircraftDetailsFutureData || [])
                        .filter((position) => typeof position?.lat === 'number' && typeof position?.lon === 'number')
                        .map((position) => [position.lat, position.lon]);
                    const futureFeature = this._createTrailFeature(hex, futurePoints, {
                        trailType: 'future',
                        color: estimate,
                        width: 1.5,
                        lineDash: [2, 4],
                    });
                    if (futureFeature) {
                        features.push(futureFeature);
                    }

                    const futurePositions = (this.store.aircraftDetailsFutureData || [])
                        .filter((position) => typeof position?.lat === 'number' && typeof position?.lon === 'number');
                    if (futurePositions.length > 0) {
                        let futureMetadataCount = 0;
                        const maxFutureMetadata = 12;

                        futurePositions.forEach((position, index) => {
                            if (futureMetadataCount >= maxFutureMetadata) return;
                            const is15SecondInterval = (index + 1) % 3 === 0;
                            const isLast = index === futurePositions.length - 1;
                            if (!is15SecondInterval && !isLast) return;

                            const timeLabel = (typeof this.store?.formatPredictionTime === 'function')
                                ? this.store.formatPredictionTime(position)
                                : `+${(index + 1) * 5}s`;
                            const altitudeRaw = Number(position?.altitude ?? position?.alt_baro);
                            const altitudeText = Number.isFinite(altitudeRaw)
                                ? (altitudeRaw === 0 ? 'GND' : `${Math.round(altitudeRaw / 100) * 100}`)
                                : '';
                            const labelText = altitudeText ? `${timeLabel} ${altitudeText}` : timeLabel;

                            const metadataFeatures = this._createTrailMetadataFeature(hex, position.lat, position.lon, {
                                trailType: 'future-metadata',
                                radius: isLast ? 4 : 3,
                                fillColor: window.MapPalette.color('text-disabled'),
                                strokeColor: window.MapPalette.color('text-disabled'),
                                labelText,
                                labelColor: window.MapPalette.color('text-disabled'),
                                labelOffsetX: 8,
                                labelOffsetY: 0,
                            });
                            features.push(...metadataFeatures);
                            futureMetadataCount++;
                        });
                    }
                }

                this._replaceTrailFeatures(hex, features);
            });

            this._stats.trailFeatureCount = this._trailSource.getFeatures().length;
        }
        updateTracksMiniMap() {
            if (window.MapMinimapFeature && typeof window.MapMinimapFeature.updateTracksMiniMap === 'function') {
                window.MapMinimapFeature.updateTracksMiniMap(this);
            }
        }
        initTracksMiniMap(containerId, retryCount = 0) {
            if (window.MapMinimapFeature && typeof window.MapMinimapFeature.initTracksMiniMap === 'function') {
                window.MapMinimapFeature.initTracksMiniMap(this, containerId, retryCount);
            }
        }
        cleanupTracksMiniMap() {
            if (window.MapMinimapFeature && typeof window.MapMinimapFeature.cleanupTracksMiniMap === 'function') {
                window.MapMinimapFeature.cleanupTracksMiniMap(this);
            }
        }
        clearAircraftTrails(hex, options = {}) {
            if (!hex) {
                if (this._trailSource) {
                    this._trailSource.clear();
                }
                this._trailFeaturesByHex.clear();
                return;
            }

            this._replaceTrailFeatures(hex, []);

            if (options?.clearHistory === true && this.trails) {
                delete this.trails[hex];
            }
        }
        ensureMapObjects(aircraft) {
            if (!aircraft || !aircraft.hex) return;
            this.updateSingleAircraft(aircraft.hex, aircraft);
        }

        removeStaleMarkers(currentAircraftHexes) {
            const keep = currentAircraftHexes instanceof Set
                ? currentAircraftHexes
                : new Set(Array.isArray(currentAircraftHexes) ? currentAircraftHexes : []);

            if (!this._aircraftRenderer || typeof this._aircraftRenderer.getFeatureHexes !== 'function') {
                return;
            }

            const existingHexes = this._aircraftRenderer.getFeatureHexes();
            existingHexes.forEach((hex) => {
                if (!keep.has(hex)) {
                    this.removeAircraft(hex);
                }
            });
        }

        removeAircraft(hex) {
            if (!hex) return;
            if (this._aircraftRenderer && typeof this._aircraftRenderer.removeAircraftFeature === 'function') {
                this._aircraftRenderer.removeAircraftFeature(hex);
                this.updateVisibleAircraftList();
            }
            this._replaceTrailFeatures(hex, []);

            if (this.store?.selectedAircraft?.hex === hex) {
                this.store.selectedAircraft = null;
            }

            if (this.store?.hoveredAircraft?.hex === hex) {
                this.store.hoveredAircraft = null;
            }
        }
        drawRunways(runwayData) {
            this._ensureReferenceLayers();
            this.runwayData = runwayData || null;
            this._renderRunways();
        }
        drawAirports(airports) {
            this._ensureReferenceLayers();
            this.airportsData = Array.isArray(airports) ? airports : [];
            this._renderAirports();
        }
        drawHeliports(heliports) {
            this._ensureReferenceLayers();
            this.heliportsData = Array.isArray(heliports) ? heliports : [];
            this._renderHeliports();
        }
        drawNavaids(navaids) {
            this._ensureReferenceLayers();
            this.navaidsData = Array.isArray(navaids) ? navaids : [];
            this._renderNavaids();
        }
        drawAllRunways(runways) {
            this._ensureReferenceLayers();
            this.allRunwaysData = Array.isArray(runways) ? runways : [];
            this._renderAllRunways();
        }
        showTakeoffLandingEffect() {
            const hex = arguments[0];
            const eventType = arguments[1];
            const phase = arguments[2];

            const aircraft = this.store?.aircraft?.[hex];
            const lat = aircraft?.adsb?.lat;
            const lon = aircraft?.adsb?.lon;
            if (typeof lat !== 'number' || typeof lon !== 'number') return;

            this._ensureOverlayLayer();
            if (!this._overlaySource) return;

            // The phase family's colour (phase-families.js), as the label's prefix.
            const phaseColor = window.PhaseFamilies ? window.PhaseFamilies.colorOf(phase) : window.MapPalette.color('text-tertiary');
            const P = window.MapPalette;
            const center = this._toMapCoordinate(lat, lon);
            const feature = new window.ol.Feature({
                geometry: new window.ol.geom.Circle(center, 400),
            });
            feature.setStyle(new window.ol.style.Style({
                stroke: new window.ol.style.Stroke({ color: phaseColor, width: 2 }),
                fill: new window.ol.style.Fill({ color: P.withAlpha(phaseColor, 0.12) }),
            }));

            this._overlaySource.addFeature(feature);
            this._effectFeatures.add(feature);

            const startedAt = Date.now();
            const durationMs = 1400;
            const animate = () => {
                if (!this._overlaySource || !this._effectFeatures.has(feature)) return;

                const progress = Math.min(1, (Date.now() - startedAt) / durationMs);
                const radius = 400 + (2200 * progress);
                const alpha = Math.max(0, 0.2 * (1 - progress));
                feature.setGeometry(new window.ol.geom.Circle(center, radius));
                feature.setStyle(new window.ol.style.Style({
                    stroke: new window.ol.style.Stroke({ color: phaseColor, width: 2 }),
                    fill: new window.ol.style.Fill({ color: P.withAlpha(phaseColor, alpha.toFixed(3)) }),
                }));

                if (progress < 1) {
                    window.requestAnimationFrame(animate);
                } else {
                    this._overlaySource.removeFeature(feature);
                    this._effectFeatures.delete(feature);
                }
            };

            window.requestAnimationFrame(animate);
        }
        // A procedure's published route (ProcedureRoute, Q56), drawn until it
        // is closed; it replaces the ring of its point. runways: the ones it is
        // drawn from or to (map/procedure-routes.js).
        _procedureFeature() {
            if (!this._procedures && window.MapProcedureFeature) this._procedures = window.MapProcedureFeature.create(this);
            return this._procedures;
        }
        showProcedureRoute(route, runways) {
            if (this._placeFeature && this._overlaySource) {
                this._overlaySource.removeFeature(this._placeFeature);
                this._placeFeature = null;
            }
            this._procedureFeature()?.show(route, runways);
        }
        clearProcedureRoute() {
            this._procedures?.clear();
        }
        // The selected departure's candidate exits (Q57): [{ route, runways,
        // strong }], the likeliest strong; an empty list clears them.
        showExitRoutes(items) {
            this._procedureFeature()?.setCandidates(items || []);
        }
                // The Procedures layer's routes; an empty list hides it.
        setProcedureLayer(routes, runwaysOf) {
            this._procedureFeature()?.setLayer(routes, runwaysOf);
        }
        // A procedure's point shown for ten seconds (Chips: Place, Q55): a ring
        // and its name where it is, the view moved to it when it is outside.
        showPlace(lat, lon, name) {
            if (typeof lat !== 'number' || typeof lon !== 'number') return;
            this._ensureOverlayLayer();
            if (!this._overlaySource) return;
            if (this._placeFeature) {
                this._overlaySource.removeFeature(this._placeFeature);
                this._placeFeature = null;
            }
            clearTimeout(this._placeTimer);
            const P = window.MapPalette;
            const at = this._toMapCoordinate(lat, lon);
            const feature = new window.ol.Feature({ geometry: new window.ol.geom.Point(at) });
            feature.setStyle(new window.ol.style.Style({
                image: new window.ol.style.Circle({
                    radius: 14,
                    stroke: new window.ol.style.Stroke({ color: P.color('selected'), width: 2 }),
                }),
                text: new window.ol.style.Text({
                    text: name || '',
                    font: '11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                    offsetY: -24,
                    fill: new window.ol.style.Fill({ color: P.color('text-primary') }),
                    backgroundFill: new window.ol.style.Fill({ color: 'rgba(0,0,0,0.6)' }),
                    padding: [2, 4, 2, 4],
                }),
            }));
            this._overlaySource.addFeature(feature);
            this._placeFeature = feature;
            const map = this._olMap;
            if (map) {
                const extent = map.getView().calculateExtent(map.getSize());
                if (!window.ol.extent.containsCoordinate(extent, at)) {
                    map.getView().animate({ center: at, duration: window.smoothScroll && window.smoothScroll() === 'auto' ? 0 : 400 });
                }
            }
            this._placeTimer = setTimeout(() => {
                if (this._placeFeature && this._overlaySource) this._overlaySource.removeFeature(this._placeFeature);
                this._placeFeature = null;
            }, 10000);
        }
        showPositionHighlight(lat, lon) {
            if (typeof lat !== 'number' || typeof lon !== 'number') return;
            this._ensureOverlayLayer();
            if (!this._overlaySource) return;

            this.clearPositionHighlight();

            const point = new window.ol.geom.Point(this._toMapCoordinate(lat, lon));
            const feature = new window.ol.Feature({ geometry: point });
            feature.setStyle(new window.ol.style.Style({
                image: new window.ol.style.Circle({
                    radius: 7,
                    fill: new window.ol.style.Fill({ color: window.MapPalette.color('selected') }),
                    stroke: new window.ol.style.Stroke({ color: window.MapPalette.color('text-primary'), width: 2 }),
                }),
            }));

            this._positionHighlightFeature = feature;
            this._overlaySource.addFeature(feature);
        }
        clearPositionHighlight() {
            if (this._overlaySource && this._positionHighlightFeature) {
                this._overlaySource.removeFeature(this._positionHighlightFeature);
            }
            this._positionHighlightFeature = null;
        }

        drawProximityCircle(position, distanceNM) {
            if (!Array.isArray(position) || position.length < 2) return;
            const lat = position[0];
            const lon = position[1];
            if (typeof lat !== 'number' || typeof lon !== 'number') return;

            this._ensureOverlayLayer();
            this.removeProximityCircle();

            const radiusMeters = Number(distanceNM) * 1852;
            if (!Number.isFinite(radiusMeters) || radiusMeters <= 0 || !this._overlaySource) return;

            this.proximityRefHex = this.store?.selectedAircraft?.hex || null;
            this.proximityDistanceNM = distanceNM;

            const circle = new window.ol.geom.Circle(this._toMapCoordinate(lat, lon), radiusMeters);
            const feature = new window.ol.Feature({ geometry: circle });
            feature.setStyle(new window.ol.style.Style({
                stroke: new window.ol.style.Stroke({
                    color: window.MapPalette.color('proximity'),
                    width: 2,
                    lineDash: [5, 5],
                }),
                fill: new window.ol.style.Fill({ color: window.MapPalette.withAlpha(window.MapPalette.color('proximity'), 0.06) }),
            }));

            this._proximityCircleFeature = feature;
            this._overlaySource.addFeature(feature);
            this.proximityCircle = { position: [lat, lon], distanceNM };
        }

        removeProximityCircle() {
            if (this._overlaySource && this._proximityCircleFeature) {
                this._overlaySource.removeFeature(this._proximityCircleFeature);
            }
            this._proximityCircleFeature = null;
            this.proximityCircle = null;
            this.proximityRefHex = null;
            this.proximityDistanceNM = null;
        }

        updateProximityCircle() {
            if (!this._proximityCircleFeature || !this.proximityRefHex) return;

            const aircraft = this.store?.aircraft?.[this.proximityRefHex];
            const lat = aircraft?.adsb?.lat;
            const lon = aircraft?.adsb?.lon;
            if (typeof lat !== 'number' || typeof lon !== 'number') return;

            const radiusMeters = Number(this.proximityDistanceNM) * 1852;
            if (!Number.isFinite(radiusMeters) || radiusMeters <= 0) return;

            this._proximityCircleFeature.setGeometry(
                new window.ol.geom.Circle(this._toMapCoordinate(lat, lon), radiusMeters)
            );
        }

        highlightProximityAircraft(proximityHexSet) {
            this.proximityHexSet = proximityHexSet || null;
            this.store.proximityHighlightedAircraft = proximityHexSet instanceof Set ? new Set(proximityHexSet) : new Set();
            this.updateVisualState();
            this.refreshStaleLabels();
        }

        removeProximityHighlighting() {
            this.proximityHexSet = null;
            this.store.proximityHighlightedAircraft = new Set();
            this.updateVisualState();
            this.refreshStaleLabels();
        }

        getPerformanceStats() {
            const rendererStats = this._aircraftRenderer && typeof this._aircraftRenderer.getStats === 'function'
                ? this._aircraftRenderer.getStats()
                : null;

            const avgSingleAircraftUpdateMs = this._stats.singleAircraftUpdates > 0
                ? this._stats.singleAircraftUpdateDurationMs / this._stats.singleAircraftUpdates
                : 0;

            return {
                engine: 'openlayers',
                windowSec: 1,
                refreshPerSec: this._stats.refreshCalls,
                avgRefreshMs: 0,
                coalescedPct: 0,
                markerOpsPerSec: 0,
                initCalls: this._stats.initCalls,
                refreshCalls: this._stats.refreshCalls,
                singleUpdatesPerSec: this._stats.singleAircraftUpdates,
                singleAircraftAvgUpdateMs: Number(avgSingleAircraftUpdateMs.toFixed(3)),
                visualSkipPct: 0,
                trailLayerOpsPerSec: 0,
                fullVisibilityPasses: 0,
                rendererBackend: rendererStats?.backend || 'none',
                rendererFeatureCount: rendererStats?.featureCount || 0,
                rendererUpserts: rendererStats?.upserts || 0,
                rendererRemoves: rendererStats?.removes || 0,
                rendererBulkSyncs: rendererStats?.bulkSyncs || 0,
                rendererAvgUpdateMs: rendererStats?.avgUpdateMs || 0,
                trailFeatureCount: this._stats.trailFeatureCount || 0,
                overlayCount: Array.isArray(this._overlayStatuses) ? this._overlayStatuses.length : 0,
                overlayDegradedCount: Array.isArray(this._overlayStatuses)
                    ? this._overlayStatuses.filter((overlay) => overlay.degraded).length
                    : 0,
            };
        }

        setOverlayVisibility(overlayId, visible) {
            if (!this._overlayRegistry || typeof this._overlayRegistry.setVisible !== 'function') return;
            this._overlayRegistry.setVisible(overlayId, !!visible);
            this._overlayStatuses = this._overlayRegistry.getStatuses();
            if (this._overlayRegistry.updateZoomVisibility) {
                this._overlayRegistry.updateZoomVisibility(this.getZoom());
            }
        }

        setOverlayOpacity(overlayId, opacity) {
            if (!this._overlayRegistry || typeof this._overlayRegistry.setOpacity !== 'function') return;
            this._overlayRegistry.setOpacity(overlayId, opacity);
            this._overlayStatuses = this._overlayRegistry.getStatuses();
        }

        setLayerOpacity(layerName, opacity) {
            const value = Math.max(0, Math.min(1, Number.isFinite(opacity) ? opacity : 1));

            if (layerName === 'rangeRings') {
                if (this._rangeRingLayerState?.layer) {
                    this._rangeRingLayerState.layer.setOpacity(value);
                    this._scheduleReferenceRender();
                }
                return;
            }

            const referenceState = this._referenceLayerState?.[layerName];
            if (referenceState?.layer) {
                referenceState.layer.setOpacity(value);
                this._scheduleReferenceRender();
                return;
            }

            if (this._overlayRegistry && typeof this._overlayRegistry.setOpacity === 'function') {
                this._overlayRegistry.setOpacity(layerName, value);
                this._overlayStatuses = this._overlayRegistry.getStatuses();
            }
        }

        getOverlayStatuses() {
            if (!this._overlayRegistry || typeof this._overlayRegistry.getStatuses !== 'function') {
                return [];
            }
            this._overlayStatuses = this._overlayRegistry.getStatuses();
            return this._overlayStatuses;
        }

        cleanup() {
            if (this._interactionCleanup) {
                this._interactionCleanup();
                this._interactionCleanup = null;
            }
            this.stopLabelRefreshTimer();
            this.cleanupTracksMiniMap();
            const mapRef = this._olMap;
            if (this._aircraftRenderer && typeof this._aircraftRenderer.dispose === 'function') {
                this._aircraftRenderer.dispose();
            }
            this._aircraftRenderer = null;
            if (this._trailLayer && mapRef) {
                mapRef.removeLayer(this._trailLayer);
            }
            if (this._trailSource) {
                this._trailSource.clear();
            }
            this._trailSource = null;
            this._trailLayer = null;
            this._trailFeaturesByHex.clear();
            if (this._overlayLayer && mapRef) {
                mapRef.removeLayer(this._overlayLayer);
            }
            if (this._overlaySource) {
                this._overlaySource.clear();
            }
            this._overlaySource = null;
            this._overlayLayer = null;
            this._positionHighlightFeature = null;
            this._proximityCircleFeature = null;
            Object.keys(this._referenceLayerState).forEach((layerName) => {
                const state = this._referenceLayerState[layerName];
                if (state?.layer && mapRef) {
                    mapRef.removeLayer(state.layer);
                }
                if (state?.source) {
                    state.source.clear();
                }
            });
            this._referenceLayerState = {};
            if (this._rangeRingLayerState?.layer && mapRef) {
                mapRef.removeLayer(this._rangeRingLayerState.layer);
            }
            if (this._rangeRingLayerState?.source) {
                this._rangeRingLayerState.source.clear();
            }
            this._rangeRingLayerState = null;
            this._referenceRenderScheduled = false;
            if (this._overlayRegistry && typeof this._overlayRegistry.dispose === 'function') {
                this._overlayRegistry.dispose();
            }
            this._overlayRegistry = null;
            this._overlayStatuses = [];
            if (this._referencePopupOverlay && mapRef) {
                mapRef.removeOverlay(this._referencePopupOverlay);
            }
            this._referencePopupOverlay = null;
            this._referencePopupElement = null;
            if (this.engine) {
                this.engine.dispose();
                this.engine = null;
            }
            this.map = null;
            this._olMap = null;
            this.markers = {};
            this.trails = {};
            this.proximityHexSet = null;
            this.proximityRefHex = null;
            this.proximityDistanceNM = null;
            this.proximityCircle = null;
        }
    }

    function createOpenLayersMapManager(store, CONFIG) {
        return new OpenLayersMapManager(store, CONFIG);
    }

    window.OpenLayersMapManager = {
        createOpenLayersMapManager,
    };
})();
