package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// Trimmed from the station's /radio/etat and /radio/frequences, 25/09/2026.
const stateJSON = `{"airband": "running", "version": 19, "depuis": "2026-09-25T21:19:26+02:00",
 "selection": {"nom": "en-route-et-descente-cdg", "titre": "En route et descente CDG - 132-133 MHz",
  "frequences": [
   {"designation": "132.275", "mhz": 132.275, "id": "132275", "service": "Paris Controle - secteur arrivee CDG",
    "langue": "mixte", "flux": "/aero-132275.mp3"},
   {"designation": "132.733", "mhz": 132.733333, "id": "132733", "service": "en route - croisiere haute",
    "langue": "en", "flux": "/aero-132733.mp3"},
   {"designation": "118.000", "mhz": 118.0, "id": "118000", "service": "Saint-Cyr Tour (LFPZ)",
    "langue": "fr", "flux": "/aero-118000.mp3"},
   {"designation": "121.500", "mhz": 121.5, "id": "121500", "service": "no stream yet", "langue": "en"}]}}`

const catalogueJSON = `{"frequences": [{"id": "132275", "transcrire": true}, {"id": "132733", "transcrire": false},
 {"id": "118000", "transcrire": true}]}`

func parsed(t *testing.T) (stationState, catalogue) {
	t.Helper()
	var st stationState
	var cat catalogue
	if err := json.Unmarshal([]byte(stateJSON), &st); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(catalogueJSON), &cat); err != nil {
		t.Fatal(err)
	}
	return st, cat
}

func TestEachChannelWithAStreamBecomesASource(t *testing.T) {
	st, cat := parsed(t)
	w := wanted(st, cat, "http://audio.lan/")
	if len(w) != 3 {
		t.Fatalf("3 channels have a stream, got %d sources: %v", len(w), w)
	}
	got := w["132275"]
	want := source{Name: "132.275 Paris Controle - secteur arrivee CDG", FrequencyMHz: 132.275,
		URL: "http://audio.lan/aero-132275.mp3", Order: 132275, TranscribeAudio: true, Language: "en"}
	if got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if w["118000"].Language != "fr" {
		t.Errorf("a French channel is read in French: %+v", w["118000"])
	}
	if w["132733"].TranscribeAudio {
		t.Error("the catalogue said not to transcribe 132733")
	}
}

// With the receiver off or lent to another program there are no streams, and
// co-atc should stop reading them rather than retry them.
func TestNoSourcesWhenTheReceiverIsNotRunning(t *testing.T) {
	st, cat := parsed(t)
	st.Airband = "exited"
	if w := wanted(st, cat, "http://audio.lan"); len(w) != 0 {
		t.Errorf("got %v, want nothing", w)
	}
}

func TestThePlanOnlyTouchesWhatChanged(t *testing.T) {
	st, cat := parsed(t)
	want := wanted(st, cat, "http://audio.lan")
	sent := map[string]source{}
	var have []listed
	for id, s := range want {
		sent[id] = s
		have = append(have, listed{ID: id, Name: s.Name, URL: s.URL, FrequencyMHz: s.FrequencyMHz,
			Order: s.Order, TranscribeAudio: s.TranscribeAudio, Runtime: true})
	}
	// the configured mixed stream is not ours to touch
	have = append(have, listed{ID: "aero-melange", URL: "http://audio.lan/aero.mp3"})
	// a channel the station no longer listens to
	have = append(have, listed{ID: "124350", URL: "http://audio.lan/aero-124350.mp3", Runtime: true})

	set, remove := plan(want, have, sent)
	if len(set) != 0 {
		t.Errorf("nothing changed, nothing to set: %v", set)
	}
	if !reflect.DeepEqual(remove, []string{"124350"}) {
		t.Errorf("remove = %v, want only the channel that left", remove)
	}

	// co-atc restarted: it has none of them any more
	set, _ = plan(want, have[len(have)-2:len(have)-1], sent)
	if len(set) != 3 {
		t.Errorf("after a co-atc restart all 3 are set again, got %v", set)
	}
}

