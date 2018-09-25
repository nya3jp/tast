// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package errors provides basic utilities to construct errors.
//
// To construct new errors or wrap other errors, use this package rather than
// standard libraries (errors.New, fmt.Errorf) or any other third-party
// libraries. This package records stack traces and chained errors, and leaves
// nicely formatted logs when tests fail.
//
// To construct a new error, use New or Errorf.
//
//  errors.New("process not found")
//  errors.Errorf("process %d not found", pid)
//
// To construct an error by adding context to an existing error, use Wrap or
// Wrapf.
//
//  errors.Wrap(err, "failed to connect to Chrome browser process")
//  errors.Wrapf(err, "failed to connect to Chrome renderer process %d", pid)
//
// A stack trace can be printed by passing an error to error-reporting methods
// in testing.State, or formatting it with the fmt package with the "%+v" verb.
package errors

import (
	"fmt"
	"io"
	"strings"

	"chromiumos/tast/errors/stack"
)

// impl is the error implementation used by this package.
type impl struct {
	msg   string      // error message to be prepended to cause
	stk   stack.Stack // stack trace where this error was created
	cause error       // original error that caused this error if non-nil
}

// Error implements the error interface.
func (e *impl) Error() string {
	if e.cause == nil {
		return e.msg
	}
	return fmt.Sprintf("%s: %s", e.msg, e.cause.Error())
}

// formatChain formats an error chain.
func formatChain(err error) string {
	var chain []string
	for err != nil {
		if e, ok := err.(*impl); !ok {
			chain = append(chain, fmt.Sprintf("%s\n\tat ???", err.Error()))
			err = nil
		} else {
			chain = append(chain, fmt.Sprintf("%s\n%v", e.msg, e.stk))
			err = e.cause
		}
	}
	return strings.Join(chain, "\n")
}

// Format implements the fmt.Formatter interface.
// In particular, it is supported to format an error chain by "%+v" verb.
func (e *impl) Format(s fmt.State, verb rune) {
	if verb == 'v' && s.Flag('+') {
		io.WriteString(s, formatChain(e))
	} else {
		io.WriteString(s, e.Error())
	}
}

// New creates a new error with the given message.
// This is similar to the standard errors.New, but also records the location
// where it was called.
func New(msg string) error {
	s := stack.New(1)
	return &impl{msg, s, nil}
}

// Errorf creates a new error with the given message.
// This is similar to the standard fmt.Errorf, but also records the location
// where it was called.
func Errorf(format string, args ...interface{}) error {
	s := stack.New(1)
	msg := fmt.Sprintf(format, args...)
	return &impl{msg, s, nil}
}

// Wrap creates a new error with the given message, wrapping another error.
// This function also records the location where it was called.
// If cause is nil, this is the same as New.
func Wrap(cause error, msg string) error {
	s := stack.New(1)
	return &impl{msg, s, cause}
}

// Wrapf creates a new error with the given message, wrapping another error.
// This function also records the location where it was called.
// If cause is nil, this is the same as Errorf.
func Wrapf(cause error, format string, args ...interface{}) error {
	s := stack.New(1)
	msg := fmt.Sprintf(format, args...)
	return &impl{msg, s, cause}
}
