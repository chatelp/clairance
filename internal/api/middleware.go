package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/pkg/logger"
)

// Middleware contains custom middleware functions
type Middleware struct {
	logger *logger.Logger

	mu             sync.Mutex
	refusedOrigins map[string]bool // logged once each; see refused
}

// NewMiddleware creates a new middleware
func NewMiddleware(logger *logger.Logger) *Middleware {
	return &Middleware{
		logger:         logger.Named("api-middleware"),
		refusedOrigins: map[string]bool{},
	}
}

// Logger is a middleware that logs HTTP requests
func (m *Middleware) Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			m.logger.Debug("HTTP request",
				logger.String("method", r.Method),
				logger.String("path", r.URL.Path),
				logger.String("remote_addr", r.RemoteAddr),
				logger.String("user_agent", r.UserAgent()),
				logger.Int("status", ww.Status()),
				logger.Int("bytes", ww.BytesWritten()),
				logger.Duration("duration", time.Since(start)),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}

// CORS answers a browser that asks from another origin, for the pages the
// policy allows and for them only: their exact origin, never "*", with
// credentials, since the audio is fetched with the session cookie. An origin
// that is not allowed is given no CORS header at all rather than a 403 -- the
// browser then keeps the answer from the page, and a request that carries no
// Origin, which is every same-origin GET and every client that is not a
// browser, goes through untouched.
func (m *Middleware) CORS(origins *auth.Origins) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				// The answer depends on who asks: a cache must not hand one
				// page's answer to another.
				w.Header().Add("Vary", "Origin")
				if origins.Allowed(r) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					// Another origin reads only the safelisted headers: the
					// page's player, on an additional port, would miss the
					// stream's rate and play 16 kHz as 8.
					w.Header().Set("Access-Control-Expose-Headers", "X-Sample-Rate")
				} else {
					m.refused(origin)
				}
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// refused says once per origin that a page was refused, so that the owner of
// a page reached through a name the server cannot know -- a VPN, a proxy that
// rewrites the Host header -- finds in the log what to add to
// server.allowed_origins. Bounded: a stream of invented origins must not grow
// memory or the log.
func (m *Middleware) refused(origin string) {
	m.mu.Lock()
	first := !m.refusedOrigins[origin] && len(m.refusedOrigins) < 64
	if first {
		m.refusedOrigins[origin] = true
	}
	m.mu.Unlock()
	if first {
		m.logger.Warn("Refused a page from another origin; add it to server.allowed_origins if it is yours",
			logger.String("origin", origin))
	}
}

// RequestID is a middleware that adds a request ID to the context
func (m *Middleware) RequestID(next http.Handler) http.Handler {
	return middleware.RequestID(next)
}

// Recoverer is a middleware that recovers from panics
func (m *Middleware) Recoverer(next http.Handler) http.Handler {
	return middleware.Recoverer(next)
}
