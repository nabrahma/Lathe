// Package version holds build-stamped identity for the app and CLI.
package version

// Version is set at build time via -ldflags.
var Version = "dev"

// Name is the product name used in window titles, CLI help and user agents.
const Name = "Lathe"
