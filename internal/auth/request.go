package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TrustedProxies decides whether the X-Forwarded-* headers of a request may be
// believed.
//
// This exists because of a trap worth naming. Behind a reverse proxy, co-atc is
// spoken to in plain HTTP even though the browser is on HTTPS: without reading
// X-Forwarded-Proto it would set a cookie without the Secure flag and refuse
// passkeys. But a forwarded header believed from anyone is a way in -- any client
// on the network could declare itself already on HTTPS, or claim someone else's
// address to escape rate limiting. So the list is explicit and empty by default:
// with no trusted proxy configured, no forwarded header is read at all.
type TrustedProxies struct {
	nets []*net.IPNet
}

func NewTrustedProxies(cidrs []string) (*TrustedProxies, error) {
	t := &TrustedProxies{}
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			if ip := net.ParseIP(c); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				c = c + "/" + itoa(bits)
			}
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, err
		}
		t.nets = append(t.nets, n)
	}
	return t, nil
}

func itoa(i int) string {
	if i == 32 {
		return "32"
	}
	return "128"
}

func (t *TrustedProxies) trusts(addr string) bool {
	if t == nil || len(t.nets) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range t.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Scheme reports the scheme the browser actually used: what co-atc is serving,
// unless a trusted proxy says otherwise.
func (t *TrustedProxies) Scheme(r *http.Request) string {
	if t.trusts(r.RemoteAddr) {
		if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
			return strings.ToLower(strings.TrimSpace(strings.Split(p, ",")[0]))
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// Forwarded says that a request came through a trusted proxy: the browser
// is not talking to co-atc directly, and reaches only the address the proxy
// serves -- not the other ports co-atc listens on.
func (t *TrustedProxies) Forwarded(r *http.Request) bool {
	if !t.trusts(r.RemoteAddr) {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
}

// ClientIP is the address to hold responsible for a request -- for rate limiting,
// and for saying in a log who failed to sign in.
//
// X-Forwarded-For is read from the right, where each proxy appends the address
// it was spoken to from: past the addresses of trusted proxies, the first one
// that is not is the browser as the outermost trusted proxy saw it. Taking the
// last address alone was right behind a single proxy and wrong behind two --
// a CDN in front of Traefik made every visitor the CDN's address, so eight
// wrong passwords from anyone locked everyone out. Every X-Forwarded-For line
// is read, in order: a proxy that adds its own line rather than appending to
// the client's would otherwise leave the client's invented one first.
func (t *TrustedProxies) ClientIP(r *http.Request) string {
	if t.trusts(r.RemoteAddr) {
		if f := strings.Join(r.Header.Values("X-Forwarded-For"), ","); strings.TrimSpace(f) != "" {
			parts := strings.Split(f, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				if p := strings.TrimSpace(parts[i]); p != "" && (i == 0 || !t.trusts(p)) {
					return p
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limitKey is what the sign-in limit counts under: the address, and for IPv6
// its /64. An IPv6 subscriber, phone or server holds a whole /64 and can
// take a new address from it for every guess; counted per address, it could
// guess without end. A /64 is one subscriber, so nobody else is caught with
// it. What does not parse as an address is its own key.
func limitKey(addr string) string {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return addr
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// Limiter slows down password guessing.
//
// Per client address, with a window rather than a token bucket: the useful property
// is not smooth throughput but a hard stop after a handful of wrong answers, and a
// window says that in a way anyone reading it can check.
type Limiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
	swept    time.Time        // the last time expired addresses were forgotten
	now      func() time.Time // time.Now; a test's clock
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{attempts: map[string][]time.Time{}, max: max, window: window, now: time.Now}
}

// Allow records an attempt and reports whether it may proceed.
func (l *Limiter) Allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(now)

	var kept []time.Time
	for _, t := range l.attempts[key] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.attempts[key] = kept
		return false
	}
	l.attempts[key] = append(kept, now)
	return true
}

// Wait is how long until key may try again: zero when it may now, else
// until enough of its attempts are older than the window.
func (l *Limiter) Wait(key string) time.Duration {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	var kept []time.Time
	for _, t := range l.attempts[key] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) < l.max {
		return 0
	}
	return kept[len(kept)-l.max].Add(l.window).Sub(now)
}

// sweepLocked forgets, at most once per window, the addresses whose last
// attempt is older than the window. An address was otherwise kept for the
// life of the process once it had tried, unless it later got the password
// right: every visitor who ever mistyped, and every address a guesser drew
// for one attempt, stayed in memory -- without bound on a public server.
func (l *Limiter) sweepLocked(now time.Time) {
	if now.Sub(l.swept) < l.window {
		return
	}
	l.swept = now
	for key, times := range l.attempts {
		if len(times) == 0 || now.Sub(times[len(times)-1]) >= l.window {
			delete(l.attempts, key)
		}
	}
}

// Succeeded clears the count after a correct password, so a user who fumbled twice
// and then got it right is not left one mistake from a lockout.
func (l *Limiter) Succeeded(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}
