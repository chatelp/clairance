package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/adsb"
)

// The keys /api/v1/vitals may carry, and no other: the sign-in page's console
// (docs-fr/fiches-cloud/G-chiffres-avant-connexion.md).
var vitalsKeys = map[string]bool{
	"aircraft": true, "messages_per_second": true, "farthest_nm": true,
	"channels_on_air": true, "transmissions_today": true, "matched_today": true, "matched_share": true,
	"transcription_model": true, "transcription_backend": true, "seconds_per_transmission": true,
	"airports_followed": true, "runways_in_use": true,
	"uptime_seconds": true, "version": true, "commit": true, "page": true,
}

const vitalsHealth = `{"status":"ok","pid":4242,"model_en":"sfabriece/whisper-large-v3-atco2-asr-mlx","backend":"fake",` +
	`"models":{"en":{"id":"sfabriece/whisper-large-v3-atco2-asr-mlx","available":true}}}`

// vitalsStack is a stack with a sidecar that has been read once, as Start
// reads it in production, and three transmissions today, two of them matched.
func vitalsStack(t *testing.T) *stack {
	t.Helper()
	s := newStack(t, stackOptions{sidecar: healthSaying(t, vitalsHealth)})
	if _, err := s.handler.sttSidecar.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.transcribe("mix", "air france one two three descend flight level one hundred", "AFR123", now)
	s.transcribe("mix", "ryanair four five contact one one niner decimal two five five", "RYR45", now)
	s.transcribe("mix", "station calling say again", "", now)
	return s
}

// One computation per ten seconds, whoever asks: a visit must never cost a
// query. And it needs no session.
func TestTheVitalsAreComputedOncePerTenSeconds(t *testing.T) {
	s := newStack(t, stackOptions{})
	clock := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	computed := 0
	s.handler.vitals.now = func() time.Time { return clock }
	s.handler.vitals.compute = func() Vitals { computed++; return Vitals{Aircraft: computed} }

	get := func() Vitals {
		t.Helper()
		rec := s.do("GET", "/api/v1/vitals", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/vitals without a session: %d %s", rec.Code, rec.Body.String())
		}
		var v Vitals
		decode(t, rec, &v)
		return v
	}

	get()
	clock = clock.Add(900 * time.Millisecond)
	if v := get(); computed != 1 || v.Aircraft != 1 {
		t.Fatalf("two visits within the same second: %d computations, want 1", computed)
	}
	clock = clock.Add(9 * time.Second)
	if get(); computed != 1 {
		t.Fatalf("a visit 9.9 s after the first: %d computations, want 1", computed)
	}
	clock = clock.Add(200 * time.Millisecond)
	if v := get(); computed != 2 || v.Aircraft != 2 {
		t.Errorf("a visit 10.1 s after the first: %d computations, want 2", computed)
	}
}

