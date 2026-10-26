package api

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/startcheck"
	"github.com/yegors/co-atc/pkg/disk"
	"github.com/yegors/co-atc/pkg/logger"
)

// The launcher's health (docs-fr/34): what the menu-bar launcher polls, every
// 15 s, to tell a running server from a working one. From 16 to 18/10 the
// process ran, every frequency read "connected", and nothing was transcribed
// for a day and a half; this route says whether audio arrives, whether it
// becomes text, whether the ADS-B answers, and what the sidecar, the archive
// and the disks are doing.
//
// Read-only, and for this machine or the owner: a request straight from the
// loopback with no session, or an owner's session from anywhere. A visit
// costs no query -- every figure is in memory, the sidecar is read at most
// every 15 s, the archive's size is the hourly purge's -- and one computation
// serves every visit for healthDetailTTL.

// healthDetailTTL is how long one computation serves every visit.
const healthDetailTTL = 5 * time.Second

// HealthDetail is what /api/v1/health/detail serves. A figure that is not
// known is absent rather than zero; times are UTC.
type HealthDetail struct {
	PID           int    `json:"pid"`
	UptimeSeconds int64  `json:"uptime_s"`
	Version       string `json:"version,omitempty"`
	Commit        string `json:"commit,omitempty"`
	// Where the page comes from: built in, with the binary's commit, or a
	// directory (server.static_dir).
	Page PageSource `json:"page"`

	ADSB         *HealthADSB                   `json:"adsb,omitempty"`
	Frequencies  []frequencies.FrequencyHealth `json:"frequencies"`
	Sidecar      *HealthSidecar                `json:"sidecar,omitempty"`
	Receiver     *receiverState                `json:"receiver"`
	AudioArchive HealthArchive                 `json:"audio_archive"`
	Databases    HealthDatabases               `json:"databases"`

	// What the startup check found missing, with what suffers from each
	// (internal/startcheck); empty when nothing is. Said at start: a program
	// installed since is found when it is next run, and listed until a restart.
	Missing []startcheck.Missing `json:"missing"`
	// Added by the launcher's contract (docs-fr/36-contrat-lanceur.md,
	// process.go): its version, the process, the companions.
	Contract   int               `json:"contract"`
	Process    HealthProcess     `json:"process"`
	Companions []HealthCompanion `json:"companions"`
}

type HealthADSB struct {
	OK                bool       `json:"ok"`
	LastFetch         *time.Time `json:"last_fetch,omitempty"`
	FailingSince      *time.Time `json:"failing_since,omitempty"`
	FailingWhy        string     `json:"failing_why,omitempty"`
	Aircraft          int        `json:"aircraft"`
	MessagesPerSecond float64    `json:"messages_per_second"`
}

// HealthSidecar is the transcription sidecar as last read. Status is "off"
// when nothing transcribes and no sidecar was started.
type HealthSidecar struct {
	Reachable bool     `json:"reachable"`
	Status    string   `json:"status"`
	Degraded  []string `json:"degraded,omitempty"`
	PID       int      `json:"pid,omitempty"`
	Queue     *struct {
		First  int `json:"first"`
		Second int `json:"second"`
	} `json:"queue,omitempty"`
	SecondPending           int        `json:"second_pending"`
	Archiving               bool       `json:"archiving"`
	ArchivingStoppedLowDisk bool       `json:"archiving_stopped_low_disk"`
	ReadAt                  *time.Time `json:"read_at,omitempty"`
}

// HealthArchive is the sidecar's archive of clips. Bytes is what the hourly
// purge measured, at MeasuredAt: absent before the first purge and when no
// cap is set (CapBytes 0), since then nothing walks the archive.
type HealthArchive struct {
	Available     bool       `json:"available"`
	Bytes         *int64     `json:"bytes,omitempty"`
	MeasuredAt    *time.Time `json:"measured_at,omitempty"`
	CapBytes      int64      `json:"cap_bytes"`
	DiskFreeBytes *int64     `json:"disk_free_bytes,omitempty"`
}

// HealthDatabases is the daily databases' directory.
type HealthDatabases struct {
	DirBytes      *int64 `json:"dir_bytes,omitempty"`
	DiskFreeBytes *int64 `json:"disk_free_bytes,omitempty"`
}

type healthDetailCache struct {
	mu   sync.Mutex
	at   time.Time
	body []byte
}

