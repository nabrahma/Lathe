// Package frontend embeds the built interface into the binary, so the shipped
// app is a single file with no loose assets beside it.
//
// It lives here rather than in cmd/ because go:embed cannot reach a parent
// directory.
package frontend

import "embed"

// Assets is the compiled interface. Run "npm run build" in this directory, or
// let Wails do it, before building the desktop binary.
//
//go:embed all:dist
var Assets embed.FS
