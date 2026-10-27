// Package usage keeps what each account did, for the owner (D84, 27/10): the
// pages it had open, the sound it listened to, the clips it played again, and
// how each ended. The sign-in trace (internal/auth) says who came in; this says
// what they did once in.
//
// A record is written when what it describes ends -- a page closed, a stream
// stopped -- with its start, its end and why; what is still open is held in
// memory and shown as such. The file is kept beside the accounts and holds the
// last Days days: older records are dropped when the journal opens and once a
// day after. No address is written: the sign-in trace already has them.
package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// File is the journal's name, beside the accounts.
const File = "usage.jsonl"

// What a record is about.
const (
	KindPage  = "page"  // a page open, as its WebSocket
	KindSound = "sound" // a live stream of a frequency
	KindClip  = "clip"  // an archived transmission played again
)

// Why it ended.
const (
	EndClosed  = "closed"  // the page went: closed, reloaded, or its tab gave up
	EndLost    = "lost"    // the connection broke without a goodbye
	EndCut     = "cut"     // the owner cut the account off, or removed it
	EndStopped = "stopped" // the server stopped, for a restart or for good
	EndSource  = "source"  // the server's own source of the sound ended
)

const (
	// recordsMax bounds what is held and written, whatever the days: a page
	// that reconnects in a loop must not fill the disk.
	recordsMax = 50_000
	// clipAgain: a clip asked again by the same account within this is the
	// same listening (a browser fetches a clip in several ranges).
	clipAgain = time.Minute
)

// Record is one line of the journal.
type Record struct {
	User  string    `json:"user"`
	Kind  string    `json:"kind"`
	Freq  string    `json:"freq,omitempty"`
	Clip  string    `json:"clip,omitempty"` // the transcription played again
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Bytes int64     `json:"bytes,omitempty"`
	Ended string    `json:"ended,omitempty"`
}

// Journal is the usage journal. A nil Journal records nothing and reports
// nothing: usage_days = 0, or sign-in off.
type Journal struct {
	mu      sync.Mutex
	path    string
	keep    time.Duration
	now     func() time.Time
	done    []Record // ended, oldest end first
	open    map[int]*Item
	next    int
	stopped bool
	pruned  time.Time
	clips   map[string]time.Time // user + clip -> last recorded
}

// Item is something open: End closes it, once.
type Item struct {
	j      *Journal
	id     int
	rec    Record
	reason string // set by Cut or Stop before the item learns it is over
}

// Open opens the journal kept at path for days days, reading back what is
// there. days <= 0 is no journal. A zero path keeps it in memory only. A line
// that does not parse is skipped: a journal that cannot be read back must not
// stop a server.
func Open(path string, days int) *Journal {
	return open(path, days, time.Now)
}

func open(path string, days int, now func() time.Time) *Journal {
	if days <= 0 {
		return nil
	}
	j := &Journal{
		path:  path,
		keep:  time.Duration(days) * 24 * time.Hour,
		now:   now,
		open:  map[int]*Item{},
		clips: map[string]time.Time{},
	}
	if path != "" {
		j.load()
	}
	j.mu.Lock()
	j.pruneLocked(true)
	j.mu.Unlock()
	return j
}

// Days is how many days the journal keeps.
func (j *Journal) Days() int {
	if j == nil {
		return 0
	}
	return int(j.keep / (24 * time.Hour))
}

func (j *Journal) load() {
	f, err := os.Open(j.path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.User != "" && r.Kind != "" {
			j.done = append(j.done, r)
		}
	}
	sort.SliceStable(j.done, func(a, b int) bool { return j.done[a].End.Before(j.done[b].End) })
}

// pruneLocked drops what is older than the days kept, or past recordsMax,
// and rewrites the file when anything went -- at the opening, then once a
// day at most.
func (j *Journal) pruneLocked(force bool) {
	now := j.now()
	if !force && now.Sub(j.pruned) < 24*time.Hour {
		return
	}
	j.pruned = now
	cutoff := now.Add(-j.keep)
	i := sort.Search(len(j.done), func(i int) bool { return j.done[i].End.After(cutoff) })
	if over := len(j.done) - i - recordsMax; over > 0 {
		i += over
	}
	if i == 0 {
		return
	}
	j.done = append(j.done[:0:0], j.done[i:]...)
	if j.path != "" {
		j.rewriteLocked()
	}
}

