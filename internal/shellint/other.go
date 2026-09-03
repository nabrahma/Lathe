//go:build !windows && !linux

package shellint

// On macOS the context-menu entry ships as a Quick Action inside the app
// bundle, so there is nothing to install at runtime.
type (
	windowsIntegrator struct{ unsupported }
	linuxIntegrator   struct{ unsupported }
)
