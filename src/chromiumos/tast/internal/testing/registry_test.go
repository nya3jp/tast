// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"reflect"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
)

// REGISTRYTEST is a public test function with a name that's chosen to be
// appropriate for this file's name (registry_test.go). The obvious choice,
// "RegistryTest", is unavailable since Go's testing package will interpret it
// as itself being a unit test, so let's just pretend that "registry" and "test"
// are acronyms.
func REGISTRYTEST(context.Context, *State) {}

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
	reg := NewRegistry("bundle")
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
	reg := NewRegistry("bundle")
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
	reg := NewRegistry("bundle")
	const origDep = "olddep"
	test := &Test{
		Func:         REGISTRYTEST,
		SoftwareDeps: []string{origDep},
	}
	reg.AddTest(test)
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Registration failed: ", errs)
	}

	// Change the original Test struct's dependency slice's data.
	test.SoftwareDeps[0] = "newdep"

	// The test returned by the registry should still contain the original information.
	tests := reg.AllTests()
	if len(tests) != 1 {
		t.Fatalf("AllTests returned %v; wanted 1 test: ", tests)
	}
	if want := []string{origDep}; !reflect.DeepEqual(tests[0].SoftwareDeps, want) {
		t.Errorf("Test.Deps is %v; want %v", tests[0].SoftwareDeps, want)
	}
}

func TestAddTestConflictingPre(t *gotesting.T) {
	reg := NewRegistry("bundle")

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
	reg := NewRegistry("bundle")
	reg.AddFixture(&Fixture{Name: name}, "pkg")
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatalf("Fixture registration failed: %v", errs)
	}
	reg.AddFixture(&Fixture{Name: name}, "pkg2")
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
		reg := NewRegistry("bundle")
		reg.AddFixture(&Fixture{Name: tc.name}, "pkg")
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
	reg := NewRegistry("bundle")
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
	reg := NewRegistry("bundle")
	allFixts := []*Fixture{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}

	for _, f := range allFixts {
		reg.AddFixture(f, "pkg")
	}
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Registration failed: ", errs)
	}

	want := map[string]*FixtureInstance{
		"a": {Name: "a", Pkg: "pkg", Bundle: "bundle"},
		"b": {Name: "b", Pkg: "pkg", Bundle: "bundle"},
		"c": {Name: "c", Pkg: "pkg", Bundle: "bundle"},
	}
	if diff := cmp.Diff(reg.AllFixtures(), want); diff != "" {
		t.Errorf("Result mismatch (-got +want):\n%v", diff)
	}
}

type varType struct {
	name  string
	value string
}

func (v *varType) Unmarshal(data string) error {
	v.value = data
	return nil
}
func (v *varType) Name() string {
	return v.name
}

// TestAllVars makes sure all registered global variables return correctly.
func TestAllVars(t *gotesting.T) {
	reg := NewRegistry("bundle")
	allVars := map[string]Var{
		"a": &varType{name: "a"},
		"b": &varType{name: "b"},
		"c": &varType{name: "c"},
	}
	for _, v := range allVars {
		reg.AddVar(v)
		if errs := reg.Errors(); len(errs) > 0 {
			t.Fatal("Registration failed: ", errs)
		}
	}
	// Make sure we cannot register all the registered variable again.
	var i int
	for n, v := range allVars {
		reg.AddVar(v)
		i++
		if errs := reg.Errors(); len(errs) != i {
			t.Fatalf("Registration of the variable %q for the second time should result in failure", n)
		}
	}
}

// TestAddVarOnce makes sure global variables can only be register once.
func TestAddVarOnce(t *gotesting.T) {
	reg := NewRegistry("bundle")
	reg.AddVar(&varType{name: "a"})
	if errs := reg.Errors(); len(errs) > 0 {
		t.Fatal("Registration failed: ", errs)
	}
	reg.AddVar(&varType{name: "a"})
	if errs := reg.Errors(); len(errs) == 0 {
		t.Fatal("Variable registration was successful but failure was expected")
	}
}

// TestInitializeVarsOnce makes sure global variables can only be initialized once.
func TestInitializeVarsOnce(t *gotesting.T) {
	reg := NewRegistry("bundle")
	v := varType{name: "a"}
	reg.AddVar(&v)
	vars := map[string]string{"a": "value"}
	if err := reg.InitializeVars(vars); err != nil {
		t.Fatal("InitializeVars failed: ", err)
	}
	if err := reg.InitializeVars(vars); err != nil {
		t.Fatal("Failed to call InitializeVars twice with the same value: ", err)
	}
	if v.value != "value" {
		t.Fatalf("Got %q for v.value; wanted value", v.value)
	}
	vars["a"] = "another value"
	if err := reg.InitializeVars(vars); err == nil {
		t.Fatal("Got no error when InitializeVars was called the second time with different value")
	}
}

// TestInitializeVars tests if InitializeVars works correctly.
func TestInitializeVars(t *gotesting.T) {

	const (
		name1       = `var1`
		name2       = `var2`
		val1        = `value1`
		val2        = `value2`
		defaultVal1 = `v1`
		defaultVal2 = `v2`
	)
	values := map[string]string{
		name1: val1,
		name2: val2,
	}
	reg := NewRegistry("bundle")
	v1 := NewVarString(name1, val1, "")
	v2 := NewVarString(name2, val2, "")
	reg.AddVar(v1)
	reg.AddVar(v2)

	stringVars := map[string]*VarString{
		name1: v1,
		name2: v2,
	}

	if err := reg.InitializeVars(values); err != nil {
		t.Fatal("Failed to call InitializeVars: ", err)
	}
	for k, v := range stringVars {
		expectedVal, ok := values[k]
		if !ok {
			t.Error("Failed to find variable ", k)
		}
		val := v.Value()
		if val != expectedVal {
			t.Errorf("Variable %q has value %q; want %q", k, val, expectedVal)
		}
	}
}
