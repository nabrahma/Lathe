package main

import (
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/nabrahma/lathe/internal/app"
	"github.com/nabrahma/lathe/internal/version"
)

// buildMenu returns the application menu. macOS gets a real one — a missing
// Edit menu is the first thing a Mac user notices, and Cut/Copy/Paste stop
// working without it. Elsewhere the menu bar would be an unfamiliar addition,
// so there is none.
func buildMenu(backend *app.App) *menu.Menu {
	if runtime.GOOS != "darwin" {
		return nil
	}

	m := menu.NewMenu()
	m.Append(menu.AppMenu())

	edit := m.AddSubmenu("Edit")
	edit.AddText("Undo", keys.CmdOrCtrl("z"), nil)
	edit.AddText("Redo", keys.Combo("z", keys.CmdOrCtrlKey, keys.ShiftKey), nil)
	edit.AddSeparator()
	edit.AddText("Cut", keys.CmdOrCtrl("x"), nil)
	edit.AddText("Copy", keys.CmdOrCtrl("c"), nil)
	edit.AddText("Paste", keys.CmdOrCtrl("v"), nil)
	edit.AddText("Select All", keys.CmdOrCtrl("a"), nil)

	file := m.AddSubmenu("File")
	file.AddText("Open Files…", keys.CmdOrCtrl("o"), func(*menu.CallbackData) {
		backend.RequestOpenFiles()
	})
	file.AddSeparator()
	file.AddText("Settings…", keys.CmdOrCtrl(","), func(*menu.CallbackData) {
		backend.RequestScreen("settings")
	})

	window := m.AddSubmenu("Window")
	window.AddText("Minimise", keys.CmdOrCtrl("m"), func(*menu.CallbackData) {
		wruntime.WindowMinimise(backend.Context())
	})
	window.AddText("Zoom", nil, func(*menu.CallbackData) {
		wruntime.WindowToggleMaximise(backend.Context())
	})

	help := m.AddSubmenu("Help")
	help.AddText("About "+version.Name, nil, func(*menu.CallbackData) {
		backend.RequestScreen("about")
	})
	return m
}
