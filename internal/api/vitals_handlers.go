package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/pkg/logger"
)

// The station's vital signs, shown on the sign-in page before anyone signs in
// (docs-fr/design, StationVitals): a few aggregated figures that say the
// station is alive and what it does.
//
// The route is public on a server that listens on the local network, so two
// rules hold. It says nothing that locates the station -- no position, no
// callsign, no named frequency, no airport, no address: the farthest distance
// is computed from the configured position, and a distance locates nothing.
// And a visit never costs a query: the figures are computed at most once per
// vitalsTTL and shared by every visit, the sidecar is not called (its last
// health reading is used), and the encoded answer is served from memory.

// vitalsTTL is how long one computation serves every visit.
const vitalsTTL = 10 * time.Second

// Vitals is what /api/v1/vitals serves, and nothing else: a test enumerates
// these keys. A figure that is not known is absent rather than zero.
type Vitals struct {
	Aircraft          int  `json:"aircraft"`              // active aircraft in sight
	MessagesPerSecond int  `json:"messages_per_second"`   // the receiver's own counter, rounded
	FarthestNM        *int `json:"farthest_nm,omitempty"` // from the station, rounded to the mile

	ChannelsOnAir      int `json:"channels_on_air"` // audio sources currently connected
	TransmissionsToday int `json:"transmissions_today"`
	MatchedToday       int `json:"matched_today"`
	MatchedShare       int `json:"matched_share"` // percent of today's, rounded

	TranscriptionModel     string   `json:"transcription_model,omitempty"` // a readable family
	TranscriptionBackend   string   `json:"transcription_backend,omitempty"`
	SecondsPerTransmission *float64 `json:"seconds_per_transmission,omitempty"`

	AirportsFollowed int `json:"airports_followed"`
	RunwaysInUse     int `json:"runways_in_use"`

	UptimeSeconds int64  `json:"uptime_seconds"`
	Version       string `json:"version,omitempty"`
	Commit        string `json:"commit,omitempty"`
	Page          string `json:"page"` // "embedded" or "directory"; never which directory, a path can name a user
}

// vitalsCache holds the last answer. now and compute are the clock and the
// computation, replaced by tests; nil means time.Now and the handler's own.
type vitalsCache struct {
	mu      sync.Mutex
	at      time.Time
	body    []byte
	now     func() time.Time
	compute func() Vitals
}

