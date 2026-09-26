// Package desktop owns application interaction, not product use cases.
package desktop

import (
	"log/slog"
	"sync/atomic"
	"wailstemplate/internal/appstate"
	"wailstemplate/internal/fault"
)

const CloseRequested = "app:close-requested"

type Info struct {
	DiagnosticsAvailable bool   `json:"diagnosticsAvailable"`
	Name                 string `json:"name"`
	Version              string `json:"version"`
	AppID                string `json:"appID"`
	Server               bool   `json:"server"`
	UpdateConfigured     bool   `json:"updateConfigured"`
}
type Service struct {
	info     Info
	state    *appstate.State
	controls *Controls
	logger   *slog.Logger
}

// Controls are never registered with Wails: they are composition-only callbacks.
type Controls struct {
	Ready    atomic.Bool
	Approved atomic.Bool
	Emit     func(string, any)
}

func New(info Info, state *appstate.State, controls *Controls, logger *slog.Logger) *Service {
	return &Service{info: info, state: state, controls: controls, logger: logger}
}
func (s *Service) GetInfo() Info { return s.info }
func (s *Service) Ready()        { s.controls.Ready.Store(true) }
func (s *Service) ConfirmQuit() error {
	if s.info.Server {
		return fault.New("DESKTOP_ONLY", "終了操作はデスクトップ版で行ってください。")
	}
	if err := s.state.PrepareExit(); err != nil {
		return err
	}
	s.controls.ApproveQuit()
	return nil
}
func (c *Controls) ShouldQuit() bool {
	if c.Approved.Load() || !c.Ready.Load() {
		return true
	}
	c.Emit(CloseRequested, nil)
	return false
}
func (c *Controls) ApproveQuit() {
	c.Approved.Store(true)
}
func (s *Service) ReportFrontendError(message string) {
	if len(message) > 2000 {
		message = message[:2000]
	}
	// Callers send exception descriptions only, never input data or API payloads.
	s.logger.Error("frontend_unhandled", "message", message)
}
