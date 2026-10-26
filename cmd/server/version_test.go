package main

import (
	"runtime/debug"
	"testing"
)

// The commit a plain go build records: short, marked when the tree had
// changes, and nothing when Go recorded none.
func TestTheBinarySaysWhichCommitItWasBuiltFrom(t *testing.T) {
	info := func(settings ...debug.BuildSetting) *debug.BuildInfo { return &debug.BuildInfo{Settings: settings} }
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "134e4fb4b98d91fcdce1a74842f3e02a9c50fcc2"}
	for _, c := range []struct {
		name string
		in   *debug.BuildInfo
		want string
	}{
		{"clean", info(rev, debug.BuildSetting{Key: "vcs.modified", Value: "false"}), "134e4fb"},
		{"changed tree", info(rev, debug.BuildSetting{Key: "vcs.modified", Value: "true"}), "134e4fb-dirty"},
		{"no vcs", info(), ""},
		{"no build info", nil, ""},
	} {
		if got := vcsCommit(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
