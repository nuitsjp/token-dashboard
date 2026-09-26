package main

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"wailstemplate/internal/appstate"
	"wailstemplate/internal/desktop"
	"wailstemplate/internal/diagnostics"
	"wailstemplate/internal/fault"
	"wailstemplate/internal/notes"
	"wailstemplate/internal/updates"
)

//go:embed all:frontend/dist
var webAssets embed.FS

//go:embed build/app.json
var configJSON []byte

type appConfig struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Executable      string `json:"executable"`
	Version         string `json:"version"`
	UpdateSource    string `json:"updateSource"`
	UpdatePublicKey string `json:"updatePublicKey"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "起動できません:", err)
		showStartupFailure()
		os.Exit(1)
	}
}
func run() error {
	var cfg appConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return err
	}
	if cfg.ID == "" || cfg.Name == "" {
		return fmt.Errorf("build/app.json: id and name are required")
	}
	if _, err := updates.CompareVersion(cfg.Version, "0.0.0"); err != nil {
		return err
	}
	dir := os.Getenv("WAILS_DATA_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(base, cfg.ID)
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("WAILS_DATA_DIR must be absolute")
	}
	logger, logs, err := diagnostics.Open(filepath.Join(dir, "logs"), production)
	diagnosticsAvailable := err == nil
	if err != nil {
		// Diagnostics must not prevent useful operation. This fallback is
		// explicit and visible; it never substitutes business data.
		logger = slog.Default()
		logger.Warn("診断ログを保存できません。標準エラー出力を使用します。", "cause", err)
	} else {
		defer logs.Close()
	}
	logger.Info("starting", "version", cfg.Version, "os", runtime.GOOS, "arch", runtime.GOARCH, "server", serverMode)
	root, err := fs.Sub(webAssets, "frontend/dist")
	if err != nil {
		return err
	}
	port, err := serverPort()
	if err != nil {
		return err
	}
	state := &appstate.State{}
	var app *application.App
	var window *application.WebviewWindow
	emit := func(name string, data any) {
		if app != nil {
			app.Event.Emit(name, data)
		}
	}
	controls := &desktop.Controls{Emit: emit}
	noteService, err := notes.New(dir, logger, state, emit)
	if err != nil {
		return err
	}
	// Also clean up if application construction or startup fails.
	defer noteService.ServiceShutdown()
	info := desktop.Info{Name: cfg.Name, Version: cfg.Version, AppID: cfg.ID, Server: serverMode, UpdateConfigured: cfg.UpdateSource != "" && cfg.UpdatePublicKey != "", DiagnosticsAvailable: diagnosticsAvailable}
	appService := desktop.New(info, state, controls, logger)
	updateService := updates.New(updates.Config{
		AppID: cfg.ID, Version: cfg.Version, Arch: runtime.GOARCH, Source: cfg.UpdateSource, PublicKey: cfg.UpdatePublicKey,
		CacheDir: filepath.Join(dir, "updates"), Enabled: runtime.GOOS == "windows" && !serverMode,
	}, state, logger, emit, updates.LaunchInstaller, controls.ApproveQuit)
	options := application.Options{
		Name: cfg.Name, Description: "Wails用のユースケース駆動テンプレート", Logger: logger,
		Assets:       application.AssetOptions{Handler: application.BundledAssetFileServer(root), DisableLogging: true},
		Services:     []application.Service{application.NewService(noteService), application.NewService(appService), application.NewService(updateService)},
		MarshalError: fault.Marshal,
		ShouldQuit:   controls.ShouldQuit,
		Server:       application.ServerOptions{Host: "127.0.0.1", Port: port},
		Windows:      application.WindowsOptions{WebviewUserDataPath: filepath.Join(dir, "webview")},
	}
	if !serverMode {
		// A deterministic per-product key, not a secret or an updater key.
		key := sha256.Sum256([]byte(cfg.ID + ":single-instance"))
		options.SingleInstance = &application.SingleInstanceOptions{UniqueID: cfg.ID, EncryptionKey: key, OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			if window != nil {
				window.Show()
				window.Restore()
				window.Focus()
			}
		}}
	}
	app = application.New(options)
	if !serverMode {
		window = app.Window.NewWithOptions(application.WebviewWindowOptions{Title: cfg.Name, Width: 1160, Height: 800, URL: "/"})
		window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			if !controls.ShouldQuit() {
				e.Cancel()
			}
		})
	}
	return app.Run()
}
func serverPort() (int, error) {
	if s := os.Getenv("WAILS_SERVER_PORT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n < 65536 {
			return n, nil
		}
		return 0, fmt.Errorf("invalid WAILS_SERVER_PORT")
	}
	return 34115, nil
}
