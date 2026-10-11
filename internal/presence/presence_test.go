package presence

import "testing"

func TestAwayWhileAnyConditionHolds(t *testing.T) {
	var changes []bool
	c := New(func(away bool) { changes = append(changes, away) })
	c.Set(Idle, true)
	c.Set(MonitorOff, true)
	c.Set(Idle, false)
	c.Set(MonitorOff, false)
	c.Set(Suspended, false)
	c.Set(SessionInactive, true)
	c.Set(SessionInactive, false)
	want := []bool{true, false, true, false}
	if len(changes) != len(want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("changes = %v, want %v", changes, want)
		}
	}
}
