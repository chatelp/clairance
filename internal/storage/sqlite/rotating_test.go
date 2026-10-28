package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

func testLog(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

func countAircraft(t *testing.T, path string) int {
	t.Helper()
	return countRows(t, path, "aircraft")
}

func countRows(t *testing.T, path, table string) int {
	t.Helper()
	// The previous connection is closed on its own goroutine, so a reader that
	// arrives immediately after a rotation can still meet its WAL lock. Waiting
	// is the reader's job, not a reason to make rotation synchronous.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s in %s: %v", table, path, err)
	}
	return n
}

func insert(t *testing.T, q interface {
	Exec(string, ...any) (sql.Result, error)
}, hex string) {
	t.Helper()
	if _, err := q.Exec(`INSERT INTO aircraft (hex, last_seen) VALUES (?, ?)`,
		hex, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert %s: %v", hex, err)
	}
}

func TestRotateIfNewDayIsAnoopOnTheSameDay(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)

	db, err := Open(DailyPath(dir, now), testLog(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	rotated, err := db.RotateIfNewDay(dir, now.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("RotateIfNewDay: %v", err)
	}
	if rotated {
		t.Error("should not rotate within the same day")
	}
	if got, want := db.Path(), DailyPath(dir, now); got != want {
		t.Errorf("path moved: got %s, want %s", got, want)
	}
}

// The defect this whole type exists for: a process running past midnight kept
// writing into the file named after the day it started, and retention -- which
// skips the active file -- never reclaimed it.
func TestWritesLandInTheNewFileAfterMidnight(t *testing.T) {
	dir := t.TempDir()
	day1 := time.Date(2026, 9, 20, 23, 55, 0, 0, time.UTC)
	day2 := day1.Add(10 * time.Minute) // 00:05 the next day

	db, err := Open(DailyPath(dir, day1), testLog(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	insert(t, db, "aaa001")

	rotated, err := db.RotateIfNewDay(dir, day2)
	if err != nil {
		t.Fatalf("RotateIfNewDay: %v", err)
	}
	if !rotated {
		t.Fatal("crossing midnight must rotate")
	}
	if got, want := db.Path(), DailyPath(dir, day2); got != want {
		t.Fatalf("path did not follow: got %s, want %s", got, want)
	}

	insert(t, db, "bbb002")

	if n := countAircraft(t, DailyPath(dir, day1)); n != 1 {
		t.Errorf("yesterday's file should keep its single row, has %d", n)
	}
	if n := countAircraft(t, DailyPath(dir, day2)); n != 1 {
		t.Errorf("today's file should hold the row written after rotation, has %d", n)
	}
}

// Four storage types capture the connection at startup. Rotation is worth
// nothing if their copies keep pointing at the old file.
func TestStoragesBuiltBeforeRotationFollowIt(t *testing.T) {
	dir := t.TempDir()
	day1 := time.Date(2026, 9, 20, 22, 0, 0, 0, time.UTC)
	day2 := day1.Add(4 * time.Hour)
	log := testLog(t)

	aircraft, err := NewAircraftStorage(DailyPath(dir, day1), log)
	if err != nil {
		t.Fatalf("NewAircraftStorage: %v", err)
	}
	// Built once, before any rotation, exactly as main does.
	transcriptions := NewTranscriptionStorage(aircraft.GetDB(), log)
	clearances := NewClearanceStorage(aircraft.GetDB(), log)

	if _, err := aircraft.GetDB().RotateIfNewDay(dir, day2); err != nil {
		t.Fatalf("RotateIfNewDay: %v", err)
	}

	for name, store := range map[string]interface {
		Exec(string, ...any) (sql.Result, error)
	}{
		"aircraft":       aircraft.GetDB(),
		"transcriptions": DBOf(transcriptions),
		"clearances":     clearances.db,
	} {
		t.Run(name, func(t *testing.T) {
			if got := DBOf(transcriptions).Path(); got != DailyPath(dir, day2) {
				t.Errorf("%s still points at %s", name, got)
			}
			insert(t, store, "hex"+name[:3])
		})
	}

	if n := countAircraft(t, DailyPath(dir, day2)); n != 3 {
		t.Errorf("all three storages should have written to today's file, got %d rows", n)
	}
	if n := countAircraft(t, DailyPath(dir, day1)); n != 0 {
		t.Errorf("yesterday's file should have received nothing, got %d rows", n)
	}
}

// The tables the radio writes to were created by the storage constructors, at
// startup, and not by the open that rotation performs. On 2026-09-21 at
// 00:00:02 rotation opened the new day's file and every transcription until
// the 10:51 restart failed with "no such table: transcriptions": 1,877
// transmissions transcribed that night, none kept. The test above wrote to the
// aircraft table through every storage, which is the one table rotation did
// create. This one writes what each storage actually writes.
func TestEveryStorageCanWriteToTheFileRotationCreated(t *testing.T) {
	dir := t.TempDir()
	day1 := time.Date(2026, 9, 20, 23, 59, 0, 0, time.UTC)
	day2 := day1.Add(2 * time.Minute)
	log := testLog(t)

	aircraft, err := NewAircraftStorage(DailyPath(dir, day1), log)
	if err != nil {
		t.Fatalf("NewAircraftStorage: %v", err)
	}
	transcriptions := NewTranscriptionStorage(aircraft.GetDB(), log)
	clearances := NewClearanceStorage(aircraft.GetDB(), log)
	values := NewPhraseologyStorage(DBOf(transcriptions))

	if _, err := aircraft.GetDB().RotateIfNewDay(dir, day2); err != nil {
		t.Fatalf("RotateIfNewDay: %v", err)
	}

	id, err := transcriptions.StoreTranscription(&TranscriptionRecord{
		FrequencyID: "aero-melange", CreatedAt: day2,
		Content: "air france one zero eight one", IsComplete: true,
	})
	if err != nil {
		t.Fatalf("the first transcription after midnight: %v", err)
	}
	if err := transcriptions.UpdateMatchedTranscription(id, "air france one zero eight one",
		"ATC", "AFR1081", "en", [][2]int{{0, 10}}, nil); err != nil {
		t.Fatalf("the first match after midnight: %v", err)
	}
	if _, err := clearances.StoreClearance(&ClearanceRecord{
		TranscriptionID: id, Callsign: "AFR1081", ClearanceType: "landing",
		ClearanceText: "cleared to land", Timestamp: day2, Status: "issued", CreatedAt: day2,
	}); err != nil {
		t.Fatalf("the first clearance after midnight: %v", err)
	}
	if err := values.StoreValues([]PhraseologyValue{{
		TranscriptionID: id, Callsign: "AFR1081", Role: "flight_level",
		Digits: "120", Text: "FL120", CreatedAt: day2,
	}}); err != nil {
		t.Fatalf("the first value after midnight: %v", err)
	}

	for _, table := range []string{"transcriptions", "clearances", "phraseology_values"} {
		if n := countRows(t, DailyPath(dir, day2), table); n != 1 {
			t.Errorf("today's file should hold one row of %s, has %d", table, n)
		}
		if n := countRows(t, DailyPath(dir, day1), table); n != 0 {
			t.Errorf("yesterday's file should hold no %s, has %d", table, n)
		}
	}
}

// The file rotation lands on can already exist, written by an older build with
// a transcriptions table short of the columns added since. Startup adds the
// missing columns; the open that rotation performs must too.
func TestRotationBringsAnOlderFileUpToDate(t *testing.T) {
	dir := t.TempDir()
	day1 := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)

	// Upstream's transcriptions table, as a build before 16/09 created it.
	older, err := sql.Open("sqlite", DailyPath(dir, day2))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := older.Exec(`CREATE TABLE transcriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT, frequency_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL, content TEXT NOT NULL, is_complete BOOLEAN NOT NULL,
		is_processed BOOLEAN NOT NULL, content_processed TEXT, speaker_type TEXT, callsign TEXT)`); err != nil {
		t.Fatalf("create the older table: %v", err)
	}
	older.Close()

	db, err := Open(DailyPath(dir, day1), testLog(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	transcriptions := NewTranscriptionStorage(db, testLog(t))

	if _, err := db.RotateIfNewDay(dir, day2); err != nil {
		t.Fatalf("RotateIfNewDay: %v", err)
	}
	id, err := transcriptions.StoreTranscription(&TranscriptionRecord{
		FrequencyID: "aero-melange", CreatedAt: day2, Content: "bonjour", IsComplete: true,
		Language: "fr", ContentSecond: "bonjour",
	})
	if err != nil {
		t.Fatalf("storing into the older file after rotation: %v", err)
	}
	if err := transcriptions.UpdateMatchedTranscription(id, "bonjour", "ATC", "AFR1081", "fr",
		[][2]int{{0, 7}}, nil); err != nil {
		t.Fatalf("the columns added since should exist after rotation: %v", err)
	}
}

// Retention deletes files it can see; a file still held open by the process is
// the one it skips. Rotation has to actually let go of the old one.
func TestTheOldFileIsReleasedAndCanBeDeleted(t *testing.T) {
	dir := t.TempDir()
	day1 := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)

	db, err := Open(DailyPath(dir, day1), testLog(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	insert(t, db, "aaa001")
	if _, err := db.RotateIfNewDay(dir, day2); err != nil {
		t.Fatalf("RotateIfNewDay: %v", err)
	}

	// The close runs on its own goroutine so rotation is not held up by a slow
	// reader; give it a moment before asserting the file is free.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := os.Remove(DailyPath(dir, day1)); err == nil {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("could not delete yesterday's file: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// And the handle is unharmed by the old connection going away.
	insert(t, db, "bbb002")
	if n := countAircraft(t, DailyPath(dir, day2)); n != 1 {
		t.Errorf("today's file should still accept writes, has %d rows", n)
	}
}

func TestRotateToAnUnusablePathLeavesTheHandleWorking(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	db, err := Open(DailyPath(dir, now), testLog(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// A directory where the new file should go: opening it must fail.
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "co-atc-2026-09-21.db"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := db.RotateIfNewDay(blocked, now.AddDate(0, 0, 1)); err == nil {
		t.Error("rotating onto a directory should fail")
	}
	if got, want := db.Path(), DailyPath(dir, now); got != want {
		t.Errorf("a failed rotation must not move the handle: got %s", got)
	}
	// Still usable: a failed rotation is not allowed to take the server down.
	insert(t, db, "aaa001")
}

// A timer still running as the server stops queries a closed database: it
// gets an error, not a nil-pointer panic that made the process exit in error.
func TestACallAfterCloseIsAnErrorNotAPanic(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "co-atc-2026-10-27.db"), testLog(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Query("SELECT 1"); !errors.Is(err, sql.ErrConnDone) {
		t.Errorf("Query after Close: %v", err)
	}
	if _, err := db.Exec("SELECT 1"); !errors.Is(err, sql.ErrConnDone) {
		t.Errorf("Exec after Close: %v", err)
	}
	if _, err := db.Begin(); !errors.Is(err, sql.ErrConnDone) {
		t.Errorf("Begin after Close: %v", err)
	}
}
