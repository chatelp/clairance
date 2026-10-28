package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/api"
	"github.com/yegors/co-atc/internal/companion"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/lifecycle"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/weather"
	"github.com/yegors/co-atc/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// serve refuses to open a server nobody can lock, then opens every configured
// port and serves each in its own goroutine. It returns the servers, for
// shutdown, and what each port did, for the launcher's health. A primary port
// that cannot be opened is an error: until 26/10 the server went on without
// it, and another co-atc holding the port answered for it -- the launcher took
// that one's health for this one's. An additional port that cannot be opened
// is said, and the others serve.
func serve(cfg *config.Config, router *api.Router, log *logger.Logger) ([]*http.Server, []api.Listener, error) {
	// --- Setup for multiple HTTP servers ---
	var servers []*http.Server
	var listeners []api.Listener
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
			lifecycle.Failed("no_account", fmt.Errorf("no account exists and the server is not bound to loopback (host %q)", cfg.Server.Host))
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
	for i, port := range allPorts {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, port)
		l := api.Listener{Addr: addr, URL: listenerURL(cfg, port), Primary: i == 0}
		// Opened here rather than in the goroutine, so that the server knows
		// it listens before it says it is ready.
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			l.Error = err.Error()
			listeners = append(listeners, l)
			if i == 0 {
				return servers, listeners, fmt.Errorf("listening on %s: %w", addr, err)
			}
			log.Error("HTTP server error on startup", logger.String("addr", addr), logger.Error(err))
			continue
		}
		l.OK = true
		listeners = append(listeners, l)
		server := httpServer(cfg, addr, router.Routes()) // All servers use the same main router
		servers = append(servers, server)

		go func(s *http.Server, ln net.Listener) {
			// TLS served by co-atc itself when a certificate is configured, and by
			// nothing when it is not. Behind a reverse proxy both stay empty and the
			// proxy terminates TLS -- neither shape is the privileged one.
			if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
				log.Info("Starting HTTPS server", logger.String("addr", s.Addr))
				if err := s.ServeTLS(ln, cfg.Server.TLSCert, cfg.Server.TLSKey); err != nil && err != http.ErrServerClosed {
					log.Error("HTTPS server error on startup", logger.String("addr", s.Addr), logger.Error(err))
				}
				return
			}
			log.Info("Starting HTTP server", logger.String("addr", s.Addr))
			if err := s.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Error("HTTP server error on startup", logger.String("addr", s.Addr), logger.Error(err))
			}
		}(server, ln)
	}
	return servers, listeners, nil
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
	return answeringServer(cfg, addr, handler, requests)
}

// answeringServer is httpServer with the requests it answers held in a: their
// contexts end when a stops them, and a can name the ones still running.
func answeringServer(cfg *config.Config, addr string, handler http.Handler, a *answering) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           a.wrap(handler),
		BaseContext:       func(net.Listener) context.Context { return a.ctx },
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

	// What is still being answered is told to end, rather than waited for:
	// an answer that never ends by itself held the stop for its 10 s on
	// 27/10 (15:37 and 20:13). Whatever still holds it after a second is
	// named, so that the next slow stop says what it was.
	requests.stop()
	named := make(chan struct{})
	go func() {
		select {
		case <-named:
		case <-time.After(time.Second):
			for _, a := range requests.still() {
				log.Warn("Still answering while the server stops", logger.String("request", a))
			}
		}
	}()
	defer close(named)

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

// requests is what the servers are answering; see answering.
var requests = newAnswering()

// answering holds the requests being answered: a context they all derive
// from, ended when the server stops, and what each is, for the log.
type answering struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	next   uint64
	open   map[uint64]answer
}

type answer struct {
	method, path, from string
	since              time.Time
}

func newAnswering() *answering {
	ctx, cancel := context.WithCancel(context.Background())
	return &answering{ctx: ctx, cancel: cancel, open: map[uint64]answer{}}
}

func (a *answering) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.next++
		id := a.next
		a.open[id] = answer{method: r.Method, path: r.URL.Path, from: r.RemoteAddr, since: time.Now()}
		a.mu.Unlock()
		defer func() {
			a.mu.Lock()
			delete(a.open, id)
			a.mu.Unlock()
		}()
		h.ServeHTTP(w, r)
	})
}

// stop ends the context of every request being answered, and of any that
// still comes in.
func (a *answering) stop() { a.cancel() }

// still names what is being answered, the oldest first: method, path, client
// and how long. A WebSocket, handed over to its own goroutines, is not one.
func (a *answering) still() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	open := make([]answer, 0, len(a.open))
	for _, x := range a.open {
		open = append(open, x)
	}
	sort.Slice(open, func(i, j int) bool { return open[i].since.Before(open[j].since) })
	out := make([]string, len(open))
	for i, x := range open {
		out[i] = fmt.Sprintf("%s %s from %s, for %s", x.method, x.path, x.from, time.Since(x.since).Round(time.Second))
	}
	return out
}
