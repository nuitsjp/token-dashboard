// Package fault defines the public failure contract. Internal causes are never
// serialised to the frontend; they are recorded at the operation boundary.
package fault

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
)

type Error struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
}

func (e *Error) Error() string        { return e.Message }
func New(code, message string) *Error { return &Error{Code: code, Message: message} }
func Validation(fields map[string]string) *Error {
	return &Error{Code: "VALIDATION", Message: "入力内容を確認してください。", FieldErrors: fields}
}
func Public(err error) *Error {
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	if errors.Is(err, context.Canceled) {
		return New("CANCELLED", "処理を中止しました。")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return New("TIMEOUT", "処理が時間内に完了しませんでした。")
	}
	return New("INTERNAL", "処理を完了できませんでした。診断ログを確認してください。")
}
func Marshal(err error) []byte {
	b, marshalErr := json.Marshal(Public(err))
	if marshalErr != nil {
		return []byte(`{"code":"INTERNAL","message":"処理を完了できませんでした。"}`)
	}
	return b
}

// Boundary logs unexpected failures once and returns only public information.
func Boundary(logger *slog.Logger, operation string, err error) error {
	if err == nil {
		return nil
	}
	var public *Error
	if errors.As(err, &public) {
		return public
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Public(err)
	}
	logger.Error("operation_failed", "operation", operation, "cause", err)
	return Public(err)
}
