package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/yegors/co-atc/internal/aip"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/startcheck"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/transcription/phraseology"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// The build's version and commit, set when building:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse --short HEAD)" -o bin/co-atc ./cmd/server
//
// A plain go build leaves "dev", and the commit Go itself records in the
// binary (vcsCommit). The settings panel, the sign-in page and the launcher's
// health show them.
var (
	version = "dev"
	commit  = "unknown"
)

// vcsCommit is the commit a plain go build records in the binary, short, with
// "-dirty" when the tree had changes; "" when there is none (a build outside
// the repository, or with -buildvcs=false). On 26/10 the launcher wrote the
// repository's commit for a binary ten days older: only the binary can say
// what it was built from.
func vcsCommit(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if rev != "" && dirty {
		rev += "-dirty"
	}
	return rev
}

// main brings the server up in a fixed order, each step in startup.go and
// serve.go, waits for a signal and takes it down. The order is the one the
// comments below and in those steps date and justify; moving a step changes
// the server's behaviour.
func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file (optional - will search in configs/ and root directory)")
	addUser := flag.String("add-user", "", "Print a configuration block for a new account, reading the password without echoing it")
	flag.Parse()

	if commit == "unknown" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if c := vcsCommit(info); c != "" {
				commit = c
			}
		}
	}

	if *addUser != "" {
		if err := printNewUser(*addUser); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg, resolvedConfigPath := loadConfig(*configPath)

	// Create logger
	log := newLogger(cfg)
	defer log.Sync()

	log.Info("Starting Clairance server",
		logger.String("version", version),
		logger.String("commit", commit),
		logger.String("config_path", resolvedConfigPath),
	)
	// What the loader had to say about the file: a key written twice, a key
	// nothing reads any more. One line each; never a reason not to start.
	for _, w := range cfg.Warnings {
		log.Warn(w, logger.String("config_path", resolvedConfigPath))
	}

	// Arm the shutdown signals before anything is started, not after.
	//
	// They used to be armed once every service was up, which left a measured
	// 1.7 s window -- from spawning the STT sidecar to the end of startup --
	// where a signal took its default action and killed the server outright:
	// no deferred cleanup, and the sidecar orphaned. Arming first also lets a
	// long startup be interrupted: signalCtx is what the sidecar waits on, so
	// Ctrl-C during its health probe stops the wait and takes the child with it.
	//
	// SIGHUP is included because closing the terminal window sends it.
	signalCtx, stopSignals := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stopSignals()

	// Create ADS-B components
	adsbClient := buildADSBClient(cfg, log)

	// Values that can be changed while the server runs, from the settings
	// panel: read before the startup check, which judges the databases' disk
	// against the retention in force, and before the first retention pass.
	runtimeSettings := config.NewRuntime(cfg, *configPath, log)

	// What the server depends on, named before it relies on any of it: a
	// sidecar that cannot start stops the server just below, and its cause is
	// then the line above. Nothing missing stops the server here.
	check := startcheck.Run(startcheck.Inputs{Config: cfg, DBRetentionGB: runtimeSettings.DBRetentionGB()})
	check.Log(log)

	sttSidecar := startSidecar(signalCtx, cfg, log)
	defer sttSidecar.Stop()

	// Create SQLite storage
	store := openStorage(cfg, runtimeSettings, log)
	defer store.aircraft.Close()

	// Create WebSocket server
	wsServer := websocket.NewServer(log)

	// Start WebSocket server
	go wsServer.Run()

	adsbService := buildADSBService(cfg, adsbClient, store, wsServer, log)
	// The runways in use are learnt over an hour: kept across a restart.
	adsbService.KeepRunwaysIn(filepath.Join(cfg.Storage.SQLiteBasePath, "runways-in-use.json"))

	refService, refAirport, effectiveAirport := loadReference(cfg, runtimeSettings, adsbService, log)

	// Create and set WebSocket message handler for ADSB
	wsHandler := adsb.NewWebSocketHandler(adsbService, log)
	wsServer.SetMessageHandler(wsHandler)

	// Start ADS-B service
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runDatabaseRetentionCleanup(ctx, store.dir, store.aircraft.GetDB(), runtimeSettings, log)
	go runFilePurge(ctx, cfg, log)

	if err := adsbService.Start(ctx); err != nil {
		log.Error("Failed to start ADS-B service", logger.Error(err))
		os.Exit(1)
	}

	weatherService := startWeather(cfg, effectiveAirport, refAirport, runtimeSettings, log)

	frequenciesService := startFrequencies(ctx, cfg, adsbService, refService, runtimeSettings, wsServer, store, sttSidecar, log)

	router := buildRouter(cfg, adsbService, frequenciesService, weatherService, refService, wsServer, store, runtimeSettings, sttSidecar, log)
	router.Handler().SetMissing(check.Missing)

	// The list of official names around the station (internal/aip, Q55), made
	// from the settings page, kept beside the databases with the pages it was
	// made from.
	aipMaker := &aip.Maker{OutPath: filepath.Join(store.dir, "aip-names.json"), CacheDir: filepath.Join(store.dir, "aip-cache")}
	// The procedures heard are named against the list's procedure points
	// (Q55, usage 3), at the sound likeness the panel asks for.
	placeNamer := startPlaceNamer(cfg, aipMaker.OutPath, runtimeSettings, log)
	router.Handler().AttachAIP(aipMaker, func(n *aip.Names) {
		if placeNamer != nil {
			placeNamer.Set(procedurePoints(n))
		}
	})
	router.Handler().KeepAIPNamesUpToDate(ctx)

	servers := serve(cfg, router, log)

	// Wait for one of the signals armed at startup. A signal that arrived while
	// the services were still coming up is already recorded here.
	<-signalCtx.Done()

	shutdown(log, weatherService, frequenciesService, sttSidecar, adsbService, cancel, servers)
}

// startPlaceNamer builds the namer of the procedures heard, from the list
// made earlier when there is one.
func startPlaceNamer(cfg *config.Config, listPath string, rt *config.Runtime, log *logger.Logger) *phraseology.PlaceNamer {
	m, err := phraseology.NewMatcher(cfg.Reference.AirlinesDATPath)
	if err != nil {
		log.Warn("Procedures heard will not be named: the airlines file could not be read", logger.Error(err))
		return nil
	}
	n := phraseology.NewPlaceNamer(m)
	if list, err := aip.Load(listPath); err == nil {
		n.Set(procedurePoints(list))
		log.Info("Procedures heard are named", logger.Int("procedure_points", n.Count()), logger.String("airac", list.AIRAC))
	}
	transcription.SetPlaces(n, rt.PlaceNaming)
	return n
}

func procedurePoints(n *aip.Names) []phraseology.PlacePoint {
	out := make([]phraseology.PlacePoint, 0, len(n.ProcedurePoints))
	for _, p := range n.ProcedurePoints {
		out = append(out, phraseology.PlacePoint{Name: p.Name, Lat: p.Lat, Lon: p.Lon, Kinds: p.Kinds})
	}
	return out
}
