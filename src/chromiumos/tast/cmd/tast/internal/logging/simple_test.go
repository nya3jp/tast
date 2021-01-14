// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestDropDebug(t *testing.T) {
	b := bytes.Buffer{}
	l := NewSimple(&b, false, false) // verbose disabled
	defer l.Close()

	l.Log("log")
	l.Debug("debug")
	if exp := "log\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
}

func TestKeepDebug(t *testing.T) {
	b := bytes.Buffer{}
	l := NewSimple(&b, false, true) // verbose enabled
	defer l.Close()

	l.Log("log")
	l.Debug("debug")
	if exp := "log\ndebug\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
}

func TestAdditionalWriter(t *testing.T) {
	b := bytes.Buffer{}
	l := NewSimple(&b, false, false) // verbose disabled
	defer l.Close()

	b2 := bytes.Buffer{}
	if err := l.AddWriter(&b2, 0); err != nil {
		t.Fatal(err)
	}
	if err := l.AddWriter(&b2, 0); err == nil {
		t.Errorf("Didn't get error when double-adding writer")
	}

	l.Log("log")
	l.Debug("debug")
	if exp := "log\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
	if exp := "log\ndebug\n"; b2.String() != exp {
		t.Errorf("Writer logged %q; want %q", b2.String(), exp)
	}

	if err := l.RemoveWriter(&b2); err != nil {
		t.Error(err)
	}
	if err := l.RemoveWriter(&b2); err == nil {
		t.Errorf("Didn't get error when double-removing writer")
	}

	l.Log("log2")
	if exp := "log\nlog2\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
	if exp := "log\ndebug\n"; b2.String() != exp {
		t.Errorf("Writer logged %q; want %q", b2.String(), exp)
	}
}

func TestAppendDateTime(t *testing.T) {
	b := bytes.Buffer{}
	l := NewSimple(&b, true, true)
	defer l.Close()

	l.Log("log")
	l.Debug("debug")

	lines := strings.Split(b.String(), "\n")
	if len(lines) != 3 {
		t.Errorf("3 lines expected: %q", lines)
	}

	slice := strings.Split(lines[0], " ")
	if exp := "log"; len(slice[0]) == 27 && slice[1] != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}

	slice = strings.Split(lines[1], " ")
	if exp := "debug"; len(slice[1]) == 27 && slice[1] != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}

	if lines[2] != "" {
		t.Errorf("Empty line expected; want %q", lines[2])
	}
}
