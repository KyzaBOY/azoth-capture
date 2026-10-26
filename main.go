package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"azoth/capture/internal/config"
)

//go:embed all:frontend
var assets embed.FS

// Without Linux options Wails turns the webview's GPU drawing off (Wails
// issue 2977), which makes the window slow to scroll and redraw, above all on
// large screens. The app turns it on; AZOTH_CAPTURE_GPU=off turns it off
// again for a PC where the window then stays blank.
const (
	gpuEnv = "AZOTH_CAPTURE_GPU"
	gpuOff = "off"
)

func linuxOptions() *linux.Options {
	policy := linux.WebviewGpuPolicyOnDemand
	if os.Getenv(gpuEnv) == gpuOff {
		policy = linux.WebviewGpuPolicyNever
	}
	return &linux.Options{WebviewGpuPolicy: policy, ProgramName: "AzothCapture"}
}

func main() {
	set := config.Load()
	app := NewApp(set)

	err := wails.Run(&options.App{
		Title:            "Azoth Capture",
		Width:            normalW,
		Height:           normalH,
		MinWidth:         640,
		MinHeight:        480,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 0x5F, G: 0x6C, B: 0x77, A: 1},
		AlwaysOnTop:      set.AlwaysOnTop,
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "b3a7c1f0-azoth-capture-tools",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if app.ctx != nil {
					runtime.WindowUnminimise(app.ctx)
					runtime.WindowShow(app.ctx)
				}
			},
		},
		Linux: linuxOptions(),
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Light,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
