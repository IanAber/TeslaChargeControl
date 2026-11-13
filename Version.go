package main

import (
	"runtime/debug"
	"time"
)

var startTime = time.Now()

// buildInfo describes the running binary from the version control details Go
// records at build time.
type buildInfo struct {
	Revision  string // full git commit hash, or "unknown"
	Short     string // first 7 characters of Revision
	Time      string // commit time, local
	Modified  bool   // built with uncommitted changes
	GoVersion string
}

func getBuildInfo() buildInfo {
	b := buildInfo{Revision: "unknown", Short: "unknown"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	b.GoVersion = info.GoVersion
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision = s.Value
			b.Short = s.Value
			if len(b.Short) > 7 {
				b.Short = b.Short[:7]
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				b.Time = t.Local().Format("2006-01-02 15:04 MST")
			} else {
				b.Time = s.Value
			}
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}
	return b
}

// String gives a one-line description such as "4be3ce8 (2026-11-13 18:01 EST)".
func (b buildInfo) String() string {
	s := b.Short
	if b.Time != "" {
		s += " (" + b.Time + ")"
	}
	if b.Modified {
		s += " with uncommitted changes"
	}
	return s
}
