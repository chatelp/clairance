package api

import (
	"net/http"
	"testing"
	"time"
)

// The receiver's state, as its reporter sends it, comes back in /status with
// its age; a report that stopped coming is said stale.
func TestTheReceiverStateIsReportedAndServed(t *testing.T) {
	s := newStack(t, stackOptions{})
	s.signIn()

	var st struct {
		Receiver *struct {
			State      string    `json:"state"`
			Label      string    `json:"label"`
			Title      string    `json:"title"`
			Since      string    `json:"since"`
			ReportedAt time.Time `json:"reported_at"`
			Stale      bool      `json:"stale"`
		} `json:"receiver"`
		ADSB *struct {
			Aircraft int `json:"aircraft"`
		} `json:"adsb"`
		AudioArchive struct {
			Available bool `json:"available"`
		} `json:"audio_archive"`
	}
	s.get("/api/v1/status", &st)
	if st.Receiver != nil || st.ADSB == nil {
		t.Fatalf("before any report: %+v", st)
	}

	report := `{"state":"lent","label":"Exploration SpyServer","title":"Selection libre - 124,350 + 126,425","since":"2026-10-11T09:00:00+02:00","mode":"exploration"}`
	if rec := s.do("PUT", "/api/v1/receiver", report); rec.Code != http.StatusNoContent {
		t.Fatalf("a report: %d %s", rec.Code, rec.Body.String())
	}
	s.get("/api/v1/status", &st)
	if st.Receiver == nil || st.Receiver.State != "lent" || st.Receiver.Label != "Exploration SpyServer" || st.Receiver.Stale {
		t.Fatalf("after a report: %+v", st.Receiver)
	}

	s.handler.status.mu.Lock()
	s.handler.status.reportedAt = time.Now().Add(-2 * time.Minute)
	s.handler.status.mu.Unlock()
	s.get("/api/v1/status", &st)
	if !st.Receiver.Stale {
		t.Error("a report two minutes old is stale")
	}

	for body, want := range map[string]int{
		`{"state":"broken"}`:           http.StatusBadRequest,
		`{"state":"on_air","extra":1}`: http.StatusBadRequest,
		`not json`:                     http.StatusBadRequest,
	} {
		if rec := s.do("PUT", "/api/v1/receiver", body); rec.Code != want {
			t.Errorf("PUT %s: %d, want %d", body, rec.Code, want)
		}
	}
	s.cookie = nil
	if rec := s.do("PUT", "/api/v1/receiver", `{"state":"off"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out: %d, want 401", rec.Code)
	}
}

// The channels on the air, as the station link reports them, come back in the
// launcher's health; a report that would list more than a station has is
// refused.
func TestTheChannelsOnTheAirReachTheHealthDetail(t *testing.T) {
	s := newStack(t, stackOptions{})
	s.signIn()
	report := `{"state":"on_air","title":"Selection libre","mode":"chaine","channels":[` +
		`{"id":"124350","designation":"124.350","mhz":124.35,"stream":true},` +
		`{"id":"121500","designation":"121.500","mhz":121.5,"stream":false}]}`
	if rec := s.do("PUT", "/api/v1/receiver", report); rec.Code != http.StatusNoContent {
		t.Fatalf("a report with channels: %d %s", rec.Code, rec.Body.String())
	}
	var d HealthDetail
	decode(t, s.do("GET", "/api/v1/health/detail", ""), &d)
	if d.Receiver == nil || len(d.Receiver.Channels) != 2 {
		t.Fatalf("receiver: %+v", d.Receiver)
	}
	if c := d.Receiver.Channels[0]; c.ID != "124350" || c.Designation != "124.350" || c.MHz != 124.35 || !c.Stream {
		t.Errorf("first channel: %+v", c)
	}
	if d.Receiver.Channels[1].Stream {
		t.Error("a channel without a stream said to have one")
	}

	many := `{"state":"on_air","channels":[`
	for i := 0; i <= maxReceiverChannels; i++ {
		if i > 0 {
			many += ","
		}
		many += `{"id":"c","stream":true}`
	}
	many += `]}`
	if rec := s.do("PUT", "/api/v1/receiver", many); rec.Code != http.StatusBadRequest {
		t.Errorf("%d channels: %d, want 400", maxReceiverChannels+1, rec.Code)
	}
}
