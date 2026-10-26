package transcription

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/websocket"
)

// The client of this package against a sidecar that really transcribes, not a
// stand-in: its health as Start reads it, then one transmission sent as a
// frequency sends it, and stored. CLAIRANCE_SIDECAR_URL names the sidecar and
// CLAIRANCE_SIDECAR_PCM a transmission of speech (s16le, mono, 24 kHz); skipped
// without them. With CLAIRANCE_SIDECAR_EXPECT_SECOND=1 the second reading must
// join the transmission too. sidecar/test_contract.py starts a sidecar whose
// French gate always opens and runs this test against it.
func TestAgainstARealSidecar(t *testing.T) {
	url, pcmPath := os.Getenv("CLAIRANCE_SIDECAR_URL"), os.Getenv("CLAIRANCE_SIDECAR_PCM")
	wantSecond := os.Getenv("CLAIRANCE_SIDECAR_EXPECT_SECOND") == "1"
	if url == "" || pcmPath == "" {
		t.Skip("CLAIRANCE_SIDECAR_URL and CLAIRANCE_SIDECAR_PCM not set (sidecar/test_contract.py sets them)")
	}
	pcm, err := os.ReadFile(pcmPath)
	if err != nil {
		t.Fatal(err)
	}
	log := testLogger(t)

	s := NewSidecar(SidecarConfig{ServerURL: url}, log)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := s.LastHealth()
	if h.Status != "ok" && h.Status != "degraded" {
		t.Errorf("status %q", h.Status)
	}
	if h.PID <= 0 || h.Backend == "" || len(h.Backend) > 48 || h.ModelEN == "" || h.Models["en"].ID != h.ModelEN {
		t.Errorf("health %+v", h)
	}

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ws := websocket.NewServer(log)
	go ws.Run()
	p := &LocalProcessor{
		frequencyID: "124350",
		language:    "en",
		config:      Config{Local: LocalSTTConfig{ServerURL: url}},
		client:      &http.Client{Timeout: 300 * time.Second},
		ctx:         context.Background(),
		logger:      log,
		sink: &transcriptionSink{frequencyID: "124350", storage: sqlite.NewTranscriptionStorage(db, log),
			wsServer: ws, logger: log},
	}
	p.send(pcm, 24000, time.Now())

	var first, second string
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		_ = db.QueryRow(`SELECT content, COALESCE(content_second, '') FROM transcriptions LIMIT 1`).Scan(&first, &second)
		if first != "" && (second != "" || !wantSecond) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if first == "" {
		t.Fatal("no transcription stored")
	}
	if wantSecond && second == "" {
		t.Errorf("no second reading stored (first %q)", first)
	}
	t.Logf("first %q, second %q", first, second)
}
