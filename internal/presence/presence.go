// Package presence tells whether the user is away from the PC: the signals Windows gives for it are
// kept as conditions, and the user is away while any of them holds.
package presence

import "sync"

// Condition is one signal that the user is away.
type Condition int

const (
	// Idle: no keyboard or mouse input for the saved time.
	Idle Condition = iota
	// MonitorOff: Windows turned the monitor off.
	MonitorOff
	// Suspended: the PC sleeps or hibernates.
	Suspended
	// SessionInactive: this session is locked, signed out, switched away or cut off from the console.
	SessionInactive
)

// Conditions keeps which conditions hold and calls changed whenever the user becomes away or back.
type Conditions struct {
	mu      sync.Mutex
	held    map[Condition]bool
	away    bool
	changed func(away bool)
}

// New returns Conditions with none held; changed is called without the lock held.
func New(changed func(away bool)) *Conditions {
	return &Conditions{held: map[Condition]bool{}, changed: changed}
}

// Set records that the condition holds or no longer holds.
func (c *Conditions) Set(condition Condition, holds bool) {
	c.mu.Lock()
	c.held[condition] = holds
	away := false
	for _, h := range c.held {
		away = away || h
	}
	changed := away != c.away
	c.away = away
	c.mu.Unlock()
	if changed {
		c.changed(away)
	}
}
