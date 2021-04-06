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
		{Name: "test.Foo", Func: func(context.Context, *State) {}},
		{Name: "test.Bar", Func: func(context.Context, *State) {}},
	}
	for _, test := range allTests {
		reg.AddTestInstance(test)
	}
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Registration failed: ", errs)
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
	reg.AddTestInstance(&TestInstance{Name: name, Func: func(context.Context, *State) {}})
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Failed to add initial test: ", errs)
	}
	reg.AddTestInstance(&TestInstance{Name: name, Func: func(context.Context, *State) {}})
	if errs := reg.Errors(); len(errs) == 0 {
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
	reg.AddTestInstance(test)
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Registration failed: ", errs)
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

func TestAddTestConflictingPre(t *gotesting.T) {
	reg := NewRegistry()

	// There are different preconditions with the same name. Registering tests
	// using them should result in errors.
	pre1 := &fakePre{"fake_pre"}
	pre2 := &fakePre{"fake_pre"}

	reg.AddTestInstance(&TestInstance{
		Name: "pkg.Test1",
		Func: func(context.Context, *State) {},
		Pre:  pre1,
	})
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("AddTestInstance failed for pkg.Test1: ", errs)
	}

	reg.AddTestInstance(&TestInstance{
		Name: "pkg.Test2",
		Func: func(context.Context, *State) {},
		Pre:  pre1,
	})
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("AddTestInstance failed for pkg.Test2: ", errs)
	}

	reg.AddTestInstance(&TestInstance{
		Name: "pkg.Test3",
		Func: func(context.Context, *State) {},
		Pre:  pre2,
	})
	if errs := reg.Errors(); len(errs) == 0 {
		t.Fatal("AddTestInstance unexpectedly succeeded for pkg.Test3")
	}
}

func TestAddFixtureDuplicateName(t *gotesting.T) {
	const name = "foo"
	reg := NewRegistry()
	reg.AddFixture(&Fixture{Name: name})
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatalf("Fixture registration failed: %v", errs)
	}
	reg.AddFixture(&Fixture{Name: name})
	if errs := reg.Errors(); len(errs) == 0 {
		t.Error("Duplicated fixture registration succeeded unexpectedly")
	}
}

func TestAddFixtureInvalidName(t *gotesting.T) {
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"", false},
		{"a", true},
		{"A", false},
		{"1", false},
		{"%", false},
		{"abc", true},
		{"aBc", true},
		{"a1r", true},
		{"a1R", true},
		{"a r", false},
		{"a_r", false},
		{"a-r", false},
		{"chromeLoggedIn", true},
		{"ieee1394", true},
	} {
		reg := NewRegistry()
		reg.AddFixture(&Fixture{Name: tc.name})
		errs := reg.Errors()
		if tc.ok && len(errs) > 0 {
			t.Errorf("AddFixture(%q) failed: %v", tc.name, errs)
		}
		if !tc.ok && len(errs) == 0 {
			t.Errorf("AddFixture(%q) passed unexpectedly", tc.name)
		}
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
		reg.AddService(svc)
	}
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Registration failed: ", errs)
	}

	svcs := reg.AllServices()
	if !reflect.DeepEqual(svcs, allSvcs) {
		t.Errorf("AllServices() = %v; want %v", svcs, allSvcs)
	}
}

func TestAllFixtures(t *gotesting.T) {
	reg := NewRegistry()
	allFixts := []*Fixture{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}

	for _, f := range allFixts {
		reg.AddFixture(f)
	}
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Registration failed: ", errs)
	}

	fixts := reg.AllFixtures()
	if len(fixts) != 3 {
		t.Errorf("len(AllFixtures()) = %d; want 3", len(fixts))
	}
}