// GetHealthDetail serves the launcher's health; 403 to anyone but this
// machine or the owner.
func (h *Handler) GetHealthDetail(w http.ResponseWriter, r *http.Request) {
	if !h.mayReadHealthDetail(r) {
		WriteJSON(w, http.StatusForbidden, map[string]string{"error": "this machine or the owner only"})
		return
	}

	c := &h.healthDetail
	c.mu.Lock()
	// Under the lock on purpose, as for the vitals: visits that arrive while
	// the figures are computed wait for them rather than computing them again.
	if now := time.Now(); c.body == nil || now.Sub(c.at) >= healthDetailTTL {
		body, err := json.Marshal(h.computeHealthDetail(r))
		if err != nil {
			c.mu.Unlock()
			h.logger.Error("Failed to encode the health detail", logger.Error(err))
			http.Error(w, "health detail unavailable", http.StatusInternalServerError)
			return
		}
		c.body, c.at = append(body, '\n'), now
	}
	body := c.body
	c.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// mayReadHealthDetail: a request from this machine, or the owner's session.
func (h *Handler) mayReadHealthDetail(r *http.Request) bool {
	// Role is "" without a session, and owner when sign-in is off -- which
	// only a server listening on the loopback alone may be.
	return fromThisMachine(r) || (h.auth != nil && h.auth.Role(r) == auth.RoleOwner)
}

// fromThisMachine says that a request comes from this machine itself, not
// through it: the connection is from the loopback, the Host is a loopback
// name -- a page elsewhere that rebinds its own name to 127.0.0.1 still sends
// its name -- and no proxy header says it was forwarded. A reverse proxy on
// this machine (tailscale serve, Caddy) connects from the loopback on behalf
// of someone who is not here; it says so in X-Forwarded-For or Forwarded, and
// sends the name it was reached by.
func fromThisMachine(r *http.Request) bool {
	if forwardedByAProxy(r) {
		return false
	}
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remote = r.RemoteAddr
	}
	if ip := net.ParseIP(remote); ip == nil || !ip.IsLoopback() {
		return false
	}
	// URLSession and curl send the port: "127.0.0.1:8000".
	host := r.Host
	if hp, _, err := net.SplitHostPort(host); err == nil {
		host = hp
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return isLoopback(strings.ToLower(host))
}

// forwardedByAProxy says that a request carries a header only a proxy adds,
// declared in trusted_proxies or not: whoever made it spoke to the proxy, not
// to this server, and may be anywhere the proxy serves.
func forwardedByAProxy(r *http.Request) bool {
	for _, hdr := range []string{"X-Forwarded-For", "Forwarded", "X-Real-IP", "X-Forwarded-Host"} {
		if r.Header.Get(hdr) != "" {
			return true
		}
	}
	return false
}

// computeHealthDetail gathers the figures. Every source is optional: a
// server without frequencies, without a sidecar or before its first fetch
// still answers, with what it has.
func (h *Handler) computeHealthDetail(r *http.Request) HealthDetail {
	d := HealthDetail{PID: os.Getpid(), UptimeSeconds: int64(time.Since(h.startedAt).Seconds())}
	d.Version, d.Commit = h.build()
	d.Page = PageOf(h.config)
	if d.Page.Source == "embedded" {
		d.Page.Commit = d.Commit
	}

	if h.adsbService != nil {
		last, ok := h.adsbService.GetStatus()
		a := &HealthADSB{OK: ok, LastFetch: utcOrNil(last), MessagesPerSecond: math.Round(h.adsbService.MessageRate()*10) / 10}
		for _, ac := range h.adsbService.GetAllAircraft() {
			if ac.Status == "active" {
				a.Aircraft++
			}
		}
		if since, why := h.adsbService.FetchFailure(); !since.IsZero() {
			a.FailingSince, a.FailingWhy = utcOrNil(since), why
		}
		d.ADSB = a
	}

	d.Frequencies = []frequencies.FrequencyHealth{}
	if h.frequenciesService != nil {
		d.Frequencies = h.frequenciesService.Health()
	}

	if h.sttSidecar != nil && h.sttSidecar.Off() {
		d.Sidecar = &HealthSidecar{Status: "off"}
	} else if h.sttSidecar != nil {
		health, reached, at := h.sidecarHealth(r.Context())
		s := &HealthSidecar{Reachable: reached, Status: health.Status, Degraded: health.Degraded, PID: health.PID,
			SecondPending: health.SecondPending, Archiving: health.Archiving,
			ArchivingStoppedLowDisk: health.ArchivingStoppedLowDisk, ReadAt: utcOrNil(at)}
		if s.Status == "" {
			s.Status = "unknown"
		}
		if reached {
			s.Queue = &struct {
				First  int `json:"first"`
				Second int `json:"second"`
			}{health.Queue.First, health.Queue.Second}
		}
		d.Sidecar = s
	}

	d.Receiver = h.receiverNow()
	d.Missing = h.missing
	if d.Missing == nil {
		d.Missing = []startcheck.Missing{}
	}

	d.AudioArchive.Available = h.audioArchive.Available()
	if h.config != nil {
		d.AudioArchive.CapBytes = int64(h.config.Transcription.AudioArchiveMaxGB * (1 << 30))
	}
	if bytes, at, ok := h.audioArchive.Size(); ok {
		d.AudioArchive.Bytes, d.AudioArchive.MeasuredAt = &bytes, utcOrNil(at)
	}
	if d.AudioArchive.Available {
		if free, _, ok := disk.Space(h.audioArchive.Root()); ok {
			d.AudioArchive.DiskFreeBytes = &free
		}
	}

	if h.db != nil {
		dir := filepath.Dir(h.db.Path())
		if entries, err := os.ReadDir(dir); err == nil {
			var total int64
			for _, e := range entries {
				// The daily databases and their journals: co-atc-YYYY-MM-DD.db, -wal, -shm.
				if !e.Type().IsRegular() || !strings.HasPrefix(e.Name(), "co-atc-") {
					continue
				}
				if info, err := e.Info(); err == nil {
					total += info.Size()
				}
			}
			d.Databases.DirBytes = &total
		}
		if free, _, ok := disk.Space(dir); ok {
			d.Databases.DiskFreeBytes = &free
		}
	}

	d.Contract = contractVersion
	h.describeProcess(&d)
	return d
}

func utcOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// SetMissing records what the startup check found missing, for the launcher.
func (h *Handler) SetMissing(missing []startcheck.Missing) {
	h.missing = missing
}
