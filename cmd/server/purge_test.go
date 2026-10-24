package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/audioarchive"
	"github.com/yegors/co-atc/pkg/logger"
)

// The moment the sweeps run in these tests: noon, so that the day before and
// the day after are unambiguous in any time zone the test runs in.
var purgeNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)

func dayName(offset int) string { return purgeNow.AddDate(0, 0, offset).Format("20060102") }

// archiveFile writes a file of the given size under the archive root.
func archiveFile(t *testing.T, root, rel string, size int64) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// files lists every file under root, relative, sorted.
func files(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// fileLogger logs at info to a file as well, and returns what it wrote so far.
func fileLogger(t *testing.T) (*logger.Logger, func() string) {
	t.Helper()
	dir := t.TempDir()
	log, err := logger.New(logger.Config{Level: "info", Format: "json", File: filepath.Join(dir, "purge.log")})
	if err != nil {
		t.Fatal(err)
	}
	return log, func() string {
		_ = log.Sync()
		var all string
		for _, f := range files(t, dir) { // the logger names its file by the day
			b, _ := os.ReadFile(filepath.Join(dir, f))
			all += string(b)
		}
		return all
	}
}

// An archive laid out as the sidecar writes it: three past days and today,
// two frequencies, two clips of 1000 bytes each per frequency and day, and a
// manifest of 100 bytes per day. 16 clips and 400 bytes of manifests: 16 400.
func sidecarArchive(t *testing.T) string {
	root := t.TempDir()
	for _, off := range []int{-3, -2, -1, 0} {
		day := dayName(off)
		for _, freq := range []string{"lfpg_twr", "lfpo_app"} {
			for _, hms := range []string{"080000_000", "093000_500"} {
				archiveFile(t, root, day+"/"+freq+"/t_"+day+"_"+hms+".wav", 1000)
			}
		}
		archiveFile(t, root, day+"/manifeste.jsonl", 100)
	}
	return root
}

// Over the cap, the oldest clips go first, across frequencies, until the
// archive fits; the manifests stay, and so does every clip of today.
func TestTheArchiveLosesItsOldestClipsFirst(t *testing.T) {
	root := sidecarArchive(t)
	// 16 400 bytes; a cap of 11 000 takes six clips: the four of three days
	// ago, then the two at 08:00 of two days ago.
	log, written := fileLogger(t)
	if err := purgeAudioArchive(root, 11000, purgeNow, log); err != nil {
		t.Fatal(err)
	}
	// What is left is the size the health route reports (docs-fr/34).
	if bytes, at, ok := audioarchive.New(root).Size(); !ok || bytes != 10400 || !at.Equal(purgeNow) {
		t.Errorf("the size kept after the purge: %d at %v (%v), want 10400", bytes, at, ok)
	}
	got := files(t, root)
	d3, d2 := dayName(-3), dayName(-2)
	// Said, per day and in all.
	out := written()
	for _, want := range []string{`"day":"` + d3 + `","clips":4,"bytes":4000`, `"day":"` + d2 + `","clips":2,"bytes":2000`,
		`"deleted_clips":6,"freed_bytes":6000,"kept_bytes":10400`} {
		if !strings.Contains(out, want) {
			t.Errorf("the log does not say %s:\n%s", want, out)
		}
	}
	for _, gone := range []string{
		d3 + "/lfpg_twr/t_" + d3 + "_080000_000.wav", d3 + "/lfpo_app/t_" + d3 + "_093000_500.wav",
		d2 + "/lfpg_twr/t_" + d2 + "_080000_000.wav", d2 + "/lfpo_app/t_" + d2 + "_080000_000.wav",
	} {
		for _, f := range got {
			if f == gone {
				t.Errorf("%s was kept", gone)
			}
		}
	}
	want := 16 - 6 + 4 // clips left, and four manifests
	if len(got) != want {
		t.Errorf("%d files left, want %d:\n  %s", len(got), want, strings.Join(got, "\n  "))
	}
	for _, f := range []string{d2 + "/lfpg_twr/t_" + d2 + "_093000_500.wav", d3 + "/manifeste.jsonl"} {
		if !contains(got, f) {
			t.Errorf("%s was deleted", f)
		}
	}
	// The frequency folders of three days ago are empty and gone; the day's
	// folder stays with its manifest.
	if _, err := os.Stat(filepath.Join(root, d3, "lfpg_twr")); !os.IsNotExist(err) {
		t.Errorf("an emptied frequency folder is left: %v", err)
	}
}

// Today's clips are never deleted, even when they alone are over the cap;
// what is not the sidecar's is never deleted either.
func TestTodayAndWhatIsNotAClipAreKept(t *testing.T) {
	root := t.TempDir()
	today, past := dayName(0), dayName(-1)
	archiveFile(t, root, today+"/lfpg_twr/t_"+today+"_080000_000.wav", 5000)
	archiveFile(t, root, dayName(1)+"/lfpg_twr/t_tomorrow.wav", 1000) // a clock ahead: not past
	archiveFile(t, root, past+"/lfpg_twr/notes.txt", 1000)            // not a clip
	archiveFile(t, root, past+"/stray.wav", 1000)                     // not in a frequency folder
	archiveFile(t, root, past+"/lfpg_twr/deeper/t_x.wav", 1000)       // not where the sidecar writes
	archiveFile(t, root, "lab/t_20200101_000000_000.wav", 1000)       // not a day
	archiveFile(t, root, past+"/lfpo_app/t_"+past+"_080000_000.wav", 1000)
	before := files(t, root)

	if err := purgeAudioArchive(root, 1, purgeNow, quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	got := files(t, root)
	if len(got) != len(before)-1 || contains(got, past+"/lfpo_app/t_"+past+"_080000_000.wav") {
		t.Errorf("only the past day's clip should go:\nbefore %v\nafter  %v", before, got)
	}
}

// Under the cap, an empty cap's caller, an archive that is not there: nothing
// happens, and nothing is an error.
func TestAnArchiveUnderItsCapOrAbsentIsLeftAlone(t *testing.T) {
	root := sidecarArchive(t)
	before := files(t, root)
	if err := purgeAudioArchive(root, 16400, purgeNow, quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	if got := files(t, root); len(got) != len(before) {
		t.Errorf("an archive at its cap lost files: %d -> %d", len(before), len(got))
	}
	if err := purgeAudioArchive(filepath.Join(root, "unplugged"), 1, purgeNow, quietLogger(t)); err != nil {
		t.Errorf("an archive that is not there: %v", err)
	}
}

// Raw logs go by the date in their name, past the age: a file of exactly the
// age is kept, today's and later ones always, and a file named otherwise is
// not touched.
func TestRawLogsOlderThanTheAgeGo(t *testing.T) {
	dir := t.TempDir()
	name := func(offset int, freq string) string {
		return purgeNow.AddDate(0, 0, offset).Format("2006-01-02") + "_" + freq + ".log"
	}
	keep := []string{name(0, "lfpg_twr"), name(-1, "lfpg_twr"), name(-30, "lfpo_app"), name(2, "lfpg_twr"),
		"notes.log", "2026-13-45_bad.log", name(-90, "x") + ".bak"}
	gone := []string{name(-31, "lfpg_twr"), name(-31, "lfpo_app"), name(-400, "lfpg_twr")}
	for _, n := range append(append([]string{}, keep...), gone...) {
		daily(t, dir, n, 10)
	}
	// Old by its modification time, today's by its name: the name decides.
	old := purgeNow.AddDate(0, 0, -100)
	if err := os.Chtimes(filepath.Join(dir, name(0, "lfpg_twr")), old, old); err != nil {
		t.Fatal(err)
	}

	log, written := fileLogger(t)
	if err := purgeRawLogs(dir, 30, purgeNow, log); err != nil {
		t.Fatal(err)
	}
	got := left(t, dir)
	out := written()
	for _, g := range gone {
		if !strings.Contains(out, g) {
			t.Errorf("the log does not name %s", g)
		}
	}
	if !strings.Contains(out, `"deleted_files":3`) {
		t.Errorf("the log does not give the count:\n%s", out)
	}
	sort.Strings(keep)
	if strings.Join(got, ",") != strings.Join(keep, ",") {
		t.Errorf("left:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(keep, "\n  "))
	}
	if err := purgeRawLogs(filepath.Join(dir, "absent"), 30, purgeNow, quietLogger(t)); err != nil {
		t.Errorf("a log directory that is not there: %v", err)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
