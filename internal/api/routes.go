package api

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/reference"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/weather"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// Router is the API router
type Router struct {
	handler    *Handler
	middleware *Middleware
	config     *config.Config
	logger     *logger.Logger

	// The pages that may call this server and open its WebSocket; see
	// internal/auth/origin.go. Built by the first Routes(), which every
	// listener calls, once.
	originsOnce sync.Once
	origins     *auth.Origins
}

// NewRouter creates a new API router
func NewRouter(adsbService *adsb.Service, frequenciesService *frequencies.Service, weatherService *weather.Service, refService *reference.Service, config *config.Config, logger *logger.Logger, wsServer *websocket.Server, transcriptionStorage *sqlite.TranscriptionStorage, clearanceStorage *sqlite.ClearanceStorage) *Router {
	return &Router{
		handler:    NewHandler(adsbService, frequenciesService, weatherService, refService, config, logger, wsServer, transcriptionStorage, clearanceStorage),
		middleware: NewMiddleware(logger),
		config:     config,
		logger:     logger.Named("api-router"),
	}
}

// newOrigins is the server itself, on every port it listens on, and the pages
// server.allowed_origins lists. The configuration has checked those already;
// one that still does not parse is fatal, like any other failure to build who
// may ask -- a server must not come up answering more pages than it says.
func newOrigins(cfg *config.Config, log *logger.Logger) *auth.Origins {
	if cfg == nil {
		o, _ := auth.NewOrigins("", nil, nil)
		return o
	}
	ports := append([]int{cfg.Server.Port}, cfg.Server.AdditionalPorts...)
	o, err := auth.NewOrigins(cfg.Server.Host, ports, cfg.Server.AllowedOrigins)
	if err != nil {
		log.Error(fmt.Sprintf("Failed to build the allowed origins: %v", err))
		os.Exit(1)
	}
	return o
}

