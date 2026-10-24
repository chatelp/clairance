package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yegors/co-atc/internal/audioarchive"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/pkg/logger"
)

func runDatabaseRetentionCleanup(ctx context.Context, dbDir string, db *sqlite.DB, rt *config.Runtime, log *logger.Logger) {
	// Two cadences. Rotation is checked every minute so the switch lands close
	// to midnight; retention is an hourly sweep, which is as often as it can
	// possibly matter.
	rotateTicker := time.NewTicker(1 * time.Minute)
	defer rotateTicker.Stop()
	retentionTicker := time.NewTicker(1 * time.Hour)
	defer retentionTicker.Stop()

	sweep := func() {
		// Read on every pass, not captured once: a retention changed from the
		// settings panel takes effect on the next sweep rather than at the next
		// restart.
		capGB := rt.DBRetentionGB()
		if err := cleanupOldDailyDatabases(dbDir, db.Path(), capGB, log); err != nil {
			log.Warn("Periodic database retention cleanup failed",
				logger.Error(err),
				logger.String("path", dbDir),
				logger.Float64("retention_gb", capGB))
		}
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-rotateTicker.C:
			// The whole point of the loop. Before this, the file opened at
			// startup was written to for the life of the process and retention
			// skipped it as the active one, so a server left running simply grew
			// one database without limit.
			rotated, err := db.RotateIfNewDay(dbDir, time.Now())
			if err != nil {
				log.Error("Failed to rotate to today's database",
					logger.Error(err), logger.String("dir", dbDir))
				continue
			}
			if rotated {
				// Yesterday's file is closed now, so it is eligible for deletion
				// without waiting for the hourly sweep.
				sweep()
			}

		case <-retentionTicker.C:
			sweep()
		}
	}
}

// cleanupOldDailyDatabases keeps the daily databases within capGB together
// (28/09, D65). Newest first: the files that fit are kept, and from the first one
// that does not, it and every older file are deleted -- never a newer day for an
// older one. Today's file is never deleted, and counts. A file's -wal and -shm
// count with it and go with it.
func cleanupOldDailyDatabases(dbDir, activeDBPath string, capGB float64, log *logger.Logger) error {
	if capGB <= 0 {
		capGB = config.DefaultDBRetentionGB
	}
	capBytes := int64(capGB * (1 << 30))

	entries, err := os.ReadDir(dbDir)
	if err != nil {
		return fmt.Errorf("read db directory: %w", err)
	}

	type daily struct {
		path  string
		date  time.Time
		bytes int64
	}
	// The same file can be spelled two ways: relative and absolute, or through a
	// symbolic link (/var is one on macOS, to /private/var). Compared by name
	// only, today's file passed for an old one there, and went when over the
	// cap. It is now recognised by name or by identity.
	absActiveDBPath, _ := filepath.Abs(activeDBPath)
	activeInfo, _ := os.Stat(activeDBPath)
	isActive := func(path string) bool {
		if abs, _ := filepath.Abs(path); abs == absActiveDBPath {
			return true
		}
		if activeInfo == nil {
			return false
		}
		info, err := os.Stat(path)
		return err == nil && os.SameFile(info, activeInfo)
	}
	var files []daily
	var total int64
	for _, entry := range entries {
		fileName := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(fileName, "co-atc-") || !strings.HasSuffix(fileName, ".db") {
			continue
		}
		fileDate, parseErr := time.Parse("2006-01-02", strings.TrimSuffix(strings.TrimPrefix(fileName, "co-atc-"), ".db"))
		if parseErr != nil {
			continue
		}
		path := filepath.Join(dbDir, fileName)
		var size int64
		for _, p := range []string{path, path + "-wal", path + "-shm"} {
			if info, err := os.Stat(p); err == nil {
				size += info.Size()
			}
		}
		if isActive(path) {
			total += size // kept whatever its size
			continue
		}
		files = append(files, daily{path, fileDate, size})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].date.After(files[j].date) })

	deleted, over := 0, false
	var freed int64
	for _, f := range files {
		if !over && total+f.bytes <= capBytes {
			total += f.bytes
			continue
		}
		over = true
		for _, p := range []string{f.path, f.path + "-wal", f.path + "-shm"} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove old db '%s': %w", p, err)
			}
		}
		deleted++
		freed += f.bytes
		log.Info("Deleted old database file",
			logger.String("path", f.path),
			logger.String("file_date", f.date.Format("2006-01-02")),
			logger.Int64("bytes", f.bytes))
	}

	if deleted > 0 {
		log.Info("Database retention cleanup complete",
			logger.Int("deleted_files", deleted),
			logger.Int64("freed_bytes", freed),
			logger.Int64("kept_bytes", total),
			logger.Float64("retention_gb", capGB))
	}
	return nil
}

