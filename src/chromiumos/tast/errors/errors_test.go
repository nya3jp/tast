// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors_test

import (
	stderrors "errors"
	"fmt"
	"regexp"
	"testing"

	"chromiumos/tast/errors"
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
	at chromiumos/tast/errors_test\.TestNew \(errors_test.go:\d+\)`)

	err := errors.New(msg)

	check(t, err, msg, traceRegexp)
}

func TestErrorf(t *testing.T) {
	const msg = "meow"
	traceRegexp := regexp.MustCompile(`^meow
	at chromiumos/tast/errors_test\.TestErrorf \(errors_test.go:\d+\)`)

	err := errors.Errorf("%sow", "me")

	check(t, err, msg, traceRegexp)
}

func TestWrap(t *testing.T) {
	const msg = "meow: woof"
	traceRegexp := regexp.MustCompile(`(?s)^meow
	at chromiumos/tast/errors_test\.TestWrap \(errors_test.go:\d+\)
.*
woof
	at chromiumos/tast/errors_test\.TestWrap \(errors_test.go:\d+\)`)

	err := errors.Wrap(errors.New("woof"), "meow")

	check(t, err, msg, traceRegexp)
}

func TestWrapForeignError(t *testing.T) {
	const msg = "meow: woof"
	traceRegexp := regexp.MustCompile(`(?s)^meow
	at chromiumos/tast/errors_test\.TestWrapForeignError \(errors_test.go:\d+\)
.*
woof
	at \?\?\?$`)

	// Use standard errors package to create an error without trace.
	err := errors.Wrap(stderrors.New("woof"), "meow")

	check(t, err, msg, traceRegexp)
}

func TestWrapNil(t *testing.T) {
	const msg = "meow"
	traceRegexp := regexp.MustCompile(`^meow
	at chromiumos/tast/errors_test\.TestWrapNil \(errors_test.go:\d+\)`)

	err := errors.Wrap(nil, "meow")

	check(t, err, msg, traceRegexp)
}

func TestWrapf(t *testing.T) {
	const msg = "meow: woof"
	traceRegexp := regexp.MustCompile(`(?s)^meow
	at chromiumos/tast/errors_test\.TestWrapf \(errors_test.go:\d+\)
.*
woof
	at chromiumos/tast/errors_test\.TestWrapf \(errors_test.go:\d+\)`)

	err := errors.Wrapf(errors.New("woof"), "%sow", "me")

	check(t, err, msg, traceRegexp)
}

type customError struct {
	*errors.E
}

func TestCustomError(t *testing.T) {
	const msg = "meow: woof"
	traceRegexp := regexp.MustCompile(`(?s)^meow
	at chromiumos/tast/errors_test\.TestCustomError \(errors_test.go:\d+\)
.*
woof
	at chromiumos/tast/errors_test\.TestCustomError \(errors_test.go:\d+\)`)

	var err error = errors.Wrap(&customError{E: errors.New("woof")}, "meow")

	check(t, err, msg, traceRegexp)
}

func TestUnwrap(t *testing.T) {
	// Verifies that Unwrap interface on E can properly fit in golang errors library.
	errBase := &customError{E: errors.New("woof")}
	errWrap := errors.Wrap(errBase, "meow")
	if err := errors.Unwrap(errWrap); err != error(errBase) {
		t.Errorf("Unwrap(errWrap) returns %v, want %v", err, errBase)
	}
	if !errors.Is(errWrap, errBase) {
		t.Error("Is(errWrap, errBase) should be true")
	}
	var asErr *customError
	if !errors.As(errWrap, &asErr) {
		t.Error("As(errWrap, &asErr) should return true")
	} else if asErr != errBase {
		t.Errorf("As returns %v, want %v", asErr, errBase)
	}
}
