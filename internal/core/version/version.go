// Package version holds the build-time version stamp. Release builds override
// Version via -ldflags "-X .../internal/core/version.Version=1.2.3".
package version

// Version is the app version ("1.2.3"). Dev builds keep the default; the
// updater treats a "-dev" suffix as "never auto-update".
var Version = "0.0.0-dev"
