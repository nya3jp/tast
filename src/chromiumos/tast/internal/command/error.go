// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package command contains code shared by executables (e.g. test runners and test bundles).
package command

import (
	"fmt"
	"io"
)

// StatusError implements the error interface and contains an additional status code.
type StatusError struct {
	msg    string
	status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%v (status %v)", e.msg, e.status)
}

// Status returns e's status code.
func (e *StatusError) Status() int {
	return e.status
}

// NewStatusErrorf creates a StatusError with the passed status code and formatted string.
func NewStatusErrorf(status int, format string, args ...interface{}) *StatusError {
	return &StatusError{fmt.Sprintf(format, args...), status}
}

// WriteError writes a newline-terminated fatal error to w and returns the status code to use when exiting.
// If err is not a *StatusError, status code 1 is returned.
func WriteError(w io.Writer, err error) int {
	var msg string
	var status int

	if se, ok := err.(*StatusError); ok {
		msg = se.msg
		status = se.status
	} else {
		msg = err.Error()
		status = 1
	}

	if len(msg) > 0 && msg[len(msg)-1] != '\n' {
		msg += "\n"
	}
	io.WriteString(w, msg)

	return status
}
