//go:build desktop

package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"github.com/TerminalAddict/SADPTools/internal/update"
	"github.com/TerminalAddict/SADPTools/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	path, err := app.SettingsPath()
	if err != nil {
		log.Fatal(err)
	}
	desktop := application.New(application.Options{
		Name: "SADPTools", Description: "CCTV discovery and technician toolkit",
		Icon:   appIcon,
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Linux:  application.LinuxOptions{ApplicationID: "io.github.terminaladdict.sadptools"},
	})
	controller := app.New(path, func() { desktop.Event.Emit("inventory:changed") })
	defer controller.Close()
	cache, err := os.UserCacheDir()
	if err != nil {
		log.Fatal(err)
	}
	updates := update.NewManager(version.Value, filepath.Join(cache, "SADPTools", "updates"), func() { desktop.Event.Emit("updates:changed") })
	defer updates.Close()
	desktop.RegisterService(application.NewService(&DesktopService{controller: controller, app: desktop, updates: updates}))
	kit, kitError := toolkit.New(filepath.Join(filepath.Dir(path), "sites.sqlite"), controller, func() { desktop.Event.Emit("toolkit:changed") })
	if kit != nil {
		defer kit.Close()
	}
	previews := streaming.NewManager(nil)
	defer previews.Close()
	liveView := &LiveViewService{manager: previews, kit: kit, app: desktop}
	desktop.RegisterService(application.NewService(liveView))
	desktop.HandleStream("camera-preview", liveView.handle)
	desktop.RegisterService(application.NewService(&ToolkitService{manager: kit, app: desktop, loadError: kitError, previews: liveView}))
	mainWindow := desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "SADPTools", Width: 1440, Height: 880, MinWidth: 960, MinHeight: 640,
		BackgroundColour: application.NewRGB(17, 27, 40), URL: "/",
		Linux: application.LinuxWindow{Icon: appIcon},
	})
	mainWindow.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { liveView.closeWindow() })
	if err = desktop.Run(); err != nil {
		log.Print(err)
	}
}
