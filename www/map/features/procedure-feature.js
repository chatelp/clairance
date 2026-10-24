/**
 * Module: map/features/procedure-feature
 * Why it exists:
 * - Draws the published routes on the map (ProcedureRoute, Q56): the one a
 *   procedure chip asks for, at full strength until it is closed, and the
 *   Procedures layer's, quieter, under it.
 *
 * Key responsibilities:
 * - A neutral line (color-map-procedure, never the altitude colours nor the
 *   accent), chevrons in the direction flown, the runway's designator at a
 *   departure's start, named points as hollow triangles with their name and
 *   constraints on the side away from the line; technical points (PG271…)
 *   not drawn, the line passes through them.
 *
 * Quirks / contracts:
 * - The words and the geometry's choices are window.ProcedureRoutes'
 *   (map/procedure-routes.js); this only draws.
 * - One vector layer at z 250: above the reference layers (airports 125,
 *   navaids 127), under the trails (300) and the aircraft (400), whose
 *   labels stay above the routes.
 */
(function () {
    const MONO = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';
    // The point names of the layer follow the labels' tiers: named up to
    // 67 NM of sky across, triangles alone beyond (aircraft-webgl.js).
    const NAMES_UP_TO_NM = 67;

    const ASKED = { alpha: 1, width: 1.6, chevronPx: 46, pointAlpha: 1, constraints: true };
    const LAYER = { alpha: 0.38, width: 1.1, chevronPx: 70, pointAlpha: 0.6, constraints: false };

    function create(manager) {
        const ol = window.ol;
        let layer = null;
        let source = null;
        let asked = [];
        let layered = [];
        let askedNames = new Set();
        let candidateNames = new Set();

        function ensure() {
            if (layer || !manager._olMap) return;
            source = new ol.source.Vector();
            layer = new ol.layer.Vector({ source, updateWhileAnimating: true, updateWhileInteracting: true });
            layer.setZIndex(250);
            manager._olMap.addLayer(layer);
        }

        const P = () => window.MapPalette;

        function skyWidthNM(resolution) {
            const map = manager._olMap;
            const size = map && map.getSize();
            const center = map && map.getView().getCenter();
            if (!size || !center) return 0;
            const lat = ol.proj.toLonLat(center)[1];
            return resolution * size[0] * Math.cos(lat * Math.PI / 180) / 1852;
        }

        // The line and its chevrons, every chevronPx pixels in the direction
        // flown.
        function lineStyle(mode) {
            return (feature, resolution) => {
                const color = P().withAlpha(P().color('procedure'), mode.alpha);
                const styles = [new ol.style.Style({ stroke: new ol.style.Stroke({ color, width: mode.width }) })];
                const coords = feature.getGeometry().getCoordinates();
                const step = mode.chevronPx * resolution;
                let carry = step / 2;
                for (let i = 0; i + 1 < coords.length; i++) {
                    const [x1, y1] = coords[i], [x2, y2] = coords[i + 1];
                    const len = Math.hypot(x2 - x1, y2 - y1);
                    const rot = -Math.atan2(y2 - y1, x2 - x1);
                    for (let d = carry; d < len; d += step) {
                        const t = d / len;
                        styles.push(new ol.style.Style({
                            geometry: new ol.geom.Point([x1 + (x2 - x1) * t, y1 + (y2 - y1) * t]),
                            text: new ol.style.Text({
                                text: '›', font: `600 ${mode === ASKED ? 13 : 11}px ${MONO}`, rotation: rot,
                                fill: new ol.style.Fill({ color }),
                            }),
                        }));
                        carry = d + step - len;
                    }
                    if (len < carry) carry -= len;
                }
                return styles;
            };
        }

        // A named point: a hollow triangle, its name and constraints on the
        // side away from the route.
        function pointStyle(mode) {
            return (feature, resolution) => {
                const p = feature.get('point');
                const color = P().withAlpha(P().color('procedure'), mode.pointAlpha);
                const styles = [];
                if (!p.runway) {
                    styles.push(new ol.style.Style({
                        image: new ol.style.RegularShape({
                            points: 3, radius: 5, angle: 0,
                            stroke: new ol.style.Stroke({ color, width: 1.2 }),
                            fill: new ol.style.Fill({ color: 'rgba(0,0,0,0)' }),
                        }),
                    }));
                }
                // The layer: every named point under 67 NM of sky; beyond, the
                // points a route ends at alone (an arrival ends 30 to 40 NM
                // out: its names never showed). Not those the route asked for
                // names at full strength.
                if (mode === LAYER && !feature.get('candidate')) {
                    if (skyWidthNM(resolution) > NAMES_UP_TO_NM && !feature.get('ends')) return styles;
                    if (askedNames.has(p.name) || candidateNames.has(p.name)) return styles;
                }
                const at = feature.getGeometry().getCoordinates();
                const px = (c) => c && [c[0] / resolution, -c[1] / resolution];
                const neighbours = feature.get('neighbours');
                const side = neighbours
                    ? window.ProcedureRoutes.widestGap(px(at), neighbours.map(px))
                    : window.ProcedureRoutes.labelSide(px(feature.get('prev')), px(at), px(feature.get('next')));
                const dx = side[0] * 9, dy = side[1] * 9;
                const align = side[0] > 0.3 ? 'left' : side[0] < -0.3 ? 'right' : 'center';
                const lines = [p.name].concat(mode.constraints ? p.constraints : []);
                // The constraints always under the name (ProcedureRoute); when
                // the label goes up, the whole block rises so that its last
                // line, not the name, is the one nearest the point.
                const up = side[1] < -0.3;
                lines.forEach((text, i) => {
                    const k = up ? i - (lines.length - 1) : i;
                    styles.push(new ol.style.Style({
                        text: new ol.style.Text({
                            text, textAlign: align, textBaseline: 'middle',
                            offsetX: dx, offsetY: dy + k * 11,
                            font: i === 0 ? `10px ${MONO}` : `9.5px ${MONO}`,
                            fill: new ol.style.Fill({ color: i === 0 ? color : P().color('text-tertiary') }),
                        }),
                    }));
                });
                return styles;
            };
        }

        function featuresOf(route, runways, mode, withPoints = true) {
            const out = [];
            const lines = window.ProcedureRoutes.lines(route, runways);
            lines.forEach((pts, li) => {
                const coords = pts.map((p) => manager._toMapCoordinate(p.lat, p.lon));
                const line = new ol.Feature({ geometry: new ol.geom.LineString(coords) });
                line.setStyle(lineStyle(mode));
                out.push(line);
                if (!withPoints) return;
                pts.forEach((p, i) => {
                    // The shared points are drawn once, with the first line.
                    if (li > 0 && !p.runway) return;
                    if (!p.named && !p.runway) return;
                    const f = new ol.Feature({ geometry: new ol.geom.Point(coords[i]) });
                    f.set('point', p);
                    f.set('prev', coords[i - 1] || null);
                    f.set('next', coords[i + 1] || null);
                    f.set('ends', i === pts.length - 1);
                    f.setStyle(pointStyle(mode));
                    out.push(f);
                });
            });
            return out;
        }

        function replace(list, next) {
            for (const f of list) source.removeFeature(f);
            for (const f of next) source.addFeature(f);
            return next;
        }

        // The route a chip asks for, until clear(); the view moves to it when
        // it is out of view.
        function show(route, runways) {
            ensure();
            if (!source) return;
            asked = replace(asked, featuresOf(route, runways, ASKED));
            askedNames = new Set((route.legs || []).filter((l) => l.named).map((l) => l.point));
            drawCandidates();
            if (layer) layer.changed();
            const map = manager._olMap;
            const extent = ol.extent.createEmpty();
            for (const f of asked) ol.extent.extend(extent, f.getGeometry().getExtent());
            const view = map.getView().calculateExtent(map.getSize());
            if (!ol.extent.containsExtent(view, extent)) {
                const instant = window.smoothScroll && window.smoothScroll() === 'auto';
                map.getView().fit(extent, { padding: [80, 80, 80, 80], duration: instant ? 0 : 400, maxZoom: map.getView().getZoom() });
            }
        }

        function clear() {
            if (source) asked = replace(asked, []);
            askedNames = new Set();
            drawCandidates();
            if (layer) layer.changed();
        }

        // The layer's routes, or none.
        // The layer's routes, or none: their lines, and each named point once
        // (a point twenty routes pass was named twenty times, on as many
        // sides, design review 16/10).
        function setLayer(routes, runwaysOf) {
            ensure();
            if (!source) return;
            const next = [];
            for (const r of routes || []) next.push(...featuresOf(r, runwaysOf(r), LAYER, false));
            for (const p of window.ProcedureRoutes.layerPoints(routes, runwaysOf)) {
                const f = new ol.Feature({ geometry: new ol.geom.Point(manager._toMapCoordinate(p.lat, p.lon)) });
                f.set('point', { name: p.name, named: true, constraints: [] });
                f.set('neighbours', p.neighbours.map(([la, lo]) => manager._toMapCoordinate(la, lo)));
                f.set('ends', p.ends);
                f.setStyle(pointStyle(LAYER));
                next.push(f);
            }
            layered = replace(layered, next);
        }

        // The selected departure's candidate exits (Q57): their lines, the
        // likeliest as a route asked for, the others as the layer's; their
        // points as one set, each named once, at full strength when the
        // likeliest passes it, at 60 % otherwise (design review 16/10). The
        // view does not move. A route asked for from a chip wins: while it is
        // shown the candidates are not drawn; they come back when it closes.
        let candidates = [];
        let candidateItems = [];
        function setCandidates(items) {
            candidateItems = items || [];
            drawCandidates();
        }

        function drawCandidates() {
            ensure();
            if (!source) return;
            const items = asked.length ? [] : candidateItems;
            const next = [];
            for (const it of items) next.push(...featuresOf(it.route, it.runways, it.strong ? ASKED : LAYER, false));
            const strong = items.find((it) => it.strong);
            const strongPoints = new Map();
            if (strong) {
                for (const pts of window.ProcedureRoutes.lines(strong.route, strong.runways)) {
                    for (const p of pts) if (p.named || p.runway) strongPoints.set(p.name, p);
                }
            }
            const runwaysOf = (r) => (items.find((it) => it.route === r) || {}).runways || [];
            for (const p of window.ProcedureRoutes.layerPoints(items.map((it) => it.route), runwaysOf)) {
                const onStrong = strongPoints.get(p.name);
                const f = new ol.Feature({ geometry: new ol.geom.Point(manager._toMapCoordinate(p.lat, p.lon)) });
                f.set('point', { name: p.name, named: true, constraints: onStrong ? onStrong.constraints : [] });
                f.set('neighbours', p.neighbours.map(([la, lo]) => manager._toMapCoordinate(la, lo)));
                f.set('ends', p.ends);
                f.set('candidate', true);
                f.setStyle(pointStyle(onStrong ? ASKED : LAYER));
                next.push(f);
            }
            // The runway the departure took off from, once, on the likeliest.
            if (strong) {
                const thr = [...strongPoints.values()].find((p) => p.runway);
                if (thr) {
                    const f = new ol.Feature({ geometry: new ol.geom.Point(manager._toMapCoordinate(thr.lat, thr.lon)) });
                    f.set('point', thr);
                    f.set('candidate', true);
                    f.setStyle(pointStyle(ASKED));
                    next.push(f);
                }
            }
            candidateNames = new Set(items.length ? next.filter((f) => f.get('candidate')).map((f) => f.get('point').name) : []);
            candidates = replace(candidates, next);
            if (layer) layer.changed();
        }

        // The palette changed (a bright base map): restyle.
        function refresh() {
            if (layer) layer.changed();
        }

        return { show, clear, setLayer, setCandidates, refresh };
    }

    window.MapProcedureFeature = { create };
})();
