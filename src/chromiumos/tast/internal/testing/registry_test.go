// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"reflect"
	gotesting "testing"

	"google.golang.org/grpc"
)

// testsEqual returns true if a and b contain tests with matching fields.
// This is useful when comparing slices that contain copies of the same underlying tests.
func testsEqual(a, b []*TestInstance) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// Just do a basic comparison, since we clone tests and set additional attributes
		// when adding them to the registry.
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

// getDupeTestPtrs returns pointers present in both a and b.
func getDupeTestPtrs(a, b []*TestInstance) []*TestInstance {
	am := make(map[*TestInstance]struct{}, len(a))
	for _, t := range a {
		am[t] = struct{}{}
	}
	var dupes []*TestInstance
	for _, t := range b {
		if _, ok := am[t]; ok {
			dupes = append(dupes, t)
		}
	}
	return dupes
}

func TestAllTests(t *gotesting.T) {
	reg := NewRegistry()
	allTests := []*TestInstance{
		&TestInstance{Name: "test.Foo", Func: func(context.Context, *State) {}},
		&TestInstance{Name: "test.Bar", Func: func(context.Context, *State) {}},
	}
	for _, test := range allTests {
		if err := reg.AddTestInstance(test); err != nil {
			t.Fatal(err)
		}
	}

	tests := reg.AllTests()
	if !testsEqual(tests, allTests) {
		t.Errorf("AllTests() = %v; want %v", tests, allTests)
	}
	if dupes := getDupeTestPtrs(tests, allTests); len(dupes) != 0 {
		t.Errorf("AllTests() returned non-copied test(s): %v", dupes)
	}
}

func TestAddTestDuplicateName(t *gotesting.T) {
	const name = "test.Foo"
	reg := NewRegistry()
	if err := reg.AddTestInstance(&TestInstance{Name: name, Func: func(context.Context, *State) {}}); err != nil {
		t.Fatal("Failed to add initial test: ", err)
	}
	if err := reg.AddTestInstance(&TestInstance{Name: name, Func: func(context.Context, *State) {}}); err == nil {
		t.Fatal("Duplicate test name unexpectedly not rejected")
	}
}

func TestAddTestModifyOriginal(t *gotesting.T) {
	reg := NewRegistry()
	const origName = "test.OldName"
	const origDep = "olddep"
	test := &TestInstance{
		Name:         origName,
		Func:         func(context.Context, *State) {},
		SoftwareDeps: []string{origDep},
	}
	if err := reg.AddTestInstance(test); err != nil {
		t.Fatal("AddTest failed: ", err)
	}

	// Change the original Test struct's name and modify the dependency slice's data.
	test.Name = "test.NewName"
	test.SoftwareDeps[0] = "newdep"

	// The test returned by the registry should still contain the original information.
	tests := reg.AllTests()
	if len(tests) != 1 {
		t.Fatalf("AllTests returned %v; wanted 1 test: ", tests)
	}
	if tests[0].Name != origName {
		t.Errorf("Test.Name is %q; want %q", tests[0].Name, origName)
	}
	if want := []string{origDep}; !reflect.DeepEqual(tests[0].SoftwareDeps, want) {
		t.Errorf("Test.Deps is %v; want %v", tests[0].SoftwareDeps, want)
	}
}

func TestAllServices(t *gotesting.T) {
	reg := NewRegistry()
	allSvcs := []*Service{
		{Register: func(*grpc.Server, *ServiceState) {}},
		{Register: func(*grpc.Server, *ServiceState) {}},
		{Register: func(*grpc.Server, *ServiceState) {}},
	}

	for _, svc := range allSvcs {
		if err := reg.AddService(svc); err != nil {
			t.Fatal(err)
		}
	}

	svcs := reg.AllServices()
	if !reflect.DeepEqual(svcs, allSvcs) {
		t.Errorf("AllServices() = %v; want %v", svcs, allSvcs)
	}
}
