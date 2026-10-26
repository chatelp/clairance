package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/yegors/co-atc/internal/companion"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/lifecycle"
	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/pkg/tool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The companions co-atc runs beside itself, which the production script
// started until 26/10 (docs-fr/36-contrat-lanceur.md, plan 35 B3): the link
// with a radio-ctl station, and the keep-awake assertion on macOS. The
// sidecar is started by startSidecar, as before.

// stationLinkProgram finds radio-ctl-sync when [station_link] is on, before
// anything is started: a link asked for and missing is a refusal to start,
// said at once rather than after the sidecar's minutes. "" when it is off.
func stationLinkProgram(cfg *config.Config, log *logger.Logger) string {
	if !cfg.StationLink.Enabled {
		return ""
	}
	name := cfg.StationLink.Program
	if name == "" {
		// bin/ holds both programs.
		if exe, err := os.Executable(); err == nil {
			p := filepath.Join(filepath.Dir(exe), "radio-ctl-sync"+exeSuffix())
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
		name = "radio-ctl-sync" + exeSuffix()
	}
	p, ok := tool.Find(name)
	if !ok {
		lifecycle.Failed("station_link", fmt.Errorf("station link program not found: %s", name))
		fatalLog := log.WithOptions(zap.AddStacktrace(zapcore.PanicLevel))
		fatalLog.Fatal("Station link program not found",
			logger.String("program", name),
			logger.String("fix", "go build -o bin/radio-ctl-sync ./cmd/radio-ctl-sync, set station_link.program, or set station_link.enabled = false"))
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// newLinkToken is the key the station link is started with: 256 random bits,
// made at each start, kept in memory and in the link's environment only.
func newLinkToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("no randomness for the station link's key: %v", err))
	}
	return hex.EncodeToString(b)
}

// listenerURL is where this machine reaches the server on port: the loopback
// for a server bound to it or to every address, else the address it is bound
// to. For the station link the configuration has checked it is the loopback,
// in plain HTTP.
func listenerURL(cfg *config.Config, port int) string {
	host := cfg.Server.Host
	switch host {
	case "", "0.0.0.0", "127.0.0.1", "localhost":
		host = "127.0.0.1"
	case "::", "::1":
		host = "[::1]"
	default:
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
	}
	scheme := "http"
	if cfg.Server.TLSCert != "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

// startStationLink starts radio-ctl-sync on this server, with the key that
// opens its routes, and keeps it running. Started once the server listens:
// before, its first reading would find nobody to tell. nil when it is off.
func startStationLink(ctx context.Context, cfg *config.Config, program, token string, log *logger.Logger) *companion.Process {
	if program == "" {
		return nil
	}
	l := cfg.StationLink
	args := []string{"-station", l.StationURL, "-streams", l.StreamsURL, "-coatc", listenerURL(cfg, cfg.Server.Port),
		"-interval", strconv.Itoa(l.IntervalSeconds) + "s"}
	if l.StationHost != "" {
		args = append(args, "-station-host", l.StationHost)
	}
	if l.MixID != "" {
		args = append(args, "-mix-id", l.MixID)
	}
	// Its own lines go to this log, so that what it did is read in one place.
	said := log.Named("station_link")
	p, err := companion.Start(ctx, companion.Config{
		Role: "station_link", Path: program, Args: args,
		Env:    []string{"COATC_SPAWNED=1", "COATC_LINK_TOKEN=" + token},
		Output: func(line string) { said.Info(line) },
	}, log)
	if err != nil {
		lifecycle.Failed("station_link", err)
		fatalLog := log.WithOptions(zap.AddStacktrace(zapcore.PanicLevel))
		fatalLog.Fatal("Station link did not start", logger.Error(err))
	}
	return p
}

// keepAwakeOS and caffeinate are variables for the test, which runs on any
// system.
var (
	keepAwakeOS = runtime.GOOS
	caffeinate  = "/usr/bin/caffeinate"
)

// startKeepAwake keeps the Mac from sleeping while the server runs, as the
// production script's caffeinate did: -i holds off idle sleep, -w lets it go
// with this process, even one killed outright. nil when it is not asked for,
// not on macOS, or when caffeinate does not start -- a server that may sleep is
// still a server, and it is said.
func startKeepAwake(ctx context.Context, cfg *config.Config, log *logger.Logger) *companion.Process {
	if !cfg.Server.KeepAwake {
		return nil
	}
	if keepAwakeOS != "darwin" {
		log.Info("server.keep_awake is for macOS: nothing to do here", logger.String("os", keepAwakeOS))
		return nil
	}
	p, err := companion.Start(ctx, companion.Config{
		Role: "keep_awake", Path: caffeinate, Args: []string{"-i", "-w", strconv.Itoa(os.Getpid())},
	}, log)
	if err != nil {
		log.Warn("The Mac may sleep: caffeinate did not start", logger.Error(err))
		return nil
	}
	return p
}