// GetVitals serves the station's vital signs, without a session. Turned off
// from the settings panel, the route answers 404 as if it did not exist.
func (h *Handler) GetVitals(w http.ResponseWriter, r *http.Request) {
	if h.runtime != nil && !h.runtime.VitalsEnabled() {
		http.NotFound(w, r)
		return
	}

	c := &h.vitals
	c.mu.Lock()
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	compute := h.computeVitals
	if c.compute != nil {
		compute = c.compute
	}
	// Under the lock on purpose: visits that arrive while the figures are being
	// computed wait for them rather than computing them again.
	if t := now(); c.body == nil || t.Sub(c.at) >= vitalsTTL {
		body, err := json.Marshal(compute())
		if err != nil {
			c.mu.Unlock()
			h.logger.Error("Failed to encode the vitals", logger.Error(err))
			http.Error(w, "vitals unavailable", http.StatusInternalServerError)
			return
		}
		c.body, c.at = append(body, '\n'), t
	}
	body := c.body
	c.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// computeVitals gathers the figures. Every source is optional: a server
// without frequencies, without a sidecar or before its first fetch still
// answers, with what it has.
func (h *Handler) computeVitals() Vitals {
	v := Vitals{UptimeSeconds: int64(time.Since(h.startedAt).Seconds())}
	v.Version, v.Commit = h.build()
	v.Page = PageOf(h.config).Source

	if h.adsbService != nil {
		v.MessagesPerSecond = int(math.Round(h.adsbService.MessageRate()))
		lat, lon := h.config.Station.Latitude, h.config.Station.Longitude
		farthest := -1.0
		for _, a := range h.adsbService.GetAllAircraft() {
			if a.Status != "active" {
				continue
			}
			v.Aircraft++
			if a.ADSB == nil || a.ADSB.Lat == nil || a.ADSB.Lon == nil || (lat == 0 && lon == 0) {
				continue
			}
			if d := adsb.Haversine(lat, lon, *a.ADSB.Lat, *a.ADSB.Lon) / 1852; d > farthest {
				farthest = d
			}
		}
		if farthest >= 0 {
			nm := int(math.Round(farthest))
			v.FarthestNM = &nm
		}
		v.AirportsFollowed = len(h.adsbService.PhaseReferences())
		v.RunwaysInUse = h.adsbService.RunwaysInUse()
	}

	if h.frequenciesService != nil {
		for _, f := range h.frequenciesService.GetAllFrequencies() {
			if f.Status == "connected" {
				v.ChannelsOnAir++
			}
		}
		if mean, ok := h.frequenciesService.MeanTranscriptionSeconds(); ok {
			rounded := math.Round(mean*10) / 10
			v.SecondsPerTransmission = &rounded
		}
	}

	if h.transcriptionStorage != nil {
		total, matched, err := h.transcriptionStorage.CountTransmissions()
		if err != nil {
			h.logger.Warn("Vitals: could not count today's transmissions", logger.Error(err))
		} else {
			v.TransmissionsToday, v.MatchedToday = total, matched
			if total > 0 {
				v.MatchedShare = int(math.Round(100 * float64(matched) / float64(total)))
			}
		}
	}

	if h.sttSidecar != nil {
		// The last reading, not a fresh one: a public route must not reach the
		// sidecar, which is busy decoding. The model does not change while the
		// sidecar runs, and every start and restart reads its health.
		health := h.sttSidecar.LastHealth()
		v.TranscriptionModel = ModelFamily(health.ModelEN)
		if b := strings.TrimSpace(health.Backend); len(b) <= 48 {
			v.TranscriptionBackend = b
		}
	}

	return v
}

// ModelFamily turns a model's configured name -- a Hugging Face id or a local
// path -- into a readable family:
//
//	sfabriece/whisper-large-v3-atco2-asr-mlx -> Whisper large-v3 ATCO2
//	mlx-community/whisper-large-v3-turbo     -> Whisper large-v3 turbo
//	/Volumes/Models/whisper-large-v3-french-mlx -> Whisper large-v3 French
//
// The rule: keep the last path segment only (the owner and any local directory
// are dropped -- a path can carry a user name); split it on "-" and "_"; drop
// the packaging words (mlx, asr, hf, ct2, ggml, gguf, fp16, fp32, int8, 4bit,
// 8bit, q4, q8); join a size to the version after it ("large" "v3" ->
// "large-v3"); capitalise the first word and a language name; write in capitals
// a word of letters ending in digits that is not a version (atco2 -> ATCO2, a
// dataset). The rest stays as written. At most 48 characters.
func ModelFamily(name string) string {
	name = strings.TrimRight(strings.TrimSpace(name), "/")
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '-' || r == '_' })

	var out []string
	for i := 0; i < len(words); i++ {
		w := words[i]
		if modelPackaging[w] {
			continue
		}
		switch {
		case modelSizes[w] && i+1 < len(words) && isModelVersion(words[i+1]):
			w += "-" + words[i+1]
			i++
		case modelLanguages[w]:
			w = strings.ToUpper(w[:1]) + w[1:]
		case isModelVersion(w):
		case isDatasetName(w):
			w = strings.ToUpper(w)
		}
		if len(out) == 0 {
			w = strings.ToUpper(w[:1]) + w[1:]
		}
		out = append(out, w)
	}
	s := strings.Join(out, " ")
	if len(s) > 48 {
		s = strings.TrimSpace(s[:48])
	}
	return s
}

var (
	modelPackaging = map[string]bool{"mlx": true, "asr": true, "hf": true, "ct2": true, "ggml": true,
		"gguf": true, "fp16": true, "fp32": true, "int8": true, "4bit": true, "8bit": true, "q4": true, "q8": true}
	modelSizes     = map[string]bool{"tiny": true, "base": true, "small": true, "medium": true, "large": true}
	modelLanguages = map[string]bool{"english": true, "french": true, "german": true, "spanish": true, "italian": true}
)

// isModelVersion is "v2", "v3", "v3.5": a v then digits.
func isModelVersion(w string) bool {
	if len(w) < 2 || w[0] != 'v' {
		return false
	}
	for _, r := range w[1:] {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

// isDatasetName is letters then digits, "atco2", "atcosim1".
func isDatasetName(w string) bool {
	i := 0
	for i < len(w) && w[i] >= 'a' && w[i] <= 'z' {
		i++
	}
	if i == 0 || i == len(w) {
		return false
	}
	for _, r := range w[i:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
