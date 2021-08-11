// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package command_test

import (
	"bytes"
	"errors"
	"testing"

	"chromiumos/tast/internal/command"
)

func TestWriteErrorStatusError(t *testing.T) {
	const (
		status = 127
		msg    = "this is the error message"
	)

	// When passed a *StatusError, WriteError should return the attached status code.
	err := command.NewStatusErrorf(status, msg)
	b := bytes.Buffer{}
	if ret := command.WriteError(&b, err); ret != status {
		t.Errorf("WriteError(%v) = %v; want %v", err, ret, status)
	}
	if b.String() != msg+"\n" {
		t.Errorf("WriteError(%v) wrote %q; want %q", err, b.String(), msg+"\n")
	}
}

func TestWriteErrorGenericError(t *testing.T) {
	const msg = "this is the error message"

	// When passed a *StatusError, WriteError should just return 1.
	err := errors.New(msg)
	b := bytes.Buffer{}
	if ret := command.WriteError(&b, err); ret != 1 {
		t.Errorf("WriteError(%v) = %v; want 1", err, ret)
	}
	if b.String() != msg+"\n" {
		t.Errorf("WriteError(%v) wrote %q; want %q", err, b.String(), msg+"\n")
	}
}
