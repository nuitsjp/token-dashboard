package appstate

import "testing"

func TestExitInterlock(t *testing.T) {
	s := &State{}
	done, err := s.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if s.PrepareExit() == nil {
		t.Fatal("exit allowed while operation active")
	}
	done()
	done()
	if err = s.PrepareExit(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Begin(); err == nil {
		t.Fatal("new operation allowed during exit")
	}
	s.AbortExit()
	end, err := s.Begin()
	if err != nil {
		t.Fatal(err)
	}
	end()
	if s.Active() != 0 {
		t.Fatal("operation leaked")
	}
}
