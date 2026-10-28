package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/yegors/co-atc/pkg/logger"
)

const gib = int64(1 << 30)

// daily writes a file of the given size without writing the bytes: a sparse
// file reports its full size, which is all retention looks at.
func daily(t *testing.T, dir, name string, size int64) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := os.Truncate(p, size); err != nil {
		t.Fatal(err)
	}
	return p
}

func left(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func quietLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// Newest first, the days that fit are kept; from the first that does not, it and
// every older day go -- even an older day small enough to fit, since keeping it
// would mean keeping an older day over a newer one.
func TestRetentionKeepsTheNewestDaysWithinTheSize(t *testing.T) {
	dir := t.TempDir()
	active := daily(t, dir, "co-atc-2026-09-28.db", gib)
	daily(t, dir, "co-atc-2026-09-27.db", gib)
	daily(t, dir, "co-atc-2026-09-26.db", gib/2)
	daily(t, dir, "co-atc-2026-09-25.db", gib)
	daily(t, dir, "co-atc-2026-09-24.db", gib/10)

	if err := cleanupOldDailyDatabases(dir, active, 3, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{"co-atc-2026-09-26.db", "co-atc-2026-09-27.db", "co-atc-2026-09-28.db"}
	if got := left(t, dir); !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}

// A pause in running changes nothing: the dates are not what decides. Ten days
// apart and well under the size, both days stay -- the case that deleted
// everything under the rule of seven days.
func TestAPauseDeletesNothingUnderTheSize(t *testing.T) {
	dir := t.TempDir()
	active := daily(t, dir, "co-atc-2026-10-08.db", gib/100)
	daily(t, dir, "co-atc-2026-09-28.db", gib/5)

	if err := cleanupOldDailyDatabases(dir, active, 20, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	if got := left(t, dir); len(got) != 2 {
		t.Errorf("left %v, want both days", got)
	}
}

// Today's file is kept whatever its size, and counts: past the size on its own,
// it leaves no room for any other day.
func TestTodayIsKeptAndCounts(t *testing.T) {
	dir := t.TempDir()
	active := daily(t, dir, "co-atc-2026-09-28.db", 2*gib)
	daily(t, dir, "co-atc-2026-09-27.db", gib/100)

	if err := cleanupOldDailyDatabases(dir, active, 1, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{"co-atc-2026-09-28.db"}
	if got := left(t, dir); !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}

// A database's -wal and -shm are part of it: they count toward the size, and go
// with it.
func TestTheWriteAheadLogCountsAndGoes(t *testing.T) {
	dir := t.TempDir()
	active := daily(t, dir, "co-atc-2026-09-28.db", gib/10)
	daily(t, dir, "co-atc-2026-09-27.db", gib/2)
	daily(t, dir, "co-atc-2026-09-27.db-wal", gib)
	daily(t, dir, "co-atc-2026-09-27.db-shm", gib/1000)

	if err := cleanupOldDailyDatabases(dir, active, 1, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{"co-atc-2026-09-28.db"}
	if got := left(t, dir); !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}

// Only co-atc's daily databases are candidates.
func TestOtherFilesAreNeverTouched(t *testing.T) {
	dir := t.TempDir()
	active := daily(t, dir, "co-atc-2026-09-28.db", gib/10)
	daily(t, dir, "co-atc-2026-09-01.db", 2*gib)
	daily(t, dir, "co-atc-notes.db", 2*gib)
	daily(t, dir, "other-2026-09-01.db", 2*gib)
	daily(t, dir, "users.json", 1)

	if err := cleanupOldDailyDatabases(dir, active, 1, "", quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{"co-atc-2026-09-28.db", "co-atc-notes.db", "other-2026-09-01.db", "users.json"}
	if got := left(t, dir); !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
