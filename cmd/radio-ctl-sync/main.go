// Command radio-ctl-sync keeps co-atc's sources in step with a radio-ctl station.
//
// radio-ctl is the control service of one private receiving station
// (docs-fr/01-station.md). It switches the receiver between groups of channels,
// and publishes each channel it listens to on its own Icecast mount. co-atc does
// not know about it, on purpose (docs-fr/05-decisions.md, D36): this program reads
// the station's state and tells co-atc, through its API, which streams to
// transcribe -- one per channel, which is worth four times the matched callsigns
// of the mixed stream (docs-fr/28-gros-porteurs-par-canal.md).
//
//	go run ./cmd/radio-ctl-sync -station http://station.lan -station-host station.lan \
//	    -streams http://audio.lan -coatc http://127.0.0.1:8000 [-mix-id aero-melange]
//
// It only reads the station, and never switches it. With -mix-id, the label of
// the mixed stream co-atc already reads follows the station's selection too.
// When co-atc requires sign-in: -user, and the password in COATC_PASSWORD.
//
// co-atc can also run it itself ([station_link], docs-fr/36-contrat-lanceur.md).
// It then passes COATC_SPAWNED=1 and, in COATC_LINK_TOKEN, a key made at its
// start that opens the routes this program calls, from this machine only: no
// account and no password are needed, and this program stops when co-atc is
// gone.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

// The station's state, as much of it as this program needs.
type stationState struct {
	Airband  string `json:"airband"`
	Version  int    `json:"version"`
	Since    string `json:"depuis"`
	Mode     string `json:"mode"`
	Label    string `json:"libelle"`
	Detail   string `json:"detail"`
	Separate *struct {
		Active  bool     `json:"actifs"`
		Missing []string `json:"manquants"`
	} `json:"flux_separes"`
	Selection struct {
		Name     string    `json:"nom"`
		Title    string    `json:"titre"`
		Channels []channel `json:"frequences"`
	} `json:"selection"`
}

