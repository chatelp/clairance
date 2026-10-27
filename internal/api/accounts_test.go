package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
)

// accountsSeen is GET /api/v1/accounts, by name.
func accountsSeen(t *testing.T, s *stack) map[string]auth.Account {
	t.Helper()
	var out struct {
		Accounts []auth.Account `json:"accounts"`
		You      string         `json:"you"`
	}
	s.get("/api/v1/accounts", &out)
	if out.You != testUser {
		t.Errorf("you = %q, want %q", out.You, testUser)
	}
	m := map[string]auth.Account{}
	for _, a := range out.Accounts {
		m[a.Name] = a
	}
	return m
}

// The owner's panel (D83), end to end: an account created from it signs in
// at once; cutting it off ends its session, its WebSocket and its stream, and
// a new sign-in still works; a new password retires the old one; removing it
// leaves nothing to sign in as. The password is never in an answer.
func TestTheOwnerManagesAnAccountWhileTheServerRuns(t *testing.T) {
	s := newStack(t, stackOptions{goMP3: true})
	s.cfg.Server.AudioReplay = config.AudioReplayAll
	if r, _, ok := s.freqs.ULawStream("station"); !ok {
		t.Fatal("no sound decoded")
	} else {
		r.Close()
	}
	owner := s.signIn()

	rec := s.do("POST", "/api/v1/accounts", `{"name":"ami","password":"`+testPassword+`","role":"viewer"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), testPassword) || strings.Contains(rec.Body.String(), "argon2") {
		t.Fatal("the answer carries the password or its hash")
	}
	if a := accountsSeen(t, s)["ami"]; a.Role != auth.RoleViewer || a.Source != "file" || a.Sessions != 0 {
		t.Fatalf("the new account as listed: %+v", a)
	}

	// Signed in at once, with a WebSocket and a stream open.
	ami := s.signInAs("ami")
	hdr := http.Header{}
	hdr.Set("Cookie", ami.Name+"="+ami.Value)
	conn, _, err := gorilla.DefaultDialer.Dial(s.wsURL(), hdr)
	if err != nil {
		t.Fatalf("the account's WebSocket: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", s.srv.URL+"/api/v1/stream/station?id=ami-1&format=ulaw", nil)
	req.AddCookie(ami)
	stream, err := http.DefaultClient.Do(req)
	if err != nil || stream.StatusCode != http.StatusOK {
		t.Fatalf("the account's stream: %v %v", err, stream)
	}
	defer stream.Body.Close()
	if n, _ := stream.Body.Read(make([]byte, 64)); n == 0 {
		t.Fatal("no sound on the account's stream")
	}
	s.cookie = owner
	if a := accountsSeen(t, s)["ami"]; a.Sessions != 1 || a.LastSeen == nil || a.LastSignIn == nil {
		t.Errorf("signed in, as listed: %+v", a)
	}

	// Cut off: everything it had open ends, now.
	rec = s.do("POST", "/api/v1/accounts/ami/cut", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("cut: %d %s", rec.Code, rec.Body.String())
	}
	var cut struct {
		Ended map[string]int `json:"ended"`
	}
	json.Unmarshal(rec.Body.Bytes(), &cut)
	if cut.Ended["sessions"] != 1 || cut.Ended["sockets"] != 1 || cut.Ended["streams"] != 1 {
		t.Errorf("ended %v, want one session, one WebSocket, one stream", cut.Ended)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				t.Error("the WebSocket is still open 5 s after the cut")
			}
			break
		}
	}
	done := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, stream.Body); done <- err }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the stream still plays 5 s after the cut")
	}
	s.cookie = ami
	if rec := s.do("GET", "/api/v1/aircraft", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("its session after the cut: %d, want 401", rec.Code)
	}
	s.signInAs("ami") // the password is kept

	// A new password: the old one no longer works, the session it had ends.
	s.cookie = owner
	if rec := s.do("PUT", "/api/v1/accounts/ami/password", `{"password":"another long one"}`); rec.Code != http.StatusOK {
		t.Fatalf("new password: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.do("POST", "/api/v1/auth/login", `{"name":"ami","password":"`+testPassword+`"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("the old password: %d, want 401", rec.Code)
	}
	if rec := s.do("POST", "/api/v1/auth/login", `{"name":"ami","password":"another long one"}`); rec.Code != http.StatusOK {
		t.Errorf("the new password: %d", rec.Code)
	}

	// Removed: nothing to sign in as, and gone from the list and the file.
	s.cookie = owner
	if rec := s.do("DELETE", "/api/v1/accounts/ami", ""); rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.do("POST", "/api/v1/auth/login", `{"name":"ami","password":"another long one"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("sign-in after the removal: %d, want 401", rec.Code)
	}
	if _, ok := accountsSeen(t, s)["ami"]; ok {
		t.Error("still listed after the removal")
	}

	// Every change is in the trace, with who made it, and no password.
	var trace struct {
		SignIns []auth.SignInEvent `json:"signins"`
	}
	s.get("/api/v1/signins", &trace)
	seen := map[string]bool{}
	for _, ev := range trace.SignIns {
		if ev.User == "ami" && ev.By == testUser {
			seen[ev.Event] = true
		}
	}
	for _, ev := range []string{auth.EventAccountCreated, auth.EventAccessCut, auth.EventPasswordChanged, auth.EventAccountRemoved} {
		if !seen[ev] {
			t.Errorf("%s by the owner is not in the trace", ev)
		}
	}
}

// What the panel refuses, and that it is the owner's alone.
func TestThePanelRefusesWhatWouldLockSomeoneOut(t *testing.T) {
	s := newStack(t, stackOptions{viewer: true})
	owner := s.signIn()

	cases := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/api/v1/accounts", `{"name":"x","password":"short","role":"viewer"}`, http.StatusBadRequest},
		{"POST", "/api/v1/accounts", `{"name":"a b","password":"long enough here","role":"viewer"}`, http.StatusBadRequest},
		{"POST", "/api/v1/accounts", `{"name":"x","password":"long enough here","role":"admin"}`, http.StatusBadRequest},
		{"POST", "/api/v1/accounts", `{"name":"GUEST","password":"long enough here","role":"viewer"}`, http.StatusConflict},
		{"DELETE", "/api/v1/accounts/" + testUser, "", http.StatusConflict},        // yourself
		{"POST", "/api/v1/accounts/" + testUser + "/cut", "", http.StatusConflict}, // yourself
		{"DELETE", "/api/v1/accounts/" + viewerUser, "", http.StatusConflict},      // written in config.toml
		{"DELETE", "/api/v1/accounts/nobody", "", http.StatusNotFound},
		{"PUT", "/api/v1/accounts/nobody/password", `{"password":"long enough here"}`, http.StatusNotFound},
	}
	for _, c := range cases {
		if rec := s.do(c.method, c.path, c.body); rec.Code != c.want {
			t.Errorf("%s %s %s: %d %s, want %d", c.method, c.path, c.body, rec.Code, rec.Body.String(), c.want)
		}
	}
	// A config account can still be cut off: its sessions are the server's.
	if rec := s.do("POST", "/api/v1/accounts/"+viewerUser+"/cut", ""); rec.Code != http.StatusOK {
		t.Errorf("cutting off an account of config.toml: %d", rec.Code)
	}

	// A viewer reads nothing of it and changes nothing.
	s.signInAs(viewerUser)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/v1/accounts", ""},
		{"POST", "/api/v1/accounts", `{"name":"x","password":"long enough here","role":"owner"}`},
		{"POST", "/api/v1/accounts/" + testUser + "/cut", ""},
		{"DELETE", "/api/v1/accounts/" + testUser, ""},
	} {
		if rec := s.do(c.method, c.path, c.body); rec.Code != http.StatusForbidden {
			t.Errorf("a viewer, %s %s: %d, want 403", c.method, c.path, rec.Code)
		}
	}
	s.cookie = owner
	if _, ok := accountsSeen(t, s)["x"]; ok {
		t.Error("an account was created by a refused request")
	}
}

// The last owner cannot be removed, even by another owner about to be the
// last: there would be nobody left to manage the accounts.
func TestTheLastOwnerStays(t *testing.T) {
	s := newStack(t, stackOptions{})
	s.signIn()
	if rec := s.do("POST", "/api/v1/accounts", `{"name":"second","password":"`+testPassword+`","role":"owner"}`); rec.Code != http.StatusCreated {
		t.Fatalf("a second owner: %d %s", rec.Code, rec.Body.String())
	}
	s.signInAs("second")
	// The first owner is written in config.toml: refused for that reason.
	if rec := s.do("DELETE", "/api/v1/accounts/"+testUser, ""); rec.Code != http.StatusConflict {
		t.Errorf("removing an owner of config.toml: %d", rec.Code)
	}
	if err := s.handler.auth.RemoveAccount("second", testUser); err != nil {
		t.Fatalf("removing the second owner while another remains: %v", err)
	}
	if rec := s.do("GET", "/api/v1/aircraft", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("the removed owner's session: %d, want 401", rec.Code)
	}
}
