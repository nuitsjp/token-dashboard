package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"token-monitor-turzx/internal/appstate"
	"token-monitor-turzx/internal/desktop"
	"token-monitor-turzx/internal/diagnostics"
	"token-monitor-turzx/internal/display"
	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/hub"
	"token-monitor-turzx/internal/localusage"
	"token-monitor-turzx/internal/settings"
	"token-monitor-turzx/internal/turzx"
	"token-monitor-turzx/internal/updates"
	"token-monitor-turzx/internal/usage"
)

//go:embed all:frontend/dist
var webAssets embed.FS

//go:embed build/app.json
var configJSON []byte

// Fixed at build time through -ldflags -X (see scripts/build.mjs): the release CI sets
// the version from the tag, and the desktop E2E also sets a local update source.
// An empty value keeps the one in build/app.json.
var buildVersion, buildUpdateSource, buildUpdatePublicKey string

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
	for target, value := range map[*string]string{&cfg.Version: buildVersion, &cfg.UpdateSource: buildUpdateSource, &cfg.UpdatePublicKey: buildUpdatePublicKey} {
		if value != "" {
			*target = value
		}
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
	settingsService := settings.New(filepath.Join(dir, "settings.json"), cfg.ID, turzx.List, logger)
	info := desktop.Info{Name: cfg.Name, Version: cfg.Version, AppID: cfg.ID, Server: serverMode, UpdateConfigured: cfg.UpdateSource != "" && cfg.UpdatePublicKey != "", DiagnosticsAvailable: diagnosticsAvailable}
	appService := desktop.New(info, state, controls, logger)
	updateConfig := updates.Config{
		AppID: cfg.ID, Version: cfg.Version, Arch: runtime.GOARCH, Source: cfg.UpdateSource, PublicKey: cfg.UpdatePublicKey,
		CacheDir: filepath.Join(dir, "updates"), Enabled: runtime.GOOS == "windows" && !serverMode,
	}
	updateService := updates.New(updateConfig, state, logger, emit, updates.LaunchInstaller, controls.ApproveQuit)
	renderer, err := display.NewRenderer()
	if err != nil {
		return err
	}
	usageState := usage.NewState()
	displayService := &display.Service{}
	output := display.NewOutput(func() (string, error) { return settings.DisplayTarget(settingsService) }, logger)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	options := application.Options{
		Name: cfg.Name, Description: "利用状況を TURZX に表示する常駐アプリ", Logger: logger,
		Assets:       application.AssetOptions{Handler: application.BundledAssetFileServer(root), DisableLogging: true},
		Services:     []application.Service{application.NewService(settingsService), application.NewService(appService), application.NewService(updateService), application.NewService(displayService)},
		MarshalError: fault.Marshal,
		ShouldQuit:   controls.ShouldQuit,
		OnShutdown: func() {
			stop()
			if !serverMode {
				output.Wait()
			}
		},
		Server:  application.ServerOptions{Host: "127.0.0.1", Port: port},
		Windows: application.WindowsOptions{WebviewUserDataPath: filepath.Join(dir, "webview")},
	}
	if port := os.Getenv("WAILS_WEBVIEW_DEBUG_PORT"); port != "" && !production {
		// Development only: lets Playwright CLI attach to the WebView2 over CDP.
		options.Windows.AdditionalBrowserArgs = []string{"--remote-debugging-port=" + port}
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
	sink := func(*image.RGBA) {}
	if !serverMode {
		go output.Run(ctx)
		sink = output.Submit
	}
	settingsService.OnStyleSaved = usageState.Touch
	go display.Run(ctx, displayService, renderer, usageState, redrawInterval(), func() display.Style { return display.ParseStyle(settings.LimitStyle(settingsService)) }, sink, emit, logger)
	sourceChanged := make(chan struct{}, 1)
	settingsService.OnSaved = func() {
		select {
		case sourceChanged <- struct{}{}:
		default:
		}
	}
	go runUsageSource(ctx, settingsService, usageState, logger, sourceChanged, dir)
	updateReady := func(string) {}
	if !serverMode {
		// The app lives in the task tray. Closing the window only hides it.
		// The title bar is the only place that shows the name and version.
		title := cfg.Name + " v" + cfg.Version
		window = app.Window.NewWithOptions(application.WebviewWindowOptions{Title: title, Width: 1160, Height: 800, URL: "/", Hidden: true,
			Windows: application.WindowsWindow{Theme: application.Dark}})
		window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			e.Cancel()
			window.Hide()
		})
		show := func() {
			window.Show()
			window.Restore()
			window.Focus()
		}
		menu := application.NewMenu()
		update := menu.Add("").SetHidden(true)
		menu.Add("Open").OnClick(func(*application.Context) { show() })
		menu.AddSeparator()
		menu.Add("Exit").OnClick(func(*application.Context) {
			// Exit from the tray quits without a dialogue unless work is in progress.
			if err := state.PrepareExit(); err != nil {
				show()
				emit(desktop.CloseRequested, nil)
				return
			}
			controls.ApproveQuit()
			app.Quit()
		})
		tray := app.SystemTray.New()
		tray.SetTooltip(cfg.Name)
		tray.SetMenu(menu)
		tray.OnClick(show)
		setUpdate := func(label string) {
			application.InvokeSync(func() {
				update.SetLabel(label).SetHidden(label == "")
				// The Windows tray builds its popup when the menu is set.
				tray.SetMenu(menu)
			})
		}
		update.OnClick(func(*application.Context) {
			// Same path as the window's button; a failure keeps the app running.
			if err := updateService.Apply(); err != nil {
				setUpdate("")
				show()
				return
			}
			app.Quit()
		})
		updateReady = func(version string) { setUpdate("Update and restart (v" + version + ")") }
	}
	if serverMode {
		// Server mode has no tray and never emits ApplicationStarted.
		go updates.Run(ctx, updateService, updateReady)
	} else {
		// The tray menu can be rebuilt only once the application is running.
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			go updates.Run(ctx, updateService, updateReady)
		})
	}
	return app.Run()
}

