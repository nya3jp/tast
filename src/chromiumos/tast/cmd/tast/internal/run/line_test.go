// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestFirstLineReaderSuccess(t *testing.T) {
	const (
		first  = "first line"
		second = "second line"
	)
	b := bytes.Buffer{}
	fmt.Fprintf(&b, "%s\n%s\n", first, second)

	if ln, err := newFirstLineReader(&b).getLine(time.Second); err != nil {
		t.Errorf("getLine() returned error: %v", err)
	} else if ln != first {
		t.Errorf("getLine() = %q; want %q", ln, first)
	}
}

func TestFirstLineReaderEOF(t *testing.T) {
	pr, pw := io.Pipe()
	pw.Close()
	if _, err := newFirstLineReader(pr).getLine(time.Minute); err != io.EOF {
		t.Errorf("getLine() returned error %v; want EOF", err)
	}
}

func TestFirstLineReaderTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	if _, err := newFirstLineReader(pr).getLine(time.Millisecond); err == nil {
		t.Errorf("getLine() didn't return error for timeout")
	}
}