// Since 26/09 the station gives each stream's full address and says whether the
// mount exists yet (trimmed from /radio/etat, version 25).
const stateWithPresenceJSON = `{"airband": "running", "version": 25, "depuis": "2026-09-26T13:04:52+02:00",
 "flux_separes": {"actifs": true, "attendus": 2, "presents": 1, "manquants": ["124.350"]},
 "selection": {"titre": "Selection libre", "frequences": [
   {"designation": "123.875", "mhz": 123.875, "id": "123875", "service": "Orly approche", "langue": "mixte",
    "flux": "/aero-123875.mp3", "flux_url": "http://audio.lan/aero-123875.mp3", "flux_present": true, "ampfactor_canal": 1.1},
   {"designation": "124.350", "mhz": 124.35, "id": "124350", "service": "CDG approche", "langue": "en",
    "flux": "/aero-124350.mp3", "flux_url": "http://audio.lan/aero-124350.mp3", "flux_present": false}]}}`

func TestOnlyStreamsOnIcecastAreOpened(t *testing.T) {
	var st stationState
	if err := json.Unmarshal([]byte(stateWithPresenceJSON), &st); err != nil {
		t.Fatal(err)
	}
	w := wanted(st, catalogue{}, "http://ignored")
	if len(w) != 1 || w["123875"].URL != "http://audio.lan/aero-123875.mp3" {
		t.Errorf("only the present stream, at the address the station gives: %v", w)
	}
	st.Separate.Active = false
	if w := wanted(st, catalogue{}, "http://ignored"); len(w) != 0 {
		t.Errorf("streams switched off at the station: nothing to open, got %v", w)
	}
}

// The station's table gives each channel its airport and kind; control sectors
// and ATIS get none.
func TestChannelsGetTheirSectorFromTheStationTable(t *testing.T) {
	cases := map[string][2]string{
		"123875": {"LFPO", "approach"}, "124350": {"LFPG", "approach"}, "127750": {"LFPO", "departure"},
		"118000": {"LFPZ", "tower"}, "121608": {"LFPG", "ground"}, "121842": {"LFPG", "ground"},
		"124625": {"", ""}, "128225": {"", ""}, "121500": {"", ""},
	}
	for id, want := range cases {
		if got := sectorTable[id]; got != want {
			t.Errorf("%s: got %v, want %v", id, got, want)
		}
	}
	if n := len(sectorTable); n < 35 || n > 45 {
		t.Errorf("%d channels with a sector, expected about 40 of 68", n)
	}
}

func TestTheStationModesMapOntoReceiverStates(t *testing.T) {
	for mode, want := range map[string]string{
		"chaine": "on_air", "exploration": "lent", "transition": "switching", "arret": "off", "": "unknown", "autre": "unknown",
	} {
		st := stationState{Mode: mode, Label: "A l antenne", Since: "2026-10-09T11:35:15+02:00"}
		st.Selection.Title = "Selection libre - 124,350 + 126,425"
		got := report(st)
		if got.State != want || got.Label != "A l antenne" || got.Title != st.Selection.Title || got.Since != st.Since {
			t.Errorf("mode %q: %+v, want state %s", mode, got, want)
		}
	}
}

