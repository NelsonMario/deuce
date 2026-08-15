// Package version holds the backend's build version, set via -ldflags at
// build time (see Dockerfile). Kept as its own package so nothing else needs
// to import cmd/server just to read it (e.g. from /healthz).
package version

// Version defaults to "dev" for local, unversioned builds. Release builds
// override it: -ldflags "-X deuce/backend/internal/version.Version=v0.1.0".
var Version = "dev"
