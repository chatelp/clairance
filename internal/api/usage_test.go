package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/usage"
)

type usageSeen struct {
	Enabled  bool            `json:"enabled"`
	Days     int             `json:"days"`
	Now      []usage.Present `json:"now"`
	Accounts []usage.Account `json:"accounts"`
}

// eventually polls until ok, a page's and a stream's ends being told from
// their own goroutines.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("not after 5 s: %s", what)
}

func usageLines(t *testing.T, path string) []usage.Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []usage.Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r usage.Record
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// The owner's usage panel (D84), end to end: a viewer's page and stream are
// seen while open, written with why they ended when the owner cuts it off,
// and the stop of the server ends what is left; a viewer reads none of it.
func TestTheOwnerSeesWhatEachAccountDid(t *testing.T) {
	s := newStack(t, stackOptions{goMP3: true, viewer: true, usage: true})
	s.cfg.Server.AudioReplay = config.AudioReplayAll
	if r, _, ok := s.freqs.ULawStream("station"); !ok {
		t.Fatal("no sound decoded")
	} else {
		r.Close()
	}
	owner := s.signIn()

	// The viewer opens a page and listens.
	ami := s.signInAs(viewerUser)
	hdr := http.Header{}
	hdr.Set("Cookie", ami.Name+"="+ami.Value)
	conn, _, err := gorilla.DefaultDialer.Dial(s.wsURL(), hdr)
	if err != nil {
		t.Fatalf("the viewer's WebSocket: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", s.srv.URL+"/api/v1/stream/station?id=u-1&format=ulaw", nil)
	req.AddCookie(ami)
	stream, err := http.DefaultClient.Do(req)
	if err != nil || stream.StatusCode != http.StatusOK {
		t.Fatalf("the viewer's stream: %v %v", err, stream)
	}
	defer stream.Body.Close()
	if n, _ := stream.Body.Read(make([]byte, 64)); n == 0 {
		t.Fatal("no sound on the viewer's stream")
	}

	// A viewer reads none of it.
	if rec := s.do("GET", "/api/v1/usage", ""); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer reading the usage: %d, want 403", rec.Code)
	}

	// The owner sees it, open.
	s.cookie = owner
	var seen usageSeen
	s.get("/api/v1/usage", &seen)
	if !seen.Enabled || seen.Days != 30 {
		t.Errorf("enabled %v, days %d", seen.Enabled, seen.Days)
	}
	var here *usage.Present
	for i := range seen.Now {
		if seen.Now[i].User == viewerUser {
			here = &seen.Now[i]
		}
	}
	if here == nil || here.Pages != 1 || len(here.Listening) != 1 || here.Listening[0].Freq != "station" {
		t.Fatalf("the viewer, now: %+v (all: %+v)", here, seen.Now)
	}

	// Cut off: its page and its stream are written as cut.
	if rec := s.do("POST", "/api/v1/accounts/"+viewerUser+"/cut", ""); rec.Code != http.StatusOK {
		t.Fatalf("cut: %d %s", rec.Code, rec.Body.String())
	}
	go io.Copy(io.Discard, stream.Body)
	path := filepath.Join(filepath.Dir(s.handler.auth.AccountsFile()), usage.File)
	eventually(t, "the viewer's page and stream written as cut", func() bool {
		cut := map[string]bool{}
		for _, r := range usageLines(t, path) {
			if r.User == viewerUser && r.Ended == usage.EndCut {
				cut[r.Kind] = true
				if r.Kind == usage.KindSound && (r.Freq != "station" || r.Bytes == 0) {
					t.Errorf("the stream as written: %+v", r)
				}
			}
		}
		return cut[usage.KindPage] && cut[usage.KindSound]
	})
	s.get("/api/v1/usage", &seen)
	for _, p := range seen.Now {
		if p.User == viewerUser {
			t.Errorf("the viewer still here after the cut: %+v", p)
		}
	}
	var week *usage.Account
	for i := range seen.Accounts {
		if seen.Accounts[i].User == viewerUser {
			week = &seen.Accounts[i]
		}
	}
	if week == nil || len(week.Days) != 7 {
		t.Fatalf("the viewer's week: %+v", seen.Accounts)
	}

	// The owner's own page, open when the server stops: written as stopped.
	ohdr := http.Header{}
	ohdr.Set("Cookie", owner.Name+"="+owner.Value)
	oconn, _, err := gorilla.DefaultDialer.Dial(s.wsURL(), ohdr)
	if err != nil {
		t.Fatalf("the owner's WebSocket: %v", err)
	}
	defer oconn.Close()
	eventually(t, "the owner's page open", func() bool {
		for _, p := range s.handler.usage.Report(time.Local).Now {
			if p.User == testUser && p.Pages == 1 {
				return true
			}
		}
		return false
	})
	s.handler.Stopping()
	stopped := false
	for _, r := range usageLines(t, path) {
		if r.User == testUser && r.Kind == usage.KindPage && r.Ended == usage.EndStopped {
			stopped = true
		}
	}
	if !stopped {
		t.Error("the owner's page is not written as stopped by the stop")
	}
}

// How a page went, in the words of internal/websocket.
func TestHowAPageWent(t *testing.T) {
	for reason, want := range map[string]string{
		"the page closed it (1001)":   usage.EndClosed,
		"the page closed it (1005)":   usage.EndClosed,
		"the page closed it (1000)":   usage.EndClosed,
		"the page closed it (1006)":   usage.EndLost,
		"no answer to pings for 1m0s": usage.EndLost,
		"reading failed: read: reset": usage.EndLost,
		"access cut by the owner":     usage.EndLost, // the cut is marked by the journal first
	} {
		if got := pageEnded(reason); got != want {
			t.Errorf("%q: %s, want %s", reason, got, want)
		}
	}
}