// Routes returns the API routes
func (r *Router) Routes() http.Handler {
	router := chi.NewRouter()
	r.originsOnce.Do(func() {
		r.origins = newOrigins(r.config, r.logger)
		// The same pages for the WebSocket, plus the clients that send no
		// Origin, which are not browsers and carry no borrowed cookie.
		if r.handler != nil && r.handler.wsServer != nil {
			r.handler.wsServer.SetOriginCheck(r.origins.AllowedOrAbsent)
		}
	})

	// Middleware
	router.Use(r.middleware.RequestID)
	router.Use(r.middleware.Logger)
	router.Use(r.middleware.Recoverer)
	router.Use(r.middleware.CORS(r.origins))
	// gzip for text (audit 4.3): the page's scripts, the airport layout and the
	// aircraft list are the bulk of what a page loads -- 1.2 MB of the 1.76
	// measured on 11/10 -- and were sent uncompressed. Audio (audio/wav) and the
	// WebSocket are not among the types; chi's writer passes Flush and Hijack
	// through, so the stream and the socket are untouched.
	router.Use(chimw.Compress(5, "text/html", "text/css", "text/javascript", "application/javascript",
		"application/json", "application/geo+json", "image/svg+xml", "text/plain"))

	// API routes
	router.Route("/api/v1", func(router chi.Router) {
		// Reachable without a session: the login form must be able to ask whether
		// anyone needs to sign in, and health has to answer a probe that has no
		// credentials to offer.
		router.Get("/health", r.handler.GetHealth)
		// The launcher's health: this machine without a session, or the
		// owner's session; 403 to anyone else. See health_detail_handlers.go.
		router.Get("/health/detail", r.handler.GetHealthDetail)
		router.Get("/auth/status", r.handler.GetAuthStatus)
		router.Post("/auth/login", r.handler.PostLogin)
		router.Post("/auth/logout", r.handler.PostLogout)

		// First run: the page must be able to ask what is needed before anyone
		// can sign in, and to answer it once.
		router.Get("/setup/status", r.handler.GetSetupStatus)
		router.Post("/setup", r.handler.PostSetup)

		// The station's vital signs on the sign-in page, before anyone signs in:
		// aggregated figures only, computed at most every 10 s, and a 404 when
		// the settings panel turns them off. See vitals_handlers.go.
		router.Get("/vitals", r.handler.GetVitals)

		// Everything below carries data from the receiver. A Group is a mux of
		// its own, which is what lets its middleware be declared after the public
		// routes above -- chi refuses a Use() that follows a route on the same mux.
		router.Group(func(router chi.Router) {
			// A session, or the key of the station link co-atc started, on the
			// link's routes alone; see link_token.go.
			router.Use(r.handler.RequireSignIn)
			// A viewer account reads, and is not given the audio; see
			// auth_handlers.go.
			router.Use(r.handler.ViewerGuard)

			// Aircraft routes
			router.Get("/aircraft", r.handler.GetAllAircraft)
			router.Get("/aircraft/{id}", r.handler.GetAircraftByHex)
			router.Get("/aircraft/{id}/tracks", r.handler.GetAircraftTracks)

			// Frequency routes
			router.Get("/frequencies", r.handler.GetAllFrequencies)
			router.Get("/frequencies/{id}", r.handler.GetFrequencyByID)
			// What a frequency is currently carrying, when that is not what the
			// configuration says -- a mixed stream's contents can change while the
			// server runs. See internal/frequencies/label.go.
			router.Put("/frequencies/{id}/label", r.handler.PutFrequencyLabel)
			// Streams added and removed while the server runs, for a station that
			// publishes one per channel. See internal/frequencies/runtime_sources.go.
			router.Put("/sources/{id}", r.handler.PutSource)
			router.Delete("/sources/{id}", r.handler.DeleteSource)

			// Audio stream route
			router.Get("/stream/{id}", r.handler.StreamAudio)
			router.Head("/stream/{id}", r.handler.StreamAudio) // Add support for HEAD requests

			// WebSocket route
			router.Get("/ws", r.handler.HandleWebSocket)

			// Transcription routes
			router.Get("/transcriptions", r.handler.GetAllTranscriptions)
			router.Get("/transcriptions/frequency/{id}", r.handler.GetTranscriptionsByFrequency)
			router.Get("/transcriptions/{id}/audio", r.handler.GetTranscriptionAudio)
			router.Get("/status", r.handler.GetStatus)
			router.Put("/receiver", r.handler.PutReceiver)
			router.Get("/transcriptions/time-range", r.handler.GetTranscriptionsByTimeRange)
			router.Get("/transcriptions/speaker/{type}", r.handler.GetTranscriptionsBySpeaker)
			router.Get("/transcriptions/callsign/{callsign}", r.handler.GetTranscriptionsByCallsign)

			// Configuration
			router.Get("/config", r.handler.GetConfig)

			// How the server is actually running: database growth, disk, retention,
			// log level. See internal/api/server_handlers.go.
			router.Get("/server", r.handler.GetServerState)
			// Writing is behind the same group, so it needs a session whenever
			// authentication is on -- retention decides when data is deleted.
			router.Put("/server/settings", r.handler.PutServerSettings)
			// Local-only or sign-in, switched from the same panel. In this group so
			// that turning sign-in off takes a session whenever it is on.
			router.Put("/access", r.handler.PutAccess)
			// Who signed in, who was refused, from where: the owner's only
			// (ownerRoutes, auth_handlers.go).
			router.Get("/signins", r.handler.GetSignIns)
			// The list of official names around the station (Q55): its state, and
			// making it again, which reaches the SIA's site.
			router.Get("/aip-names", r.handler.GetAIPNames)
			router.Post("/aip-names/refresh", r.handler.PostAIPNamesRefresh)
			// The published routes of the followed airports, for the map (Q56).
			router.Get("/procedure-routes", r.handler.GetProcedureRoutes)
			router.Get("/adsb/source", r.handler.GetADSBSourceStatus)

			// Station Configuration
			router.Get("/station", r.handler.GetStationConfig) // New route for station config

			// Weather Data
			router.Get("/wx", r.handler.GetWeatherData)

			// Reference data (airports, navaids, runways)
			router.Get("/airports", r.handler.GetAirports)
			router.Get("/airports/{ident}", r.handler.GetAirportByIdent)
			router.Get("/heliports", r.handler.GetHeliports)
			router.Get("/navaids", r.handler.GetNavaids)
			router.Get("/navaids/{ident}", r.handler.GetNavaidByIdent)
			router.Get("/runways", r.handler.GetRunways)
		})
	})

	// The page, from server.static_dir, or www/ next to the working directory.
	staticDir := "www"
	if r.config != nil && r.config.Server.StaticDir != "" {
		staticDir = r.config.Server.StaticDir
	}
	staticHandler := NewStaticFileHandler(staticDir, r.logger)
	router.Handle("/*", staticHandler)

	return router
}

// Handler exposes the router's handler so the caller can attach what only it knows
// -- the live settings and the database file actually opened at start.
func (r *Router) Handler() *Handler { return r.handler }
