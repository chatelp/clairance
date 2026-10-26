package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"

	"github.com/yegors/co-atc/internal/aip"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/websocket"
)

// What a guest -- a viewer account, the friend who reaches the production
// from the Internet (Q62) -- reads that says where the house's things are:
// an address on the owner's network, a path on the owner's disk, the
// station's own name. A guest's page needs none of it.
//
// The fixture puts such values wherever the server can carry them: every
// service it reads is on the loopback, as the station and the sidecar are on
// the owner's network; every file it writes is under its directory; the
// receiver's report names the station. Then the receiver stops answering, a
// source added from outside cannot be reached, the sidecar is not there, and
// the list of official names cannot be written -- the errors that repeat an
// address or a path.

// internalAddress is any IPv4 address: the fixture's services are all on the
// loopback, the station's report names 192.0.2.20.
var internalAddress = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)

// stationReport is what radio-ctl-sync says when it cannot reach the station.
const stationReport = `{"state":"unknown","label":"?","detail":"station not reached: Get \"http://station.lan/radio/etat\": dial tcp 192.0.2.20:80: connect: no route to host"}`

// carries says whether a value says where something of the house is.
func carries(dir, v string) bool {
	return internalAddress.MatchString(v) || strings.Contains(v, dir) || strings.Contains(v, "station.lan")
}

// leaks lists the places in a JSON value where a string carries an address:
// the path to it, with "*" for every element of a list.
func leaks(dir string, v any, at string, out map[string]bool) {
	switch t := v.(type) {
	case string:
		if carries(dir, t) {
			out[at] = true
		}
	case []any:
		for _, x := range t {
			leaks(dir, x, join(at, "*"), out)
		}
	case map[string]any:
		for k, x := range t {
			leaks(dir, x, join(at, k), out)
		}
	}
}

func join(at, k string) string {
	if at == "" {
		return k
	}
	return at + "." + k
}

// guestFixture is the production as the friend reaches it, with an address
// or a path wherever one can show.
type guestFixture struct {
	*stack
	owner, viewer *http.Cookie
	ownerWS       *gorilla.Conn
	viewerWS      *gorilla.Conn
}

func newGuestFixture(t *testing.T) *guestFixture {
	t.Helper()
	// A port nobody listens on: the sidecar that is not there, the source
	// that cannot be reached.
	closed := httptest.NewServer(http.NotFoundHandler())
	nobody := closed.URL
	closed.Close()

	s := publicStack(t, stackOptions{goMP3: true, sidecar: nobody, adsbEvery: 100 * time.Millisecond})
	s.cfg.Server.AudioReplay = config.AudioReplayAll // the friend's production
	s.cfg.Logging.File = filepath.Join(s.dir, "logs", "co-atc.log")
	g := &guestFixture{stack: s, owner: s.signInAs(testUser), viewer: s.signInAs(viewerUser)}

	// Both pages open the WebSocket before anything fails, and are known to
	// the hub before it says so.
	s.cookie = g.owner
	g.ownerWS = s.connectWS()
	s.cookie = g.viewer
	g.viewerWS = s.connectWS()
	for _, conn := range []*gorilla.Conn{g.ownerWS, g.viewerWS} {
		sendMessage(t, conn, websocket.MessageTypeAircraftBulkRequest, nil)
		waitForMessage(t, conn, websocket.MessageTypeAircraftBulkResponse, 5*time.Second)
	}
	time.Sleep(200 * time.Millisecond)

	// The station's report, as radio-ctl-sync sends it when the station
	// does not answer.
	s.cookie = g.owner
	if rec := s.do("PUT", "/api/v1/receiver", stationReport); rec.Code != http.StatusNoContent {
		t.Fatalf("the receiver's report: %d %s", rec.Code, rec.Body.String())
	}

	// A source added from outside, which cannot be reached; tried again
	// every second.
	s.cfg.Frequencies.ReconnectIntervalSecs = 1
	if err := s.freqs.AddSource(config.FrequencyConfig{ID: "dead", Name: "Unreachable", FrequencyMHz: 121.5,
		URL: nobody + "/aero-121500.mp3", Order: 9}); err != nil {
		t.Fatal(err)
	}

	// The receiver stops answering.
	s.tar1090.srv.Close()

	// The list of official names, made where it cannot be written.
	sia := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "FR-ENR-4.4-fr-FR.html") {
			w.Write([]byte(fakeENR44))
			return
		}
		w.Write([]byte(`<p>DE GAULLE Approche.</p>`))
	}))
	t.Cleanup(sia.Close)
	blocker := filepath.Join(s.dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	maker := &aip.Maker{OutPath: filepath.Join(blocker, "aip-names.json"), CacheDir: filepath.Join(s.dir, "aip-cache"),
		Client: &http.Client{Transport: toServer{sia.URL}}}
	s.handler.AttachAIP(maker, nil)
	if _, err := maker.Make(context.Background(), aip.Request{Lat: 49.0, Lon: 2.5, RadiusNM: AIPRadiusNM}); err == nil {
		t.Fatal("the list was written where it cannot be")
	}

	// Wait for the failures to be known.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var freq struct {
			LastError string `json:"last_error"`
		}
		s.cookie = g.owner
		s.get("/api/v1/frequencies/dead", &freq)
		if freq.LastError != "" && s.adsb.GetSourceStatus().Aircraft.LastError != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the failures are not known after 10 s: source %q, receiver %q",
				freq.LastError, s.adsb.GetSourceStatus().Aircraft.LastError)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return g
}

