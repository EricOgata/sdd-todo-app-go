// Package apperr defines the error taxonomy shared by every command.
//
// The product contract distinguishes user or validation failures (exit code 1)
// from unexpected or system failures (exit code 2). Only this package decides
// which is which.
package apperr

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Kind identifies the exit-code class of an error.
type Kind int

const (
	// KindUser marks a user or validation error. It maps to exit code 1.
	KindUser Kind = iota + 1
	// KindSystem marks an unexpected or system error. It maps to exit code 2.
	KindSystem
)

// Error is the typed error carried across the application boundary.
type Error struct {
	kind Kind
	msg  string
}

// Error implements the error interface.
func (e *Error) Error() string { return e.msg }

// Kind reports the error class.
func (e *Error) Kind() Kind { return e.kind }

// User returns a user or validation error.
func User(msg string) error { return &Error{kind: KindUser, msg: msg} }

// Userf returns a formatted user or validation error.
func Userf(format string, args ...any) error {
	return User(fmt.Sprintf(format, args...))
}

// System returns an unexpected or system error.
func System(msg string) error { return &Error{kind: KindSystem, msg: msg} }

// Systemf returns a formatted unexpected or system error.
func Systemf(format string, args ...any) error {
	return System(fmt.Sprintf(format, args...))
}

// Unexpected wraps a raw error as a system error. Typed errors are preserved so
// callers can wrap opportunistically without changing the classification.
func Unexpected(err error) error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return err
	}
	return System("unexpected failure: " + err.Error())
}

// As returns the typed error behind err, if any.
func As(err error) (*Error, bool) {
	var typed *Error
	if errors.As(err, &typed) {
		return typed, true
	}
	return nil, false
}

// IsUser reports whether err is classified as a user error.
func IsUser(err error) bool {
	typed, ok := As(err)
	return ok && typed.kind == KindUser
}

// ExitCode maps an error to the documented process exit code.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	typed, ok := As(err)
	if !ok {
		return 2
	}
	if typed.kind == KindUser {
		return 1
	}
	return 2
}

// Message returns the message that follows the "error: " prefix.
func Message(err error) string {
	if err == nil {
		return ""
	}
	typed, ok := As(err)
	if ok {
		return typed.msg
	}
	return "unexpected failure: " + err.Error()
}

// ClassifyCobra turns framework-originated failures (unknown commands and
// flags, argument validation) into user errors. Context cancellation becomes
// the documented interrupted system error. Other errors are returned
// unchanged for the caller to wrap with Unexpected.
func ClassifyCobra(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return System("interrupted.")
	}
	if _, ok := As(err); ok {
		return err
	}
	msg := firstLine(err.Error())
	for _, prefix := range cobraUserPrefixes {
		if strings.HasPrefix(msg, prefix) {
			return User(msg)
		}
	}
	return err
}

var cobraUserPrefixes = []string{
	"unknown command",
	"unknown flag",
	"unknown shorthand flag",
	"flag needs an argument",
	"invalid argument",
	"invalid value",
	"required flag",
	"required flag(s)",
	"accepts ",
	"shorthand flag",
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimRight(s, " \t\r")
}
