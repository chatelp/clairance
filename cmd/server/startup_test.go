//go:build !windows

// Not built on Windows: these tests start main() in a process group of its own,
// stop it with SIGTERM and kill the group, none of which Windows has. The
// helpers below serve only this file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The server as a newcomer starts it: main() itself, on the example
// configuration, in a process of its own -- a refusal to start is an exit, and
// only a child can be watched making one. The test binary is that child when
// asked through the environment.
const runMainEnv = "CLAIRANCE_TEST_RUN_MAIN"

// argsEnv carries more arguments for main(), one per line.
const argsEnv = "CLAIRANCE_TEST_MAIN_ARGS"

func TestMain(m *testing.M) {
	if cfgPath := os.Getenv(runMainEnv); cfgPath != "" {
		os.Args = []string{"co-atc", "-config", cfgPath}
		if more := os.Getenv(argsEnv); more != "" {
			os.Args = append(os.Args, strings.Split(more, "\n")...)
		}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// freePort is a port nothing listens on now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// exampleConfig is configs/config.toml.example with what cannot leave the
// test replaced: the receiver is a fake tar1090, the streams, the weather and
// the sidecar point at local addresses, the database and the log stay in dir.
// transcribe says whether the first source keeps transcribe_audio = true;
// every other source is set to false. It returns the file and the port the
// server listens on.
func exampleConfig(t *testing.T, dir string, transcribe bool) (string, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "configs", "config.toml.example"))
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/aircraft.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"now":%d,"messages":1,"aircraft":[]}`, time.Now().Unix())
	})
	mux.HandleFunc("/receiver.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"version":"test","lat":43.68,"lon":-79.62}`)
	})
	mux.HandleFunc("/stats.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"total":{"messages":1}}`)
	})
	tar := httptest.NewServer(mux)
	t.Cleanup(tar.Close)
	gone := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(gone.Close)

	port := freePort(t)
	cfg := string(raw)
	replace := func(pattern, with string, want int) {
		t.Helper()
		re := regexp.MustCompile(`(?m)` + pattern)
		if n := len(re.FindAllStringIndex(cfg, -1)); n != want {
			t.Fatalf("the example configuration changed: %q found %d times, want %d", pattern, n, want)
		}
		cfg = re.ReplaceAllLiteralString(cfg, with)
	}
	replace(`^port = .*$`, fmt.Sprintf("port = %d", port), 1)
	replace(`^additional_ports = .*$`, fmt.Sprintf("additional_ports = [%d]", freePort(t)), 1)
	replace(`^source_type = .*$`, `source_type = "tar1090"`, 1)
	replace(`^tar1090_base_url = .*$`, fmt.Sprintf("tar1090_base_url = %q", tar.URL+"/"), 1)
	replace(`^url = .*$`, fmt.Sprintf("url = %q", gone.URL+"/stream.mp3"), 4)
	replace(`^transcribe_audio = true$`, "transcribe_audio = false", 2)
	replace(`^log_dir = .*$`, `log_dir = ""`, 1)
	replace(`^sqlite_base_path = .*$`, fmt.Sprintf("sqlite_base_path = %q", filepath.Join(dir, "data")), 1)
	replace(`^file = .*$`, `file = ""`, 1)
	replace(`^fetch_metar = true$`, fmt.Sprintf("api_base_url = %q\nfetch_metar = true", gone.URL), 1)
	// Nothing answers here, so the server cannot have leaned on a sidecar
	// running on this machine; one second is enough to find nothing.
	replace(`^command = \[\].*$`, fmt.Sprintf("command = []\nserver_url = %q\nstartup_timeout_seconds = 1",
		fmt.Sprintf("http://127.0.0.1:%d", freePort(t))), 1)
	if transcribe {
		cfg = strings.Replace(cfg, "transcribe_audio = false", "transcribe_audio = true", 1)
	}

	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, port
}

// syncBuffer is the child's output, read while it is written.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// child is main() running in a process of its own. exited is closed once it
// has ended, and err then says how.
type child struct {
	cmd    *exec.Cmd
	out    *syncBuffer
	exited chan struct{}
	err    error
}

// runServer starts main() on cfgPath in a child process, with args after
// -config.
func runServer(t *testing.T, cfgPath string, args ...string) *child {
	t.Helper()
	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^$")
	// From the root of the repository, where the server is run: the
	// configuration is checked against www/ there.
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), runMainEnv+"="+cfgPath, argsEnv+"="+strings.Join(args, "\n"))
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	// Its own process group, so the ffmpeg it starts goes with it; and a bound
	// on Wait, which otherwise waits for every holder of the output pipe.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &child{cmd: cmd, out: out, exited: make(chan struct{})}
	go func() {
		c.err = cmd.Wait()
		close(c.exited)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-c.exited
	})
	return c
}

// Nothing transcribes and nothing starts a sidecar: the server starts without
// one and answers.
func TestTheExampleStartsWithoutTranscription(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed: the frequencies service needs it")
	}
	cfgPath, port := exampleConfig(t, t.TempDir(), false)
	c := runServer(t, cfgPath)

	url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", port)
	deadline := time.Now().Add(60 * time.Second)
	for {
		select {
		case <-c.exited:
			t.Fatalf("the server stopped (%v):\n%s", c.err, c.out.String())
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		cancel()
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no answer at %s:\n%s", url, c.out.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.Contains(c.out.String(), "No source transcribes and no sidecar is configured: running without transcription") {
		t.Errorf("the server did not say it runs without transcription:\n%s", c.out.String())
	}

	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-c.exited:
		if c.err != nil {
			t.Errorf("stopping on SIGTERM: %v\n%s", c.err, c.out.String())
		}
	case <-time.After(30 * time.Second):
		t.Errorf("the server did not stop on SIGTERM:\n%s", c.out.String())
	}
}

// A source transcribes and nothing answers: the server refuses to start, with
// the message it has always given.
func TestTheExampleWithATranscribingSourceNeedsItsSidecar(t *testing.T) {
	cfgPath, _ := exampleConfig(t, t.TempDir(), true)
	c := runServer(t, cfgPath)
	select {
	case <-c.exited:
		if c.err == nil {
			t.Fatalf("the server exited cleanly, want a refusal:\n%s", c.out.String())
		}
	case <-time.After(60 * time.Second):
		t.Fatalf("the server is still running with no sidecar:\n%s", c.out.String())
	}
	for _, want := range []string{"Local transcription sidecar unavailable", "no local STT sidecar answering at"} {
		if !strings.Contains(c.out.String(), want) {
			t.Errorf("the refusal should say %q:\n%s", want, c.out.String())
		}
	}
}