func runUsageSource(ctx context.Context, service *settings.Service, state *usage.State, logger *slog.Logger, changed <-chan struct{}, dataDir string) {
	for {
		source, err := settings.Source(service)
		var cancel context.CancelFunc
		var done chan struct{}
		if err != nil {
			logger.Warn("usage_source_unavailable", "cause", err)
		} else {
			state.SetSource(source)
			child, stop := context.WithCancel(ctx)
			cancel, done = stop, make(chan struct{})
			go func() {
				defer close(done)
				if source == "Hub" {
					hub.New(func() (string, string, error) { return settings.Connection(service) }, state, logger).Run(child)
				} else {
					path, err := os.Executable()
					if err != nil {
						logger.Warn("local_usage_unavailable", "cause", err)
						return
					}
					localusage.New(filepath.Join(filepath.Dir(path), "tokscale.exe"), filepath.Join(dataDir, "tokscale"), filepath.Join(dataDir, "local-scan.json"), localIntervals(), state, logger).Run(child)
				}
			}()
		}
		select {
		case <-ctx.Done():
			if cancel != nil {
				cancel()
				<-done
			}
			return
		case <-changed:
			if cancel != nil {
				cancel()
				<-done
			}
		}
	}
}

// shortIntervals lets the E2E tests shorten the app's waits so that they do not wait for real time.
// Only the server build, which the tests use, reads it.
func shortIntervals() bool {
	return serverMode && os.Getenv("WAILS_TEST_INTERVALS") == "short"
}

// localIntervals are the waits of local reading.
func localIntervals() localusage.Intervals {
	if shortIntervals() {
		return localusage.Intervals{Settle: 200 * time.Millisecond, Graph: time.Second, Poll: time.Second, Limits: time.Second, MaxSyncDelay: 4 * time.Second}
	}
	return localusage.DefaultIntervals
}

// redrawInterval is the longest time between two images, which keeps the time until reset current.
func redrawInterval() time.Duration {
	if shortIntervals() {
		return time.Second
	}
	return time.Minute
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
