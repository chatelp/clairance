package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// What a guest -- a viewer account, the friend who reaches the production
// from the Internet (Q62) -- is not told: where the house's things are. The
// station's streams and receiver are addresses on the owner's network, the
// database and the log are paths in the owner's home, and an error repeats
// whichever it failed on. A guest's page uses none of it: it plays the
// stream through stream_url, on the page's own address, and shows an error
// as "connection failed" when it has no text. The owner reads all of it.
//
// One table, by route as chi routed the request (like viewerMay), and the
// same for the WebSocket by message type: the JSON paths, "*" for every
// element of a list. testdata/guest.txt is where they were found
// (TestWhatAGuestReads), and says they are found nowhere else.
var guestHidden = map[string][]string{
	// The station's stream, and why it could not be read.
	"/api/v1/frequencies":      {"frequencies.*.url", "frequencies.*.last_error"},
	"/api/v1/frequencies/{id}": {"url", "last_error"},
	// Why the receiver could not be read: its URL.
	"/api/v1/adsb/source": {"aircraft.last_error", "receiver.last_error", "stats.last_error"},
	// The paths of the database and the log; the sidecar's error, which names
	// its address or the model's path; who else has an account.
	"/api/v1/server": {"storage.dir", "storage.active_db", "storage.archive.dir", "settings.db_archive.dir", "logging.file", "transcription.detail", "access.accounts"},
	"/api/v1/config": {"storage.sqlite_base_path"},
	// radio-ctl-sync's words when the station does not answer: its URL.
	"/api/v1/status": {"receiver.detail"},
	// Why the list of official names could not be made: a path, a page.
	"/api/v1/aip-names": {"last_error", "skipped"},
}

var guestHiddenMessages = map[string][]string{
	websocket.MessageTypeFrequencyStatus: {"error"},
}

// guestStreamsPerFrequency is how many streams of one frequency a guest
// account holds at once, in whatever format: the page that plays it, and a
// second while that page reloads -- the old stream ends only when the server
// sees the old page gone. The owner's Mac decodes and sends every stream,
// and a guest who opened dozens would take the owner's sound: until then
// the WAV stream was bounded at ten per frequency for everyone, and the
// MP3 and mu-law ones, what the page plays, not at all.
const guestStreamsPerFrequency = 2

// streamSlots counts the streams each guest account holds, by frequency.
type streamSlots struct {
	mu   sync.Mutex
	held map[[2]string]int // account, frequency -> streams open
}

// take holds one of the account's places on the frequency, or says none is
// left; release gives it back, once however often it is called.
func (s *streamSlots) take(account, frequency string) (release func(), ok bool) {
	k := [2]string{account, frequency}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[k] >= guestStreamsPerFrequency {
		return nil, false
	}
	if s.held == nil {
		s.held = map[[2]string]int{}
	}
	s.held[k]++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.held[k]--; s.held[k] <= 0 {
				delete(s.held, k)
			}
		})
	}, true
}

// guestStream holds a place for a guest's stream of a frequency, counted by
// account: two browsers signed in as the same guest share them. Anyone else
// is not counted.
func (h *Handler) guestStream(r *http.Request, frequency string) (release func(), ok bool) {
	if !h.guest(r) {
		return func() {}, true
	}
	sess, _ := h.auth.SessionFrom(r) // a guest is signed in
	return h.guestStreams.take(sess.User, frequency)
}

// guest says whether the request comes from a guest's session.
func (h *Handler) guest(r *http.Request) bool {
	return h.auth != nil && h.auth.Role(r) == auth.RoleViewer
}

// GuestView empties, in a guest's answer, what guestHidden names for the
// route. The answer is held, read, emptied and written again: a guest's only,
// on these routes only, which a page asks every few seconds at most. An
// answer that is not JSON -- an error in plain text -- goes out as it is.
func (h *Handler) GuestView(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.guest(r) {
			next.ServeHTTP(w, r)
			return
		}
		pattern := ""
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			pattern = rctx.RoutePattern()
		}
		paths := guestHidden[pattern]
		if len(paths) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		held := &heldResponse{header: w.Header(), status: http.StatusOK}
		next.ServeHTTP(held, r)
		body := held.body.Bytes()
		if strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			var v any
			dec := json.NewDecoder(bytes.NewReader(body))
			dec.UseNumber() // the numbers as written, not as float64 makes them
			if err := dec.Decode(&v); err != nil {
				// Not sent rather than sent unread: what it held is unknown.
				h.logger.Error("A guest's answer could not be read to empty it",
					logger.String("route", pattern), logger.Error(err))
				http.Error(w, "the answer could not be prepared", http.StatusInternalServerError)
				return
			}
			for _, p := range paths {
				v = emptied(v, strings.Split(p, "."))
			}
			var out bytes.Buffer
			if err := json.NewEncoder(&out).Encode(v); err != nil {
				http.Error(w, "the answer could not be prepared", http.StatusInternalServerError)
				return
			}
			body = out.Bytes()
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(held.status)
		_, _ = w.Write(body)
	})
}

// guestMessage is a WebSocket message as a guest's page receives it: a copy
// with what guestHiddenMessages names emptied, the same message otherwise.
// Every page is sent the same message, so it is never changed in place.
func guestMessage(m *websocket.Message) *websocket.Message {
	paths := guestHiddenMessages[m.Type]
	if len(paths) == 0 {
		return m
	}
	var data any = m.Data
	for _, p := range paths {
		data = emptied(data, strings.Split(p, "."))
	}
	out := *m
	out.Data, _ = data.(map[string]any)
	return &out
}

// emptied is v with the value at path emptied: a string becomes "", a list
// [], an object {}, anything else null. A path that is not there changes
// nothing; "*" is every element of a list. What lies on the path is copied,
// never changed in place.
func emptied(v any, path []string) any {
	if len(path) == 0 {
		switch v.(type) {
		case string:
			return ""
		case []any:
			return []any{}
		case map[string]any:
			return map[string]any{}
		default:
			return nil
		}
	}
	switch t := v.(type) {
	case map[string]any:
		x, ok := t[path[0]]
		if !ok {
			return v
		}
		out := make(map[string]any, len(t))
		for k, y := range t {
			out[k] = y
		}
		out[path[0]] = emptied(x, path[1:])
		return out
	case []any:
		if path[0] != "*" {
			return v
		}
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = emptied(x, path[1:])
		}
		return out
	}
	return v
}

// heldResponse keeps what a handler writes, for GuestView to read before it
// is sent. The headers are the real ones.
type heldResponse struct {
	header http.Header
	status int
	wrote  bool // the status is set: the first one counts, as on the wire
	body   bytes.Buffer
}

func (h *heldResponse) Header() http.Header { return h.header }

func (h *heldResponse) WriteHeader(status int) {
	if !h.wrote {
		h.status, h.wrote = status, true
	}
}

func (h *heldResponse) Write(b []byte) (int, error) {
	h.wrote = true
	return h.body.Write(b)
}
