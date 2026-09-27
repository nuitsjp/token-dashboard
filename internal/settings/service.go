// Package settings owns the Hub connection and the TURZX display selection.
package settings

import "token-monitor-turzx/internal/fault"

// Display is a TURZX device that can be selected as the output.
type Display struct {
	DeviceID  string `json:"deviceID"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

// View never contains the token itself, only whether one is saved.
type View struct {
	URL      string `json:"url"`
	TokenSet bool   `json:"tokenSet"`
	// DisplayID is empty when the first connected display is used automatically.
	DisplayID string    `json:"displayID"`
	Displays  []Display `json:"displays"`
}

type SaveRequest struct {
	URL string `json:"url"`
	// An empty token keeps the saved one.
	Token     string `json:"token"`
	DisplayID string `json:"displayID"`
}

type Service struct{}

func New() *Service { return &Service{} }

// Stage 2 exposes the contract only. The frontend mock replaces these calls.
var errNotImplemented = fault.New("NOT_IMPLEMENTED", "設定の読み書きはまだ実装されていません。")

func (s *Service) Get() (View, error)               { return View{}, errNotImplemented }
func (s *Service) Save(_ SaveRequest) (View, error) { return View{}, errNotImplemented }
