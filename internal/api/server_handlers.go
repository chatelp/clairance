package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"context"
	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/reference"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/pkg/disk"
	"github.com/yegors/co-atc/pkg/logger"
)

// ServerState is what the settings panel shows about the running server.
//
// It exists because the only way to know how co-atc was actually behaving was to
// read its code: nothing surfaced that the active database grows without bound and
// is never rotated. A figure on screen finds that in a glance; a code reading finds
// it if someone happens to look.
type ServerState struct {
	Version       string                 `json:"version"`
	Commit        string                 `json:"commit"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	Storage       StorageState           `json:"storage"`
	Logging       LoggingState           `json:"logging"`
	Settings      config.RuntimeSettings `json:"settings"`
	Writable      bool                   `json:"writable"`

	// Absent when the transcription backend is not the local sidecar.
	Transcription *TranscriptionState `json:"transcription,omitempty"`

	// The airport phases are judged against, and those that could replace it.
	Reference *ReferenceState `json:"reference,omitempty"`

	// Local-only or sign-in, and whether local-only is possible here.
	Access auth.AccessState `json:"access"`
}

// ReferenceState is what the panel needs to choose a reference airport without
// knowing ICAO codes by heart: the one in force, and the nearby airports whose
// runways phase detection can actually use.
type ReferenceState struct {
	Airport    string                       `json:"airport"`
	Also       []string                     `json:"also,omitempty"` // followed besides it, in force
	Name       string                       `json:"name,omitempty"`
	DistanceNM float64                      `json:"distance_nm"` // from the receiver
	Runways    int                          `json:"runways"`
	Candidates []reference.AirportCandidate `json:"candidates"`
}

// referenceCandidateRangeNM bounds the airports offered. Phases are about the
// last and first few thousand feet around an airport; a receiver farther than
// this rarely sees that low, whatever it hears at altitude.
const referenceCandidateRangeNM = 50

// TranscriptionState is the half of the product that used to have no operational
// state at all. Degraded is the field worth reading: a model that has become
// unreachable does not stop the server, it stops one of its capabilities, and
// the only other symptom is fewer aircraft identified with no stated cause.
type TranscriptionState struct {
	Reachable           bool     `json:"reachable"`
	Stale               bool     `json:"stale,omitempty"` // the reading is the last one, not a fresh probe
	Status              string   `json:"status"`          // "ok", "degraded", "unknown", "off" (nothing transcribes)
	Degraded            []string `json:"degraded,omitempty"`
	Detail              string   `json:"detail,omitempty"`
	SecondOpinion       bool     `json:"second_opinion"`
	SecondOpinionOK     int      `json:"second_opinion_ok"`
	SecondOpinionFailed int      `json:"second_opinion_failed"`
}

type StorageState struct {
	Dir                string      `json:"dir"`
	ActiveDB           string      `json:"active_db"`
	ActiveDBBytes      int64       `json:"active_db_bytes"`
	GrowthBytesPerHour int64       `json:"growth_bytes_per_hour"` // -1 while still measuring
	Rotates            bool        `json:"rotates"`
	RetentionGB        float64     `json:"retention_gb"`
	DailyFiles         []DailyFile `json:"daily_files"`
	TotalBytes         int64       `json:"total_bytes"`
	DiskFreeBytes      int64       `json:"disk_free_bytes"`
	DiskTotalBytes     int64       `json:"disk_total_bytes"`
	DaysUntilFull      float64     `json:"days_until_full"` // -1 when not computable
}

type DailyFile struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	Active bool   `json:"active"`
}

type LoggingState struct {
	Level string `json:"level"`
	// Where the log goes and what bounds it. Empty File means stdout only, which
	// is upstream's behaviour and leaves whatever it is redirected into unbounded.
	File      string `json:"file,omitempty"`
	MaxSizeMB int    `json:"max_size_mb,omitempty"`
	MaxFiles  int    `json:"max_files,omitempty"`
	Bounded   bool   `json:"bounded"`
	TotalMB   int64  `json:"total_mb,omitempty"`
}

// GetServerState reports how the server is actually running.
func (h *Handler) GetServerState(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		http.Error(w, "server state unavailable", http.StatusServiceUnavailable)
		return
	}

	version, commit := h.build()
	st := ServerState{
		Version:       version,
		Commit:        commit,
		UptimeSeconds: int64(time.Since(h.startedAt).Seconds()),
		Logging:       h.loggingState(),
		Settings:      h.runtime.Settings(),
		Writable:      false, // flips once authentication is in place
		Access:        h.auth.Access(),
	}
	// Who else has an account is the owner's to know: a viewer is told the
	// mode, not the names, which are half of what signing in asks for.
	if h.auth.Role(r) == auth.RoleViewer {
		st.Access.Accounts = []string{}
	}

	if h.sttSidecar != nil && h.sttSidecar.Off() {
		st.Transcription = &TranscriptionState{Status: "off", Detail: "no source transcribes"}
	} else if h.sttSidecar != nil {
		// A short timeout on purpose: the sidecar is busy decoding for seconds at
		// a time, and an operational endpoint must not block behind it. Failing
		// to reach it in two seconds does not mean it is down, so we fall back to
		// what it last said and mark the reading stale rather than inventing an
		// outage.
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		health, err := h.sttSidecar.Refresh(ctx)
		cancel()

		ts := &TranscriptionState{
			Reachable:           err == nil,
			Stale:               err != nil,
			Status:              health.Status,
			Degraded:            health.Degraded,
			SecondOpinion:       health.SecondOpinion,
			SecondOpinionOK:     health.SecondOpinionOK,
			SecondOpinionFailed: health.SecondOpinionFailed,
		}
		if ts.Status == "" {
			ts.Status = "unknown"
		}
		for _, lang := range health.Degraded {
			if m, ok := health.Models[lang]; ok {
				ts.Detail = m.ID + ": " + m.Detail
			}
		}
		if err != nil {
			ts.Detail = strings.TrimSpace(ts.Detail + " (not reached just now: " + err.Error() + ")")
		}
		st.Transcription = ts
	}

	if h.refService != nil && h.adsbService != nil {
		st.Reference = h.referenceState()
	}

	dir := h.config.Storage.SQLiteBasePath
	st.Storage = StorageState{
		Dir:         dir,
		ActiveDB:    h.db.Path(),
		RetentionGB: h.runtime.DBRetentionGB(),
		// True since the daily database began rotating at midnight. It used to be
		// opened once at startup and never reopened, so a long-running server
		// wrote one unbounded file that retention skipped as the active one. The
		// field stays because it is the difference between a database that is
		// bounded and one that is not, and a reader deserves to see which.
		Rotates:            true,
		GrowthBytesPerHour: -1,
		DaysUntilFull:      -1,
	}

	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "co-atc-") || !strings.HasSuffix(e.Name(), ".db") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			active := filepath.Base(h.db.Path()) == e.Name()
			st.Storage.DailyFiles = append(st.Storage.DailyFiles, DailyFile{e.Name(), info.Size(), active})
			st.Storage.TotalBytes += info.Size()
			if active {
				st.Storage.ActiveDBBytes = info.Size()
			}
		}
		sort.Slice(st.Storage.DailyFiles, func(i, j int) bool {
			return st.Storage.DailyFiles[i].Name > st.Storage.DailyFiles[j].Name
		})
	}

	if free, total, ok := disk.Space(dir); ok {
		st.Storage.DiskFreeBytes = free
		st.Storage.DiskTotalBytes = total
	}

	// Growth is measured against a sample taken at start rather than divided by the
	// file's age: the file survives restarts, so its age says nothing about how fast
	// this process is filling it.
	if elapsed := time.Since(h.dbSampleAt); elapsed > 5*time.Minute && st.Storage.ActiveDBBytes > h.dbSampleBytes {
		perHour := float64(st.Storage.ActiveDBBytes-h.dbSampleBytes) / elapsed.Hours()
		st.Storage.GrowthBytesPerHour = int64(perHour)
		if perHour > 0 && st.Storage.DiskFreeBytes > 0 {
			st.Storage.DaysUntilFull = float64(st.Storage.DiskFreeBytes) / perHour / 24
		}
	}

	WriteJSON(w, http.StatusOK, st)
}

// AttachRuntime gives the handler the live settings and the database handle.
// Kept out of NewHandler so the two upstream constructors it sits between do
// not have to grow another parameter.
//
// It takes the handle rather than a path because the file changes: the daily
// database rotates at midnight, and a path captured here would name yesterday's.
func (h *Handler) AttachRuntime(rt *config.Runtime, db *sqlite.DB, stt *transcription.Sidecar) {
	h.runtime = rt
	h.db = db
	h.sttSidecar = stt
	if info, err := os.Stat(db.Path()); err == nil {
		h.dbSampleBytes = info.Size()
		h.dbSampleAt = time.Now()
	}
}

// SetBuild gives the handler the binary's version and commit, which main
// receives from the linker (-ldflags "-X main.version=… -X main.commit=…").
func (h *Handler) SetBuild(version, commit string) {
	h.version, h.commit = version, commit
}

// build is the version and commit, "dev" and "unknown" when none was set.
func (h *Handler) build() (version, commit string) {
	version, commit = h.version, h.commit
	if version == "" {
		version = "dev"
	}
	if commit == "" {
		commit = "unknown"
	}
	return version, commit
}

// loggingState reports where the log goes and whether anything bounds it. The
// question it answers is the one nobody asks until a disk is full: is this
// growing for ever?
func (h *Handler) loggingState() LoggingState {
	st := LoggingState{
		Level:     logger.Level(),
		File:      h.config.Logging.File,
		MaxSizeMB: h.config.Logging.MaxSizeMB,
		MaxFiles:  h.config.Logging.MaxFiles,
		Bounded:   h.config.Logging.File != "",
	}
	if st.File == "" {
		return st
	}
	dir := filepath.Dir(st.File)
	base := strings.TrimSuffix(filepath.Base(st.File), filepath.Ext(st.File))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return st
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), base+"-") {
			continue
		}
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	st.TotalMB = total / (1024 * 1024)
	return st
}

// PutServerSettings changes what can be changed while the server runs.
//
// Only two things can: the log level and the database retention. Everything else
// on the settings page is reported and not editable, because changing it means
// reopening something -- the log file, the audio pipeline, the listening port --
// and a settings page that silently fails to apply half of what it shows is
// worse than one that says which half it can.
//
// Behind authentication, unlike the read: retention decides when data is
// deleted, and that is not a knob for anyone who can reach the port.
func (h *Handler) PutServerSettings(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		http.Error(w, "runtime settings unavailable", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil {
		http.Error(w, "invalid settings: "+err.Error(), http.StatusBadRequest)
		return
	}
	var next config.RuntimeSettings
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &next); err != nil {
		http.Error(w, "invalid settings: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.Unmarshal(body, &present)

	// Absent fields keep their current value rather than becoming zero, so a
	// client that only wants to change the log level does not silently set the
	// retention to nothing.
	current := h.runtime.Settings()
	if next.DBRetentionGB == 0 {
		next.DBRetentionGB = current.DBRetentionGB
	}
	if next.LogLevel == "" {
		next.LogLevel = current.LogLevel
	}
	if next.ReferenceAirport == "" {
		next.ReferenceAirport = current.ReferenceAirport
	}
	// An empty list is a choice -- the reference airport alone -- so only an
	// absent one keeps the airports in force.
	if _, ok := present["also_airports"]; !ok {
		next.AlsoAirports = current.AlsoAirports
	}
	if next.Matching == nil {
		next.Matching = current.Matching
	}
	if next.Vitals == nil {
		next.Vitals = current.Vitals
	}
	if next.AIPNames == nil {
		next.AIPNames = current.AIPNames
	}

	if err := h.runtime.Apply(next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Who changed it, when authentication is on. A retention that quietly halves
	// one morning should be traceable to someone.
	by := "anonymous"
	if sess, ok := h.auth.SessionFrom(r); ok {
		by = sess.User
	}
	h.logger.Info("Server settings changed from the panel",
		logger.String("by", by),
		logger.Float64("db_retention_gb", next.DBRetentionGB),
		logger.String("log_level", next.LogLevel),
		logger.String("reference_airport", next.ReferenceAirport),
		logger.String("also_airports", strings.Join(next.AlsoAirports, ",")),
		logger.Bool("match_letters", next.Matching.Letters),
		logger.Bool("match_approx_operators", next.Matching.ApproxOperators),
		logger.Int("match_min_digits", next.Matching.MinDigits),
		logger.Bool("match_one_digit_off", next.Matching.OneDigitOff),
		logger.Bool("match_context_letters", next.Matching.ContextLetters),
		logger.Bool("match_context_digits", next.Matching.ContextDigits),
		logger.Bool("match_context_names", next.Matching.ContextNames),
		logger.Bool("vitals_enabled", next.Vitals == nil || next.Vitals.Enabled),
		logger.Bool("aip_auto_update", next.AIPNames != nil && next.AIPNames.AutoUpdate))

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(h.runtime.Settings()); err != nil {
		h.logger.Error("Failed to encode settings response", logger.Error(err))
	}
}

// referenceState describes the reference airport in force and the candidates.
func (h *Handler) referenceState() *ReferenceState {
	code := h.adsbService.PhaseReference().Airport
	st := &ReferenceState{
		Airport:    code,
		Candidates: h.refService.HomeAirportCandidates(referenceCandidateRangeNM),
	}
	for _, ref := range h.adsbService.PhaseReferences()[1:] {
		st.Also = append(st.Also, ref.Airport)
	}
	for _, c := range st.Candidates {
		if c.Code == code {
			st.Name, st.DistanceNM, st.Runways = c.Name, c.DistanceNM, c.Runways
		}
	}
	// In force but not among the candidates -- farther than the range, set by
	// hand in the configuration. Still say what it is.
	if st.Name == "" {
		if a := h.refService.GetAirport(code); a != nil {
			st.Name = a.Name
			st.Runways = len(h.refService.GetHomeRunwayData().RunwayThresholds)
		}
	}
	return st
}