// read is one GET as the owner or the guest, through the proxy.
func (g *guestFixture) read(cookie *http.Cookie, path string) *httptest.ResponseRecorder {
	g.t.Helper()
	g.cookie = cookie
	return g.do("GET", path, "", throughTheProxy("203.0.113.30"))
}

// places is where a response carries an address: the JSON paths, a header,
// or "(body)" when the body is not JSON.
func places(dir string, rec *httptest.ResponseRecorder) map[string]bool {
	out := map[string]bool{}
	for name, values := range rec.Header() {
		for _, v := range values {
			if carries(dir, v) {
				out["(header "+name+")"] = true
			}
		}
	}
	var v any
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") &&
		json.Unmarshal(rec.Body.Bytes(), &v) == nil {
		leaks(dir, v, "", out)
	} else if carries(dir, rec.Body.String()) {
		out["(body)"] = true
	}
	return out
}

// frequencyStatus reads the WebSocket until the unreachable source's failure
// arrives, and returns where the messages read meanwhile carried an address.
func frequencyStatus(t *testing.T, dir string, conn *gorilla.Conn) (failure map[string]any, where map[string]bool) {
	t.Helper()
	where = map[string]bool{}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for the source's failure on the WebSocket: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("a message that is not JSON: %q", data)
		}
		typ, _ := m["type"].(string)
		found := map[string]bool{}
		leaks(dir, m["data"], "", found)
		for p := range found {
			where[typ+" "+p] = true
		}
		d, _ := m["data"].(map[string]any)
		if typ == websocket.MessageTypeFrequencyStatus && d["frequency_id"] == "dead" && d["status"] == "failed" {
			return d, where
		}
	}
}

