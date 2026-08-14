// Package buildinfo reports the version control revision that the Go
// toolchain stamps into a binary at build time.
package buildinfo

import "runtime/debug"

const unknownRevision = "unknown"

// RevisionFromSettings reads the commit the toolchain stamps into a binary
// built inside a repository, and whether that tree carried uncommitted
// changes. A build made outside a repository carries no stamp, which is
// reported as an unknown revision so the log line always has a value.
func RevisionFromSettings(settings []debug.BuildSetting) (revision string, modified bool) {
	revision = unknownRevision
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			if setting.Value != "" {
				revision = setting.Value
			}
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

// RunningBuild reports the stamp of the binary that is executing.
func RunningBuild() (revision string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownRevision, false
	}
	return RevisionFromSettings(info.Settings)
}
