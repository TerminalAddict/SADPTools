//go:build desktop

package main

import (
	"embed"
	"log"

	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	path, err := app.SettingsPath()
	if err != nil {
		log.Fatal(err)
	}
	desktop := application.New(application.Options{
		Name: "SADPTools", Description: "Hikvision discovery and device management",
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	controller := app.New(path, func() { desktop.Event.Emit("inventory:changed") })
	defer controller.Close()
	desktop.RegisterService(application.NewService(&DesktopService{controller: controller, app: desktop}))
	desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "SADPTools", Width: 1440, Height: 880, MinWidth: 960, MinHeight: 640,
		BackgroundColour: application.NewRGB(20, 25, 34), URL: "/",
	})
	if err = desktop.Run(); err != nil {
		log.Print(err)
	}
}
