package fault

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestInternalCauseDoesNotEscape(t *testing.T) {
	b := Marshal(errors.New("password=secret"))
	if strings.Contains(string(b), "secret") {
		t.Fatal("internal cause leaked")
	}
	var e Error
	if err := json.Unmarshal(b, &e); err != nil || e.Code != "INTERNAL" {
		t.Fatalf("%s: %v", b, err)
	}
}
func TestPublicValidationAndCancellation(t *testing.T) {
	e := Validation(map[string]string{"title": "required"})
	b := Marshal(e)
	if !strings.Contains(string(b), "fieldErrors") {
		t.Fatal("field mapping lost")
	}
	if Public(context.Canceled).Code != "CANCELLED" {
		t.Fatal("cancellation not distinct")
	}
}
