package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/audioarchive"
	"github.com/yegors/co-atc/internal/companion"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/pkg/logger"
)

// The launcher's contract (docs-fr/36-contrat-lanceur.md), held by three
// tests. The launcher in production decodes /api/v1/health/detail in one
// piece: a field it requires that disappears, or changes type, loses it the
// whole detail ("Audio not watched"). So:
//
//  1. the schema of the answer, read from the Go types, is frozen in
//     testdata/health_detail.txt -- a field removed, renamed, retyped or made
//     optional fails; a field added fails too, until the file is rewritten
//     with ATC_WRITE_CONTRACT=1 and the change shows in the diff;
//  2. the fields the Swift decoder requires (tools/menubar/Health.swift) are
//     in it, required, of the type it decodes;
//  3. a real answer, from the whole stack, conforms to the frozen schema --
//     every field there with its type, nothing that is not in it.

const frozenContract = "testdata/health_detail.txt"

// degradedQueuedHealth is a sidecar that says all it can: a model missing, a
// queue.
const degradedQueuedHealth = `{"status":"degraded","degraded":["fr"],"pid":4242,"backend":"mlx","queue":{"first":1,"second":0},` +
	`"second_pending":0,"archiving":true,"archiving_stopped_low_disk":false}`

// schemaField is one line of the schema: where, what, and whether it may be
// absent ("optional", from omitempty) or null ("nullable", a pointer without
// it). "required" is always there, and never null, when its parent is.
type schemaField struct {
	Path, Type, Presence string
}

func (f schemaField) String() string { return fmt.Sprintf("%-42s %-8s %s", f.Path, f.Type, f.Presence) }

var timeType = reflect.TypeOf(time.Time{})

// schemaOf reads the schema of t as encoding/json writes it.
func schemaOf(t *testing.T, typ reflect.Type) []schemaField {
	t.Helper()
	var out []schemaField
	var walk func(prefix string, typ reflect.Type)
	walk = func(prefix string, typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" || (!f.IsExported() && !f.Anonymous) {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if f.Anonymous && name == "" {
				walk(prefix, f.Type) // embedded: its fields are the parent's
				continue
			}
			if name == "" {
				name = f.Name
			}
			path := prefix + name
			presence := "required"
			switch {
			case strings.Contains(opts, "omitempty"):
				presence = "optional"
			case f.Type.Kind() == reflect.Pointer:
				presence = "nullable"
			}
			field(&out, path, f.Type, presence, walk)
		}
	}
	walk("", typ)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// field adds path, and what is under it.
func field(out *[]schemaField, path string, typ reflect.Type, presence string, walk func(string, reflect.Type)) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch {
	case typ == timeType:
		*out = append(*out, schemaField{path, "time", presence})
	case typ.Kind() == reflect.Struct:
		*out = append(*out, schemaField{path, "object", presence})
		walk(path+".", typ)
	case typ.Kind() == reflect.Slice:
		*out = append(*out, schemaField{path, "array", presence})
		field(out, path+"[]", typ.Elem(), "required", walk)
	case typ.Kind() == reflect.String:
		*out = append(*out, schemaField{path, "string", presence})
	case typ.Kind() == reflect.Bool:
		*out = append(*out, schemaField{path, "boolean", presence})
	case typ.Kind() >= reflect.Int && typ.Kind() <= reflect.Float64:
		*out = append(*out, schemaField{path, "number", presence})
	default:
		panic(fmt.Sprintf("%s: a %s has no place in the contract's schema", path, typ))
	}
}

func readFrozen(t *testing.T) []schemaField {
	t.Helper()
	b, err := os.ReadFile(frozenContract)
	if err != nil {
		t.Fatal(err)
	}
	var out []schemaField
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 {
			t.Fatalf("%s: %q", frozenContract, line)
		}
		out = append(out, schemaField{f[0], f[1], f[2]})
	}
	return out
}

