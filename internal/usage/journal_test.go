package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// clock is a journal's time, moved by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func at(s string) time.Time          { t, _ := time.ParseInLocation("2006-01-02 15:04:05", s, paris); return t }
func opened(t *testing.T, path string, days int, c *clock) *Journal {
	t.Helper()
	return open(path, days, c.now)
}

var paris, _ = time.LoadLocation("Europe/Paris")

func lines(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("a line that does not parse: %q", sc.Text())
		}
		out = append(out, r)
	}
	return out
}

// Off is off: no journal, nothing written, nothing reported, nothing breaks.
func TestNoDaysIsNoJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	j := Open(path, 0)
	if j != nil {
		t.Fatal("a journal with 0 days")
	}
	j.Begin("ami", KindPage, "").End(EndClosed, 0)
	j.Clip("ami", "124350", "42")
	j.Cut("ami")
	j.Stop()
	if r := j.Report(paris); len(r.Now) != 0 || len(r.Accounts) != 0 {
		t.Errorf("a report from no journal: %+v", r)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a file written with 0 days")
	}
}

// What ends is written with why; what was cut or stopped says so, whatever
// the item says when it learns it; nothing is written twice; the journal
// reads back what it wrote.
func TestWhatEndsIsWrittenWithWhy(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	c := &clock{at("2026-10-27 17:00:00")}
	j := opened(t, path, 30, c)

	page := j.Begin("ami", KindPage, "")
	sound := j.Begin("ami", KindSound, "124350")
	other := j.Begin("chatelp", KindSound, "126425")
	if j.Begin("", KindPage, "") != nil {
		t.Error("an item without an account")
	}
	c.add(90 * time.Second)
	sound.End(EndClosed, 720_000)
	sound.End(EndLost, 1) // once only
	j.Clip("ami", "124350", "42")
	j.Clip("ami", "124350", "42") // the same listening, in ranges
	c.add(2 * time.Minute)
	j.Clip("ami", "124350", "42") // played again
	j.Cut("ami")
	page.End(EndLost, 0) // learns it after the cut: the cut is why
	c.add(time.Minute)
	j.Stop()
	other.End(EndSource, 5) // after the stop: already written
	if j.Begin("ami", KindPage, "") != nil {
		t.Error("an item opened after the stop")
	}

	got := lines(t, path)
	want := []struct{ user, kind, ended string }{
		{"ami", KindSound, EndClosed},
		{"ami", KindClip, ""},
		{"ami", KindClip, ""},
		{"ami", KindPage, EndCut},
		{"chatelp", KindSound, EndStopped},
	}
	if len(got) != len(want) {
		t.Fatalf("%d lines, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].User != w.user || got[i].Kind != w.kind || got[i].Ended != w.ended {
			t.Errorf("line %d: %+v, want %v", i, got[i], w)
		}
	}
	if got[0].Bytes != 720_000 || got[0].End.Sub(got[0].Start) != 90*time.Second || got[0].Freq != "124350" {
		t.Errorf("the stream as written: %+v", got[0])
	}
	if got[4].End.Sub(got[4].Start) != 4*time.Minute+30*time.Second {
		t.Errorf("the stopped stream ends at the stop: %v", got[4].End.Sub(got[4].Start))
	}

	again := opened(t, path, 30, c)
	if len(again.done) != len(want) {
		t.Errorf("read back %d records, want %d", len(again.done), len(want))
	}
}

// Thirty days are kept: older records go at the opening, and once a day.
func TestOldRecordsGo(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	c := &clock{at("2026-09-01 12:00:00")}
	j := opened(t, path, 30, c)
	j.Begin("ami", KindPage, "").End(EndClosed, 0)
	c.add(20 * 24 * time.Hour)
	j.Begin("ami", KindPage, "").End(EndClosed, 0)
	if n := len(lines(t, path)); n != 2 {
		t.Fatalf("%d lines before the days are up", n)
	}

	c.add(11 * 24 * time.Hour) // the first is 31 days old, the second 11
	j.Begin("ami", KindPage, "").End(EndClosed, 0)
	if n := len(lines(t, path)); n != 2 {
		t.Errorf("%d lines once the first is past 30 days, want 2", n)
	}

	c.add(25 * 24 * time.Hour)
	again := opened(t, path, 30, c) // 36 and 25 days: one left
	_ = again
	if n := len(lines(t, path)); n != 1 {
		t.Errorf("%d lines after reopening, want 1", n)
	}
}

// A line that does not parse is skipped, not fatal.
func TestABadLineIsSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	now := time.Now().UTC()
	good, _ := json.Marshal(Record{User: "ami", Kind: KindPage, Start: now.Add(-time.Minute), End: now})
	os.WriteFile(path, append(append([]byte("{not json\n"), good...), '\n'), 0o600)
	if j := Open(path, 30); len(j.done) != 1 {
		t.Errorf("read back %d records, want 1", len(j.done))
	}
}
