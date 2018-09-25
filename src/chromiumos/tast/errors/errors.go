// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// TODO(nya): Write more explanation here

package errors

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
)

type richError struct {
	msg   string  // error message to be prepended to cause
	pc    uintptr // program counter where this error was created
	cause error   // original error that caused this error
}

// trace formats a trace of chained errors.
func trace(err error) string {
	var frames []string
	for err != nil {
		var f string
		if e, ok := err.(*richError); !ok {
			f = err.Error()
			err = nil
		} else {
			fc := runtime.FuncForPC(e.pc)
			var loc string
			if fc == nil {
				loc = "???"
			} else {
				fn, ln := fc.FileLine(e.pc)
				loc = fmt.Sprintf("%s (%s:%d)", fc.Name(), filepath.Base(fn), ln)
			}
			f = fmt.Sprintf("%s\n\tat %s", e.msg, loc)
			err = e.cause
		}
		frames = append(frames, f)
	}
	return strings.Join(frames, "\n")
}

// Error implements error interface.
func (e *richError) Error() string {
	if e.cause == nil {
		return e.msg
	}
	return fmt.Sprintf("%s: %s", e.msg, e.cause.Error())
}

// Format implements fmt.Formatter interface.
// In addition to usual %v and %s, richError also supports formatting a trace
// of chained errors by %+v.
func (e *richError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			io.WriteString(s, trace(e))
			return
		}
		fallthrough
	case 's':
		io.WriteString(s, e.Error())
	}
}

// New creates a new error with the given message.
// This is similar to standard errors.New, but also records the location where
// it was called.
func New(msg string) error {
	pc, _, _, _ := runtime.Caller(1)
	return &richError{msg, pc, nil}
}

// Errorf creates a new error with the given message.
// This is similar to standard fmt.Errorf, but also records the location where
// it was called.
func Errorf(format string, args ...interface{}) error {
	pc, _, _, _ := runtime.Caller(1)
	msg := fmt.Sprintf(format, args...)
	return &richError{msg, pc, nil}
}

// Wrap creates a new error with the given message, wrapping another error.
// This function also records the location where it was called.
// If cause is nil, this is the same as New.
func Wrap(cause error, msg string) error {
	pc, _, _, _ := runtime.Caller(1)
	return &richError{msg, pc, cause}
}

// Wrapf creates a new error with the given message, wrapping another error.
// This function also records the location where it was called.
// If cause is nil, this is the same as Errorf.
func Wrapf(cause error, format string, args ...interface{}) error {
	pc, _, _, _ := runtime.Caller(1)
	msg := fmt.Sprintf(format, args...)
	return &richError{msg, pc, cause}
}
