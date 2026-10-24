package adsb

import (
	"sort"
	"strings"
	"testing"
)

// keyPaths notes every key of a JSON object, and those of the objects that
// carry the aircraft (data, aircraft, adsb, delta, timing), as dotted paths; arrays
// and the other objects are leaves.
func keyPaths(prefix string, v any, into map[string]bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	for k, x := range obj {
		into[prefix+k] = true
		switch k {
		case "data", "aircraft", "adsb", "delta", "timing":
			keyPaths(prefix+k+".", x, into)
		}
	}
}

const aircraftForm = `aircraft.adsb aircraft.adsb.alert aircraft.adsb.alt_baro aircraft.adsb.alt_geom
aircraft.adsb.atc_derived aircraft.adsb.baro_rate aircraft.adsb.category aircraft.adsb.flight
aircraft.adsb.geom_rate aircraft.adsb.gs aircraft.adsb.gva aircraft.adsb.hex aircraft.adsb.ias
aircraft.adsb.lat aircraft.adsb.lon aircraft.adsb.mach aircraft.adsb.mag_heading aircraft.adsb.messages
aircraft.adsb.mlat aircraft.adsb.nac_p aircraft.adsb.nac_v aircraft.adsb.nav_altitude_fms
aircraft.adsb.nav_altitude_mcp aircraft.adsb.nav_heading aircraft.adsb.nav_qnh aircraft.adsb.nic
aircraft.adsb.nic_baro aircraft.adsb.oat aircraft.adsb.r_dir aircraft.adsb.r_dst aircraft.adsb.rc
aircraft.adsb.roll aircraft.adsb.rssi aircraft.adsb.sda aircraft.adsb.seen aircraft.adsb.seen_pos
aircraft.adsb.sil aircraft.adsb.sil_type aircraft.adsb.source_type aircraft.adsb.spi aircraft.adsb.squawk
aircraft.adsb.tas aircraft.adsb.tat aircraft.adsb.tisb aircraft.adsb.track aircraft.adsb.track_rate
aircraft.adsb.true_heading aircraft.adsb.type aircraft.adsb.version aircraft.adsb.wd aircraft.adsb.ws
aircraft.airline aircraft.created_at aircraft.date_landed aircraft.date_tookoff aircraft.flight
aircraft.future aircraft.hex aircraft.hindcast aircraft.last_seen aircraft.on_ground aircraft.phase
aircraft.status`

// The messages the page animates from keep their form, key by key, as the
// replay bench records them (fiche L, step 3): today's page must keep working
// with yesterday's server and the other way round. A key added, removed or
// renamed fails here; change the form on purpose, both ends together.
func TestMotionMessagesKeepTheirForm(t *testing.T) {
	rec := recordReplay(t)
	data := func(form string) string {
		var out []string
		for _, k := range strings.Fields(form) {
			out = append(out, "data."+k)
		}
		return strings.Join(out, " ")
	}
	want := map[string]string{
		"aircraft_predicted_state": `data data.based_on data.delta data.delta.alt_baro data.delta.confidence
			data.delta.gs data.delta.lat data.delta.lon data.delta.source data.delta.track data.hex
			data.predicted data.predicted_at type`,
		// An aircraft is added on its first poll, before any hindcast.
		"aircraft_added": "data data.aircraft data.hex data.observed_at data.type type " +
			strings.Replace(data(aircraftForm), "data.aircraft.hindcast ", "", 1),
		// The delta and the two figures the page dates the position by; the
		// whole aircraft no longer rides along (16/10).
		"aircraft_update": `data data.delta data.delta.alt_baro data.delta.atc_derived
			data.delta.gs data.delta.lat data.delta.lon data.delta.mag_heading data.delta.phase
			data.delta.track data.delta.true_heading data.hex data.observed_at data.timing
			data.timing.seen data.timing.seen_pos data.type type`,
	}
	for typ, form := range want {
		var got []string
		for k := range rec.Forms[typ] {
			got = append(got, k)
		}
		sort.Strings(got)
		w := strings.Fields(form)
		sort.Strings(w)
		if strings.Join(got, " ") != strings.Join(w, " ") {
			t.Errorf("%s carries\n  %s\nwant\n  %s", typ, strings.Join(got, " "), strings.Join(w, " "))
		}
	}
}
