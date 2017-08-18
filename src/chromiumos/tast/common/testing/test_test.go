// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"path/filepath"
	gotesting "testing"
	"time"
)

func Func1(*State) {}

func TestAssignName(t *gotesting.T) {
	test := Test{Func: Func1}
	if err := test.fillEmptyFields(); err != nil {
		t.Fatal(err)
	}
	const exp = "testing.Func1"
	if test.Name != exp {
		t.Errorf("Name = %q; want %q", test.Name, exp)
	}
}

func TestPreserveHardcodedName(t *gotesting.T) {
	const name = "category.MyName"
	test := Test{Name: name, Func: Func1}
	if err := test.fillEmptyFields(); err != nil {
		t.Fatal(err)
	}
	if test.Name != name {
		t.Errorf("Name = %q; want %q", test.Name, name)
	}
}

func TestDataDir(t *gotesting.T) {
	test := Test{Func: Func1}
	if err := test.fillEmptyFields(); err != nil {
		t.Fatal(err)
	}
	exp := filepath.Join("chromiumos/tast/common/testing", testDataSubdir)
	if test.DataDir() != exp {
		t.Errorf("DataDir() = %q; want %q", test.DataDir(), exp)
	}
}

func TestRejectUnexportedTestFunction(t *gotesting.T) {
	test := Test{Func: func(*State) {}}
	if err := test.fillEmptyFields(); err == nil {
		t.Errorf("Didn't get error for test with unexported test function")
	}
}

func TestSuccess(t *gotesting.T) {
	test := Test{Func: func(*State) {}}
	ch := make(chan Output, 0)
	s := NewState(context.Background(), ch, "", "", "", time.Minute)
	test.Run(s)
	if s.Failed() {
		t.Errorf("Got unexpected error(s) for test: %v", s.Errors())
	}
}

func TestPanic(t *gotesting.T) {
	test := Test{Func: func(*State) { panic("intentional panic") }}
	ch := make(chan Output, 1)
	s := NewState(context.Background(), ch, "", "", "", time.Minute)
	test.Run(s)
	if !s.Failed() {
		t.Errorf("No failure for panicking test")
	} else if len(s.Errors()) != 1 {
		t.Errorf("Got %v errors; want 1", s.Errors())
	}
}

func TestTimeout(t *gotesting.T) {
	test := Test{Func: func(*State) { time.Sleep(10 * time.Millisecond) }}
	ch := make(chan Output, 1)
	s := NewState(context.Background(), ch, "", "", "", time.Millisecond)
	test.Run(s)
	if !s.Failed() {
		t.Errorf("No failure for slow test")
	} else if len(s.Errors()) != 1 {
		t.Errorf("Got %v errors; want 1", s.Errors())
	}
}

func TestCleanUpAfterTimeout(t *gotesting.T) {
	cleanedUp := false
	test := Test{Func: func(s *State) {
		// Wait for the context to report that the deadline has been hit.
		<-s.Context().Done()
		time.Sleep(10 * time.Millisecond)
		cleanedUp = true
	}}
	ch := make(chan Output, 1)
	s := NewState(context.Background(), ch, "", "", "", time.Millisecond)
	test.Run(s)
	if !s.Failed() {
		t.Errorf("No failure for slow test")
	}
	if !cleanedUp {
		t.Errorf("Test didn't clean up after itself")
	}
}
