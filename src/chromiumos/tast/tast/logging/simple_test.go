// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"bytes"
	"testing"
)

func TestDropDebug(t *testing.T) {
	b := bytes.Buffer{}
	l := NewSimple(&b, 0, false) // verbose disabled
	defer l.Close()

	l.Log("log")
	l.Debug("debug")
	if exp := "log\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
}

func TestKeepDebug(t *testing.T) {
	b := bytes.Buffer{}
	l := NewSimple(&b, 0, true) // verbose enabled
	defer l.Close()

	l.Log("log")
	l.Debug("debug")
	if exp := "log\ndebug\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
}

func TestAdditionalWriter(t *testing.T) {
	b := bytes.Buffer{}
	l := NewSimple(&b, 0, false) // verbose disabled
	defer l.Close()

	b2 := bytes.Buffer{}
	l.AddWriter(&b2, 0)

	l.Log("log")
	l.Debug("debug")
	if exp := "log\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
	if exp := "log\ndebug\n"; b2.String() != exp {
		t.Errorf("Writer logged %q; want %q", b2.String(), exp)
	}

	l.RemoveWriter(&b2)
	l.Log("log2")
	if exp := "log\nlog2\n"; b.String() != exp {
		t.Errorf("Logged %q; want %q", b.String(), exp)
	}
	if exp := "log\ndebug\n"; b2.String() != exp {
		t.Errorf("Writer logged %q; want %q", b2.String(), exp)
	}
}
