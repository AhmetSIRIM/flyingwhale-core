package buildinfo

import (
	"runtime/debug"
	"testing"
)

// TestRevisionFromSettings covers the four shapes the toolchain's build
// settings can take by the time they reach a running binary: a clean build
// from a repository, a build from a repository with uncommitted changes, a
// build made outside one, and a stamp that is present but empty.
func TestRevisionFromSettings(t *testing.T) {
	testCases := []struct {
		name         string
		settings     []debug.BuildSetting
		wantRevision string
		wantModified bool
	}{
		{
			name: "a clean repository build reports its commit",
			settings: []debug.BuildSetting{
				{Key: "-trimpath", Value: "true"},
				{Key: "vcs.revision", Value: "6a78b77013904dae42832193c9579c7395bfed3e"},
				{Key: "vcs.modified", Value: "false"},
			},
			wantRevision: "6a78b77013904dae42832193c9579c7395bfed3e",
			wantModified: false,
		},
		{
			name: "uncommitted changes are reported alongside the commit",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "6a78b77013904dae42832193c9579c7395bfed3e"},
				{Key: "vcs.modified", Value: "true"},
			},
			wantRevision: "6a78b77013904dae42832193c9579c7395bfed3e",
			wantModified: true,
		},
		{
			name:         "a build made outside a repository reports an unknown revision",
			settings:     []debug.BuildSetting{{Key: "-trimpath", Value: "true"}},
			wantRevision: unknownRevision,
			wantModified: false,
		},
		{
			name:         "an empty stamp is treated as no stamp at all",
			settings:     []debug.BuildSetting{{Key: "vcs.revision", Value: ""}},
			wantRevision: unknownRevision,
			wantModified: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			revision, modified := RevisionFromSettings(testCase.settings)
			if revision != testCase.wantRevision {
				t.Errorf("revision = %q, want %q", revision, testCase.wantRevision)
			}
			if modified != testCase.wantModified {
				t.Errorf("modified = %t, want %t", modified, testCase.wantModified)
			}
		})
	}
}
