package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// rotatingFile is a log file that closes itself at midnight and when it grows
// past a ceiling, and that deletes its own old copies.
//
// Upstream logs to stdout only, so the file it ends up in -- and whether that
// file ever stops growing -- is the operator's problem. Measured here: a single
// server writes 0.7 MB an hour, 17 MB a day, which is nothing next to the
// database. The ceiling is not for that case. It is for the one that happened:
// a supervision bug started seventeen servers and they wrote 103 MB into one
// file in a night, and nothing anywhere would have stopped them.
//
// Three bounds, because they protect against different things. One file per day
// keeps the log readable -- "what happened on the night of the 20th?" is a
// question you ask of a file, not of a stream. A size ceiling catches the day
// that goes mad. A file count stops the whole thing growing without end.
//
// No dependency: this is eighty lines, and the alternative was pulling in a
// package to do them.
type rotatingFile struct {
	dir      string
	base     string // file name without the date or the extension
	ext      string
	maxBytes int64
	maxFiles int

	mu      sync.Mutex
	file    *os.File
	written int64
	day     string
	seq     int
}

// newRotatingFile opens today's log file, creating the directory if needed.
func newRotatingFile(path string, maxSizeMB, maxFiles int) (*rotatingFile, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory %s: %w", dir, err)
	}
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	if ext == "" {
		ext = ".log"
	}
	r := &rotatingFile{
		dir:      dir,
		base:     strings.TrimSuffix(name, filepath.Ext(name)),
		ext:      ext,
		maxBytes: int64(maxSizeMB) * 1024 * 1024,
		maxFiles: maxFiles,
	}
	if r.maxBytes <= 0 {
		r.maxBytes = 100 * 1024 * 1024
	}
	if r.maxFiles <= 0 {
		r.maxFiles = 7
	}
	if err := r.open(time.Now()); err != nil {
		return nil, err
	}
	return r, nil
}

// current is the path of the file being written.
func (r *rotatingFile) current() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return ""
	}
	return r.file.Name()
}

func (r *rotatingFile) name(day string, seq int) string {
	if seq == 0 {
		return filepath.Join(r.dir, fmt.Sprintf("%s-%s%s", r.base, day, r.ext))
	}
	return filepath.Join(r.dir, fmt.Sprintf("%s-%s.%d%s", r.base, day, seq, r.ext))
}

// open switches to the file for `at`, picking the first sequence number that is
// not already full. Reopening an existing file appends to it: a restart in the
// middle of a day continues the day's log rather than truncating it.
func (r *rotatingFile) open(at time.Time) error {
	day := at.Format("2006-01-02")
	seq := 0
	var f *os.File
	var size int64
	for {
		p := r.name(day, seq)
		info, err := os.Stat(p)
		if err == nil && info.Size() >= r.maxBytes {
			seq++
			continue
		}
		f, err = os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("open log file %s: %w", p, err)
		}
		if info != nil {
			size = info.Size()
		}
		break
	}
	if r.file != nil {
		r.file.Close()
	}
	r.file, r.written, r.day, r.seq = f, size, day, seq
	r.prune()
	return nil
}

// prune keeps the most recent maxFiles log files and removes the rest. Sorting
// by name works because the name carries the date first and the sequence after.
func (r *rotatingFile) prune() {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	var ours []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, r.base+"-") || !strings.HasSuffix(n, r.ext) {
			continue
		}
		ours = append(ours, n)
	}
	if len(ours) <= r.maxFiles {
		return
	}
	sort.Strings(ours)
	for _, n := range ours[:len(ours)-r.maxFiles] {
		os.Remove(filepath.Join(r.dir, n))
	}
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if day := time.Now().Format("2006-01-02"); day != r.day {
		if err := r.open(time.Now()); err != nil {
			// A failed rotation must not lose the line: keep writing where we are.
			fmt.Fprintf(os.Stderr, "log rotation failed, still writing to %s: %v\n",
				r.name(r.day, r.seq), err)
		}
	} else if r.written+int64(len(p)) > r.maxBytes {
		r.seq++
		next := r.name(r.day, r.seq)
		f, err := os.OpenFile(next, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "log rotation failed, still writing to %s: %v\n",
				r.name(r.day, r.seq-1), err)
		} else {
			r.file.Close()
			r.file, r.written = f, 0
			r.prune()
		}
	}

	n, err := r.file.Write(p)
	r.written += int64(n)
	return n, err
}

func (r *rotatingFile) Sync() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.file.Sync()
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}