func (j *Journal) rewriteLocked() {
	tmp := j.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range j.done {
		if enc.Encode(r) != nil {
			f.Close()
			os.Remove(tmp)
			return
		}
	}
	if w.Flush() != nil || f.Close() != nil {
		os.Remove(tmp)
		return
	}
	os.Rename(tmp, j.path)
}

func (j *Journal) appendLocked(r Record) {
	j.done = append(j.done, r)
	if len(j.done) > recordsMax {
		j.done = append(j.done[:0:0], j.done[len(j.done)-recordsMax:]...)
	}
	if j.path != "" {
		if line, err := json.Marshal(r); err == nil {
			if f, err := os.OpenFile(j.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				f.Write(append(line, '\n'))
				f.Close()
			}
		}
	}
	j.pruneLocked(false)
}

// Begin opens an item for user: a page or a sound stream. nil when there is
// no journal, no account, or the server is stopping; End on nil does nothing.
func (j *Journal) Begin(user, kind, freq string) *Item {
	if j == nil || user == "" {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.stopped {
		return nil
	}
	j.next++
	it := &Item{j: j, id: j.next, rec: Record{User: user, Kind: kind, Freq: freq, Start: j.now().UTC()}}
	j.open[it.id] = it
	return it
}

// End closes the item with what it moved and why it ended -- unless Cut or
// Stop said why first, which wins: a stream cut off ends with an error of its
// own, and the cut is the reason.
func (it *Item) End(why string, bytes int64) {
	if it == nil {
		return
	}
	j := it.j
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.open[it.id]; !ok {
		return
	}
	delete(j.open, it.id)
	if it.reason != "" {
		why = it.reason
	}
	r := it.rec
	r.End, r.Bytes, r.Ended = j.now().UTC(), bytes, why
	j.appendLocked(r)
}

// Forget drops the item unwritten: it never amounted to anything (a stream
// refused, or that sent nothing).
func (it *Item) Forget() {
	if it == nil {
		return
	}
	it.j.mu.Lock()
	delete(it.j.open, it.id)
	it.j.mu.Unlock()
}

// Cut marks what user has open as ended by the owner, before it is closed.
func (j *Journal) Cut(user string) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, it := range j.open {
		if it.rec.User == user {
			it.reason = EndCut
		}
	}
}

// Stop ends everything open as stopped by the server, now, and opens nothing
// more: what the shutdown then closes has already been written.
func (j *Journal) Stop() {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.stopped = true
	ids := make([]int, 0, len(j.open))
	for id := range j.open {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	now := j.now().UTC()
	for _, id := range ids {
		r := j.open[id].rec
		r.End, r.Ended = now, EndStopped
		delete(j.open, id)
		j.appendLocked(r)
	}
}

// Clip records a clip played again by user: once a minute at most for the
// same clip, since a browser fetches one in several ranges.
func (j *Journal) Clip(user, freq, clip string) {
	if j == nil || user == "" {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.now()
	key := user + "\x00" + clip
	if last, ok := j.clips[key]; ok && now.Sub(last) < clipAgain {
		return
	}
	if len(j.clips) >= 1024 {
		for k, at := range j.clips {
			if now.Sub(at) >= clipAgain {
				delete(j.clips, k)
			}
		}
	}
	j.clips[key] = now
	at := now.UTC()
	j.appendLocked(Record{User: user, Kind: KindClip, Freq: freq, Clip: clip, Start: at, End: at})
}

// snapshot is what was ended and what is open, copied under the lock.
func (j *Journal) snapshot() (done []Record, open []Record, now time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	done = append([]Record(nil), j.done...)
	for _, it := range j.open {
		open = append(open, it.rec)
	}
	sort.Slice(open, func(a, b int) bool { return open[a].Start.Before(open[b].Start) })
	return done, open, j.now()
}
