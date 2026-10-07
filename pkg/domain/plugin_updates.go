package domain

import "time"

// PluginRelease describes update metadata needed outside the catalog transport adapter.
type PluginRelease struct {
	// Builtin selects an embedded offline update instead of an HTTP download.
	Builtin bool
	// Version is the compatible plugin version offered to administrators.
	Version string
	// ReleasedAt records when the release was published.
	ReleasedAt time.Time
}

// PluginUpdateNotice describes one newly discovered compatible plugin release.
type PluginUpdateNotice struct {
	// ID is the stable plugin identifier.
	ID string
	// Name is the human-readable plugin name.
	Name string
	// CurrentVersion is the version currently active in Kumbuka.
	CurrentVersion string
	// AvailableVersion is the newer compatible catalog version.
	AvailableVersion string
}
