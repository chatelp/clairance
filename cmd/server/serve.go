package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/api"
	"github.com/yegors/co-atc/internal/companion"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/weather"
	"github.com/yegors/co-atc/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// serve refuses to open a server nobody can lock, then starts one HTTP server
// per configured port, each in its own goroutine, and returns them for
// shutdown.
func serve(cfg *config.Config, router *api.Router, log *logger.Logger) []*http.Server {
	// --- Setup for multiple HTTP servers ---
	var servers []*http.Server
	allPorts := []int{cfg.Server.Port}       // Start with the primary port
	if len(cfg.Server.AdditionalPorts) > 0 { // Only append if there are additional ports
		allPorts = append(allPorts, cfg.Server.AdditionalPorts...)
	}

	// No account anywhere. On loopback that is a legitimate state and the setup
	// page handles it; bound to anything else it is a server about to serve the
	// receiver's data to whoever asks, which is precisely what upstream's README
	// warns against. Refusing to start is the only honest answer -- a warning
	// scrolls past and the server stays open.
	if router.Handler().Auth().NeedsSetup() {
		if !isLoopbackHost(cfg.Server.Host) {
			fatalLog := log.WithOptions(zap.AddStacktrace(zapcore.PanicLevel))
			fatalLog.Fatal("Refusing to start: no account exists and the server is not bound to loopback",
				logger.String("host", cfg.Server.Host),
				logger.String("fix", "set server.host to 127.0.0.1 and open the setup page, or create an account with: co-atc -add-user <name>"))
		}
		log.Warn("No account configured -- open the setup page to choose local-only or an account",
			logger.String("url", fmt.Sprintf("http://%s:%d/", cfg.Server.Host, cfg.Server.Port)))
	}

	log.Info("Configured listener ports", logger.Any("ports", allPorts))

	// Start a server for each configured port
	for _, port := range allPorts {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, port)
		server := httpServer(cfg, addr, router.Routes()) // All servers use the same main router
		servers = append(servers, server)

		go func(s *http.Server) {
			// TLS served by co-atc itself when a certificate is configured, and by
			// nothing when it is not. Behind a reverse proxy both stay empty and the
			// proxy terminates TLS -- neither shape is the privileged one.
			if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
				log.Info("Starting HTTPS server", logger.String("addr", s.Addr))
				if err := s.ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey); err != nil && err != http.ErrServerClosed {
					log.Error("HTTPS server error on startup", logger.String("addr", s.Addr), logger.Error(err))
				}
				return
			}
			log.Info("Starting HTTP server", logger.String("addr", s.Addr))
			if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("HTTP server error on startup", logger.String("addr", s.Addr), logger.Error(err))
				// If one server fails to start, log the error. Depending on requirements,
				// you might want to os.Exit(1) here or implement more complex error handling.
			}
		}(server)
	}
	return servers
}

// readHeaderTimeout bounds how long a client may take to send a request's
// headers. Without it, and with read_timeout_seconds at its default of 0, a
// client that trickles its headers a byte at a time held a connection and a
// goroutine for as long as it liked -- on a server reached from the
// Internet, the cheapest way to exhaust it. Ten seconds is ages for a
// header; it says nothing about how long the answer may last, so the audio
// streams and the WebSocket are untouched (measured on 26/10: a stream of
// 2 s through a server with ReadHeaderTimeout, or ReadTimeout, of 0.5 s
// arrives whole; WriteTimeout is what cuts one). A variable for the test.
var readHeaderTimeout = 10 * time.Second

// httpServer is one listener: the configured timeouts, and the header
// timeout, which is not configurable.
func httpServer(cfg *config.Config, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSecs) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSecs) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeoutSecs) * time.Second,
	}
}

// shutdown stops the background services, then the HTTP servers, in that
// order. cancel is the context the services were started with. The station
// link goes first, so that it does not set sources on a server going away;
// the keep-awake assertion last, so that the Mac does not sleep mid-way.
func shutdown(log *logger.Logger, stationLink *companion.Process, weatherService *weather.Service, frequenciesService *frequencies.Service, sttSidecar *transcription.Sidecar, adsbService *adsb.Service, cancel context.CancelFunc, servers []*http.Server, keepAwake *companion.Process) {
	log.Info("Shutting down server...")

	stationLink.Stop()

	// Stop background services first
	log.Info("Stopping weather service...")
	weatherService.Stop()
	log.Info("Weather service stopped.")

	log.Info("Stopping frequencies service...")
	frequenciesService.Stop()
	log.Info("Frequencies service stopped.")

	// After the frequencies service: nothing will ask it to transcribe again.
	sttSidecar.Stop()

	// Stop any active transcription processors
	// This will be handled by the frequencies service when we integrate the transcription service

	log.Info("Stopping ADS-B service...")
	adsbService.Stop()
	log.Info("ADS-B service stopped.")

	// Cancel the main context
	cancel()

	// Shutdown all HTTP servers
	log.Info("Shutting down HTTP servers...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second) // Increased timeout slightly for multiple servers
	defer shutdownCancel()

	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(srv *http.Server) {
			defer wg.Done()
			log.Info("Attempting to shutdown HTTP server", logger.String("addr", srv.Addr))
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Error("HTTP server shutdown error", logger.String("addr", srv.Addr), logger.Error(err))
			} else {
				log.Info("HTTP server shutdown complete", logger.String("addr", srv.Addr))
			}
		}(s)
	}
	wg.Wait() // Wait for all server shutdowns to complete

	log.Info("All HTTP servers shutdown.")

	keepAwake.Stop()

	log.Info("Server fully stopped")
}

// isLoopbackHost reports whether a bind address can only be reached from this
// machine. An empty host means every interface, which is the one to get right.
func isLoopbackHost(host string) bool {
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}
