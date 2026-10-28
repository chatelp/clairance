package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A size of zero or less, should one reach the sweep, means the default
// (config.DefaultDBRetentionGB, 20 GB) -- not "keep nothing".
func TestAZeroSizeMeansTheDefault(t *testing.T) {
	dir := t.TempDir()
	active := daily(t, dir, "co-atc-2026-09-28.db", gib)
	daily(t, dir, "co-atc-2026-09-27.db", 6*gib)
	daily(t, dir, "co-atc-2026-09-26.db", 6*gib)
	daily(t, dir, "co-atc-2026-09-25.db", 6*gib)
	daily(t, dir, "co-atc-2026-09-24.db", 6*gib)

	if err := cleanupOldDailyDatabases(dir, active, 0, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	// 1 + 6 + 6 + 6 = 19 GB fit in 20; the fourth older day does not.
	want := []string{"co-atc-2026-09-25.db", "co-atc-2026-09-26.db", "co-atc-2026-09-27.db", "co-atc-2026-09-28.db"}
	if got := left(t, dir); !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}

// The configuration may give a relative directory, and main joins it with the
// day's name: the active file is recognised by where it is, not by how its
// path is spelled.
func TestTheActiveFileIsRecognisedHoweverItsPathIsSpelled(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "data")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	daily(t, dir, "co-atc-2026-09-28.db", 2*gib)
	daily(t, dir, "co-atc-2026-09-27.db", gib/100)
	// os.Chdir rather than t.Chdir: the module still declares Go 1.23.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	for _, active := range []string{
		"data/co-atc-2026-09-28.db",
		"./data/../data/co-atc-2026-09-28.db",
		filepath.Join(dir, "co-atc-2026-09-28.db"),
	} {
		if err := cleanupOldDailyDatabases("data", active, 1, "", quietLogger(t)); err != nil {
			t.Fatal(err)
		}
		if got := left(t, dir); len(got) != 1 || got[0] != "co-atc-2026-09-28.db" {
			t.Fatalf("active given as %q: left %v, want only today's file", active, got)
		}
	}
}

// Today's file is kept even when a later-dated file exists -- a clock set back,
// or a file copied in -- and still counts first.
func TestTodayIsKeptWhenALaterDayExists(t *testing.T) {
	dir := t.TempDir()
	active := daily(t, dir, "co-atc-2026-09-28.db", gib)
	daily(t, dir, "co-atc-2026-09-30.db", gib/2)
	daily(t, dir, "co-atc-2026-09-29.db", gib)

	if err := cleanupOldDailyDatabases(dir, active, 2, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{"co-atc-2026-09-28.db", "co-atc-2026-09-30.db"}
	if got := left(t, dir); !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}

// The first pass runs before the day's file is opened: with no active file on
// disk, the other days are judged on their own.
func TestRetentionBeforeTodaysFileExists(t *testing.T) {
	dir := t.TempDir()
	daily(t, dir, "co-atc-2026-09-27.db", gib/2)
	daily(t, dir, "co-atc-2026-09-26.db", gib)

	if err := cleanupOldDailyDatabases(dir, filepath.Join(dir, "co-atc-2026-09-28.db"), 1, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{"co-atc-2026-09-27.db"}
	if got := left(t, dir); !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}

// A directory that cannot be read is reported, not taken for an empty one.
func TestAMissingDirectoryIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	if err := cleanupOldDailyDatabases(missing, filepath.Join(missing, "co-atc-2026-09-28.db"), 1, "", quietLogger(t)); err == nil {
		t.Error("reading a missing directory should fail")
	}
}
