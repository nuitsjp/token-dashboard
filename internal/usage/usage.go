// Package usage holds the latest Hub statistics in memory only.
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
}

// State is the in-memory latest Stats. Nil means no snapshot has arrived yet.
type State struct {
	mu      sync.Mutex
	latest  *Stats
	changed chan struct{}
}

func NewState() *State { return &State{changed: make(chan struct{}, 1)} }

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

// Changed receives after one or more Set calls; only the newest value matters.
func (s *State) Changed() <-chan struct{} { return s.changed }
