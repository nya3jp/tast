// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	gotesting "testing"
	"time"
)

func Func1(*State) {}

// getOutputErrors returns all errors from out.
func getOutputErrors(out []Output) []*Error {
	errs := make([]*Error, 0)
	for _, o := range out {
		if o.Err != nil {
			errs = append(errs, o.Err)
		}
	}
	return errs
}

// runTestAndCloseChan runs t using s and then closes s.ch.
func runTestAndCloseChan(t *Test, s *State) {
	t.Run(s)
	close(s.ch)
}

func TestAssignName(t *gotesting.T) {
	test := Test{Func: Func1}
	if err := test.populateNameAndPkg(); err != nil {
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
	if err := test.populateNameAndPkg(); err != nil {
		t.Fatal(err)
	}
	if test.Name != name {
		t.Errorf("Name = %q; want %q", test.Name, name)
	}
}

func TestDataDir(t *gotesting.T) {
	test := Test{Func: Func1}
	if err := test.populateNameAndPkg(); err != nil {
		t.Fatal(err)
	}
	exp := filepath.Join("chromiumos/tast/testing", testDataSubdir)
	if test.DataDir() != exp {
		t.Errorf("DataDir() = %q; want %q", test.DataDir(), exp)
	}
}

func TestSuccess(t *gotesting.T) {
	test := Test{Func: func(*State) {}}
	s := NewState(context.Background(), make(chan Output, 1), "", "", time.Minute)
	go runTestAndCloseChan(&test, s)
	if errs := getOutputErrors(readOutput(s.ch)); len(errs) != 0 {
		t.Errorf("Got unexpected error(s) for test: %v", errs)
	}
}

func TestPanic(t *gotesting.T) {
	test := Test{Func: func(*State) { panic("intentional panic") }}
	s := NewState(context.Background(), make(chan Output, 1), "", "", time.Minute)
	go runTestAndCloseChan(&test, s)
	if errs := getOutputErrors(readOutput(s.ch)); len(errs) != 1 {
		t.Errorf("Got %v errors for panicking test; want 1", errs)
	}
}

func TestTimeout(t *gotesting.T) {
	test := Test{Func: func(*State) { time.Sleep(10 * time.Millisecond) }}
	s := NewState(context.Background(), make(chan Output, 1), "", "", time.Millisecond)
	go runTestAndCloseChan(&test, s)
	if errs := getOutputErrors(readOutput(s.ch)); len(errs) != 1 {
		t.Errorf("Got %v errors for slow test; want 1", errs)
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
	s := NewState(context.Background(), make(chan Output, 1), "", "", time.Millisecond)
	go runTestAndCloseChan(&test, s)
	if errs := getOutputErrors(readOutput(s.ch)); len(errs) != 1 {
		t.Errorf("Got %v errors for slow test; want 1", errs)
	}
	if !cleanedUp {
		t.Errorf("Test didn't clean up after itself")
	}
}

func TestJSON(t *gotesting.T) {
	orig := Test{
		Name: "pkg.Name",
		Func: Func1,
		Desc: "Description",
		Attr: []string{"attr1", "attr2"},
		Data: []string{"foo.txt"},
		Pkg:  "chromiumos/foo/bar",
	}
	b, err := json.Marshal(&orig)
	if err != nil {
		t.Fatalf("Failed to marshal %v: %v", orig, err)
	}
	loaded := Test{}
	if err = json.Unmarshal(b, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal %s: %v", string(b), err)
	}

	// The function should be omitted.
	orig.Func = nil
	if !reflect.DeepEqual(loaded, orig) {
		t.Fatalf("Unmarshaled to %v; want %v", loaded, orig)
	}
}
