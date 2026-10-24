// Package audioarchive finds the audio of a transmission in the archive the
// transcription sidecar keeps (sidecar/whisper_server.py, keep_audio): one
// folder per day, YYYYMMDD/<frequency_id>/t_….wav, and beside each day's audio
// a manifest, YYYYMMDD/manifeste.jsonl, one JSON line per clip.
//
// A manifest line carries the created_at co-atc sent with the audio
// (X-Created-At), which is the created_at of the transmission's row, and the
// clip's path relative to the archive. The pair (frequency, created_at) is
// therefore the join key, compared as instants: lines written before 11/10
// carry it in local time with an offset, later ones in UTC.
package audioarchive

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Archive answers which file holds a transmission's audio. Safe for
// concurrent use; manifests are read once and then only their new lines.
type Archive struct {
	root string

	mu   sync.Mutex
	days map[string]*dayIndex // manifest path -> what it says so far
}

type dayIndex struct {
	offset int64             // bytes read so far
	clips  map[string]string // frequency_id + "|" + unix second -> relative path
}

// New returns an archive rooted at dir, or nil when dir is empty.
func New(dir string) *Archive {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	return &Archive{root: filepath.Clean(dir), days: map[string]*dayIndex{}}
}

// FromSidecarCommand is the --save-audio directory of a sidecar command line,
// so the archive is found where the sidecar writes it without saying it twice.
func FromSidecarCommand(command []string) string {
	for i, arg := range command {
		if arg == "--save-audio" && i+1 < len(command) {
			return command[i+1]
		}
		if v, ok := strings.CutPrefix(arg, "--save-audio="); ok {
			return v
		}
	}
	return ""
}

// DirOf is where the archive is: the directory the configuration names, or
// else the --save-audio of the sidecar's command line, or "" for none.
func DirOf(configured string, sidecarCommand []string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return FromSidecarCommand(sidecarCommand)
}

// Root is the archive's directory.
func (a *Archive) Root() string { return a.root }

// Available says whether the archive's directory is there: an external disk
// may be unplugged.
func (a *Archive) Available() bool {
	if a == nil {
		return false
	}
	st, err := os.Stat(a.root)
	return err == nil && st.IsDir()
}

// Find returns the absolute path of the clip of the transmission received on
// frequencyID at createdAt, or "" when the archive has none.
func (a *Archive) Find(frequencyID string, createdAt time.Time) string {
	if a == nil || frequencyID == "" || createdAt.IsZero() {
		return ""
	}
	key := frequencyID + "|" + strconv.FormatInt(createdAt.Unix(), 10)
	// The sidecar names the day by its own local clock when the clip arrives,
	// up to a minute or so after the transmission: around midnight that is the
	// next day's folder.
	seen := map[string]bool{}
	for _, t := range []time.Time{createdAt, createdAt.Add(10 * time.Minute), createdAt.Add(-10 * time.Minute)} {
		day := t.Local().Format("20060102")
		if seen[day] {
			continue
		}
		seen[day] = true
		if rel := a.lookup(filepath.Join(a.root, day, "manifeste.jsonl"), key); rel != "" {
			if abs, ok := a.inside(rel); ok {
				if _, err := os.Stat(abs); err == nil {
					return abs
				}
			}
		}
	}
	return ""
}

// inside joins a manifest's relative path to the root, refusing anything that
// would leave it.
func (a *Archive) inside(rel string) (string, bool) {
	abs := filepath.Join(a.root, filepath.FromSlash(rel))
	if abs != a.root && strings.HasPrefix(abs, a.root+string(filepath.Separator)) {
		return abs, true
	}
	return "", false
}

func (a *Archive) lookup(manifest, key string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := a.days[manifest]
	if idx == nil {
		idx = &dayIndex{clips: map[string]string{}}
		a.days[manifest] = idx
	}
	if rel, ok := idx.clips[key]; ok {
		return rel
	}
	idx.readNew(manifest)
	return idx.clips[key]
}

// manifestLine is the part of a manifest line this package reads.
type manifestLine struct {
	FrequencyID  string `json:"frequency_id"`
	CreatedAt    string `json:"created_at"`
	SegmentAtUTC string `json:"segment_at_utc"`
	File         string `json:"fichier"`
}

// readNew reads the lines appended since the last read. A manifest that
// shrank was replaced: it is read again from the start.
func (d *dayIndex) readNew(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() < d.offset {
		d.offset = 0
		d.clips = map[string]string{}
	}
	if _, err := f.Seek(d.offset, io.SeekStart); err != nil {
		return
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			// A last line without its newline is being written: leave it for
			// the next read.
			return
		}
		d.offset += int64(len(line))
		var m manifestLine
		if json.Unmarshal(line, &m) != nil || m.File == "" || m.FrequencyID == "" {
			continue
		}
		at, ok := parseInstant(m.CreatedAt)
		if !ok {
			// Lines older than X-Created-At: the segment's own moment, which
			// co-atc stores truncated to the second.
			at, ok = parseInstant(m.SegmentAtUTC)
		}
		if !ok {
			continue
		}
		d.clips[m.FrequencyID+"|"+strconv.FormatInt(at.Unix(), 10)] = m.File
	}
}

func parseInstant(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// The archive's size, as the hourly purge last measured it (cmd/server,
// purgeAudioArchive): walking tens of gigabytes of clips is the purge's job,
// once an hour, not a health probe's every few seconds. Kept by directory, so
// the purge and the handler need not share an object.
var sizes = struct {
	sync.Mutex
	byRoot map[string]measured
}{byRoot: map[string]measured{}}

type measured struct {
	bytes int64
	at    time.Time
}

// RecordSize keeps what a walk of the archive at root found.
func RecordSize(root string, bytes int64, at time.Time) {
	sizes.Lock()
	defer sizes.Unlock()
	sizes.byRoot[filepath.Clean(root)] = measured{bytes, at}
}

// Size is the archive's size in bytes as last measured, and when; false
// before the first measure.
func (a *Archive) Size() (int64, time.Time, bool) {
	if a == nil {
		return 0, time.Time{}, false
	}
	sizes.Lock()
	defer sizes.Unlock()
	m, ok := sizes.byRoot[a.root]
	return m.bytes, m.at, ok
}
