package api

import (
	"encoding/json"
	"fmt"
	"github.com/yegors/co-atc/internal/aip"
	"github.com/yegors/co-atc/internal/audioarchive"
	"net/http"
	"os"
	"time"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/lifecycle"
	"github.com/yegors/co-atc/internal/reference"
	"github.com/yegors/co-atc/internal/startcheck"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/usage"
	"github.com/yegors/co-atc/internal/weather"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// Handler contains the API handlers
type Handler struct {
	adsbService          *adsb.Service
	frequenciesService   *frequencies.Service
	weatherService       *weather.Service
	refService           *reference.Service
	config               *config.Config
	logger               *logger.Logger
	wsServer             *websocket.Server
	transcriptionStorage *sqlite.TranscriptionStorage
	clearanceStorage     *sqlite.ClearanceStorage
	valueStorage         *sqlite.PhraseologyStorage // what the grammar recovered, see docs-fr/19
	audioArchive         *audioarchive.Archive      // the sidecar's clips, nil without one
	status               statusCache                // the receiver as reported, the sidecar as last read
	vitals               vitalsCache                // the sign-in page's figures, see vitals_handlers.go
	healthDetail         healthDetailCache          // the launcher's health, see health_detail_handlers.go
	guestStreams         streamSlots                // the streams each guest holds, see guest.go
	missing              []startcheck.Missing       // what the startup check found missing; see SetMissing
	process              processState               // what main says of the process, see process.go

	// Who is asking. See internal/auth and auth_handlers.go.
	auth *auth.Service
	// The audio streams each account has open, for the owner to end
	// (accounts_handlers.go).
	live liveStreams
	// What each account did, for the owner (usage_handlers.go, D84); nil
	// with auth.usage_days = 0.
	usage *usage.Journal
	// The key of the station link co-atc started; see link_token.go.
	linkToken string

	// Operational state for the settings panel. See server_handlers.go.
	runtime       *config.Runtime
	db            *sqlite.DB
	sttSidecar    *transcription.Sidecar
	startedAt     time.Time
	version       string    // of the binary, set at build time; see SetBuild
	pageVersion   string    // the page's fingerprint, see version.go
	builtAt       time.Time // when the server's commit was made
	commit        string
	dbSampleAt    time.Time
	dbSampleBytes int64

	// The list of official names around the station, made from the panel
	// (aip_handlers.go, Q55); nil without one.
	aip     *aip.Maker
	aipMade func(*aip.Names)
}

// NewHandler creates a new API handler
func NewHandler(adsbService *adsb.Service, frequenciesService *frequencies.Service, weatherService *weather.Service, refService *reference.Service, config *config.Config, logger *logger.Logger, wsServer *websocket.Server, transcriptionStorage *sqlite.TranscriptionStorage, clearanceStorage *sqlite.ClearanceStorage) *Handler {
	// The grammar's values live in the same daily database as the transcriptions
	// they came from.
	valueStorage := sqlite.NewPhraseologyStorage(sqlite.DBOf(transcriptionStorage))

	// Built here rather than attached afterwards: the router hangs its middleware
	// on this service while it is constructing the routes, so it has to exist by
	// then. A failure is fatal by design -- a server that cannot build its
	// authentication must not come up serving the data unprotected.
	// Accounts created from the first-run page live beside config.toml, which
	// the program never rewrites. A file that cannot be read is fatal rather
	// than ignored: silently starting without the accounts someone created is
	// how a server ends up open.
	userFile, err := auth.LoadUserFile(config.ConfigPath)
	if err != nil {
		lifecycle.Failed("auth", err)
		logger.Error(fmt.Sprintf("Failed to read accounts: %v", err))
		os.Exit(1)
	}

	authService, err := auth.NewService(auth.Config{
		Enabled:        config.Auth.Enabled,
		Users:          authUsers(config),
		UserFile:       userFile,
		SessionTTL:     time.Duration(orDefault(config.Auth.SessionTTLHours, 720)) * time.Hour,
		TrustedProxies: config.Server.TrustedProxies,
		Loopback:       isLoopback(config.Server.Host),
		MaxAttempts:    config.Auth.MaxAttempts,
		AttemptWindow:  time.Duration(orDefault(config.Auth.AttemptWindowMins, 15)) * time.Minute,
	})
	if err != nil {
		lifecycle.Failed("auth", err)
		logger.Error(fmt.Sprintf("Failed to build authentication: %v", err))
		os.Exit(1)
	}
	// Sessions are kept across restarts (fiche S). A file that cannot be read
	// costs a sign-in, not the server: it starts with none, and says so.
	if kept, dropped, err := authService.Sessions().Restored(); err != nil {
		logger.Warn(fmt.Sprintf("Sessions could not be read back, everyone signs in again; "+
			"the file is replaced at the next sign-in: %v", err))
	} else if kept+dropped > 0 {
		logger.Info(fmt.Sprintf("Sessions read back: %d kept, %d set aside (expired, or their account changed), from %s",
			kept, dropped, authService.Sessions().Path()))
	}
	if authService.LocalRefused() {
		// Chosen when the server was reachable from this machine only; it no
		// longer is. The accounts, if any, are in force -- and with none the
		// server refuses to start, see cmd/server/main.go.
		why := fmt.Sprintf("the server listens on %q", config.Server.Host)
		if isLoopback(config.Server.Host) {
			why = "a proxy is declared in server.trusted_proxies"
		}
		logger.Warn(fmt.Sprintf("Local-only access was chosen but is not honoured: %s, so other machines "+
			"can reach it and sign-in stays on (%s)", why, authService.AccountsFile()))
	}

	usageJournal := openUsage(authService, config.Auth.UsageDays)
	if wsServer != nil {
		wsServer.SetPresence(pagePresence(usageJournal))
	}

	return &Handler{
		auth:                 authService,
		usage:                usageJournal,
		valueStorage:         valueStorage,
		startedAt:            time.Now(),
		dbSampleAt:           time.Now(),
		adsbService:          adsbService,
		frequenciesService:   frequenciesService,
		weatherService:       weatherService,
		refService:           refService,
		config:               config,
		logger:               logger.Named("api-handler"),
		wsServer:             wsServer,
		transcriptionStorage: transcriptionStorage,
		clearanceStorage:     clearanceStorage,
		audioArchive:         newAudioArchive(config),
	}
}

// GetHealth returns the health status of the API
func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	lastFetch, status := h.adsbService.GetStatus()

	response := map[string]interface{}{
		"status":         status,
		"last_fetch":     lastFetch,
		"aircraft_count": len(h.adsbService.GetAllAircraft()),
	}

	WriteJSON(w, http.StatusOK, response)
}

// GetConfig returns the public configuration
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	// Create a sanitized config with only public values
	publicConfig := map[string]interface{}{
		"adsb": map[string]interface{}{
			"fetch_interval_seconds": h.config.ADSB.FetchIntervalSecs,
		},
		"storage": map[string]interface{}{
			"sqlite_base_path": h.config.Storage.SQLiteBasePath,
		},
		"frequencies": map[string]interface{}{
			"buffer_size_kb":          h.config.Frequencies.BufferSizeKB,
			"reconnect_interval_secs": h.config.Frequencies.ReconnectIntervalSecs,
		},
	}

	WriteJSON(w, http.StatusOK, publicConfig)
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// authUsers maps the accounts from the configuration file into the auth package,
// which knows nothing about TOML.
func authUsers(cfg *config.Config) []auth.User {
	out := make([]auth.User, 0, len(cfg.Auth.Users))
	for _, u := range cfg.Auth.Users {
		out = append(out, auth.User{Name: u.Name, PasswordHash: u.PasswordHash, Role: u.Role})
	}
	return out
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
