package usage

import (
	"sort"
	"time"
)

// What the owner's panel shows (D84): who is here now, and per account, over
// the last 7 days, the time connected and listening, the clips, the visits,
// a bar a day, and the interruptions it met.

const (
	reportDays = 7
	// visitGap: pages closer than this make one visit (a reload, a second tab).
	visitGap = 10 * time.Minute
	// backWithin: a page or a stream that comes back within this after an
	// interruption is the same listening resumed; later, it is a new one.
	backWithin = 10 * time.Minute
	// interruptionsShown, the newest first.
	interruptionsShown = 10
)

// Report is the panel's answer.
type Report struct {
	Days     int       `json:"days"` // how many the journal keeps
	Now      []Present `json:"now"`
	Accounts []Account `json:"accounts"`
}

// Present is an account with a page or a stream open now.
type Present struct {
	User      string      `json:"user"`
	Pages     int         `json:"pages"`
	Since     *time.Time  `json:"since,omitempty"` // its oldest page still open
	Listening []Listening `json:"listening"`
}

// Listening is a frequency an account hears now.
type Listening struct {
	Freq  string    `json:"freq"`
	Since time.Time `json:"since"`
}

// Account is an account's last 7 days.
type Account struct {
	User          string         `json:"user"`
	LastSeen      time.Time      `json:"last_seen"`
	Week          Totals         `json:"week"`
	Days          []Day          `json:"days"` // oldest first, today last
	Interruptions []Interruption `json:"interruptions"`
}

// Totals are seconds, two tabs counted once.
type Totals struct {
	Connected int64            `json:"connected_s"`
	Listening int64            `json:"listening_s"` // any frequency
	ByFreq    map[string]int64 `json:"by_freq_s"`
	Clips     int              `json:"clips"`
	Visits    int              `json:"visits"`
}

// Day is one local day.
type Day struct {
	Date      string `json:"date"` // 2006-01-02
	Connected int64  `json:"connected_s"`
	Listening int64  `json:"listening_s"`
	Clips     int    `json:"clips"`
}

// Interruption is a page or a stream an account lost: the server stopped, its
// sound's source ended, or the connection broke and came back.
type Interruption struct {
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Freq      string    `json:"freq,omitempty"`
	Why       string    `json:"why"`
	BackAfter *float64  `json:"back_after_s"` // nil: not back within 10 min
}

type span struct{ a, b time.Time }

// Report sums the journal up, its days cut at local midnight in loc.
func (j *Journal) Report(loc *time.Location) Report {
	if j == nil {
		return Report{Now: []Present{}, Accounts: []Account{}}
	}
	done, open, now := j.snapshot()
	return build(done, open, now, loc, j.Days())
}

