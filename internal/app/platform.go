package app

import "runtime"

// hostOS reports the platform to the interface, so it can choose the right
// modifier key and window chrome from a fact rather than from a user-agent
// string.
func hostOS() string { return runtime.GOOS }
