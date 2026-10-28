package usage

import (
	"testing"
	"time"
)

// The panel's sums: two tabs counted once, the frequencies apart, a reload in
// the same visit, the days cut at local midnight, what is open counted to now.
func TestTheReportSumsAnAccountsWeek(t *testing.T) {
	c := &clock{at("2026-10-26 23:50:00")}
	j := opened(t, "", 30, c)

	// The evening before, across midnight: 20 min of page, 10 of them listening.
	p := j.Begin("ami", KindPage, "")
	c.add(5 * time.Minute)
	s := j.Begin("ami", KindSound, "124350")
	c.add(10 * time.Minute)
	s.End(EndClosed, 0)
	c.add(5 * time.Minute)
	p.End(EndClosed, 0) // 00:10

	// Today: two tabs, then a reload; both frequencies, one of them twice.
	c.t = at("2026-10-27 17:00:00")
	a := j.Begin("ami", KindPage, "")
	b := j.Begin("ami", KindPage, "") // a second tab: not twice the time
	s1 := j.Begin("ami", KindSound, "124350")
	s2 := j.Begin("ami", KindSound, "126425")
	s3 := j.Begin("ami", KindSound, "124350") // the second tab's: once
	c.add(30 * time.Minute)
	b.End(EndClosed, 0)
	s3.End(EndClosed, 0)
	s2.End(EndClosed, 0)
	c.add(30 * time.Minute)
	a.End(EndClosed, 0)
	s1.End(EndClosed, 0)
	c.add(2 * time.Minute)
	j.Begin("ami", KindPage, "") // reloaded: the same visit, still open
	j.Clip("ami", "126425", "7")
	c.add(8 * time.Minute) // 18:10

	rep := j.Report(paris)
	if len(rep.Now) != 1 || rep.Now[0].User != "ami" || rep.Now[0].Pages != 1 || len(rep.Now[0].Listening) != 0 {
		t.Errorf("now: %+v", rep.Now)
	}
	if len(rep.Accounts) != 1 {
		t.Fatalf("accounts: %+v", rep.Accounts)
	}
	acc := rep.Accounts[0]
	w := acc.Week
	if w.Connected != (20+60+8)*60 {
		t.Errorf("connected %d s, want %d", w.Connected, (20+60+8)*60)
	}
	if w.Listening != (10+60)*60 {
		t.Errorf("listening %d s, want %d", w.Listening, (10+60)*60)
	}
	if w.ByFreq["124350"] != (10+60)*60 || w.ByFreq["126425"] != 30*60 {
		t.Errorf("by frequency %v", w.ByFreq)
	}
	if w.Clips != 1 || w.Visits != 2 {
		t.Errorf("clips %d, visits %d; want 1 and 2", w.Clips, w.Visits)
	}
	if len(acc.Days) != 7 || acc.Days[6].Date != "2026-10-27" || acc.Days[5].Date != "2026-10-26" {
		t.Fatalf("days %+v", acc.Days)
	}
	if acc.Days[5].Connected != 10*60 || acc.Days[6].Connected != (10+60+8)*60 {
		t.Errorf("cut at midnight: the 26th %d s, the 27th %d s", acc.Days[5].Connected, acc.Days[6].Connected)
	}
	if acc.Days[5].Listening != 5*60 || acc.Days[6].Listening != (5+60)*60 || acc.Days[6].Clips != 1 {
		t.Errorf("listening by day: %+v", acc.Days[5:])
	}
	if !acc.LastSeen.Equal(c.t.UTC()) {
		t.Errorf("last seen %v, want now %v", acc.LastSeen, c.t.UTC())
	}
}

// The interruptions an account met: a restart, and how soon it came back; a
// connection that broke and came back; not a page left to sleep.
func TestTheReportFindsTheInterruptions(t *testing.T) {
	c := &clock{at("2026-10-27 16:50:00")}
	j := opened(t, "", 30, c)
	j.Begin("ami", KindPage, "")
	j.Begin("ami", KindPage, "") // a second tab: one interruption, not two
	j.Begin("ami", KindSound, "124350")
	c.t = at("2026-10-27 16:59:09")
	j.Stop()

	// The server is back; so is the page, 12 s later, and its sound, 15 s.
	j = &Journal{keep: j.keep, now: c.now, open: map[int]*Item{}, clips: map[string]time.Time{}, done: j.done}
	c.add(12 * time.Second)
	p := j.Begin("ami", KindPage, "")
	c.add(3 * time.Second)
	s := j.Begin("ami", KindSound, "124350")
	c.add(10 * time.Minute)
	p.End(EndLost, 0) // the network broke...
	s.End(EndLost, 0)
	c.add(4 * time.Second)
	p = j.Begin("ami", KindPage, "") // ...and the page came back alone
	c.add(time.Hour)
	p.End(EndLost, 0) // then the laptop slept: not back, not an interruption

	got := j.Report(paris).Accounts[0].Interruptions
	// The lost page (back in 4 s), the stopped page (12 s) and sound (15 s);
	// not the stream lost with the network and not resumed, nor the sleep.
	if len(got) != 3 || got[0].Why != EndLost {
		t.Fatalf("%d interruptions, want 3, the newest first: %+v", len(got), got)
	}
	var stoppedPage, stoppedSound, lostPage *Interruption
	for i := range got {
		g := &got[i]
		switch {
		case g.Kind == KindPage && g.Why == EndStopped:
			stoppedPage = g
		case g.Kind == KindSound && g.Why == EndStopped:
			stoppedSound = g
		case g.Kind == KindPage && g.Why == EndLost:
			lostPage = g
		}
	}
	if stoppedPage == nil || stoppedPage.BackAfter == nil || *stoppedPage.BackAfter != 12 {
		t.Errorf("the page stopped by the restart: %+v", stoppedPage)
	}
	if stoppedSound == nil || stoppedSound.BackAfter == nil || *stoppedSound.BackAfter != 15 || stoppedSound.Freq != "124350" {
		t.Errorf("the sound stopped by the restart: %+v", stoppedSound)
	}
	if lostPage == nil || *lostPage.BackAfter != 4 {
		t.Errorf("the page lost and back: %+v", lostPage)
	}
}

// A sound that plays on after its page's live connection broke is still being
// there: listening never comes out longer than connected (27-28/10, 12 h of
// sound against 56 min of pages, bars drawn at 1 300 %).
func TestListeningIsNeverLongerThanConnected(t *testing.T) {
	c := &clock{at("2026-10-27 20:40:00")}
	j := opened(t, "", 30, c)
	p := j.Begin("ami", KindPage, "")
	s := j.Begin("ami", KindSound, "124350")
	c.add(20 * time.Minute)
	p.End(EndLost, 0) // 21:00, the page's connection lost...
	c.add(3 * time.Hour)
	s.End(EndStopped, 0) // ...its sound playing on until midnight
	c.add(time.Minute)

	acc := j.Report(paris).Accounts[0]
	if acc.Week.Connected != acc.Week.Listening || acc.Week.Listening != (20+180)*60 {
		t.Errorf("connected %d s, listening %d s, want both %d", acc.Week.Connected, acc.Week.Listening, (20+180)*60)
	}
	for _, d := range acc.Days {
		if d.Listening > d.Connected {
			t.Errorf("%s: listening %d s past connected %d s", d.Date, d.Listening, d.Connected)
		}
	}
	if !acc.LastSeen.Equal(at("2026-10-28 00:00:00").UTC()) {
		t.Errorf("last seen %v, want the end of the sound", acc.LastSeen)
	}
}
