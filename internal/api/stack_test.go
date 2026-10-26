package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// The whole server behind the real routes, with what the browser talks to every
// day: an ADS-B service fed by a fake tar1090, the day's SQLite database, the
// frequencies service, the WebSocket hub, accounts, live settings. Nothing
// reaches the network: the fake receiver and the fake sidecar are httptest
// servers, and audio, when asked for, is a tone ffmpeg generates itself.
//
// access_test.go keeps its smaller fixture, which is enough for the first-run
// page; this one is for the data routes behind the lock.

// The account every stack knows. The hash is Argon2id with tiny parameters,
// which VerifyPassword reads from the hash itself: the production parameters
// cost 0.65 s per check under -race, and a package that signs in thirty times
// would spend its whole budget on them. What is tested here is the API, not
// the cost of a password.
const (
	testUser     = "pierre"
	testPassword = "correcthorsebattery"
	viewerUser   = "guest" // with stackOptions.viewer
)

var (
	testHashOnce sync.Once
	testHash     string
)

func cheapHash(password string) string {
	testHashOnce.Do(func() {
		salt := []byte("sixteen byte salt")[:16]
		key := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, 32)
		testHash = fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, 8*1024, 1, 1,
			base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
	})
	return testHash
}

// The station the stack sits at, and the four aircraft its receiver sees:
// one in cruise, one descending, one climbing out farther away, one on the
// ground. Generic coordinates.
const (
	stationLat = 48.80
	stationLon = 2.05

	hexCruise  = "3c6444" // AFR123, FL350, 26 NM east
	hexDescent = "4ca1d2" // RYR45, 3 000 ft, descending, 18 NM south-east
	hexClimb   = "406a1b" // EZY77, FL200, 34 NM north-east
	hexGround  = "39b4c5" // no callsign, on the ground, 13 NM east

	aircraftSeen = 4
)

func f(v float64) *float64 { return &v }

func stationAircraft() []adsb.ADSBTarget {
	return []adsb.ADSBTarget{
		{Hex: hexCruise, Flight: "AFR123  ", AltBaro: 35000, Lat: f(48.80), Lon: f(2.70), GS: f(450), TAS: f(460),
			Track: f(90), BaroRate: f(0), Squawk: "1000", Category: "A3", Seen: f(0.5), Messages: intp(120)},
		{Hex: hexDescent, Flight: "RYR45", AltBaro: 3000, Lat: f(48.60), Lon: f(2.40), GS: f(180), TAS: f(190),
			Track: f(270), BaroRate: f(-800), Squawk: "4321", Category: "A3", Seen: f(1), Messages: intp(80)},
		{Hex: hexClimb, Flight: "EZY77", AltBaro: 20000, Lat: f(49.10), Lon: f(2.80), GS: f(400), TAS: f(420),
			Track: f(200), BaroRate: f(1500), Squawk: "2000", Category: "A3", Seen: f(0.8), Messages: intp(60)},
		{Hex: hexGround, AltBaro: 0, Lat: f(48.72), Lon: f(2.37), GS: f(8), Track: f(180),
			Squawk: "7000", Seen: f(2), Messages: intp(10)},
	}
}

func intp(v int) *int { return &v }

// fakeTar1090 serves aircraft.json, receiver.json and stats.json the way
// tar1090 does. Its aircraft can be replaced between fetches.
type fakeTar1090 struct {
	srv *httptest.Server
	mu  sync.Mutex
	acs []adsb.ADSBTarget
}

func newFakeTar1090(t *testing.T) *fakeTar1090 {
	t.Helper()
	f := &fakeTar1090{acs: stationAircraft()}
	mux := http.NewServeMux()
	mux.HandleFunc("/aircraft.json", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		acs := f.acs
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"now": float64(time.Now().Unix()), "messages": 1234, "aircraft": acs,
		})
	})
	mux.HandleFunc("/receiver.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":"test","lat":48.80,"lon":2.05}`)
	})
	mux.HandleFunc("/stats.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total":{"messages":1234}}`)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// tone is a source ffmpeg generates itself, in real time, like
// internal/frequencies does in its own tests.
const tone = "sine=frequency=440:sample_rate=16000"

var toneOptions = []string{"-re", "-f", "lavfi"}

type stackOptions struct {
	host      string   // bind address; "127.0.0.1" when empty
	proxies   []string // server.trusted_proxies
	noAccount bool     // local-only, nothing to sign in as
	viewer    bool     // a second account, viewerUser, with the viewer role
	tone      bool     // a configured frequency carrying audio; needs ffmpeg
	goMP3     bool     // a frequency serving MP3 over HTTP, decoded in Go (no ffmpeg)
	sidecar   string   // health URL of a transcription sidecar; none when empty
	idle      bool     // started without transcription, as main does when nothing asks for it
	static    string   // server.static_dir; www next to the package, which has none, when empty
	origins   []string // server.allowed_origins
}

