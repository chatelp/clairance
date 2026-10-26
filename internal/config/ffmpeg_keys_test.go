package config

import (
	"reflect"
	"strings"
	"testing"
)

// The keys of ffmpeg, removed with it (docs-fr/35-plan.md, B1 ter): a file
// that still writes them starts, and is told why each does nothing -- once
// per key, though a source's comes once per source.
func TestTheKeysOfFFmpegAreSaidOnceWithTheReason(t *testing.T) {
	atRepoRoot(t)
	text := withLines(minimalConfig, "transcription", `ffmpeg_path = "/opt/homebrew/bin/ffmpeg"`+"\nffmpeg_channels = 1")
	text = withLines(text, "frequencies", `decoder = "go"`+"\nffmpeg_timeout_secs = 0")
	for _, id := range []string{"a", "b"} {
		text += "\n[[frequencies.sources]]\nid = \"" + id + "\"\nairport = \"LFPG\"\nname = \"" + id +
			"\"\nurl = \"http://station.lan/aero-" + id + ".mp3\"\norder = " + map[string]string{"a": "1", "b": "2"}[id] +
			"\nffmpeg_reconnect = false\n"
	}
	cfg := loadAndValidate(t, writeConfig(t, text))
	want := []string{
		ffmpegGone("transcription.ffmpeg_path"), ffmpegGone("transcription.ffmpeg_channels"),
		ffmpegGone("frequencies.decoder"), ffmpegGone("frequencies.ffmpeg_timeout_secs"),
		ffmpegGone("frequencies.sources.ffmpeg_reconnect"),
	}
	if !sameWarnings(cfg.Warnings, want) {
		t.Errorf("warnings:\n  %s\nwant:\n  %s", strings.Join(cfg.Warnings, "\n  "), strings.Join(want, "\n  "))
	}
	if len(cfg.Frequencies.Sources) != 2 {
		t.Errorf("sources: %+v", cfg.Frequencies.Sources)
	}
}

// sameWarnings compares two lists of warnings in any order.
func sameWarnings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, g := range got {
		seen[g]++
	}
	for _, w := range want {
		seen[w]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

// ffmpeg_sample_rate is sample_rate's older name: read when it is the only
// one written, ignored beside the new one, and said either way.
func TestFFmpegSampleRateIsReadAsSampleRate(t *testing.T) {
	atRepoRoot(t)
	for _, c := range []struct {
		lines string
		rate  int
		say   string
	}{
		{"ffmpeg_sample_rate = 22050", 22050,
			"transcription.ffmpeg_sample_rate (22050) is read as transcription.sample_rate: write it so"},
		{"sample_rate = 16000\nffmpeg_sample_rate = 24000", 16000,
			"transcription.ffmpeg_sample_rate (24000) is ignored: sample_rate is written, and is its new name; remove it"},
		{"sample_rate = 16000", 16000, ""},
		{"", Defaults().Transcription.SampleRate, ""},
	} {
		cfg := loadAndValidate(t, writeConfig(t, withLines(minimalConfig, "transcription", c.lines)))
		if cfg.Transcription.SampleRate != c.rate {
			t.Errorf("%q: sample_rate %d, want %d", c.lines, cfg.Transcription.SampleRate, c.rate)
		}
		var want []string
		if c.say != "" {
			want = []string{c.say}
		}
		if !reflect.DeepEqual(cfg.Warnings, want) {
			t.Errorf("%q: warnings %q, want %q", c.lines, cfg.Warnings, want)
		}
	}
}

// A raw PCM source described for ffmpeg keeps its audio: its options are read
// as pcm_rate and pcm_channels, and the file is told how to write it. What
// was not raw PCM -- a sound card -- cannot be read any more, and is said;
// pcm_rate written wins over the options.
func TestTheFFmpegOptionsOfRawPCMAreReadAsPCMRate(t *testing.T) {
	atRepoRoot(t)
	source := func(id, order, rest string) string {
		return "\n[[frequencies.sources]]\nid = \"" + id + "\"\nairport = \"LFPG\"\nname = \"" + id +
			"\"\norder = " + order + "\n" + rest + "\n"
	}
	text := minimalConfig +
		source("sdr", "1", `url = "udp://127.0.0.1:7355"`+"\n"+`ffmpeg_input_options = ["-f", "s16le", "-ar", "48000", "-ac", "2"]`) +
		source("scanner", "2", `url = ":0"`+"\n"+`ffmpeg_input_options = ["-f", "avfoundation"]`) +
		source("both", "3", `url = "udp://127.0.0.1:7356"`+"\npcm_rate = 16000\n"+`ffmpeg_input_options = ["-f", "s16le", "-ar", "48000"]`) +
		source("icecast", "4", `url = "http://station.lan/aero.mp3"`)
	cfg := loadAndValidate(t, writeConfig(t, text))

	byID := map[string]FrequencyConfig{}
	for _, s := range cfg.Frequencies.Sources {
		byID[s.ID] = s
	}
	for id, want := range map[string][2]int{"sdr": {48000, 2}, "scanner": {0, 0}, "both": {16000, 0}, "icecast": {0, 0}} {
		if got := [2]int{byID[id].PCMRate, byID[id].PCMChannels}; got != want {
			t.Errorf("%s: pcm_rate, pcm_channels %v, want %v", id, got, want)
		}
	}
	want := []string{
		`frequencies.sources "sdr": ffmpeg_input_options is read as pcm_rate = 48000, pcm_channels = 2: write it so`,
		`frequencies.sources "scanner": ffmpeg_input_options ["-f" "avfoundation"] is ignored: ffmpeg is no longer used, and only raw 16-bit PCM over UDP is read without it`,
		`frequencies.sources "both": ffmpeg_input_options is ignored: pcm_rate is written; remove it`,
	}
	if !reflect.DeepEqual(cfg.Warnings, want) {
		t.Errorf("warnings:\n  %s\nwant:\n  %s", strings.Join(cfg.Warnings, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestTheFFmpegOptionsOfRawPCM(t *testing.T) {
	for _, c := range []struct {
		opts           []string
		rate, channels int
		ok             bool
	}{
		{[]string{"-f", "s16le", "-ar", "48000", "-ac", "1"}, 48000, 1, true},
		{[]string{"-ar", "22050", "-f", "s16le"}, 22050, 1, true},
		{[]string{"-f", "s16le", "-ar", "48000", "-ac", "2"}, 48000, 2, true},
		{[]string{"-f", "s16le"}, 0, 0, false}, // no rate: nothing says it
		{[]string{"-f", "f32le", "-ar", "48000"}, 0, 0, false},
		{[]string{"-f", "s16le", "-ar", "48000", "-ac", "6"}, 0, 0, false},
		{[]string{"-f", "avfoundation"}, 0, 0, false},
		{[]string{"-re", "-f", "lavfi"}, 0, 0, false},
		{[]string{"-f"}, 0, 0, false},
		{nil, 0, 0, false},
	} {
		rate, channels, ok := pcmFromFFmpegOptions(c.opts)
		if ok != c.ok || rate != c.rate || channels != c.channels {
			t.Errorf("%q: %d, %d, %v; want %d, %d, %v", c.opts, rate, channels, ok, c.rate, c.channels, c.ok)
		}
	}
}