// What the route says, and that it says nothing else: the keys are the list,
// no number has more than two decimals.
func TestTheVitalsCarryTheListedKeysOnly(t *testing.T) {
	s := vitalsStack(t)
	rec := s.do("GET", "/api/v1/vitals", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/vitals: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range raw {
		keys = append(keys, k)
		if !vitalsKeys[k] {
			t.Errorf("key %q is not in the list", k)
		}
	}
	sort.Strings(keys)
	t.Logf("keys: %s", strings.Join(keys, " "))

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var values map[string]any
	if err := dec.Decode(&values); err != nil {
		t.Fatal(err)
	}
	for k, val := range values {
		if n, ok := val.(json.Number); ok {
			if i := strings.IndexByte(n.String(), '.'); i >= 0 && len(n.String())-i-1 > 2 {
				t.Errorf("%s = %s: more than two decimals", k, n)
			}
		}
	}

	var v Vitals
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	if v.Aircraft != aircraftSeen {
		t.Errorf("aircraft = %d, want %d", v.Aircraft, aircraftSeen)
	}
	// The climbing aircraft is the farthest from the station.
	want := int(adsb.Haversine(stationLat, stationLon, 49.10, 2.80)/1852 + 0.5)
	if v.FarthestNM == nil || *v.FarthestNM != want {
		t.Errorf("farthest_nm = %v, want %d", v.FarthestNM, want)
	}
	if v.TransmissionsToday != 3 || v.MatchedToday != 2 || v.MatchedShare != 67 {
		t.Errorf("today: %d transmissions, %d matched, %d%%; want 3, 2, 67%%",
			v.TransmissionsToday, v.MatchedToday, v.MatchedShare)
	}
	if v.TranscriptionModel != "Whisper large-v3 ATCO2" || v.TranscriptionBackend != "fake" {
		t.Errorf("transcription_model = %q, transcription_backend = %q", v.TranscriptionModel, v.TranscriptionBackend)
	}
	// Stored by hand, not transcribed by this server: no time to show yet.
	if v.SecondsPerTransmission != nil {
		t.Errorf("seconds_per_transmission = %v before any transcription, want absent", *v.SecondsPerTransmission)
	}
	if v.AirportsFollowed != 1 || v.RunwaysInUse != 0 || v.ChannelsOnAir != 0 {
		t.Errorf("airports %d, runways %d, channels %d: want 1, 0, 0 (no reference data, no stream started)",
			v.AirportsFollowed, v.RunwaysInUse, v.ChannelsOnAir)
	}
}

// Nothing that locates the station or names what it hears: no number with
// three decimals (a coordinate, a frequency), no callsign, no airport, no
// frequency name -- in the values and anywhere in the body.
func TestTheVitalsSayNothingThatLocatesTheStation(t *testing.T) {
	s := vitalsStack(t)
	rec := s.do("GET", "/api/v1/vitals", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/vitals: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	var values map[string]any
	if err := json.Unmarshal([]byte(body), &values); err != nil {
		t.Fatal(err)
	}
	threeDecimals := regexp.MustCompile(`[0-9]\.[0-9]{3}`)
	// At a word boundary: "ATCO2", the model's dataset, holds "TCO2" but names
	// no aircraft; a callsign is a word of its own.
	callsign := regexp.MustCompile(`\b[A-Z]{3}[0-9]`)
	for k, val := range values {
		str, ok := val.(string)
		if !ok {
			continue
		}
		if threeDecimals.MatchString(str) || callsign.MatchString(str) {
			t.Errorf("%s = %q looks like a position, a frequency or a callsign", k, str)
		}
	}
	for _, secret := range []string{"48.8", "2.05", "AFR123", "RYR45", "EZY77", "LFPG", "125.825",
		"Configured mix", "mix", "aero.mp3", "127.0.0.1", "sfabriece"} {
		if strings.Contains(body, secret) {
			t.Errorf("the answer carries %q: %s", secret, body)
		}
	}
}

// Off from the settings panel, the route does not exist; on again, it does.
// The setting is in the panel's settings, kept like the others.
func TestTheVitalsCanBeTurnedOff(t *testing.T) {
	s := newStack(t, stackOptions{})
	s.signIn()

	var st ServerState
	s.get("/api/v1/server", &st)
	if st.Settings.Vitals == nil || !st.Settings.Vitals.Enabled {
		t.Fatalf("settings.vitals = %+v, want on by default", st.Settings.Vitals)
	}

	if rec := s.do("PUT", "/api/v1/server/settings", `{"vitals":{"enabled":false}}`); rec.Code != http.StatusOK {
		t.Fatalf("turning the vitals off: %d %s", rec.Code, rec.Body.String())
	}
	s.cookie = nil
	if rec := s.do("GET", "/api/v1/vitals", ""); rec.Code != http.StatusNotFound {
		t.Errorf("vitals off: %d, want 404", rec.Code)
	}

	s.signIn()
	// A change that does not mention them leaves them as they are.
	if rec := s.do("PUT", "/api/v1/server/settings", `{"log_level":"warn"}`); rec.Code != http.StatusOK {
		t.Fatalf("changing the log level: %d %s", rec.Code, rec.Body.String())
	}
	if s.runtime.VitalsEnabled() {
		t.Error("a change that does not mention the vitals turned them back on")
	}
	if rec := s.do("PUT", "/api/v1/server/settings", `{"vitals":{"enabled":true}}`); rec.Code != http.StatusOK {
		t.Fatalf("turning the vitals on: %d %s", rec.Code, rec.Body.String())
	}
	s.cookie = nil
	if rec := s.do("GET", "/api/v1/vitals", ""); rec.Code != http.StatusOK {
		t.Errorf("vitals on again: %d, want 200", rec.Code)
	}
}

// The shortening rule, on the station's model and two others.
func TestModelNamesAreShortenedToTheirFamily(t *testing.T) {
	for name, want := range map[string]string{
		"sfabriece/whisper-large-v3-atco2-asr-mlx":    "Whisper large-v3 ATCO2",
		"mlx-community/whisper-large-v3-turbo":        "Whisper large-v3 turbo",
		"/Volumes/Models/whisper-large-v3-french-mlx": "Whisper large-v3 French",
		"openai/whisper-small":                        "Whisper small",
		"":                                            "",
	} {
		if got := ModelFamily(name); got != want {
			t.Errorf("ModelFamily(%q) = %q, want %q", name, got, want)
		}
	}
}

// The binary's version and commit, as the linker set them, in the vitals and
// in the settings panel; "dev" and "unknown" from a plain go build.
func TestTheVersionAndCommitAreTheBuilds(t *testing.T) {
	s := newStack(t, stackOptions{})
	var v Vitals
	s.get("/api/v1/vitals", &v)
	if v.Version != "dev" || v.Commit != "unknown" {
		t.Errorf("a plain build: version %q commit %q, want dev and unknown", v.Version, v.Commit)
	}

	s = newStack(t, stackOptions{})
	s.handler.SetBuild("v0.2.0-14-g5fdf8f2", "5fdf8f2")
	s.get("/api/v1/vitals", &v)
	if v.Version != "v0.2.0-14-g5fdf8f2" || v.Commit != "5fdf8f2" {
		t.Errorf("vitals: version %q commit %q", v.Version, v.Commit)
	}
	s.signIn()
	var st ServerState
	s.get("/api/v1/server", &st)
	if st.Version != "v0.2.0-14-g5fdf8f2" || st.Commit != "5fdf8f2" {
		t.Errorf("server state: version %q commit %q", st.Version, st.Commit)
	}
}