type stack struct {
	t        *testing.T
	dir      string
	cfg      *config.Config
	log      *logger.Logger
	routes   http.Handler
	srv      *httptest.Server // the same routes on a real port, for WebSocket and streaming
	cookie   *http.Cookie
	handler  *Handler
	runtime  *config.Runtime
	adsb     *adsb.Service
	freqs    *frequencies.Service
	ws       *websocket.Server
	aircraft *sqlite.AircraftStorage
	tx       *sqlite.TranscriptionStorage
	clr      *sqlite.ClearanceStorage
	values   *sqlite.PhraseologyStorage
	tar1090  *fakeTar1090
	gone     string // a server whose streams all answer 404, for sources
}

func newStack(t *testing.T, o stackOptions) *stack {
	t.Helper()
	if o.host == "" {
		o.host = "127.0.0.1"
	}
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tar := newFakeTar1090(t)
	gone := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(gone.Close)

	cfg := &config.Config{ConfigPath: filepath.Join(dir, "config.toml")}
	cfg.Server.Host = o.host
	cfg.Server.Port = 8000
	cfg.Server.AdditionalPorts = []int{8001}
	cfg.Server.ReadTimeoutSecs = 5
	cfg.Server.TrustedProxies = o.proxies
	cfg.Server.StaticDir = o.static
	cfg.Server.AllowedOrigins = o.origins
	cfg.ADSB = config.ADSBConfig{SourceType: adsb.SourceTypeTar1090, Tar1090BaseURL: tar.srv.URL,
		FetchIntervalSecs: 3600, SignalLostTimeoutSecs: 60}
	cfg.Station = config.StationConfig{Latitude: stationLat, Longitude: stationLon, ElevationFeet: 400,
		AirportCode: "LFPG", AirportRangeNM: 5, DisplayRangeNM: 100, RunwayExtensionLengthNM: 10}
	cfg.Storage = config.StorageConfig{Type: "sqlite", SQLiteBasePath: filepath.Join(dir, "db"), DBRetentionGB: 20}
	cfg.Logging = config.LoggingConfig{Level: "error", Format: "console"}
	cfg.Transcription.Backend = config.BackendLocal
	cfg.Transcription.FFmpegPath = "ffmpeg"
	cfg.Transcription.FFmpegSampleRate = 16000
	cfg.Transcription.FFmpegChannels = 1
	cfg.Transcription.FFmpegFormat = "s16le"
	cfg.Frequencies.ReconnectIntervalSecs = 3600
	cfg.Frequencies.Sources = []config.FrequencyConfig{
		{ID: "mix", Name: "Configured mix", Airport: "LFPG", FrequencyMHz: 125.825, URL: gone.URL + "/aero.mp3", Order: 2},
	}
	if o.goMP3 {
		reconnect := false
		cfg.Frequencies.Decoder = "go"
		cfg.Frequencies.Sources = append(cfg.Frequencies.Sources, config.FrequencyConfig{
			ID: "station", Name: "Station", FrequencyMHz: 124.35, URL: mp3Server(t) + "/aero-124350.mp3", Order: 3,
			FFmpegReconnect: &reconnect,
		})
	}
	if o.tone {
		reconnect := false
		cfg.Frequencies.Sources = append(cfg.Frequencies.Sources, config.FrequencyConfig{
			ID: "tone", Name: "Tone", FrequencyMHz: 118.7, URL: tone, Order: 1,
			FFmpegInputOptions: toneOptions, FFmpegReconnect: &reconnect,
		})
	}
	cfg.FlightPhases = config.FlightPhasesConfig{
		CruiseAltitudeFt: 18000, DepartureAltitudeFt: 1000, ApproachMaxAltitudeFt: 5000, AirportRangeNM: 5,
		FlyingMinTASKts: 50, FlyingMinAltFt: 300, HelicopterAltMultiplier: 2, HighSpeedThresholdKts: 200,
		HighAltitudeOverrideFt: 5000, ImpossibleAltDropThresholdFt: 10000,
		ImpossibleSpeedDropThresholdKts: 100, ImpossibleSpeedDropMinAltFt: 5000,
	}
	cfg.PostProcessing.MinDigits = 3

	// The day's database, where main.go puts it.
	if err := os.MkdirAll(cfg.Storage.SQLiteBasePath, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := sqlite.DailyPath(cfg.Storage.SQLiteBasePath, time.Now())
	store, err := sqlite.NewAircraftStorage(dbPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.GetDB()
	tx := sqlite.NewTranscriptionStorage(db, log)
	clr := sqlite.NewClearanceStorage(db, log)

	ws := websocket.NewServer(log)
	go ws.Run()

	client := adsb.NewClient(cfg.ADSB, stationLat, stationLon, 5*time.Second, log)
	adsbSvc := adsb.NewService(client, store, time.Hour, log, cfg.Station, cfg.ADSB, cfg.FlightPhases, ws)
	ws.SetMessageHandler(adsb.NewWebSocketHandler(adsbSvc, log))
	ctx, cancel := context.WithCancel(context.Background())
	if err := adsbSvc.Start(ctx); err != nil {
		cancel()
		t.Fatalf("starting the ADS-B service on the fake receiver: %v", err)
	}
	t.Cleanup(func() { adsbSvc.Stop(); cancel() })

	freqs := frequencies.NewService(cfg, log, ws, tx, clr, nil)
	var sidecar *transcription.Sidecar
	if o.sidecar != "" || o.idle {
		url := o.sidecar
		if url == "" {
			url = "http://127.0.0.1:1"
		}
		sidecar = transcription.NewSidecar(transcription.SidecarConfig{ServerURL: url}, log)
		if o.idle {
			sidecar.Idle()
			freqs.SetTranscriptionGate(sidecar.Ensure)
		}
	}
	if o.tone || o.goMP3 {
		if err := freqs.Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(freqs.Stop)

	// Accounts: one in the configuration, none in the file beside it.
	userFile, err := auth.LoadUserFile(cfg.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	authCfg := auth.Config{UserFile: userFile, Loopback: isLoopback(o.host), TrustedProxies: o.proxies,
		SessionTTL: time.Hour}
	if !o.noAccount {
		authCfg.Enabled = true
		authCfg.Users = []auth.User{{Name: testUser, PasswordHash: cheapHash(testPassword)}}
		if o.viewer {
			authCfg.Users = append(authCfg.Users, auth.User{Name: viewerUser, PasswordHash: cheapHash(testPassword), Role: auth.RoleViewer})
		}
	}
	svc, err := auth.NewService(authCfg)
	if err != nil {
		t.Fatal(err)
	}

	rt := config.NewRuntime(cfg, cfg.ConfigPath, log)

	h := &Handler{
		adsbService: adsbSvc, frequenciesService: freqs, config: cfg, logger: log.Named("api-handler"),
		wsServer: ws, transcriptionStorage: tx, clearanceStorage: clr,
		valueStorage: sqlite.NewPhraseologyStorage(db), auth: svc,
		startedAt: time.Now(), dbSampleAt: time.Now(),
	}
	h.AttachRuntime(rt, db, sidecar)
	r := &Router{handler: h, middleware: NewMiddleware(log), config: cfg, logger: log}
	routes := r.Routes()
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)

	return &stack{t: t, dir: dir, cfg: cfg, log: log, routes: routes, srv: srv, handler: h, runtime: rt,
		adsb: adsbSvc, freqs: freqs, ws: ws, aircraft: store, tx: tx, clr: clr,
		values: sqlite.NewPhraseologyStorage(db), tar1090: tar, gone: gone.URL}
}

// with is the same stack seen from a subtest: Fatal has to be called from
// the goroutine of the test that is running, and that is the subtest's.
func (s *stack) with(t *testing.T) *stack {
	c := *s
	c.t = t
	return &c
}

// do sends one request through the routes, with the session cookie if one is
// held, and with whatever else the caller sets on it.
func (s *stack) do(method, path, body string, set ...func(*http.Request)) *httptest.ResponseRecorder {
	s.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.cookie != nil {
		req.AddCookie(s.cookie)
	}
	for _, f := range set {
		f(req)
	}
	rec := httptest.NewRecorder()
	s.routes.ServeHTTP(rec, req)
	return rec
}

// get decodes a 200 answer into out, and fails on anything else.
func (s *stack) get(path string, out any) {
	s.t.Helper()
	rec := s.do("GET", path, "")
	if rec.Code != http.StatusOK {
		s.t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	if out != nil {
		if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
			s.t.Fatalf("GET %s: decoding: %v", path, err)
		}
	}
}

// signIn takes the test account's session.
func (s *stack) signIn() *http.Cookie {
	s.t.Helper()
	return s.signInAs(testUser)
}

// signInAs signs in as one of the stack's accounts, which share a password.
func (s *stack) signInAs(name string) *http.Cookie {
	s.t.Helper()
	rec := s.do("POST", "/api/v1/auth/login", `{"name":"`+name+`","password":"`+testPassword+`"}`)
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			s.cookie = c
			return c
		}
	}
	s.t.Fatalf("sign-in failed: %d %s", rec.Code, rec.Body.String())
	return nil
}

// wsURL is where /ws listens on the real port.
func (s *stack) wsURL() string {
	return "ws" + strings.TrimPrefix(s.srv.URL, "http") + "/api/v1/ws"
}

// A transmission as the transcription side stores it: first the text, then --
// when an aircraft was found for it -- the match with its evidence.
func (s *stack) transcribe(freq, content, callsign string, at time.Time) int64 {
	s.t.Helper()
	id, err := s.tx.StoreTranscription(&sqlite.TranscriptionRecord{
		FrequencyID: freq, CreatedAt: at, Content: content, IsComplete: true, Language: "en",
		ContentSecond: "deuxième lecture : " + content,
	})
	if err != nil {
		s.t.Fatal(err)
	}
	if callsign != "" {
		if err := s.tx.UpdateMatchedTranscription(id, content, "PILOT", callsign, "second", [][2]int{{0, 6}}, nil); err != nil {
			s.t.Fatal(err)
		}
	}
	return id
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
}