// The inventory: every GET route a viewer may read and every WebSocket
// message, read as the viewer and as the owner, and each place an address,
// a path or the station's name shows, with who reads it. Frozen in
// testdata/guest.txt; ATC_WRITE_GUEST=1 rewrites it.
func TestWhatAGuestReads(t *testing.T) {
	g := newGuestFixture(t)
	type place struct{ route, path string }
	who := map[place][2]bool{} // read by the viewer, by the owner

	for _, mr := range mountedRoutes(t, g.stack) {
		method, route := mr[0], mr[1]
		if method != http.MethodGet {
			continue
		}
		path, _ := example(t, g.stack, method, route, 1)
		if route == "/api/v1/frequencies/{id}" {
			path = "/api/v1/frequencies/dead" // the one with an error
		}
		viewer := g.read(g.viewer, path)
		if viewer.Code != http.StatusOK {
			continue // not the viewer's to read
		}
		owner := g.read(g.owner, path)
		for i, rec := range []*httptest.ResponseRecorder{viewer, owner} {
			for p := range places(g.dir, rec) {
				k := place{route, p}
				w := who[k]
				w[i] = true
				who[k] = w
			}
		}
	}

	_, viewerSaw := frequencyStatus(t, g.dir, g.viewerWS)
	_, ownerSaw := frequencyStatus(t, g.dir, g.ownerWS)
	for i, saw := range []map[string]bool{viewerSaw, ownerSaw} {
		for p := range saw {
			typ, path, _ := strings.Cut(p, " ")
			k := place{"WS " + typ, "data." + path}
			w := who[k]
			w[i] = true
			who[k] = w
		}
	}

	var lines []string
	for k, w := range who {
		if w[0] {
			t.Errorf("a guest reads %s %s", k.route, k.path)
		}
		readers := []string{"-", "-"}
		if w[0] {
			readers[0] = "viewer"
		}
		if w[1] {
			readers[1] = "owner"
		}
		lines = append(lines, fmt.Sprintf("%-28s %-34s %s", k.route, k.path, strings.Join(readers, " ")))
	}
	sort.Strings(lines)
	got := "# Where an address, a path or the station's name shows, and who reads it (viewer, owner).\n" +
		strings.Join(lines, "\n") + "\n"

	const frozen = "testdata/guest.txt"
	if os.Getenv("ATC_WRITE_GUEST") == "1" {
		if err := os.WriteFile(frozen, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("what a guest reads differs from %s\n--- now\n%s--- frozen\n%s", frozen, got, want)
	}
}

// The guest's page still works without them: the frequencies with their
// state and where to play them, the receiver's state, the source's, the
// figures of the settings panel, the failure on the WebSocket -- and the
// sound, which passes by untouched.
func TestAGuestsPageWorksWithoutTheAddresses(t *testing.T) {
	g := newGuestFixture(t)

	var freqs struct {
		Frequencies []map[string]any `json:"frequencies"`
	}
	decode(t, g.read(g.viewer, "/api/v1/frequencies"), &freqs)
	if len(freqs.Frequencies) != 3 {
		t.Fatalf("%d frequencies, want mix, station, dead", len(freqs.Frequencies))
	}
	for _, f := range freqs.Frequencies {
		if f["url"] != "" || f["stream_url"] != "/api/v1/stream/"+f["id"].(string) || f["stream_port"] != 0.0 ||
			f["name"] == "" || f["status"] == "" {
			t.Errorf("a frequency as a guest reads it: %v", f)
		}
		if f["id"] == "dead" && (f["status"] != "failed" || f["last_error"] != "") {
			t.Errorf("the unreachable source: status %v, error %q", f["status"], f["last_error"])
		}
	}

	var status struct {
		Receiver map[string]any `json:"receiver"`
		ADSB     map[string]any `json:"adsb"`
	}
	decode(t, g.read(g.viewer, "/api/v1/status"), &status)
	if status.Receiver["state"] != "unknown" || status.Receiver["label"] != "?" || status.Receiver["detail"] != "" {
		t.Errorf("the receiver as a guest reads it: %v", status.Receiver)
	}
	if status.ADSB["ok"] != false || status.ADSB["failing_why"] != "the station refuses the connection" {
		t.Errorf("the ADS-B feed as a guest reads it: %v", status.ADSB)
	}

	var source map[string]any
	decode(t, g.read(g.viewer, "/api/v1/adsb/source"), &source)
	aircraft, _ := source["aircraft"].(map[string]any)
	if source["status"] != "error" || aircraft["available"] != false || aircraft["last_error"] != "" {
		t.Errorf("the ADS-B source as a guest reads it: %v", source)
	}

	var server ServerState
	decode(t, g.read(g.viewer, "/api/v1/server"), &server)
	if server.Storage.ActiveDBBytes == 0 || server.Storage.DiskFreeBytes == 0 || !server.Logging.Bounded ||
		server.Access.Mode != "account" || server.Storage.Dir != "" || server.Logging.File != "" {
		t.Errorf("the settings panel's figures as a guest reads them: %+v", server)
	}

	failure, _ := frequencyStatus(t, g.dir, g.viewerWS)
	if failure["error"] != "" {
		t.Errorf("the source's failure on a guest's WebSocket: %v", failure)
	}
	failure, _ = frequencyStatus(t, g.dir, g.ownerWS)
	if !strings.Contains(fmt.Sprint(failure["error"]), "connection refused") {
		t.Errorf("the source's failure on the owner's WebSocket: %v", failure)
	}

	// The sound, on the real port, as the page asks for it, once the first
	// frames are decoded (ULawStream waits for them).
	r, _, ok := g.freqs.ULawStream("station")
	if !ok {
		t.Fatal("no sound decoded")
	}
	r.Close()
	g.cookie = g.viewer
	if ct, head := streamHead(t, g.stack, "/api/v1/stream/station?id=guest-1&format=ulaw"); ct != "audio/basic" || len(head) == 0 {
		t.Errorf("a guest's sound: %q, %d bytes", ct, len(head))
	}
}

// The table names routes the router serves and a guest may read: a route
// renamed would otherwise leave its addresses to a guest without a word.
func TestTheGuestTableNamesRoutesAGuestReads(t *testing.T) {
	s := publicStack(t, stackOptions{})
	mounted := map[string]bool{}
	for _, mr := range mountedRoutes(t, s) {
		if mr[0] == http.MethodGet {
			mounted[mr[1]] = true
		}
	}
	for route, paths := range guestHidden {
		if !mounted[route] {
			t.Errorf("guestHidden names %s, which is not a GET route", route)
		}
		if len(paths) == 0 {
			t.Errorf("guestHidden names %s with nothing to empty", route)
		}
	}
}

// emptied changes the copy it returns, never what it was given: a WebSocket
// message is the same for every page.
func TestEmptiedCopiesWhatItChanges(t *testing.T) {
	v := map[string]any{
		"frequencies": []any{
			map[string]any{"id": "a", "url": "http://station.lan:8000/a.mp3", "order": json.Number("1")},
			map[string]any{"id": "b", "url": "http://station.lan:8000/b.mp3"},
		},
		"storage":  map[string]any{"dir": "/home/someone/db", "bytes": 12.0},
		"accounts": []any{"pierre", "guest"},
	}
	before, _ := json.Marshal(v)
	got := v
	for _, p := range []string{"frequencies.*.url", "storage.dir", "accounts", "absent.path", "storage.bytes.below"} {
		got = emptied(got, strings.Split(p, ".")).(map[string]any)
	}
	if after, _ := json.Marshal(v); string(after) != string(before) {
		t.Errorf("the original changed:\n%s\n%s", before, after)
	}
	out, _ := json.Marshal(got)
	want := `{"accounts":[],"frequencies":[{"id":"a","order":1,"url":""},{"id":"b","url":""}],"storage":{"bytes":12,"dir":""}}`
	if string(out) != want {
		t.Errorf("emptied:\n got %s\nwant %s", out, want)
	}

	m := &websocket.Message{Type: websocket.MessageTypeFrequencyStatus,
		Data: map[string]any{"frequency_id": "a", "status": "failed", "error": "dial tcp 192.0.2.20:8000"}}
	if g := guestMessage(m); g == m || g.Data["error"] != "" || g.Data["status"] != "failed" || m.Data["error"] == "" {
		t.Errorf("a guest's frequency status: %+v, the original %+v", g, m)
	}
	other := &websocket.Message{Type: websocket.MessageTypeFrequencyActivity, Data: map[string]any{"level": 0.5}}
	if guestMessage(other) != other {
		t.Error("a message with nothing to empty was copied")
	}
}
