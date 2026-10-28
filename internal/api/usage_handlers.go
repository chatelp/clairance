package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/usage"
	"github.com/yegors/co-atc/internal/websocket"
)

// The owner's view of what each account did (D84, 27/10): the pages it had
// open, the sound it heard, the clips it played again, and how each ended.
// Recorded only for a signed-in account; read by the owner alone (ownerRoutes).

// openUsage opens the journal beside the accounts, for the days the
// configuration keeps; with no accounts file, in memory only.
func openUsage(a *auth.Service, days int) *usage.Journal {
	path := ""
	if f := a.AccountsFile(); f != "" {
		path = filepath.Join(filepath.Dir(f), usage.File)
	}
	return usage.Open(path, days)
}

// pagePresence tells the journal of each page a signed-in account opens, and
// why it went, in the words of internal/websocket.
func pagePresence(j *usage.Journal) websocket.Presence {
	if j == nil {
		return nil
	}
	return func(user string) func(string) {
		it := j.Begin(user, usage.KindPage, "")
		return func(reason string) { it.End(pageEnded(reason), 0) }
	}
}

// pageEnded: a page that said goodbye -- 1000, 1001 going away, 1005 no
// status, what a reload or a closed tab sends -- was closed; one whose
// connection broke without a word (1006, a read that failed) or stopped
// answering pings was lost. The words are internal/websocket's readPump. The
// owner's cut is marked before the page learns it (usage.Journal.Cut).
func pageEnded(reason string) string {
	for _, code := range []string{"(1000)", "(1001)", "(1005)"} {
		if strings.HasPrefix(reason, "the page closed it ") && strings.HasSuffix(reason, code) {
			return usage.EndClosed
		}
	}
	return usage.EndLost
}

// soundTracked records a live stream for the account asking, if any: the
// writer it must write through, and what to call when it is over.
func (h *Handler) soundTracked(w http.ResponseWriter, r *http.Request, freq string) (http.ResponseWriter, func()) {
	sess, ok := h.signedIn(r)
	if !ok || h.usage == nil {
		return w, func() {}
	}
	it := h.usage.Begin(sess.User, usage.KindSound, freq)
	cw := &countingWriter{ResponseWriter: w}
	return cw, func() {
		if cw.n == 0 {
			it.Forget() // refused, or nothing to hear: not a listening
			return
		}
		why := usage.EndSource
		if cw.err != nil || r.Context().Err() != nil {
			why = usage.EndClosed // the page went, or was cut off (Cut says so)
		}
		it.End(why, cw.n)
	}
}

// countingWriter counts what reaches the client, and passes the flushes on.
type countingWriter struct {
	http.ResponseWriter
	n   int64
	err error
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.n += int64(n)
	if err != nil && c.err == nil {
		c.err = err
	}
	return n, err
}

func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Stopping is said by main when the server is about to stop: what is still
// open is written as ended by the stop, before the shutdown closes it.
func (h *Handler) Stopping() {
	h.usage.Stop()
}

// GetUsage is the owner's panel of usage: who is here now, and each
// account's last 7 days.
func (h *Handler) GetUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rep := h.usage.Report(time.Local)
	WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":  h.usage != nil,
		"days":     rep.Days,
		"now":      rep.Now,
		"accounts": rep.Accounts,
	})
}