// receiverReport is what co-atc's PUT /api/v1/receiver takes: the station's
// mode mapped onto co-atc's receiver states, its own words for the rest.
type receiverReport struct {
	State  string `json:"state"`
	Label  string `json:"label,omitempty"`
	Title  string `json:"title,omitempty"`
	Since  string `json:"since,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// report maps radio-ctl's modes: "chaine" is the airband chain on the air,
// "exploration" the receiver lent to SpyServer or rtl_tcp, "transition" a
// switch under way, "arret" everything stopped.
func report(st stationState) receiverReport {
	state := map[string]string{
		"chaine": "on_air", "exploration": "lent", "transition": "switching", "arret": "off",
	}[st.Mode]
	if state == "" {
		state = "unknown"
	}
	return receiverReport{
		State: state, Label: st.Label, Title: st.Selection.Title,
		Since: st.Since, Mode: st.Mode, Detail: st.Detail,
	}
}

type channel struct {
	ID          string  `json:"id"`
	Designation string  `json:"designation"`
	MHz         float64 `json:"mhz"`
	Service     string  `json:"service"`
	Language    string  `json:"langue"`
	Stream      string  `json:"flux"`
	StreamURL   string  `json:"flux_url"`
	// Present is whether the mount is on Icecast right now. Since 26/09 the
	// station says so; before, a channel with a stream path was assumed live.
	Present     *bool    `json:"flux_present"`
	ChannelGain *float64 `json:"ampfactor_canal"`
}

// The station's catalogue says which channels are worth transcribing.
type catalogue struct {
	Channels []struct {
		ID         string `json:"id"`
		Transcribe *bool  `json:"transcrire"`
	} `json:"frequences"`
}

// The station's table of its 68 frequencies, 26/09/2026: for each, the airport
// it serves and its kind, from the eAIP (GEN 3.4 and AD 2.18, AIRAC 03/09/2026)
// or, where the eAIP is silent, the station's catalogue and listening. Copied
// from the station agent's reply (docs-fr/05-decisions.md, Q46); a channel it
// does not list gets no sector, which leaves co-atc the whole sky.
//
//go:embed secteurs-frequences.csv
var sectorsCSV string

var sectorTable = loadSectorTable(sectorsCSV)

// sectorKinds maps the table's kinds to co-atc's. Control sectors are left out:
// the eAIP publishes only their organisation's limits, and those mislead --
// 124.625 is filed with the upper centre, above FL195, while the aircraft
// matched on it on 24/09 flew at 15,700 ft median.
var sectorKinds = map[string]string{"approche": "approach", "depart": "departure", "tour": "tower", "sol": "ground", "prevol": "ground"}

func loadSectorTable(data string) map[string][2]string {
	rows, err := csv.NewReader(strings.NewReader(data)).ReadAll()
	if err != nil || len(rows) < 2 {
		log.Printf("sector table unreadable: %v", err)
		return nil
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	out := map[string][2]string{}
	for _, r := range rows[1:] {
		id, airport, kind := r[col["id"]], r[col["terrain"]], sectorKinds[r[col["nature"]]]
		if kind != "" && len(airport) == 4 && strings.ToUpper(airport) == airport {
			out[id] = [2]string{airport, kind}
		}
	}
	return out
}

// source is what co-atc's PUT /api/v1/sources/{id} takes.
type source struct {
	Name            string  `json:"name"`
	FrequencyMHz    float64 `json:"frequency_mhz"`
	URL             string  `json:"url"`
	Order           int     `json:"order"`
	TranscribeAudio bool    `json:"transcribe_audio"`
	Language        string  `json:"language"`
	FFmpegReconnect bool    `json:"ffmpeg_reconnect"`
	SectorAirport   string  `json:"sector_airport,omitempty"`
	SectorKind      string  `json:"sector_kind,omitempty"`
}

// wanted turns the station's state into the sources co-atc should have, by id.
func wanted(st stationState, cat catalogue, streams string) map[string]source {
	out := map[string]source{}
	if st.Airband != "running" || (st.Separate != nil && !st.Separate.Active) {
		return out // the receiver is off, lent to another program, or streams are off
	}
	transcribe := map[string]bool{}
	for _, c := range cat.Channels {
		transcribe[c.ID] = c.Transcribe == nil || *c.Transcribe
	}
	for _, c := range st.Selection.Channels {
		if c.ID == "" || c.Stream == "" || (c.Present != nil && !*c.Present) {
			continue // no stream, or not on Icecast yet: opened when it appears
		}
		url := c.StreamURL
		if url == "" {
			url = strings.TrimRight(streams, "/") + "/" + strings.TrimLeft(c.Stream, "/")
		}
		t, known := transcribe[c.ID]
		out[c.ID] = source{
			Name:         label(c.Designation + " " + c.Service),
			FrequencyMHz: c.MHz,
			URL:          url,
			// By frequency, not by position in the selection: a channel keeps its
			// place when others join or leave.
			Order:           int(c.MHz*1000 + 0.5),
			TranscribeAudio: t || !known,
			Language:        language(c.Language),
			// A channel's mount disappears when the station stops listening to
			// it; ffmpeg's own reconnection would retry its 404 in a loop.
			FFmpegReconnect: false,
			SectorAirport:   sectorTable[c.ID][0],
			SectorKind:      sectorTable[c.ID][1],
		}
	}
	return out
}

// language maps the station's language to co-atc's. A mixed channel is read in
// English first: co-atc's sidecar asks its French model too when the English
// reading contains French (docs-fr/05-decisions.md, D24).
func language(l string) string {
	switch l {
	case "fr":
		return "fr"
	case "en", "mixte":
		return "en"
	}
	return ""
}

func label(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:119]) + "…"
	}
	return s
}

// listed is what co-atc reports of a source it has.
type listed struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	URL             string  `json:"url"`
	FrequencyMHz    float64 `json:"frequency_mhz"`
	Order           int     `json:"order"`
	TranscribeAudio bool    `json:"transcribe_audio"`
	Runtime         bool    `json:"runtime"`
}

// plan says which sources to set and which to remove, given what co-atc has.
// A source co-atc already has as wanted is left alone; sent is what this
// program last set, for what co-atc's listing does not show (the language).
func plan(want map[string]source, have []listed, sent map[string]source) (set []string, remove []string) {
	current := map[string]listed{}
	for _, h := range have {
		if h.Runtime {
			current[h.ID] = h
		}
	}
	for id, w := range want {
		h, ok := current[id]
		if !ok || h.URL != w.URL || h.Name != w.Name || h.FrequencyMHz != w.FrequencyMHz ||
			h.Order != w.Order || h.TranscribeAudio != w.TranscribeAudio || sent[id] != w {
			set = append(set, id)
		}
	}
	for id := range current {
		if _, ok := want[id]; !ok {
			remove = append(remove, id)
		}
	}
	sort.Strings(set)
	sort.Strings(remove)
	return set, remove
}

type syncer struct {
	station, stationHost, streams, coatc, mixID, user string
	token                                             string // COATC_LINK_TOKEN, from the co-atc that started us
	http                                              *http.Client
	sent                                              map[string]source
	lastVersion                                       int
	lastErr                                           string
	lastReportErr                                     string
}

// sendReport tells co-atc the receiver's state. Never in the way of the sources:
// a co-atc without the endpoint, or a failure, is said once in the log.
func (s *syncer) sendReport(rep receiverReport) {
	err := s.coatcCall("PUT", "/api/v1/receiver", rep, nil)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if msg != s.lastReportErr {
		if msg != "" {
			log.Printf("receiver state not reported: %s", msg)
		} else if s.lastReportErr != "" {
			log.Printf("receiver state reported again")
		}
		s.lastReportErr = msg
	}
}

func (s *syncer) getStation(path string, into any) error {
	req, err := http.NewRequest("GET", strings.TrimRight(s.station, "/")+path, nil)
	if err != nil {
		return err
	}
	if s.stationHost != "" {
		req.Host = s.stationHost
	}
	return s.do(req, into)
}

func (s *syncer) coatcCall(method, path string, body any, into any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimRight(s.coatc, "/")+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	err = s.do(req, into)
	if err != nil && strings.Contains(err.Error(), "401") && s.user != "" {
		if lerr := s.signIn(); lerr != nil {
			return fmt.Errorf("%w; sign-in: %v", err, lerr)
		}
		if body != nil {
			b, _ := json.Marshal(body)
			req.Body = io.NopCloser(bytes.NewReader(b))
		}
		return s.do(req, into)
	}
	return err
}

func (s *syncer) signIn() error {
	b, _ := json.Marshal(map[string]string{"name": s.user, "password": os.Getenv("COATC_PASSWORD")})
	req, _ := http.NewRequest("POST", strings.TrimRight(s.coatc, "/")+"/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return s.do(req, nil)
}

func (s *syncer) do(req *http.Request, into any) error {
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if into == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// once brings co-atc in step with the station, and says what it did.
func (s *syncer) once() error {
	var st stationState
	if err := s.getStation("/radio/etat", &st); err != nil {
		s.sendReport(receiverReport{State: "unknown", Detail: "station not reached: " + err.Error()})
		return fmt.Errorf("station: %w", err) // leave co-atc as it is
	}
	s.sendReport(report(st))
	var cat catalogue
	if err := s.getStation("/radio/frequences", &cat); err != nil {
		return fmt.Errorf("station catalogue: %w", err)
	}
	var have struct {
		Frequencies []listed `json:"frequencies"`
	}
	if err := s.coatcCall("GET", "/api/v1/frequencies", nil, &have); err != nil {
		return fmt.Errorf("co-atc: %w", err)
	}

	if st.Version != s.lastVersion {
		log.Printf("station version %d since %s: %s, %d channels", st.Version, st.Since,
			st.Selection.Title, len(st.Selection.Channels))
		for _, c := range st.Selection.Channels {
			if c.ChannelGain != nil && *c.ChannelGain != 1 {
				// a lone channel's level is applied to its own stream too
				log.Printf("  %s: channel level x%.2f in its stream", c.ID, *c.ChannelGain)
			}
		}
		if st.Separate != nil && len(st.Separate.Missing) > 0 {
			log.Printf("  streams expected but not on Icecast: %v", st.Separate.Missing)
		}
		s.lastVersion = st.Version
		if s.mixID != "" {
			if err := s.coatcCall("PUT", "/api/v1/frequencies/"+url.PathEscape(s.mixID)+"/label",
				map[string]string{"label": label(st.Selection.Title)}, nil); err != nil {
				log.Printf("label of %s: %v", s.mixID, err)
			}
		}
	}

	want := wanted(st, cat, s.streams)
	set, remove := plan(want, have.Frequencies, s.sent)
	for _, id := range remove {
		if err := s.coatcCall("DELETE", "/api/v1/sources/"+url.PathEscape(id), nil, nil); err != nil {
			return fmt.Errorf("removing %s: %w", id, err)
		}
		delete(s.sent, id)
		log.Printf("removed %s", id)
	}
	for _, id := range set {
		if err := s.coatcCall("PUT", "/api/v1/sources/"+url.PathEscape(id), want[id], nil); err != nil {
			return fmt.Errorf("setting %s: %w", id, err)
		}
		s.sent[id] = want[id]
		log.Printf("set %s: %s (%s)", id, want[id].Name, want[id].URL)
	}
	return nil
}

func main() {
	var s syncer
	flag.StringVar(&s.station, "station", "http://station.lan", "radio-ctl base URL")
	flag.StringVar(&s.stationHost, "station-host", "", "Host header radio-ctl expects, if any")
	flag.StringVar(&s.streams, "streams", "http://audio.lan", "base URL of the station's Icecast mounts")
	flag.StringVar(&s.coatc, "coatc", "http://127.0.0.1:8000", "co-atc base URL")
	flag.StringVar(&s.mixID, "mix-id", "", "co-atc id of the mixed stream whose label should follow the station")
	flag.StringVar(&s.user, "user", "", "co-atc account, when sign-in is required (password in COATC_PASSWORD)")
	interval := flag.Duration("interval", 10*time.Second, "how often to read the station")
	one := flag.Bool("once", false, "bring co-atc in step once and exit")
	flag.Parse()

	jar, _ := cookiejar.New(nil)
	s.http = &http.Client{Timeout: 10 * time.Second, Jar: jar}
	s.sent = map[string]source{}
	s.token = os.Getenv("COATC_LINK_TOKEN")
	if os.Getenv("COATC_SPAWNED") == "1" {
		// co-atc dates every line it logs for us.
		log.SetFlags(0)
		watchParent(time.Second)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		err := s.once()
		if *one {
			if err != nil {
				log.Fatal(err)
			}
			return
		}
		// Said once when it starts and once when it stops, not every poll.
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		if msg != s.lastErr {
			if msg != "" {
				log.Printf("%s -- sources left as they are, retrying every %s", msg, *interval)
			} else if s.lastErr != "" {
				log.Printf("back in step")
			}
			s.lastErr = msg
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*interval):
		}
	}
}

// watchParent stops this program when the co-atc that started it is gone. co-atc
// stops it on every ordinary exit, but cannot when it is killed outright, and a
// link left behind would go on setting the sources of whichever co-atc next
// answers on that port -- the sidecar does the same for its port. Only when
// co-atc says it started us: a link started by hand under nohup keeps running
// when its terminal closes.
func watchParent(every time.Duration) {
	parent := os.Getppid()
	if parent == 1 {
		return // orphaned already
	}
	go func() {
		for range time.Tick(every) {
			if os.Getppid() != parent {
				log.Printf("co-atc (pid %d) is gone, stopping", parent)
				os.Exit(0)
			}
		}
	}()
}
