// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

import (
	"errors"
	"fmt"
	"regexp"
	"testing"
)

func check(t *testing.T, err error, msg string, traceRegexp *regexp.Regexp) {
	if s := err.Error(); s != msg {
		t.Errorf("Wrong error message %q; want %q", s, msg)
	}
	if s := fmt.Sprintf("%v", err); s != msg {
		t.Errorf("Wrong default value %q; want %q", s, msg)
	}
	if tr := fmt.Sprintf("%+v", err); !traceRegexp.MatchString(tr) {
		t.Errorf("Wrong trace %q; should match %q", tr, traceRegexp)
	}
}

func TestNew(t *testing.T) {
	const msg = "meow"
	traceRegexp := regexp.MustCompile(`^meow
	at chromiumos/tast/errors\.TestNew \(errors_test.go:\d+\)`)

	err := New(msg)

	check(t, err, msg, traceRegexp)
}

func TestErrorf(t *testing.T) {
	const msg = "meow"
	traceRegexp := regexp.MustCompile(`^meow
	at chromiumos/tast/errors\.TestErrorf \(errors_test.go:\d+\)`)

	err := Errorf("%sow", "me")

	check(t, err, msg, traceRegexp)
}

func TestWrap(t *testing.T) {
	const msg = "meow: woof"
	traceRegexp := regexp.MustCompile(`(?s)^meow
	at chromiumos/tast/errors\.TestWrap \(errors_test.go:\d+\)
.*
woof
	at chromiumos/tast/errors\.TestWrap \(errors_test.go:\d+\)`)

	err := Wrap(New("woof"), "meow")

	check(t, err, msg, traceRegexp)
}

func TestWrapForeignError(t *testing.T) {
	const msg = "meow: woof"
	traceRegexp := regexp.MustCompile(`(?s)^meow
	at chromiumos/tast/errors\.TestWrapForeignError \(errors_test.go:\d+\)
.*
woof
	at \?\?\?$`)

	// Use standard errors package to create an error without trace.
	err := Wrap(errors.New("woof"), "meow")

	check(t, err, msg, traceRegexp)
}

func TestWrapNil(t *testing.T) {
	const msg = "meow"
	traceRegexp := regexp.MustCompile(`^meow
	at chromiumos/tast/errors\.TestWrapNil \(errors_test.go:\d+\)`)

	err := Wrap(nil, "meow")

	check(t, err, msg, traceRegexp)
}

func TestWrapf(t *testing.T) {
	const msg = "meow: woof"
	traceRegexp := regexp.MustCompile(`(?s)^meow
	at chromiumos/tast/errors\.TestWrapf \(errors_test.go:\d+\)
.*
woof
	at chromiumos/tast/errors\.TestWrapf \(errors_test.go:\d+\)`)

	err := Wrapf(New("woof"), "%sow", "me")

	check(t, err, msg, traceRegexp)
}
