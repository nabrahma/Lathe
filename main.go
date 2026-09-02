// Command lathe is the desktop application.
//
// It lives at the repository root rather than under cmd/ because Wails v2
// requires the main package to sit beside wails.json and the frontend
// directory. The headless CLI keeps its conventional place in cmd/lathe-cli.
package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/nabrahma/lathe/frontend"
	"github.com/nabrahma/lathe/internal/app"
	"github.com/nabrahma/lathe/internal/version"
)

func main() {
	backend, err := app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lathe:", err)
		os.Exit(1)
	}

	prefs := backend.Settings()

	if err := wails.Run(&options.App{
		Title:  version.Name,
		Width:  prefs.Window.Width,
		Height: prefs.Window.Height,
		// A window smaller than this cannot show the task grid without
		// horizontal scrolling, which the design forbids.
		MinWidth:  860,
		MinHeight: 600,

		AssetServer:      &assetserver.Options{Assets: frontend.Assets},
		BackgroundColour: &options.RGBA{R: 8, G: 8, B: 8, A: 1},

		// Frameless on Windows and macOS, where custom chrome is well-trodden;
		// the window manager's own bar on Linux, where they vary too much to
		// own for a first release. Recorded in docs/DECISIONS.md as D5.
		Frameless: runtime.GOOS != "linux",

		OnStartup:     backend.Startup,
		OnShutdown:    backend.Shutdown,
		OnBeforeClose: backend.BeforeClose,
		Bind:          []any{backend},

		// A second launch focuses the window that is already open rather than
		// starting a duplicate, and routes any file it was given into it.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "dev.lathe.app",
			OnSecondInstanceLaunch: backend.OnSecondInstance,
		},

		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: false,
			About: &mac.AboutInfo{
				Title:   version.Name,
				Message: "Convert, compress and read files — offline.\n" + version.Version,
			},
		},
		Menu: buildMenu(backend),
	}); err != nil {
		fmt.Fprintln(os.Stderr, "lathe:", err)
		os.Exit(1)
	}
}
