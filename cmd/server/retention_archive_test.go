package main

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filled writes a file of n random bytes, and returns them.
func filled(t *testing.T, dir, name string, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

func gunzipped(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// About 40 KB: room for today's file and one more day.
const smallCapGB = 40.0 / (1 << 20)

// A day past the cap is archived, gzipped, with its write-ahead log when that
// holds something, read back, summed in SHA256SUMS, and only then deleted.
func TestRetentionArchivesBeforeDeleting(t *testing.T) {
	dir, archive := t.TempDir(), t.TempDir()
	filled(t, dir, "co-atc-2026-10-28.db", 20<<10)
	active := filepath.Join(dir, "co-atc-2026-10-28.db")
	filled(t, dir, "co-atc-2026-10-27.db", 16<<10)
	old := filled(t, dir, "co-atc-2026-10-26.db", 16<<10)
	wal := filled(t, dir, "co-atc-2026-10-26.db-wal", 4<<10)
	filled(t, dir, "co-atc-2026-10-25.db", 16<<10)
	filled(t, dir, "co-atc-2026-10-25.db-wal", 0) // empty: not archived

	if err := cleanupOldDailyDatabases(dir, active, smallCapGB, archive, quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	if got, want := left(t, dir), []string{"co-atc-2026-10-27.db", "co-atc-2026-10-28.db"}; !equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
	if got, want := left(t, archive), []string{"SHA256SUMS", "co-atc-2026-10-25.db.gz", "co-atc-2026-10-26.db-wal.gz", "co-atc-2026-10-26.db.gz"}; !equal(got, want) {
		t.Errorf("archived %v, want %v", got, want)
	}
	if !bytes.Equal(gunzipped(t, filepath.Join(archive, "co-atc-2026-10-26.db.gz")), old) ||
		!bytes.Equal(gunzipped(t, filepath.Join(archive, "co-atc-2026-10-26.db-wal.gz")), wal) {
		t.Error("an archive does not hold its original's bytes")
	}
	sums, _ := os.ReadFile(filepath.Join(archive, "SHA256SUMS"))
	h := sha256.Sum256(old)
	if !strings.Contains(string(sums), hex.EncodeToString(h[:])+"  co-atc-2026-10-26.db\n") || strings.Count(string(sums), "\n") != 3 {
		t.Errorf("SHA256SUMS:\n%s", sums)
	}
}

// An archive that cannot be found -- its disk unplugged -- deletes nothing,
// and the directory is not made.
func TestAMissingArchiveDeletesNothing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "unplugged")
	filled(t, dir, "co-atc-2026-10-28.db", 20<<10)
	filled(t, dir, "co-atc-2026-10-27.db", 16<<10)
	filled(t, dir, "co-atc-2026-10-26.db", 16<<10)

	if err := cleanupOldDailyDatabases(dir, filepath.Join(dir, "co-atc-2026-10-28.db"), smallCapGB, missing, quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	if n := len(left(t, dir)); n != 3 {
		t.Errorf("%d files left, want all 3", n)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("the archive directory was made")
	}
}

// A copy already in the archive is kept when it holds the same bytes; one that
// holds other bytes is never overwritten, and its day stays local.
func TestAnArchiveIsNeverOverwritten(t *testing.T) {
	dir, archive := t.TempDir(), t.TempDir()
	filled(t, dir, "co-atc-2026-10-28.db", 20<<10)
	active := filepath.Join(dir, "co-atc-2026-10-28.db")
	filled(t, dir, "co-atc-2026-10-27.db", 16<<10)
	same := filled(t, dir, "co-atc-2026-10-26.db", 16<<10)
	filled(t, dir, "co-atc-2026-10-25.db", 16<<10)
	for name, b := range map[string][]byte{"co-atc-2026-10-26.db.gz": same, "co-atc-2026-10-25.db.gz": []byte("another day")} {
		f, _ := os.Create(filepath.Join(archive, name))
		zw := gzip.NewWriter(f)
		zw.Write(b)
		zw.Close()
		f.Close()
	}

	if err := cleanupOldDailyDatabases(dir, active, smallCapGB, archive, quietLogger(t)); err != nil {
		t.Fatal(err)
	}
	if got, want := left(t, dir), []string{"co-atc-2026-10-25.db", "co-atc-2026-10-27.db", "co-atc-2026-10-28.db"}; !equal(got, want) {
		t.Errorf("left %v, want the 25th kept beside the newest two", got)
	}
	if string(gunzipped(t, filepath.Join(archive, "co-atc-2026-10-25.db.gz"))) != "another day" {
		t.Error("an archive holding other bytes was overwritten")
	}
}
