package skeinerr

import "fmt"

const (
	Malformed       = "malformed"
	Conflict        = "conflict"
	Stale           = "stale"
	Unauthenticated = "unauthenticated"
	Policy          = "policy"
	IO              = "io"
)

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func New(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func As(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	e, ok := err.(*Error)
	return e, ok
}