// fakeStation serves /radio/etat and /radio/frequences, and records what
// reached it.
func fakeStation(t *testing.T, state string) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var mu sync.Mutex
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Clone(r.Context()))
		mu.Unlock()
		switch r.URL.Path {
		case "/radio/etat":
			io.WriteString(w, state)
		case "/radio/frequences":
			io.WriteString(w, catalogueJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// fakeCoatc answers the five routes the link calls, and records each request.
type fakeCoatc struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
	refuse   func(r *http.Request, body string) (int, string) // a status and message to answer instead, or 0
}

func (f *fakeCoatc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, r.Clone(r.Context()))
	f.bodies = append(f.bodies, string(b))
	refuse := f.refuse
	f.mu.Unlock()
	if refuse != nil {
		if code, msg := refuse(r, string(b)); code != 0 {
			http.Error(w, msg, code)
			return
		}
	}
	if r.Method == "GET" && r.URL.Path == "/api/v1/frequencies" {
		io.WriteString(w, `{"frequencies":[]}`)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newSyncer(station, coatc string) *syncer {
	return &syncer{station: station, streams: "http://audio.lan", coatc: coatc,
		http: &http.Client{Timeout: 5 * time.Second}, sent: map[string]source{}}
}

// Started by co-atc, the link carries the key co-atc gave it on every call to
// co-atc, and never to the station.
func TestTheLinkKeyGoesToCoatcAndNowhereElse(t *testing.T) {
	station, seen := fakeStation(t, stateJSON)
	co := &fakeCoatc{}
	srv := httptest.NewServer(co)
	t.Cleanup(srv.Close)

	s := newSyncer(station.URL, srv.URL)
	s.token = "a-key-made-at-start"
	if err := s.once(); err != nil {
		t.Fatal(err)
	}
	if len(co.requests) < 4 { // the receiver, the list, three sources
		t.Fatalf("%d calls to co-atc", len(co.requests))
	}
	for _, r := range co.requests {
		if got := r.Header.Get("Authorization"); got != "Bearer a-key-made-at-start" {
			t.Errorf("%s %s: Authorization %q", r.Method, r.URL.Path, got)
		}
	}
	for _, r := range *seen {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("the key went to the station: %s", r.URL.Path)
		}
	}

	// Without a key, as the production script runs it: no header at all.
	co2 := &fakeCoatc{}
	srv2 := httptest.NewServer(co2)
	t.Cleanup(srv2.Close)
	if err := newSyncer(station.URL, srv2.URL).once(); err != nil {
		t.Fatal(err)
	}
	for _, r := range co2.requests {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("no key, yet %s %s carries one", r.Method, r.URL.Path)
		}
	}
}

// The report lists what is on the air, a channel without a stream said so. A
// co-atc from before that refuses the field: it is told the rest without it,
// at once and from then on.
func TestTheReportListsTheChannelsAndAnOlderCoatcGetsItWithout(t *testing.T) {
	st, _ := parsed(t)
	rep := report(st)
	if len(rep.Channels) != 4 {
		t.Fatalf("channels: %+v", rep.Channels)
	}
	if c := rep.Channels[0]; c.ID != "132275" || c.Designation != "132.275" || c.MHz != 132.275 || !c.Stream {
		t.Errorf("first: %+v", c)
	}
	if rep.Channels[3].Stream {
		t.Errorf("121500 has no stream: %+v", rep.Channels[3])
	}

	station, _ := fakeStation(t, stateJSON)
	co := &fakeCoatc{refuse: func(r *http.Request, body string) (int, string) {
		if r.URL.Path == "/api/v1/receiver" && strings.Contains(body, `"channels"`) {
			return http.StatusBadRequest, `invalid report: json: unknown field "channels"`
		}
		return 0, ""
	}}
	srv := httptest.NewServer(co)
	t.Cleanup(srv.Close)
	s := newSyncer(station.URL, srv.URL)
	for i := 0; i < 2; i++ {
		if err := s.once(); err != nil {
			t.Fatal(err)
		}
	}
	var reports []string
	for i, r := range co.requests {
		if r.URL.Path == "/api/v1/receiver" {
			reports = append(reports, co.bodies[i])
		}
	}
	// Refused with the channels, sent again without; then without at once.
	if len(reports) != 3 || !strings.Contains(reports[0], `"channels"`) ||
		strings.Contains(reports[1], `"channels"`) || strings.Contains(reports[2], `"channels"`) {
		t.Errorf("reports: %q", reports)
	}
	if s.lastReportErr != "" {
		t.Errorf("the report was said to fail: %s", s.lastReportErr)
	}
}
