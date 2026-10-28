// The published routes on the map (ProcedureRoute, Q56): the route a
// procedure chip asks for, drawn until it is closed and said in the sky's
// note, and the Procedures layer, off by default, for the runways in use.
// The words and choices are map/procedure-routes.js's (window.ProcedureRoutes);
// the drawing is map/features/procedure-feature.js's.
//
// One domain of the Alpine store 'atc' (docs-fr/32, palier 5.1); see
// store/station.js for how the domains are assembled.
(function () {
    window.AtcStore = window.AtcStore || [];
    window.AtcStore.push(() => ({
        // The whole routes of the followed airports (GET /procedure-routes),
        // and whether they have been asked for yet.
        procedureRoutes: [],
        procedureRoutesLoaded: false,
        // The route a chip asked for: { route, runways, note, help }, or null.
        askedRoute: null,

        async loadProcedureRoutes() {
            try {
                const r = await fetch(`${API_BASE_URL}/procedure-routes`, { credentials: 'same-origin' });
                if (!r.ok) return;
                const data = await r.json();
                this.procedureRoutes = Array.isArray(data.routes) ? data.routes : [];
                this.procedureRoutesLoaded = true;
                this.drawProcedureLayer();
            } catch (e) { /* the chips show their point alone */ }
        },

        // The route a place chip names, when the tables have it whole.
        routeForPlace(p) {
            const R = window.ProcedureRoutes;
            if (!R || !p || !p.designator) return null;
            return R.find(this.procedureRoutes, p.designator, this.followedAirports);
        },

        // What a chip's help ends with: its route, or its point and why.
        placeHelpEnd(p) {
            if (this.routeForPlace(p)) return 'Click to see its route on the map.';
            const covered = (this.followedAirports || []).some((a) => window.ProcedureRoutes?.covered(a.code));
            if (this.procedureRoutes.length && covered && p.designator) {
                return `${p.designator} is not in this cycle's tables: its route is not drawn. Click to see ${p.point} on the map.`;
            }
            return `Click to see ${p.point} on the map.`;
        },

        // A chip pressed: its route when it is known, its point otherwise.
        showRoute(p) {
            const route = this.routeForPlace(p);
            if (!route) {
                this.showPlace(p);
                return;
            }
            const R = window.ProcedureRoutes;
            const airport = (this.followedAirports || []).find((a) => a.code === route.airport);
            const runways = R.runwaysFor(route, R.inUse(airport));
            if (this.mainView !== 'map' && typeof this.setMainView === 'function') this.setMainView('map');
            this.askedRoute = { route, runways, note: R.note(route, runways), help: R.help(route, runways) };
            this.mapManager?.showProcedureRoute(route, runways);
        },

        closeRoute() {
            this.askedRoute = null;
            this.mapManager?.clearProcedureRoute();
        },

        // The layer's row: its state and words, for the runways in use now.
        get procedureLayer() {
            const R = window.ProcedureRoutes;
            if (!R) return { state: 'unknown', text: '', routes: [] };
            return R.layer(this.procedureRoutes, this.followedAirports, this.settings.proceduresKind === 'SID' ? 'SID' : 'STAR',
                this.settings.proceduresScope === 'all' ? 'all' : 'inUse');
        },

        drawProcedureLayer() {
            const R = window.ProcedureRoutes;
            if (!this.mapManager || !R) return;
            const on = !!this.settings.showProcedures;
            const routes = on ? this.procedureLayer.routes : [];
            const ends = (code) => R.inUse((this.followedAirports || []).find((a) => a.code === code));
            // Every runway: each route said with all its runways.
            const all = this.settings.proceduresScope === 'all';
            this.mapManager.setProcedureLayer(routes, (r) => all ? (r.runways || []) : R.runwaysFor(r, ends(r.airport)));
        },

        toggleProcedures() {
            this.saveSettings();
            this.drawProcedureLayer();
        },

        setProceduresScope(scope) {
            this.settings.proceduresScope = scope === 'all' ? 'all' : 'inUse';
            this.saveSettings();
            this.drawProcedureLayer();
        },

        setProceduresKind(kind) {
            this.settings.proceduresKind = kind === 'SID' ? 'SID' : 'STAR';
            this.saveSettings();
            this.drawProcedureLayer();
        },
    }));
})();