// 1. The schema of the answer is the frozen one.
func TestTheHealthDetailKeepsItsFrozenSchema(t *testing.T) {
	got := schemaOf(t, reflect.TypeOf(HealthDetail{}))
	if os.Getenv("ATC_WRITE_CONTRACT") == "1" {
		var b strings.Builder
		b.WriteString("# The schema of /api/v1/health/detail, the launcher's contract (docs-fr/36-contrat-lanceur.md).\n")
		b.WriteString("# Written by ATC_WRITE_CONTRACT=1 go test ./internal/api -run FrozenSchema; read by contract_test.go.\n")
		b.WriteString("# A field the launcher decodes is never removed, renamed or retyped: add, do not change.\n")
		for _, f := range got {
			b.WriteString(f.String() + "\n")
		}
		if err := os.WriteFile(frozenContract, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := readFrozen(t)
	frozen := map[string]schemaField{}
	for _, f := range want {
		frozen[f.Path] = f
	}
	now := map[string]schemaField{}
	for _, f := range got {
		now[f.Path] = f
		if w, ok := frozen[f.Path]; !ok {
			t.Errorf("%s is not in %s: a field added? rewrite the file with ATC_WRITE_CONTRACT=1", f.Path, frozenContract)
		} else if w != f {
			t.Errorf("%s changed: frozen %q, now %q -- the launcher decodes it as frozen", f.Path, w.String(), f.String())
		}
	}
	for _, w := range want {
		if _, ok := now[w.Path]; !ok {
			t.Errorf("%s is gone from the answer: the launcher may require it (%s)", w.Path, frozenContract)
		}
	}
}

// The fields tools/menubar/Health.swift declares without "?", and what it
// decodes them as: one missing, null or of another type, and the launcher loses
// the whole detail. Read on 26/10 in the launcher's decoder.
var launcherRequires = map[string]string{
	"pid":                          "number",
	"uptime_s":                     "number",
	"frequencies":                  "array",
	"frequencies[].id":             "string",
	"frequencies[].name":           "string",
	"frequencies[].status":         "string",
	"frequencies[].transcribes":    "boolean",
	"frequencies[].bytes_last_60s": "number",
	"audio_archive":                "object",
	"audio_archive.available":      "boolean",
	"audio_archive.cap_bytes":      "number",
	"databases":                    "object",
	"adsb.ok":                      "boolean",
	"adsb.aircraft":                "number",
	"adsb.messages_per_second":     "number",
	"sidecar.reachable":            "boolean",
	"sidecar.status":               "string",
	"sidecar.queue.first":          "number",
	"sidecar.queue.second":         "number",
}

// And the optional fields it reads, as optional: a type the Swift side
// decodes them as, never to change either.
var launcherReads = map[string]string{
	"adsb":                              "object",
	"adsb.last_fetch":                   "time",
	"adsb.failing_since":                "time",
	"adsb.failing_why":                  "string",
	"frequencies[].last_byte_at":        "time",
	"frequencies[].last_segment_at":     "time",
	"frequencies[].last_transcribed_at": "time",
	"sidecar":                           "object",
	"sidecar.degraded":                  "array",
	"sidecar.pid":                       "number",
	"sidecar.queue":                     "object",
	"audio_archive.bytes":               "number",
	"databases.disk_free_bytes":         "number",
}

// 2. What the launcher's decoder requires is in the frozen schema, required.
func TestTheLauncherDecodersFieldsAreFrozen(t *testing.T) {
	frozen := map[string]schemaField{}
	for _, f := range readFrozen(t) {
		frozen[f.Path] = f
	}
	for path, typ := range launcherRequires {
		if f, ok := frozen[path]; !ok {
			t.Errorf("the launcher requires %s, a %s, always there: absent from %s", path, typ, frozenContract)
		} else if f.Type != typ || f.Presence != "required" {
			t.Errorf("the launcher requires %s, a %s, always there: frozen as %s %s", path, typ, f.Type, f.Presence)
		}
	}
	for path, typ := range launcherReads {
		if f, ok := frozen[path]; !ok {
			t.Errorf("the launcher reads %s as a %s: absent from %s", path, typ, frozenContract)
		} else if f.Type != typ {
			t.Errorf("the launcher reads %s as a %s: frozen as a %s", path, typ, f.Type)
		}
	}
}

// 3. A real answer conforms to the frozen schema. The stack is given all it
// can have here: a sidecar that answers, a frequency with audio decoded in Go,
// a receiver's report with its channels, the process as main describes it, a
// companion running.
func TestTheHealthDetailAnswersItsFrozenSchema(t *testing.T) {
	s := newStack(t, stackOptions{sidecar: healthSaying(t, degradedQueuedHealth), goMP3: true})
	s.cfg.Transcription.AudioArchiveMaxGB = 60
	s.cfg.Transcription.Local.ServerURL = "http://127.0.0.1:8178"
	// An archive the hourly purge has measured.
	archive := t.TempDir()
	s.handler.audioArchive = audioarchive.New(archive)
	audioarchive.RecordSize(archive, 12345, time.Now())
	// The log written to a file.
	if _, err := logger.New(logger.Config{Level: "error", Format: "console", File: filepath.Join(t.TempDir(), "co-atc.log")}); err != nil {
		t.Fatal(err)
	}
	// A binary built from the commit the checkout is at.
	if head, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		s.handler.SetBuild("test", strings.TrimSpace(string(head)))
	}
	s.signIn()
	report := `{"state":"on_air","label":"A l antenne","title":"Selection libre","since":"2026-10-26T18:00:00+01:00",` +
		`"mode":"chaine","detail":"2 channels","channels":[{"id":"124350","designation":"124.350","mhz":124.35,"stream":true}]}`
	if rec := s.do("PUT", "/api/v1/receiver", report); rec.Code != 204 {
		t.Fatalf("the receiver's report: %d %s", rec.Code, rec.Body.String())
	}
	s.cookie = nil
	started := time.Now().Add(-time.Minute)
	s.handler.SetProcess(ProcessInfo{Instance: "production", StartedAt: started, Executable: os.Args[0], WorkingDir: ".",
		Config: s.cfg.ConfigPath})
	s.handler.SetListeners([]Listener{{Addr: "127.0.0.1:8000", URL: "http://127.0.0.1:8000", Primary: true, OK: true},
		{Addr: "127.0.0.1:8001", URL: "http://127.0.0.1:8001", Error: "address already in use"}})
	s.handler.SetReady(started.Add(time.Second))
	if sleep, err := exec.LookPath("sleep"); err == nil && runtime.GOOS != "windows" {
		p, err := companion.Start(context.Background(), companion.Config{Role: "station_link", Path: sleep, Args: []string{"30"}}, s.log)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Stop)
		// And one that died, waiting to be started again.
		dying, err := companion.Start(context.Background(), companion.Config{Role: "keep_awake", Path: "/bin/sh",
			Args: []string{"-c", "echo going; exit 3"}, FirstDelay: time.Hour}, s.log)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(dying.Stop)
		for i := 0; i < 200 && dying.State().State != companion.StateRestarting; i++ {
			time.Sleep(10 * time.Millisecond)
		}
		s.handler.AttachCompanions(p, dying)
	}
	// Some audio, so that a frequency says when its last byte came.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if anyByte(s.freqs.Health()) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	rec := s.do("GET", "/api/v1/health/detail", "", fromLoopback("127.0.0.1:50123", "127.0.0.1:8000"))
	if rec.Code != 200 {
		t.Fatalf("GET /api/v1/health/detail: %d %s", rec.Code, rec.Body.String())
	}
	var answer map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	schema := readFrozen(t)
	byPath := map[string]schemaField{}
	children := map[string][]schemaField{}
	for _, f := range schema {
		byPath[f.Path] = f
		parent := ""
		if i := strings.LastIndex(f.Path, "."); i >= 0 {
			parent = f.Path[:i]
		}
		if strings.HasSuffix(f.Path, "[]") {
			continue // an element, checked with its array
		}
		children[parent] = append(children[parent], f)
	}
	seen := map[string]bool{}
	var check func(path string, f schemaField, v any)
	var object func(path string, obj map[string]any)
	object = func(path string, obj map[string]any) {
		known := map[string]bool{}
		for _, c := range children[path] {
			name := c.Path[strings.LastIndex(c.Path, ".")+1:]
			known[name] = true
			v, present := obj[name]
			switch {
			case !present && c.Presence == "optional":
			case !present:
				t.Errorf("%s is missing (%s)", c.Path, c.Presence)
			case v == nil && c.Presence != "nullable":
				t.Errorf("%s is null (%s)", c.Path, c.Presence)
			case v != nil:
				check(c.Path, c, v)
			default:
				seen[c.Path] = true
			}
		}
		for name := range obj {
			if !known[name] {
				p := name
				if path != "" {
					p = path + "." + name
				}
				t.Errorf("%s is in the answer, not in the frozen schema", p)
			}
		}
	}
	check = func(path string, f schemaField, v any) {
		seen[path] = true
		ok := false
		switch f.Type {
		case "number":
			_, ok = v.(float64)
		case "string":
			_, ok = v.(string)
		case "boolean":
			_, ok = v.(bool)
		case "time":
			if str, isString := v.(string); isString {
				_, err := time.Parse(time.RFC3339Nano, str)
				ok = err == nil && strings.HasSuffix(str, "Z")
			}
		case "object":
			var obj map[string]any
			if obj, ok = v.(map[string]any); ok {
				object(path, obj)
			}
		case "array":
			var arr []any
			if arr, ok = v.([]any); ok {
				elem := byPath[path+"[]"]
				for _, e := range arr {
					if e == nil {
						t.Errorf("%s[] holds a null", path)
						continue
					}
					check(path+"[]", elem, e)
				}
			}
		}
		if !ok {
			t.Errorf("%s: %T %v, want a %s", path, v, v, f.Type)
		}
	}
	object("", answer)

	for path := range launcherRequires {
		if !seen[path] {
			t.Errorf("%s, which the launcher requires, was not in the answer checked", path)
		}
	}
	var unseen []string
	for _, f := range schema {
		if !seen[f.Path] {
			unseen = append(unseen, f.Path)
		}
	}
	t.Logf("%d fields in the schema, %d checked in this answer; not in it here: %s",
		len(schema), len(schema)-len(unseen), strings.Join(unseen, ", "))
}

func anyByte(h []frequencies.FrequencyHealth) bool {
	for _, f := range h {
		if f.LastByteAt != nil {
			return true
		}
	}
	return false
}
