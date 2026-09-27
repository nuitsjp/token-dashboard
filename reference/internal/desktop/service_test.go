package desktop

import (
	"testing"

	"wailstemplate/internal/appstate"
)

func TestQuitHandshake(t *testing.T) {
	notices := 0
	c := &Controls{Emit: func(string, any) { notices++ }}
	if !c.ShouldQuit() {
		t.Fatal("quit blocked before UI ready")
	}
	c.Ready.Store(true)
	if c.ShouldQuit() || notices != 1 {
		t.Fatal("did not ask UI")
	}
	c.ApproveQuit()
	if !c.ShouldQuit() {
		t.Fatal("approved quit blocked")
	}
}

func TestConfirmQuit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		server bool
		busy   bool
	}{
		{name: "idle"},
		{name: "busy", busy: true},
		{name: "server", server: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &appstate.State{}
			controls := &Controls{}
			if tc.busy {
				done, err := state.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer done()
			}
			s := New(Info{Server: tc.server}, state, controls, nil)
			err := s.ConfirmQuit()
			blocked := tc.server || tc.busy
			if (err != nil) != blocked || controls.Approved.Load() == blocked {
				t.Fatalf("error=%v approved=%v blocked=%v", err, controls.Approved.Load(), blocked)
			}
			done, err := state.Begin()
			if blocked {
				if err != nil {
					t.Fatalf("rejected quit left application closing: %v", err)
				}
				done()
			} else if err == nil {
				done()
				t.Fatal("work accepted after quit approval")
			}
		})
	}
}
