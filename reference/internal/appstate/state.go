// Package appstate coordinates write operations with application shutdown.
package appstate

import (
	"sync"
	"wailstemplate/internal/fault"
)

type State struct {
	mu      sync.Mutex
	active  int
	closing bool
}

func (s *State) Begin() (func(), error) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return nil, fault.New("CLOSING", "アプリは終了処理中です。")
	}
	s.active++
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); s.active--; s.mu.Unlock() }) }, nil
}
func (s *State) PrepareExit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return fault.New("CLOSING", "終了または更新がすでに開始されています。")
	}
	if s.active != 0 {
		return fault.New("BUSY", "処理の完了または中止を待ってから終了してください。")
	}
	s.closing = true
	return nil
}

// AbortExit is used only when handing off to the installer failed before exit.
func (s *State) AbortExit()  { s.mu.Lock(); s.closing = false; s.mu.Unlock() }
func (s *State) Active() int { s.mu.Lock(); defer s.mu.Unlock(); return s.active }
