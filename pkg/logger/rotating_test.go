package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func listLogs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestWritesGoIntoTodaysFile(t *testing.T) {
	dir := t.TempDir()
	r, err := newRotatingFile(filepath.Join(dir, "co-atc.log"), 1, 7)
	if err != nil {
		t.Fatalf("newRotatingFile: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := "co-atc-" + time.Now().Format("2006-01-02") + ".log"
	body, err := os.ReadFile(filepath.Join(dir, want))
	if err != nil {
		t.Fatalf("today's file should exist as %s: %v", want, err)
	}
	if string(body) != "hello\n" {
		t.Errorf("got %q", body)
	}
}

// The bound that matters: a day that goes wrong must not write one unbounded
// file. Seventeen servers once put 103 MB into a single log in a night.
func TestSizeCeilingOpensTheNextFile(t *testing.T) {
	dir := t.TempDir()
	r, err := newRotatingFile(filepath.Join(dir, "co-atc.log"), 1, 7) // 1 MB
	if err != nil {
		t.Fatalf("newRotatingFile: %v", err)
	}
	defer r.Close()

	line := []byte(strings.Repeat("x", 4095) + "\n")
	for i := 0; i < 700; i++ { // ~2.8 MB
		if _, err := r.Write(line); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	files := listLogs(t, dir)
	if len(files) < 3 {
		t.Fatalf("2.8 MB at a 1 MB ceiling should span at least 3 files, got %v", files)
	}
	for _, f := range files {
		info, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		// One line may straddle the ceiling; more than a line past it is a bug.
		if info.Size() > 1024*1024+int64(len(line)) {
			t.Errorf("%s is %d bytes, past the 1 MB ceiling", f, info.Size())
		}
	}
}

// Without this the bound is only per file, and the directory grows for ever.
func TestOldFilesArePruned(t *testing.T) {
	dir := t.TempDir()
	// Files from days gone by, named as the rotator names them.
	for i := 1; i <= 10; i++ {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		p := filepath.Join(dir, fmt.Sprintf("co-atc-%s.log", day))
		if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r, err := newRotatingFile(filepath.Join(dir, "co-atc.log"), 1, 3)
	if err != nil {
		t.Fatalf("newRotatingFile: %v", err)
	}
	defer r.Close()

	files := listLogs(t, dir)
	if len(files) != 3 {
		t.Fatalf("maxFiles 3 should leave 3 files, got %d: %v", len(files), files)
	}
	// Today's must be one of them: pruning may never delete what is being written.
	today := "co-atc-" + time.Now().Format("2006-01-02") + ".log"
	found := false
	for _, f := range files {
		if f == today {
			found = true
		}
	}
	if !found {
		t.Errorf("today's file was pruned; kept %v", files)
	}
}

// A restart in the middle of a day continues that day's log. Truncating would
// lose exactly the part an operator goes looking for after a crash.
func TestReopeningADayAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "co-atc.log")

	r1, err := newRotatingFile(path, 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	r1.Write([]byte("before the restart\n"))
	r1.Close()

	r2, err := newRotatingFile(path, 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	r2.Write([]byte("after the restart\n"))

	body, err := os.ReadFile(filepath.Join(dir, "co-atc-"+time.Now().Format("2006-01-02")+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "before the restart") {
		t.Error("the restart truncated the day's log")
	}
	if !strings.Contains(string(body), "after the restart") {
		t.Error("the restart did not append")
	}
}

// A full file must not be reopened and appended to on restart, or the ceiling
// stops being a ceiling.
func TestARestartSkipsAFullFile(t *testing.T) {
	dir := t.TempDir()
	day := time.Now().Format("2006-01-02")
	full := filepath.Join(dir, "co-atc-"+day+".log")
	if err := os.WriteFile(full, make([]byte, 2*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := newRotatingFile(filepath.Join(dir, "co-atc.log"), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Write([]byte("fresh\n"))

	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 2*1024*1024 {
		t.Errorf("the full file was appended to: %d bytes", info.Size())
	}
	next := filepath.Join(dir, "co-atc-"+day+".1.log")
	if body, err := os.ReadFile(next); err != nil || string(body) != "fresh\n" {
		t.Errorf("the write should have gone to %s, got %q err %v", next, body, err)
	}
}

// Whatever else happens, the logger must keep logging: an empty File means
// stdout only, which is upstream's behaviour and must stay reachable.
func TestNoFileConfiguredIsStillAValidLogger(t *testing.T) {
	log, err := New(Config{Level: "info", Format: "console"})
	if err != nil {
		t.Fatalf("a logger without a file must work: %v", err)
	}
	log.Info("still logging")
}

func TestFileConfiguredLogsToBoth(t *testing.T) {
	dir := t.TempDir()
	log, err := New(Config{
		Level: "info", Format: "console",
		File: filepath.Join(dir, "co-atc.log"), MaxSizeMB: 1, MaxFiles: 3,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Info("a line that should reach the file")
	_ = log.Sync()

	body, err := os.ReadFile(filepath.Join(dir, "co-atc-"+time.Now().Format("2006-01-02")+".log"))
	if err != nil {
		t.Fatalf("the log file should exist: %v", err)
	}
	if !strings.Contains(string(body), "should reach the file") {
		t.Errorf("the line did not reach the file, got %q", body)
	}
}

// The file being written is said as it is, for the launcher's health: the
// day's, with the date the launcher used to rebuild itself.
func TestTheLogFileBeingWrittenIsSaid(t *testing.T) {
	dir := t.TempDir()
	log, err := New(Config{Level: "info", Format: "json", File: filepath.Join(dir, "co-atc.log")})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("hello")
	want := filepath.Join(dir, "co-atc-"+time.Now().Format("2006-01-02")+".log")
	if got := File(); got != want {
		t.Errorf("File() = %q, want %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Error(err)
	}
}
