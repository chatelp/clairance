package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/audio/audiotest"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/storage/sqlite"
)

type frequenciesPage struct {
	Count          int                                     `json:"count"`
	Frequencies    []frequencies.Frequency                 `json:"frequencies"`
	Transcriptions map[string][]sqlite.TranscriptionRecord `json:"transcriptions"`
}

func (p frequenciesPage) byID(id string) *frequencies.Frequency {
	for i := range p.Frequencies {
		if p.Frequencies[i].ID == id {
			return &p.Frequencies[i]
		}
	}
	return nil
}

// The body cmd/radio-ctl-sync sends for one channel of the station.
func sourceBody(base, id string) string {
	return fmt.Sprintf(`{"name":"Channel %s","airport":"LFPG","frequency_mhz":125.825,"url":"%s/aero-%s.mp3",`+
		`"order":5,"language":"en","transcribe_audio":false,`+
		`"sector_airport":"LFPG","sector_kind":"approach"}`, id, base, id)
}

// What the page reads about the frequencies, and the label set from outside,
// on one stack.
func TestFrequencyListingAndLabels(t *testing.T) {
	shared := newStack(t, stackOptions{})
	shared.signIn()

	// Every frequency in display order, where its audio is served, which
	// port to take it from, and its recent transcriptions.
	t.Run("listed in order with their stream and transcriptions", func(t *testing.T) {
		s := shared.with(t)
		at := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
		s.transcribe("mix", "orly tower bonjour", "", at)
		s.transcribe("mix", "ryanair four five descend", "RYR45", at.Add(time.Minute))
		if rec := s.do("PUT", "/api/v1/sources/first", `{"name":"First","url":"`+s.gone+`/first.mp3","order":1}`); rec.Code != http.StatusOK {
			t.Fatalf("adding a source: %d %s", rec.Code, rec.Body.String())
		}

		var page frequenciesPage
		s.get("/api/v1/frequencies", &page)
		if page.Count != 2 || len(page.Frequencies) != 2 {
			t.Fatalf("count %d with %d frequencies, want 2", page.Count, len(page.Frequencies))
		}
		if page.Frequencies[0].ID != "first" || page.Frequencies[1].ID != "mix" {
			t.Errorf("order: %s then %s, want first (order 1) then mix (order 2)", page.Frequencies[0].ID, page.Frequencies[1].ID)
		}
		mix := page.byID("mix")
		if mix.Name != "Configured mix" || mix.Airport != "LFPG" || mix.FrequencyMHz != 125.825 || mix.Runtime {
			t.Errorf("mix = %+v", *mix)
		}
		if mix.StreamURL != "/api/v1/stream/mix" {
			t.Errorf("stream_url = %q, want /api/v1/stream/mix (relative: the page adds host and port)", mix.StreamURL)
		}
		if mix.Status != "available" && mix.Status != "failed" && mix.Status != "connecting" {
			t.Errorf("status = %q", mix.Status)
		}
		if !page.byID("first").Runtime {
			t.Error("a source added while running is marked runtime, which the page shows differently")
		}
		// The stream ports take turns over the configured ports (8000 and 8001).
		ports := map[int]bool{}
		for i := 0; i < 4; i++ {
			s.get("/api/v1/frequencies", &page)
			for _, f := range page.Frequencies {
				ports[f.StreamPort] = true
			}
		}
		if !ports[8000] || !ports[8001] || len(ports) != 2 {
			t.Errorf("stream ports seen: %v, want 8000 and 8001 in turn", ports)
		}
		// Transcriptions ride along, newest first, under the frequency's id.
		txs := page.Transcriptions["mix"]
		if len(txs) != 2 || txs[0].Callsign != "RYR45" || txs[1].Content != "orly tower bonjour" {
			t.Errorf("transcriptions for mix = %+v, want the two stored, newest first", txs)
		}
		if _, ok := page.Transcriptions["first"]; ok {
			t.Error("a frequency without transcriptions has no entry")
		}

		var one frequencies.Frequency
		s.get("/api/v1/frequencies/mix", &one)
		if one.ID != "mix" || one.Name != "Configured mix" || one.StreamURL != "/api/v1/stream/mix" {
			t.Errorf("by id: %+v", one)
		}
		if rec := s.do("GET", "/api/v1/frequencies/nowhere", ""); rec.Code != http.StatusNotFound {
			t.Errorf("unknown frequency: %d, want 404", rec.Code)
		}
	})

	// What a frequency is carrying right now, said from outside, until it is
	// cleared; cmd/radio-ctl-sync sets it when the station switches groups.
	t.Run("a label overrides the name until cleared", func(t *testing.T) {
		s := shared.with(t)
		rec := s.do("PUT", "/api/v1/frequencies/mix/label", `{"label":"  CDG approaches  "}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT label: %d %s", rec.Code, rec.Body.String())
		}
		var f frequencies.Frequency
		decode(t, rec, &f)
		if f.Name != "CDG approaches" {
			t.Errorf("answered name %q, want the label, trimmed", f.Name)
		}
		var page frequenciesPage
		s.get("/api/v1/frequencies", &page)
		if page.byID("mix").Name != "CDG approaches" {
			t.Errorf("listed name %q, want the label", page.byID("mix").Name)
		}
		if rec := s.do("PUT", "/api/v1/frequencies/mix/label", `{"label":""}`); rec.Code != http.StatusOK {
			t.Errorf("clearing the label: %d", rec.Code)
		}
		s.get("/api/v1/frequencies/mix", &f)
		if f.Name != "Configured mix" {
			t.Errorf("after clearing: %q, want the configured name back", f.Name)
		}
		if rec := s.do("PUT", "/api/v1/frequencies/mix/label", `{"label":"`+strings.Repeat("l", 121)+`"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("a 121-character label: %d, want 400", rec.Code)
		}
		if rec := s.do("PUT", "/api/v1/frequencies/nowhere/label", `{"label":"x"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("a label on an unknown frequency: %d, want 400", rec.Code)
		}
		if rec := s.do("PUT", "/api/v1/frequencies/mix/label", `{`); rec.Code != http.StatusBadRequest {
			t.Errorf("a malformed label body: %d, want 400", rec.Code)
		}
	})

	// The configuration is the operator's: nothing from outside replaces or
	// drops a frequency it declares.
	t.Run("configured sources answer 409 from outside", func(t *testing.T) {
		s := shared.with(t)
		rec := s.do("PUT", "/api/v1/sources/mix", sourceBody(s.gone, "mix"))
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "configuration") {
			t.Errorf("replacing a configured source: %d %s, want 409", rec.Code, rec.Body.String())
		}
		if rec := s.do("DELETE", "/api/v1/sources/mix", ""); rec.Code != http.StatusConflict {
			t.Errorf("removing a configured source: %d, want 409", rec.Code)
		}
		var f frequencies.Frequency
		s.get("/api/v1/frequencies/mix", &f)
		if f.Runtime || f.Name != "Configured mix" {
			t.Errorf("the configured source after the refusals: %+v", f)
		}
	})
}

// A station publishing one stream per channel adds and removes them while
// the server runs, on one stack.
func TestSourcesFromOutside(t *testing.T) {
	shared := newStack(t, stackOptions{})
	shared.signIn()

	// Each change tells the connected pages to fetch the list again.
	t.Run("added, renamed, removed, and the pages are told", func(t *testing.T) {
		s := shared.with(t)
		conn := s.connectWS()

		rec := s.do("PUT", "/api/v1/sources/125825", sourceBody(s.gone, "125825"))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT source: %d %s", rec.Code, rec.Body.String())
		}
		var added frequencies.Frequency
		decode(t, rec, &added)
		if added.ID != "125825" || !added.Runtime || added.Name != "Channel 125825" || added.FrequencyMHz != 125.825 ||
			added.SectorAirport != "LFPG" || added.SectorKind != "approach" || added.StreamURL != "/api/v1/stream/125825" {
			t.Errorf("the added source as answered: %+v", added)
		}
		waitForMessage(t, conn, "frequencies_changed", 5*time.Second)

		// The same body again changes nothing and says nothing.
		if rec := s.do("PUT", "/api/v1/sources/125825", sourceBody(s.gone, "125825")); rec.Code != http.StatusOK {
			t.Errorf("PUT the same source again: %d", rec.Code)
		}
		// A new name keeps the connection and is announced.
		renamed := strings.Replace(sourceBody(s.gone, "125825"), "Channel 125825", "Renamed", 1)
		if rec := s.do("PUT", "/api/v1/sources/125825", renamed); rec.Code != http.StatusOK {
			t.Errorf("PUT a renamed source: %d", rec.Code)
		}
		waitForMessage(t, conn, "frequencies_changed", 5*time.Second)
		var page frequenciesPage
		s.get("/api/v1/frequencies", &page)
		if f := page.byID("125825"); f == nil || f.Name != "Renamed" {
			t.Errorf("after the rename: %+v", f)
		}

		rec = s.do("DELETE", "/api/v1/sources/125825", "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("DELETE source: %d %s", rec.Code, rec.Body.String())
		}
		waitForMessage(t, conn, "frequencies_changed", 5*time.Second)
		s.get("/api/v1/frequencies", &page)
		if page.byID("125825") != nil || page.Count != 1 {
			t.Errorf("after the removal: %v", page.Frequencies)
		}
		if rec := s.do("DELETE", "/api/v1/sources/125825", ""); rec.Code != http.StatusNotFound {
			t.Errorf("DELETE a removed source: %d, want 404", rec.Code)
		}
	})

	// What PUT /sources refuses, and how: each with a 400 that names the field.
	t.Run("malformed sources are refused with a reason", func(t *testing.T) {
		s := shared.with(t)
		good := sourceBody(s.gone, "ok")
		bad := []struct{ name, id, body, says string }{
			{"not json", "x", `{`, "invalid request"},
			{"unknown field", "x", strings.Replace(good, `"order":5`, `"order":5,"colour":"red"`, 1), "unknown field"},
			{"id with a space", "bad%20id", good, "id"},
			{"id too long", strings.Repeat("a", 65), good, "id"},
			{"a file", "x", strings.Replace(good, s.gone+"/aero-ok.mp3", "file:///etc/passwd", 1), "network address"},
			{"a device", "x", strings.Replace(good, s.gone+"/aero-ok.mp3", ":0", 1), "network address"},
			{"no url", "x", strings.Replace(good, s.gone+"/aero-ok.mp3", "", 1), "network address"},
			{"language", "x", strings.Replace(good, `"language":"en"`, `"language":"de"`, 1), "language"},
			{"sector kind", "x", strings.Replace(good, `"sector_kind":"approach"`, `"sector_kind":"centre"`, 1), "sector_kind"},
			{"sector without airport", "x", strings.Replace(good, `"sector_airport":"LFPG",`, "", 1), "go together"},
			{"sector airport", "x", strings.Replace(good, `"sector_airport":"LFPG"`, `"sector_airport":"Paris"`, 1), "ICAO"},
			{"name too long", "x", strings.Replace(good, "Channel ok", strings.Repeat("n", 121), 1), "name"},
			{"ffmpeg options, gone with ffmpeg", "x", strings.Replace(good, `"order":5`,
				`"order":5,"ffmpeg_input_options":["-f","s16le","-ar","48000"]`, 1), "unknown field"},
			{"raw PCM without its rate", "x", strings.Replace(good, s.gone+"/aero-ok.mp3", "udp://127.0.0.1:7355", 1), "pcm_rate"},
			{"raw PCM in three channels", "x", strings.Replace(strings.Replace(good, s.gone+"/aero-ok.mp3", "udp://127.0.0.1:7355", 1),
				`"order":5`, `"order":5,"pcm_rate":48000,"pcm_channels":3`, 1), "pcm_channels"},
			{"a 5 KB body", "x", strings.Replace(good, "Channel ok", strings.Repeat("n", 5000), 1), "invalid request"},
		}
		for _, b := range bad {
			rec := s.do("PUT", "/api/v1/sources/"+b.id, b.body)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), b.says) {
				t.Errorf("%s: %d %q, want 400 mentioning %q", b.name, rec.Code, strings.TrimSpace(rec.Body.String()), b.says)
			}
		}
		var page frequenciesPage
		s.get("/api/v1/frequencies", &page)
		if page.Count != 1 {
			t.Errorf("a refused source was listed anyway: %v", page.Frequencies)
		}
		// Raw PCM over UDP with its rate is a source; and radio-ctl-sync's
		// ffmpeg_reconnect, from before ffmpeg went, is still taken, and ignored.
		rec := s.do("PUT", "/api/v1/sources/pcm", fmt.Sprintf(
			`{"name":"PCM","url":"udp://127.0.0.1:%d","pcm_rate":48000,"pcm_channels":1,"ffmpeg_reconnect":false}`,
			audiotest.FreeUDPPort(t)))
		if rec.Code != http.StatusOK {
			t.Errorf("raw PCM over UDP with its rate: %d %s", rec.Code, rec.Body.String())
		}
	})
}

// Behind a trusted proxy the browser reaches only the address the proxy
// serves: no stream port is offered, and the page asks for the stream on its
// own origin. A browser talking to co-atc directly still gets the ports in
// turn. (httptest.NewRequest comes from 192.0.2.1.)
func TestNoStreamPortBehindATrustedProxy(t *testing.T) {
	s := newStack(t, stackOptions{proxies: []string{"192.0.2.1"}})
	s.signIn()
	forwarded := func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
	}

	var page frequenciesPage
	rec := s.do("GET", "/api/v1/frequencies", "", forwarded)
	decode(t, rec, &page)
	if len(page.Frequencies) == 0 {
		t.Fatal("no frequency listed")
	}
	for _, f := range page.Frequencies {
		if f.StreamPort != 0 {
			t.Errorf("%s behind the proxy: stream_port %d, want none", f.ID, f.StreamPort)
		}
	}
	var one frequencies.Frequency
	decode(t, s.do("GET", "/api/v1/frequencies/mix", "", forwarded), &one)
	if one.ID != "mix" || one.StreamPort != 0 {
		t.Errorf("by id behind the proxy: %s, stream_port %d", one.ID, one.StreamPort)
	}

	// The same address without a forwarded header is a direct visit.
	s.get("/api/v1/frequencies", &page)
	if page.Frequencies[0].StreamPort == 0 {
		t.Error("a direct visit lost its stream port")
	}
}
