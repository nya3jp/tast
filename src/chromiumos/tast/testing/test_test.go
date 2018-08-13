// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
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

func TestAutoAttr(t *gotesting.T) {
	// The bundle name is the second-to-last component in the package's path.
	test := Test{
		Name:         "category.Name",
		Pkg:          "org/chromium/tast/mybundle/category",
		Attr:         []string{"attr1", "attr2"},
		SoftwareDeps: []string{"dep1", "dep2"},
	}
	if err := test.addAutoAttributes(); err != nil {
		t.Fatal("addAutoAttributes failed: ", err)
	}
	exp := []string{
		"attr1",
		"attr2",
		testNameAttrPrefix + "category.Name",
		testBundleAttrPrefix + "mybundle",
		testDepAttrPrefix + "dep1",
		testDepAttrPrefix + "dep2",
	}
	sort.Strings(test.Attr)
	sort.Strings(exp)
	if !reflect.DeepEqual(test.Attr, exp) {
		t.Errorf("Attr = %v; want %v", test.Attr, exp)
	}
}

func TestReservedAttrPrefixes(t *gotesting.T) {
	test := Test{Name: "cat.Test"}
	for _, attr := range []string{
		testNameAttrPrefix + "foo",
		testBundleAttrPrefix + "bar",
		testDepAttrPrefix + "dep",
	} {
		test.Attr = []string{attr}
		if err := test.addAutoAttributes(); err == nil {
			t.Errorf("addAutoAttributes didn't return error for reserved attribute %q", attr)
		}
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

func TestSoftwareDeps(t *gotesting.T) {
	test := Test{Func: Func1, SoftwareDeps: []string{"dep3", "dep1", "dep2"}}
	missing := test.MissingSoftwareDeps([]string{"dep0", "dep2", "dep4"})
	if exp := []string{"dep1", "dep3"}; !reflect.DeepEqual(missing, exp) {
		t.Errorf("MissingSoftwareDeps() = %v; want %v", missing, exp)
	}
}

func TestRunSuccess(t *gotesting.T) {
	test := Test{Func: func(*State) {}, Timeout: time.Minute}
	s := NewState(context.Background(), &test, make(chan Output, 1), "", "")
	test.Run(s)
	if errs := getOutputErrors(readOutput(s.ch)); len(errs) != 0 {
		t.Errorf("Got unexpected error(s) for test: %v", errs)
	}
}

func TestRunPanic(t *gotesting.T) {
	test := Test{Func: func(*State) { panic("intentional panic") }, Timeout: time.Minute}
	s := NewState(context.Background(), &test, make(chan Output, 1), "", "")
	test.Run(s)
	if errs := getOutputErrors(readOutput(s.ch)); len(errs) != 1 {
		t.Errorf("Got %v errors for panicking test; want 1", errs)
	}
}

func TestRunDeadline(t *gotesting.T) {
	f := func(s *State) {
		// Wait for the context to report that the deadline has been hit.
		<-s.Context().Done()
		s.Error("Saw timeout within test")
	}
	test := Test{Func: f, Timeout: time.Millisecond, CleanupTimeout: 10 * time.Second}
	s := NewState(context.Background(), &test, make(chan Output, 1), "", "")
	test.Run(s)
	// The error that was reported by the test after its deadline was hit
	// but within the cleanup delay should be available.
	if errs := getOutputErrors(readOutput(s.ch)); len(errs) != 1 {
		t.Errorf("Got %v errors for timed-out test; want 1", len(errs))
	}
}

func TestRunLogAfterTimeout(t *gotesting.T) {
	cont := make(chan bool)
	done := make(chan bool)
	f := func(s *State) {
		// Report when we're done, either after completing or after panicking before completion.
		completed := false
		defer func() { done <- completed }()

		// Ignore the deadline and wait until we're told to continue.
		<-s.Context().Done()
		<-cont
		s.Log("Done waiting")
		completed = true
	}
	test := Test{Func: f, Timeout: time.Millisecond, CleanupTimeout: time.Millisecond}

	out := make(chan Output, 10)
	test.Run(NewState(context.Background(), &test, out, "", ""))

	// Tell the test to continue even though Run has already returned. The output channel should
	// still be open so as to avoid a panic when the test writes to it: https://crbug.com/853406
	cont <- true
	if completed := <-done; !completed {
		t.Error("Test function didn't complete")
	}
	// No output errors should be written to the channel; reporting timeouts is the caller's job.
	if errs := getOutputErrors(readOutput(out)); len(errs) != 0 {
		t.Errorf("Got %v error(s) for runaway test; want 0", len(errs))
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

func TestTestClone(t *gotesting.T) {
	const (
		name    = "OldTest"
		timeout = time.Minute
	)
	attr := []string{"a", "b"}
	f := func(s *State) {}

	// Checks that tst's fields still contain the above values.
	checkTest := func(msg string, tst *Test) {
		if tst.Name != name {
			t.Errorf("%s set Name to %q; want %q", msg, tst.Name, name)
		}
		// Go doesn't allow checking functions for equality.
		if tst.Func == nil {
			t.Errorf("%s set Func to nil; want %p", msg, f)
		}
		if !reflect.DeepEqual(tst.Attr, attr) {
			t.Errorf("%s set Attr to %v; want %v", msg, tst.Attr, attr)
		}
		if tst.SoftwareDeps != nil {
			t.Errorf("%s set SoftwareDeps to %v; want nil", msg, tst.SoftwareDeps)
		}
		if tst.Timeout != timeout {
			t.Errorf("%s set Timeout to %v; want %v", msg, tst.Timeout, timeout)
		}
	}

	// First check that a cloned copy gets the correct values.
	orig := Test{
		Name:    name,
		Func:    f,
		Attr:    append([]string{}, attr...),
		Timeout: timeout,
	}
	clone := orig.clone()
	checkTest("clone()", clone)

	// Now update fields in the copy and check that the original is unaffected.
	clone.Name = "NewTest"
	clone.Func = nil
	clone.Attr[0] = "new"
	clone.Timeout = 2 * timeout
	checkTest("update after clone()", &orig)
}
