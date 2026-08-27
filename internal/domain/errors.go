package domain

import "fmt"

type ErrorCode string

const (
	CodeInvalid     ErrorCode = "invalid"
	CodeConflict    ErrorCode = "conflict"
	CodeNotFound    ErrorCode = "not_found"
	CodeState       ErrorCode = "invalid_state"
	CodeForbidden   ErrorCode = "forbidden"
	CodeUnqualified ErrorCode = "unqualified"
)

type DomainError struct {
	Code        ErrorCode         `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
}

func NewFieldError(code ErrorCode, field, message string) error {
	return &DomainError{Code: code, Message: message, FieldErrors: map[string]string{field: message}}
}

func FieldErrorsOf(err error) map[string]string {
	if e, ok := err.(*DomainError); ok {
		return e.FieldErrors
	}
	return nil
}

func (e *DomainError) Error() string { return e.Message }

func NewError(code ErrorCode, format string, args ...any) error {
	return &DomainError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func ErrorCodeOf(err error) ErrorCode {
	if e, ok := err.(*DomainError); ok {
		return e.Code
	}
	return CodeInvalid
}
