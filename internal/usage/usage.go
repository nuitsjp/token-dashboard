// Package usage holds the latest statistics in memory only.
package usage

import (
	"sync"
	"time"
)

// Stats is the subset of the Hub's stats payload that the display uses.
type Stats struct {
	Periods Periods `json:"periods"`
	Limits  Limits  `json:"limits"`
}

type Periods struct {
	Today   Period `json:"today"`
	Month   Period `json:"month"`
	AllTime Period `json:"allTime"`
}

type Period struct {
	TotalTokens int64   `json:"totalTokens"`
	CostUSD     float64 `json:"costUsd"`
	*ClientBreakdown
}

// ClientBreakdown is optional attribution; absence is different from a reported zero.
type ClientBreakdown struct {
	Clients     map[string]int64   `json:"clients,omitempty"`
	ClientCosts map[string]float64 `json:"clientCosts,omitempty"`
}

type Limits struct {
	Providers []Provider `json:"providers"`
}

type Provider struct {
	Provider     string   `json:"provider"`
	AccountLabel string   `json:"accountLabel"`
	PlanLabel    string   `json:"planLabel"`
	Windows      []Window `json:"windows"`
}

type Window struct {
	Kind             string     `json:"kind"`
	Label            string     `json:"label"`
	ShowMeter        bool       `json:"showMeter"`
	RemainingPercent *float64   `json:"remainingPercent"`
	UsedPercent      *float64   `json:"usedPercent"`
	ResetsAt         *time.Time `json:"resetsAt"`
	// WindowMinutes is the length of the window; nil when the source does not report it.
	WindowMinutes *float64 `json:"windowMinutes"`
}

// State is the in-memory latest Stats. Nil means no snapshot has arrived yet.
type State struct {
	mu      sync.Mutex
	latest  *Stats
	source  string
	changed chan struct{}
}

func NewState() *State { return &State{source: "Local", changed: make(chan struct{}, 1)} }

// Set replaces the latest Stats and signals Changed without blocking.
func (s *State) Set(stats *Stats) {
	s.mu.Lock()
	s.latest = stats
	s.mu.Unlock()
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *State) Latest() *Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

// SetSource clears data from the previous source before the new reader starts.
func (s *State) SetSource(source string) {
	s.mu.Lock()
	s.source = source
	s.latest = nil
	s.mu.Unlock()
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *State) Snapshot() (*Stats, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest, s.source
}

// Touch asks the display to redraw although the stats did not change.
func (s *State) Touch() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Changed receives after one or more Set calls; only the newest value matters.
func (s *State) Changed() <-chan struct{} { return s.changed }
