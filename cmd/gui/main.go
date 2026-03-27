// Package main is the entry point for the Browser History Manager desktop GUI.
// It uses Wails v2 to embed a React frontend in a native window, binding Go
// backend methods (App) so they are callable from JavaScript.
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "dev"

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Browser History Manager",
		Width:     1200,
		Height:    800,
		MinWidth:  860,
		MinHeight: 540,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
