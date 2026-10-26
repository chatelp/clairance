package transcription

// The paired test of the two decoders (docs-fr/35-plan.md, B1; Q60): the same
// hour of a station's stream, decoded by ffmpeg and by the Go path, cut into
// transmissions as production cuts them, each transmission transcribed by a
// sidecar. Two decoders that hear the same thing cut the same transmissions
// and get the same texts.
//
//	PAIRED_DIR=runs/b1-decodage/heure STT_URL=http://127.0.0.1:8179 \
//	  go test ./internal/transcription -run TestPairedDecoding -timeout 3h -v
//
// PAIRED_DIR holds <frequency>.ffmpeg.s16le and <frequency>.go.s16le, 16-bit
// mono at 24 kHz; the report is written there (paired-report.json). Skipped
// without them; the audio stays out of the repository. The sidecar must not
// archive (no --save-audio): these are not production's transmissions.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
)

const pairedRate = 24000

type pairedCut struct {
	From, To int    // 20 ms frames from the start of the recording
	Text     string `json:",omitempty"`
	Rejected string `json:",omitempty"`
	pcm      []byte
}

// alignTo shifts b so that it starts where a does, to the sample: the two
// decoders do not start a stream at the same instant (the Go one skips the
// frames whose bit reservoir began before the recording), and a shift of a
// fraction of a 20 ms frame would put the two cuttings on different grids.
func alignTo(a, b []byte) []byte {
	s16 := func(x []byte, i int) float64 { return float64(int16(uint16(x[2*i]) | uint16(x[2*i+1])<<8)) }
	n, m := len(a)/2, len(b)/2
	win := pairedRate // the loudest second of a
	from, loud := 0, -1.0
	for st := 0; st+win <= n; st += win / 2 {
		var e float64
		for i := st; i < st+win; i += 4 {
			e += s16(a, i) * s16(a, i)
		}
		if e > loud {
			from, loud = st, e
		}
	}
	lag, top := 0, -1e300
	for l := -pairedRate / 2; l <= pairedRate/2; l++ {
		var c float64
		for i := from; i < from+win && i < n; i++ {
			if j := i + l; j >= 0 && j < m {
				c += s16(a, i) * s16(b, j)
			}
		}
		if c > top {
			lag, top = l, c
		}
	}
	if lag >= 0 {
		return b[2*lag:]
	}
	return append(make([]byte, -2*lag), b...)
}

func cutTransmissions(t *testing.T, b []byte) []pairedCut {
	t.Helper()
	frame := pairedRate / 50 * 2
	seg := newSegmenter(pairedRate, LocalSTTConfig{SegmentSilenceMs: 600, SegmentMinMs: 400,
		SegmentMaxSeconds: 30, SegmentPrerollMs: 200, SilenceThreshold: 0.005})
	var cuts []pairedCut
	for i := 0; (i+1)*frame <= len(b); i++ {
		if pcm := seg.push(b[i*frame : (i+1)*frame]); pcm != nil {
			n := len(pcm) / frame
			cuts = append(cuts, pairedCut{From: i + 1 - n, To: i + 1, pcm: pcm})
		}
	}
	return cuts
}

func transcribeCut(t *testing.T, url string, c *pairedCut) {
	t.Helper()
	req, _ := http.NewRequest("POST", url+"/transcribe", bytes.NewReader(c.pcm))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Sample-Rate", strconv.Itoa(pairedRate))
	req.Header.Set("X-Channels", "1")
	req.Header.Set("X-Language", "en")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sidecar: %d %s", resp.StatusCode, body)
	}
	var out struct {
		Text     string `json:"text"`
		Rejected string `json:"rejected"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	c.Text, c.Rejected = out.Text, out.Rejected
}

// normalised compares texts as words: case and punctuation do not count.
func normalised(s string) string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return strings.Join(f, " ")
}

func TestPairedDecoding(t *testing.T) {
	// STT_URL=none cuts without transcribing, to check the boundaries alone.
	dir, url := os.Getenv("PAIRED_DIR"), os.Getenv("STT_URL")
	if dir == "" || url == "" {
		t.Skip("PAIRED_DIR and STT_URL not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.ffmpeg.s16le"))
	sort.Strings(files)
	type pair struct {
		FFmpeg, Go *pairedCut
		Same       string // "text", "words", "differs", "boundaries"
	}
	report := map[string]any{}
	for _, ff := range files {
		freq := strings.TrimSuffix(filepath.Base(ff), ".ffmpeg.s16le")
		rawA, err := os.ReadFile(ff)
		if err != nil {
			t.Fatal(err)
		}
		rawB, err := os.ReadFile(filepath.Join(dir, freq+".go.s16le"))
		if err != nil {
			t.Fatal(err)
		}
		a := cutTransmissions(t, rawA)
		b := cutTransmissions(t, alignTo(rawA, rawB))
		// Pair by overlap: each ffmpeg transmission with the Go one that
		// overlaps it most, more than half of the shorter.
		var pairs []pair
		usedB := map[int]bool{}
		unpairedA := 0
		for i := range a {
			best, over := -1, 0
			for j := range b {
				o := min(a[i].To, b[j].To) - max(a[i].From, b[j].From)
				if o > over {
					best, over = j, o
				}
			}
			shorter := a[i].To - a[i].From
			if best >= 0 && b[best].To-b[best].From < shorter {
				shorter = b[best].To - b[best].From
			}
			if best < 0 || usedB[best] || 2*over < shorter {
				unpairedA++
				continue
			}
			usedB[best] = true
			pairs = append(pairs, pair{FFmpeg: &a[i], Go: &b[best]})
		}
		unpairedB := len(b) - len(usedB)
		counts := map[string]int{}
		for k := range pairs {
			p := &pairs[k]
			if url != "none" {
				transcribeCut(t, url, p.FFmpeg)
				transcribeCut(t, url, p.Go)
			}
			switch {
			case p.FFmpeg.From != p.Go.From || p.FFmpeg.To != p.Go.To:
				p.Same = "boundaries"
			case p.FFmpeg.Text == p.Go.Text && p.FFmpeg.Rejected == p.Go.Rejected:
				p.Same = "text"
			case normalised(p.FFmpeg.Text) == normalised(p.Go.Text) && p.FFmpeg.Rejected == p.Go.Rejected:
				p.Same = "words"
			default:
				p.Same = "differs"
			}
			counts[p.Same]++
		}
		var differs []pair
		for _, p := range pairs {
			if p.Same != "text" {
				differs = append(differs, p)
			}
		}
		report[freq] = map[string]any{"ffmpeg_transmissions": len(a), "go_transmissions": len(b),
			"paired": len(pairs), "ffmpeg_only": unpairedA, "go_only": unpairedB, "counts": counts, "not_identical": differs}
		t.Logf("%s: %d transmissions by ffmpeg, %d in Go; %d paired (%d ffmpeg only, %d Go only); same boundaries and text %d, same words %d, other text %d, other boundaries %d",
			freq, len(a), len(b), len(pairs), unpairedA, unpairedB, counts["text"], counts["words"], counts["differs"], counts["boundaries"])
	}
	out, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "paired-report.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(out[:min(len(out), 2000)]))
}