// runFilePurge keeps the two kinds of files that grow every day outside the
// databases within what the configuration allows: the sidecar's archive of
// clips (transcription.audio_archive_max_gb) and the raw transcription logs
// (transcription.raw_log_max_days). Neither had a limit before 13/10: on a
// station archiving two frequencies the archive takes 1 to 2 GB a day. Swept at
// start, then every hour like the databases; 0 keeps everything.
func runFilePurge(ctx context.Context, cfg *config.Config, log *logger.Logger) {
	archive := audioarchive.DirOf(cfg.Transcription.Local.AudioArchiveDir, cfg.Transcription.Local.Command)
	rawDir := ""
	if cfg.Transcription.LogDir != "" {
		rawDir = filepath.Join(cfg.Transcription.LogDir, "raw")
	}
	sweep := func() {
		now := time.Now()
		if archive != "" && cfg.Transcription.AudioArchiveMaxGB > 0 {
			if err := purgeAudioArchive(archive, int64(cfg.Transcription.AudioArchiveMaxGB*(1<<30)), now, log); err != nil {
				log.Warn("Audio archive purge failed", logger.Error(err), logger.String("path", archive))
			}
		}
		if rawDir != "" && cfg.Transcription.RawLogMaxDays > 0 {
			if err := purgeRawLogs(rawDir, cfg.Transcription.RawLogMaxDays, now, log); err != nil {
				log.Warn("Raw transcription log purge failed", logger.Error(err), logger.String("path", rawDir))
			}
		}
	}

	sweep()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

// purgeAudioArchive keeps the archive within capBytes, deleting the oldest clips
// first. It touches only what the sidecar writes there (sidecar/whisper_server.py,
// keep_audio): root/YYYYMMDD/<frequency>/*.wav. Everything in the archive counts
// towards the cap; only those clips are deleted, today's never, and a frequency
// folder left empty goes with its last clip. The day's manifest stays: it is a
// few kilobytes, it carries the texts as first read, and a line whose clip is
// gone is a 404 for the page, as for a transmission never archived. An archive
// that is not there -- an external disk unplugged -- is not an error.
func purgeAudioArchive(root string, capBytes int64, now time.Time, log *logger.Logger) error {
	days, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read audio archive: %w", err)
	}
	today := now.Local().Format("20060102")

	type clip struct {
		day, dir, name string
		bytes          int64
	}
	var clips []clip
	var total int64
	for _, d := range days {
		day := d.Name()
		if !d.IsDir() {
			total += sizeOf(filepath.Join(root, day))
			continue
		}
		_, notADay := time.Parse("20060102", day)
		err := filepath.WalkDir(filepath.Join(root, day), func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			info, err := e.Info()
			if err != nil {
				return nil
			}
			total += info.Size()
			rel, _ := filepath.Rel(filepath.Join(root, day), path)
			// Exactly <frequency>/<clip>.wav, a regular file, in a past day.
			if notADay == nil && day < today && e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".wav") &&
				strings.Count(filepath.ToSlash(rel), "/") == 1 {
				clips = append(clips, clip{day: day, dir: filepath.Dir(path), name: e.Name(), bytes: info.Size()})
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("read audio archive day %s: %w", day, err)
		}
	}
	if total <= capBytes {
		audioarchive.RecordSize(root, total, now)
		return nil
	}

	// Oldest first: by day, then by name, which the sidecar starts with the
	// time of the transmission (t_YYYYMMDD_HHMMSS_mmm.wav), across frequencies.
	sort.Slice(clips, func(i, j int) bool {
		if clips[i].day != clips[j].day {
			return clips[i].day < clips[j].day
		}
		if clips[i].name != clips[j].name {
			return clips[i].name < clips[j].name
		}
		return clips[i].dir < clips[j].dir
	})
	type dayCount struct {
		clips int
		bytes int64
	}
	byDay := map[string]*dayCount{}
	var order []string
	dirs := map[string]bool{}
	deleted, freed := 0, int64(0)
	for _, c := range clips {
		if total <= capBytes {
			break
		}
		if err := os.Remove(filepath.Join(c.dir, c.name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove archived clip: %w", err)
		}
		total -= c.bytes
		deleted++
		freed += c.bytes
		dirs[c.dir] = true
		if byDay[c.day] == nil {
			byDay[c.day] = &dayCount{}
			order = append(order, c.day)
		}
		byDay[c.day].clips++
		byDay[c.day].bytes += c.bytes
	}
	for dir := range dirs {
		os.Remove(dir) // only if empty: a folder with clips left stays
	}
	for _, day := range order {
		log.Info("Deleted archived audio clips",
			logger.String("day", day),
			logger.Int("clips", byDay[day].clips),
			logger.Int64("bytes", byDay[day].bytes))
	}
	if deleted > 0 {
		log.Info("Audio archive purge complete",
			logger.String("path", root),
			logger.Int("deleted_clips", deleted),
			logger.Int64("freed_bytes", freed),
			logger.Int64("kept_bytes", total),
			logger.Int64("cap_bytes", capBytes))
	}
	if total > capBytes {
		// Today's clips, and whatever else is in the archive, are over the cap
		// on their own: said, and kept.
		log.Warn("Audio archive still over its cap with nothing older than today left to delete",
			logger.String("path", root),
			logger.Int64("kept_bytes", total),
			logger.Int64("cap_bytes", capBytes))
	}
	audioarchive.RecordSize(root, total, now)
	return nil
}

// purgeRawLogs deletes the raw transcription logs older than maxDays, by the date
// in their name (YYYY-MM-DD_<frequency>.log, internal/transcription/file_logger.go),
// not by their modification time, which every line appended moves. Today's and
// any later date are never deleted; a file named otherwise is not touched.
func purgeRawLogs(dir string, maxDays int, now time.Time, log *logger.Logger) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read raw log directory: %w", err)
	}
	y, m, d := now.Local().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	oldest := today.AddDate(0, 0, -maxDays) // a file of this date is kept

	deleted, freed := 0, int64(0)
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".log") || len(name) < len("2006-01-02_") || name[10] != '_' {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", name[:10], time.Local)
		if err != nil || !date.Before(oldest) || !date.Before(today) {
			continue
		}
		path := filepath.Join(dir, name)
		size := sizeOf(path)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove raw log '%s': %w", path, err)
		}
		deleted++
		freed += size
		log.Info("Deleted raw transcription log",
			logger.String("path", path),
			logger.String("file_date", name[:10]),
			logger.Int64("bytes", size))
	}
	if deleted > 0 {
		log.Info("Raw transcription log purge complete",
			logger.Int("deleted_files", deleted),
			logger.Int64("freed_bytes", freed),
			logger.Int("max_days", maxDays))
	}
	return nil
}

func sizeOf(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}
