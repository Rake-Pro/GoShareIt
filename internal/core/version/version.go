// Package version holds the build-time version stamp. Release builds override
// Version via -ldflags "-X .../internal/core/version.Version=1.2.3".
package version

// Version is the app version ("1.2.3"). Dev builds keep the default; the
// updater never auto-updates an unstamped build (0.0.0 with a suffix); dev
// channel builds are stamped "X.Y.Z-dev.N.SHA7".
var Version = "0.0.0-dev"
