// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package errors provides basic utilities to construct errors.
//
// To construct new errors or wrap other errors, use this package rather than
// standard libraries (errors.New, fmt.Errorf) or any other third-party
// libraries. This package records stack traces and chained errors, and leaves
// nicely formatted logs when tests fail.
//
// # Simple usage
//
// To construct a new error, use New or Errorf.
//
//	errors.New("process not found")
//	errors.Errorf("process %d not found", pid)
//
// To construct an error by adding context to an existing error, use Wrap or
// Wrapf.
//
//	errors.Wrap(err, "failed to connect to Chrome browser process")
//	errors.Wrapf(err, "failed to connect to Chrome renderer process %d", pid)
//
// A stack trace can be printed by passing an error to error-reporting methods
// in testing.State, or formatting it with the fmt package with the "%+v" verb.
//
// # Defining custom error types
//
// Sometimes you may want to define custom error types, for example, to inspect
// and react to errors. In that case, embed *E in your custom error struct.
//
//	type CustomError struct {
//	    *errors.E
//	}
//
//	if err := doSomething(); err != nil {
//	    return &CustomError{E: errors.Wrap(err, "something failed")}
//	}
package errors

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"go.chromium.org/tast/core/errors/stack"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// E is the error implementation used by this package.
type E struct {
	msg   string      // error message to be prepended to cause
	stk   stack.Stack // stack trace where this error was created
	cause error       // original error that caused this error if non-nil
}

// NewE create a new E. Do not use this function outside Tast.
// TODO: b/187792551 -- Will remove after we move all tests to use
// go.chromium.org/tast/errors.
func NewE(msg string, stk stack.Stack, cause error) *E {
	return &E{msg, stk, cause}
}

// Error implements the error interface.
func (e *E) Error() string {
	if e.cause == nil {
		return e.msg
	}
	return fmt.Sprintf("%s: %s", e.msg, e.cause.Error())
}

// Unwrap implements the error Unwrap interface introduced in go1.13.
func (e *E) Unwrap() error {
	return e.cause
}

// unwrapper is a private interface of *E providing access to its fields.
// We should access *E via this interface to allow embedding *E in
// user-defined custom error types.
type unwrapper interface {
	unwrap() (msg string, stk stack.Stack, cause error)
}

// unwrap implements the unwrapper interface.
func (e *E) unwrap() (msg string, stk stack.Stack, cause error) {
	return e.msg, e.stk, e.cause
}

// formatChain formats an error chain.
func formatChain(err error) string {
	var chain []string
	for err != nil {
		if e, ok := err.(unwrapper); ok {
			msg, stk, cause := e.unwrap()
			chain = append(chain, fmt.Sprintf("%s\n%v", msg, stk))
			err = cause
		} else {
			chain = append(chain, fmt.Sprintf("%+v", err))
			err = nil
		}
	}
	return strings.Join(chain, "\n")
}

// GRPCStatus converts the receiver into a grpc Status object.
func (e *E) GRPCStatus() *status.Status {
	s := status.New(codes.Unknown, e.Error())
	s, newErr := s.WithDetails(&errdetails.DebugInfo{Detail: fmt.Sprintf("%+v", e)})
	if newErr != nil {
		panic(fmt.Sprintf("withDetails failed: %v", newErr))
	}
	return s
}

// Format implements the fmt.Formatter interface.
// In particular, it is supported to format an error chain by "%+v" verb.
func (e *E) Format(s fmt.State, verb rune) {
	if verb == 'v' && s.Flag('+') {
		io.WriteString(s, formatChain(e))
	} else {
		io.WriteString(s, e.Error())
	}
}

// statusE is the an error with a status and detailed stacktrace.
type statusE struct {
	error
}

// FromGRPCError converts an error that came from a grpc call into a error with a stack trace.
func FromGRPCError(err error) error {
	if err == nil {
		return nil
	}
	_, ok := status.FromError(err)
	if ok {
		return &statusE{err}
	}
	return err
}

// Format implements the fmt.Formatter interface.
// Writes the detailed debug info (i.e. stack trace).
func (e *statusE) Format(s fmt.State, verb rune) {
	if verb == 'v' && s.Flag('+') {
		status, ok := status.FromError(e.error)
		if ok {
			details := status.Details()
			for _, detail := range details {
				if di, ok := detail.(*errdetails.DebugInfo); ok {
					if di.Detail != "" {
						io.WriteString(s, di.Detail)
						return
					}
				}
			}
		}
	}
	io.WriteString(s, e.error.Error())
}

// New creates a new error with the given message.
// This is similar to the standard errors.New, but also records the location
// where it was called.
func New(msg string) *E {
	s := stack.New(1)
	return &E{msg, s, nil}
}

// Errorf creates a new error with the given message.
// This is similar to the standard fmt.Errorf, but also records the location
// where it was called.
func Errorf(format string, args ...interface{}) *E {
	s := stack.New(1)
	msg := fmt.Sprintf(format, args...)
	return &E{msg, s, nil}
}

// Wrap creates a new error with the given message, wrapping another error.
// This function also records the location where it was called.
// If cause is nil, this is the same as New. Note that the above behaviour
// is different from the popular github.com/pkg/errors package.
func Wrap(cause error, msg string) *E {
	s := stack.New(1)
	return &E{msg, s, cause}
}

// Wrapf creates a new error with the given message, wrapping another error.
// This function also records the location where it was called.
// If cause is nil, this is the same as Errorf. Note that the above behaviour
// is different from the popular github.com/pkg/errors package.
func Wrapf(cause error, format string, args ...interface{}) *E {
	s := stack.New(1)
	msg := fmt.Sprintf(format, args...)
	return &E{msg, s, cause}
}

// Unwrap is a wrapper of built-in errors.Unwrap. It returns the result of
// calling the Unwrap method on err, if err's type contains an Unwrap method
// returning error. Otherwise, Unwrap returns nil.
func Unwrap(err error) error {
	return errors.Unwrap(err)
}

// As is a wrapper of built-in errors.As. It finds the first error in err's
// chain that matches target, and if so, sets target to that error value and
// returns true.
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// Is is a wrapper of built-in errors.Is. It reports whether any error in err's
// chain matches target.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// Join is a wrapper of built-in errors.Join.
func Join(errs ...error) error {
	return errors.Join(errs...)
}