func build(done, open []Record, now time.Time, loc *time.Location, days int) Report {
	rep := Report{Days: days, Now: []Present{}, Accounts: []Account{}}

	// Who is here now.
	present := map[string]*Present{}
	var order []string
	for _, r := range open {
		p := present[r.User]
		if p == nil {
			p = &Present{User: r.User, Listening: []Listening{}}
			present[r.User] = p
			order = append(order, r.User)
		}
		switch r.Kind {
		case KindPage:
			p.Pages++
			if p.Since == nil || r.Start.Before(*p.Since) {
				t := r.Start
				p.Since = &t
			}
		case KindSound:
			p.Listening = append(p.Listening, Listening{Freq: r.Freq, Since: r.Start})
		}
	}
	for _, u := range order {
		p := present[u]
		sort.Slice(p.Listening, func(a, b int) bool { return p.Listening[a].Freq < p.Listening[b].Freq })
		// Two pages listening to the same frequency: once, the earliest.
		var l []Listening
		for _, x := range p.Listening {
			if n := len(l); n > 0 && l[n-1].Freq == x.Freq {
				continue
			}
			l = append(l, x)
		}
		if l == nil {
			l = []Listening{}
		}
		p.Listening = l
		rep.Now = append(rep.Now, *p)
	}

	// Each account's last 7 days. What is open counts up to now.
	local := now.In(loc)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	var bounds []time.Time // reportDays+1 midnights, the last one tomorrow's
	for i := reportDays - 1; i >= 0; i-- {
		bounds = append(bounds, midnight.AddDate(0, 0, -i))
	}
	bounds = append(bounds, midnight.AddDate(0, 0, 1))
	from := bounds[0]

	all := make([]Record, 0, len(done)+len(open))
	all = append(all, done...)
	for _, r := range open {
		r.End = now
		all = append(all, r)
	}
	byUser := map[string][]Record{}
	for _, r := range all {
		if r.End.Before(from) {
			continue
		}
		byUser[r.User] = append(byUser[r.User], r)
	}

	for user, recs := range byUser {
		acc := Account{User: user, Week: Totals{ByFreq: map[string]int64{}}, Interruptions: []Interruption{}}
		var pages, sound []span
		perFreq := map[string][]span{}
		for _, r := range recs {
			s := span{r.Start, r.End}
			switch r.Kind {
			case KindPage:
				pages = append(pages, s)
				if r.End.After(acc.LastSeen) {
					acc.LastSeen = r.End
				}
			case KindSound:
				sound = append(sound, s)
				perFreq[r.Freq] = append(perFreq[r.Freq], s)
			}
		}
		pages, sound = union(pages), union(sound)
		acc.Week.Connected = seconds(pages, from, bounds[reportDays])
		acc.Week.Listening = seconds(sound, from, bounds[reportDays])
		for f, ss := range perFreq {
			if n := seconds(union(ss), from, bounds[reportDays]); n > 0 {
				acc.Week.ByFreq[f] = n
			}
		}
		for _, v := range joinGaps(pages, visitGap) {
			if v.b.After(from) {
				acc.Week.Visits++
			}
		}
		for i := 0; i < reportDays; i++ {
			acc.Days = append(acc.Days, Day{
				Date:      bounds[i].Format("2006-01-02"),
				Connected: seconds(pages, bounds[i], bounds[i+1]),
				Listening: seconds(sound, bounds[i], bounds[i+1]),
			})
		}
		for _, r := range recs {
			if r.Kind != KindClip || r.Start.Before(from) {
				continue
			}
			acc.Week.Clips++
			for i := 0; i < reportDays; i++ {
				if !r.Start.Before(bounds[i]) && r.Start.Before(bounds[i+1]) {
					acc.Days[i].Clips++
				}
			}
			if r.Start.After(acc.LastSeen) {
				acc.LastSeen = r.Start
			}
		}
		acc.Interruptions = interruptions(recs, from)
		rep.Accounts = append(rep.Accounts, acc)
	}
	sort.Slice(rep.Accounts, func(a, b int) bool {
		if !rep.Accounts[a].LastSeen.Equal(rep.Accounts[b].LastSeen) {
			return rep.Accounts[a].LastSeen.After(rep.Accounts[b].LastSeen)
		}
		return rep.Accounts[a].User < rep.Accounts[b].User
	})
	return rep
}

// interruptions finds what an account lost and whether it came back. A
// connection that broke and did not come back is a page left to sleep, not an
// interruption; a stop or a source that ended is one either way.
func interruptions(recs []Record, from time.Time) []Interruption {
	var out []Interruption
	seen := map[string]bool{}
	for _, r := range recs {
		if r.Kind == KindClip || r.End.Before(from) {
			continue
		}
		if r.Ended != EndStopped && r.Ended != EndSource && r.Ended != EndLost {
			continue
		}
		var back *float64
		for _, n := range recs {
			if n.Kind != r.Kind || (r.Kind == KindSound && n.Freq != r.Freq) {
				continue
			}
			gap := n.Start.Sub(r.End)
			if gap < 0 || gap > backWithin {
				continue
			}
			if s := gap.Seconds(); back == nil || s < *back {
				back = &s
			}
		}
		if r.Ended == EndLost && back == nil {
			continue
		}
		// Two tabs lost at the same second are one interruption.
		key := r.Kind + "\x00" + r.Freq + "\x00" + r.End.Truncate(time.Second).String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Interruption{At: r.End, Kind: r.Kind, Freq: r.Freq, Why: r.Ended, BackAfter: back})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].At.After(out[b].At) })
	if len(out) > interruptionsShown {
		out = out[:interruptionsShown]
	}
	if out == nil {
		out = []Interruption{}
	}
	return out
}

// union merges overlapping spans, sorted.
func union(ss []span) []span {
	return joinGaps(ss, 0)
}

// joinGaps merges spans that overlap or are closer than gap.
func joinGaps(ss []span, gap time.Duration) []span {
	if len(ss) == 0 {
		return nil
	}
	s := append([]span(nil), ss...)
	sort.Slice(s, func(a, b int) bool { return s[a].a.Before(s[b].a) })
	out := []span{s[0]}
	for _, x := range s[1:] {
		last := &out[len(out)-1]
		if !x.a.After(last.b.Add(gap)) {
			if x.b.After(last.b) {
				last.b = x.b
			}
			continue
		}
		out = append(out, x)
	}
	return out
}

// seconds is how much of the merged spans falls in [a, b).
func seconds(ss []span, a, b time.Time) int64 {
	var d time.Duration
	for _, s := range ss {
		lo, hi := s.a, s.b
		if lo.Before(a) {
			lo = a
		}
		if hi.After(b) {
			hi = b
		}
		if hi.After(lo) {
			d += hi.Sub(lo)
		}
	}
	return int64(d.Round(time.Second) / time.Second)
}
