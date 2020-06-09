// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	gotesting "testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	testpb "go.chromium.org/chromiumos/config/go/api/test/metadata/v1"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/internal/dep"
	"chromiumos/tast/testing/hwdep"
)

// TESTINSTANCETEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_instance_test.go). The obvious choice, "TestInstanceTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "instance" and "test" are acronyms.
func TESTINSTANCETEST(context.Context, *State) {}

// InvalidTestName is an arbitrary public test function used by unit tests.
func InvalidTestName(context.Context, *State) {}

// fakePre implements both Precondition and preconditionImpl for unit tests.
type fakePre struct {
	name string // name to return from String
}

func (p *fakePre) Prepare(ctx context.Context, s *PreState) interface{} {
	return nil
}

func (p *fakePre) Close(ctx context.Context, s *PreState) {
}

func (p *fakePre) Timeout() time.Duration { return time.Minute }

func (p *fakePre) String() string { return p.name }

func features(available []string, model string) *dep.Features {
	availableSet := make(map[string]struct{})
	for _, dep := range available {
		availableSet[dep] = struct{}{}
	}

	var unavailable []string
	for _, dep := range []string{"dep0", "dep1", "dep2", "dep3"} {
		if _, ok := availableSet[dep]; !ok {
			unavailable = append(unavailable, dep)
		}
	}

	return &dep.Features{
		Software: &dep.SoftwareFeatures{
			Available:   available,
			Unavailable: unavailable,
		},
		Hardware: &dep.HardwareFeatures{
			DC: &device.Config{
				Id: &device.ConfigId{
					ModelId: &device.ModelId{
						Value: model,
					},
				},
			},
		},
	}
}

func TestInstantiate(t *gotesting.T) {
	pre := &fakePre{}
	got, err := instantiate(&Test{
		Func:         TESTINSTANCETEST,
		Desc:         "hello",
		Contacts:     []string{"a@example.com", "b@example.com"},
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{"data1.txt", "data2.txt"},
		Vars:         []string{"var1", "var2"},
		SoftwareDeps: []string{"dep1", "dep2"},
		HardwareDeps: hwdep.D(hwdep.Model("model1", "model2")),
		Pre:          pre,
		Timeout:      123 * time.Second,
		ServiceDeps:  []string{"svc1", "svc2"},
	})
	if err != nil {
		t.Fatal("Failed to instantiate test: ", err)
	}
	want := []*TestInstance{{
		Name:     "testing.TESTINSTANCETEST",
		Pkg:      "chromiumos/tast/internal/testing",
		Desc:     "hello",
		Contacts: []string{"a@example.com", "b@example.com"},
		Attr: []string{
			"group:mainline",
			"informational",
			testNameAttrPrefix + "testing.TESTINSTANCETEST",
			// The bundle name is the second-to-last component in the package's path.
			testBundleAttrPrefix + "internal",
			testDepAttrPrefix + "dep1",
			testDepAttrPrefix + "dep2",
		},
		Data:         []string{"data1.txt", "data2.txt"},
		Vars:         []string{"var1", "var2"},
		SoftwareDeps: []string{"dep1", "dep2"},
		Timeout:      123 * time.Second,
		ServiceDeps:  []string{"svc1", "svc2"},
	}}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(TestInstance{}, "Func", "HardwareDeps", "Pre")); diff != "" {
		t.Errorf("Got unexpected test instances (-got +want):\n%s", diff)
	}
	if len(got) == 1 {
		if got[0].Func == nil {
			t.Error("Got nil Func")
		}
		if result := got[0].ShouldRun(features([]string{"dep1", "dep2"}, "model1")); !result.OK() {
			t.Error("Got unexpected HardwareDeps: ShouldRun returned false for model1: ", result)
		}
		if result := got[0].ShouldRun(features([]string{"dep1", "dep2"}, "modelX")); result.OK() {
			t.Error("Got unexpected HardwareDeps: ShouldRun returned true for modelX")
		}
		if got[0].Pre != pre {
			t.Errorf("Got unexpected Pre: got %v, want %v", got[0].Pre, pre)
		}
	}
}

func TestInstantiateParams(t *gotesting.T) {
	got, err := instantiate(&Test{
		Func:         TESTINSTANCETEST,
		Attr:         []string{"group:crosbolt"},
		Data:         []string{"data0.txt"},
		SoftwareDeps: []string{"dep0"},
		HardwareDeps: hwdep.D(hwdep.Model("model1", "model2")),
		Params: []Param{{
			Val:               123,
			ExtraAttr:         []string{"crosbolt_nightly"},
			ExtraData:         []string{"data1.txt"},
			ExtraSoftwareDeps: []string{"dep1"},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("model2")),
		}, {
			Name:              "foo",
			Val:               456,
			ExtraAttr:         []string{"crosbolt_weekly"},
			ExtraData:         []string{"data2.txt"},
			ExtraSoftwareDeps: []string{"dep2"},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("model1")),
		}},
	})
	if err != nil {
		t.Fatal("Failed to instantiate test: ", err)
	}

	want := []*TestInstance{
		{
			Name: "testing.TESTINSTANCETEST",
			Pkg:  "chromiumos/tast/internal/testing",
			Val:  123,
			Attr: []string{
				"group:crosbolt",
				"crosbolt_nightly",
				testNameAttrPrefix + "testing.TESTINSTANCETEST",
				// The bundle name is the second-to-last component in the package's path.
				testBundleAttrPrefix + "internal",
				testDepAttrPrefix + "dep0",
				testDepAttrPrefix + "dep1",
			},
			Data:         []string{"data0.txt", "data1.txt"},
			SoftwareDeps: []string{"dep0", "dep1"},
		},
		{
			Name: "testing.TESTINSTANCETEST.foo",
			Pkg:  "chromiumos/tast/internal/testing",
			Val:  456,
			Attr: []string{
				"group:crosbolt",
				"crosbolt_weekly",
				testNameAttrPrefix + "testing.TESTINSTANCETEST.foo",
				// The bundle name is the second-to-last component in the package's path.
				testBundleAttrPrefix + "internal",
				testDepAttrPrefix + "dep0",
				testDepAttrPrefix + "dep2",
			},
			Data:         []string{"data0.txt", "data2.txt"},
			SoftwareDeps: []string{"dep0", "dep2"},
		},
	}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(TestInstance{}, "Func", "HardwareDeps", "Pre")); diff != "" {
		t.Errorf("Got unexpected test instances (-got +want):\n%s", diff)
	}
	if len(got) == 2 {
		if got[0].Func == nil {
			t.Error("Got nil Func for the first test instance")
		}
		if result := got[0].ShouldRun(features([]string{"dep0", "dep1"}, "model1")); !result.OK() {
			t.Error("Got unexpected HardwareDeps for first test instance: ShouldRun returned false for model1: ", result)
		}
		if result := got[0].ShouldRun(features([]string{"dep0", "dep1"}, "model2")); result.OK() {
			t.Error("Got unexpected HardwareDeps for first test instance: ShouldRun returned true for model2")
		}
		if got[1].Func == nil {
			t.Error("Got nil Func for the second test instance")
		}
		if result := got[1].ShouldRun(features([]string{"dep0", "dep2"}, "model2")); !result.OK() {
			t.Error("Got unexpected HardwareDeps for second test instance: ShouldRun returned false for model2: ", result)
		}
		if result := got[1].ShouldRun(features([]string{"dep0", "dep2"}, "model1")); result.OK() {
			t.Error("Got unexpected HardwareDeps for second test instance: ShouldRun returned true for model1")
		}
	}
}

func TestInstantiateParamsPre(t *gotesting.T) {
	pre := &fakePre{}

	// Duplicated fields should be rejected.
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Pre:  pre,
		Params: []Param{{
			Pre: pre,
		}},
	}); err == nil {
		t.Error("instantiate succeeded unexpectedly for duplicated Pre")
	}

	// OK if the field in the base test is unset.
	got, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Params: []Param{{
			Pre: pre,
		}},
	})
	if err != nil {
		t.Fatal("Failed to instantiate test: ", err)
	}
	if len(got) != 1 {
		t.Fatalf("Got %d test instances; want 1", len(got))
	}
	if got[0].Pre != pre {
		t.Fatalf("TestInstance.Pre = %v; want %v", got[0].Pre, pre)
	}
}

func TestInstantiateParamsTimeout(t *gotesting.T) {
	const timeout = 123 * time.Second

	// Duplicated fields should be rejected.
	if _, err := instantiate(&Test{
		Func:    TESTINSTANCETEST,
		Timeout: timeout,
		Params: []Param{{
			Timeout: timeout,
		}},
	}); err == nil {
		t.Error("instantiate succeeded unexpectedly for duplicated Timeout")
	}

	// OK if the field in the base test is unset.
	got, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Params: []Param{{
			Timeout: timeout,
		}},
	})
	if err != nil {
		t.Fatal("Failed to instantiate test: ", err)
	}
	if len(got) != 1 {
		t.Fatalf("Got %d test instances; want 1", len(got))
	}
	if got[0].Timeout != timeout {
		t.Fatalf("TestInstance.Timeout = %v; want %v", got[0].Timeout, timeout)
	}
}

func TestDataDir(t *gotesting.T) {
	tests, err := instantiate(&Test{Func: TESTINSTANCETEST})
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 1 {
		t.Fatalf("Got %d test instances; want 1", len(tests))
	}
	test := tests[0]
	exp := filepath.Join("chromiumos/tast/internal/testing", testDataSubdir)
	if test.DataDir() != exp {
		t.Errorf("DataDir() = %q; want %q", test.DataDir(), exp)
	}
}

func TestInstantiateNoFunc(t *gotesting.T) {
	if _, err := instantiate(&Test{}); err == nil {
		t.Error("Didn't get error with missing function")
	}
}

// TestValidateName tests name validation of instantiate.
// It is better to call instantiate instead, but it is difficult to define
// Go functions with corresponding names.
func TestValidateName(t *gotesting.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"example.ChromeLogin", true},
		{"example.ChromeLogin2", true},
		{"example2.ChromeLogin", true},
		{"example.ChromeLogin.stress", true},
		{"example.ChromeLogin.more_stress", true},
		{"example.chromeLogin", false},
		{"example.7hromeLogin", false},
		{"example.Chrome_Login", false},
		{"example.Chrome@Login", false},
		{"Example.ChromeLogin", false},
		{"3xample.ChromeLogin", false},
		{"exam_ple.ChromeLogin", false},
		{"exam@ple.ChromeLogin", false},
		{"example.ChromeLogin.Stress", false},
		{"example.ChromeLogin.more-stress", false},
		{"example.ChromeLogin.more@stress", false},
	} {
		err := validateName(tc.name)
		if err != nil && tc.valid {
			t.Errorf("validateName(%q) failed: %v", tc.name, err)
		} else if err == nil && !tc.valid {
			t.Errorf("validateName(%q) didn't return expected error", tc.name)
		}
	}
}

// TestValidateFileName tests file name validation of instantiate.
// It is better to call instantiate instead, but it is difficult to define
// Go functions with corresponding names.
func TestValidateFileName(t *gotesting.T) {
	for _, tc := range []struct {
		name, fn string
		valid    bool
	}{
		{"Test", "test.go", true},                     // single word
		{"MyTest", "my_test.go", true},                // two words separated with underscores
		{"LoadURL", "load_url.go", true},              // word and acronym
		{"PlayMP3", "play_mp3.go", true},              // word contains numbers
		{"PlayMP3Song", "play_mp3_song.go", true},     // acronym followed by word
		{"ConnectToDBus", "connect_to_dbus.go", true}, // word with multiple leading caps
		{"RestartCrosVM", "restart_crosvm.go", true},  // word with ending acronym
		{"RestartCrosVM", "restart_cros_vm.go", true}, // word followed by acronym
		{"Foo123bar", "foo123bar.go", true},           // word contains digits
		{"Foo123Bar", "foo123_bar.go", true},          // word with trailing digits
		{"Foo123bar", "foo_123bar.go", true},          // word with leading digits
		{"Foo123Bar", "foo_123_bar.go", true},         // word consisting only of digits
		{"foo", "foo.go", false},                      // lowercase func name
		{"Foobar", "foo_bar.go", false},               // lowercase word
		{"FirstTest", "first.go", false},              // func name has word not in filename
		{"Firstblah", "first.go", false},              // func name has word longer than filename
		{"First", "firstabc.go", false},               // filename has word longer than func name
		{"First", "first_test.go", false},             // filename has word not in func name
		{"FooBar", "foo__bar.go", false},              // empty word in filename
		{"Foo", "bar.go", false},                      // completely different words
		{"Foo", "Foo.go", false},                      // non-lowercase filename
		{"Foo", "foo.txt", false},                     // filename without ".go" extension
	} {
		err := validateFileName(tc.name, tc.fn)
		if err != nil && tc.valid {
			t.Errorf("validateFileName(%q, %q) failed: %v", tc.name, tc.fn, err)
		} else if err == nil && !tc.valid {
			t.Errorf("validateFileName(%q, %q) didn't return expected error", tc.name, tc.fn)
		}
	}
}

// TestInstantiateFuncName makes sure the validateFileName runs against the name
// derived from the Func's function name and its source file name.
func TestInstantiateFuncName(t *gotesting.T) {
	if _, err := instantiate(&Test{Func: TESTINSTANCETEST}); err != nil {
		t.Error("instantiate failed: ", err)
	}
	if _, err := instantiate(&Test{Func: InvalidTestName}); err == nil {
		t.Error("instantiate succeeded unexpectedly for wrongly named test func")
	}
}

func TestInstantiateDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Data: []string{"foo", "bar/baz"},
	}); err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}
}

func TestInstantiateUncleanDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Data: []string{"foo", "bar/../bar/baz"},
	}); err == nil {
		t.Error("Did not get an error with unclean path")
	}
}

func TestInstantiateAbsoluteDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Data: []string{"foo", "/etc/passwd"},
	}); err == nil {
		t.Error("Did not get an error with absolute path")
	}
}

func TestInstantiateRelativeDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Data: []string{"foo", "../baz"},
	}); err == nil {
		t.Error("Did not get an error with relative path")
	}
}

func TestInstantiateReservedAttrPrefixes(t *gotesting.T) {
	for _, attr := range []string{
		testNameAttrPrefix + "foo",
		testBundleAttrPrefix + "bar",
		testDepAttrPrefix + "dep",
	} {
		if _, err := instantiate(&Test{
			Func: TESTINSTANCETEST,
			Attr: []string{attr},
		}); err == nil {
			t.Errorf("Did not get an error for reserved attribute %q", attr)
		}
	}
}
func TestValidateVars_OK(t *gotesting.T) {
	const (
		category = "a"
		name     = "B"
	)
	for _, vars := range [][]string{
		{"x"},
		{"a.B.c", "a.c"},
		{"a.B.cC1_"},
	} {
		if err := validateVars(category, name, vars); err != nil {
			t.Errorf("validateVars(%v, %v, %v) = %v; want nil", category, name, vars, err)
		}
	}
}

func TestValidateVars_Error(t *gotesting.T) {
	const (
		category = "a"
		name     = "B"
	)
	for _, vars := range [][]string{
		{"a."},
		{"a.c", "a."},
		{"a.B."},
		{"x.c"},
		{"a.X.c"},
		{"a.B.c."},
		{"a.B._"},
	} {
		if err := validateVars(category, name, vars); err == nil {
			t.Errorf("validateVars(%v, %v, %v) = nil; want error", category, name, vars)
		}
	}
}

func TestInstantiateNegativeTimeout(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func:    TESTINSTANCETEST,
		Timeout: -1 * time.Second,
	}); err == nil {
		t.Error("Didn't get error with negative timeout")
	}
}

func TestInstantiateDuplicatedParamName(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Params: []Param{{
			Name: "abc",
		}, {
			Name: "abc",
		}},
	}); err == nil {
		t.Error("Did not get an error with duplicated param case names")
	}
}

func TestInstantiateInconsistentParamValType(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Params: []Param{{
			Name: "case1",
			Val:  1,
		}, {
			Name: "case2",
			Val:  "string",
		}},
	}); err == nil {
		t.Error("Did not get an error with param cases containing different value type")
	}
}

func TestInstantiateNoManualDisabled(t *gotesting.T) {
	for _, attrs := range [][]string{
		{"disabled"},
		{"group:mainline", "disabled"},
		{"group:crosbolt", "disabled"},
	} {
		if _, err := instantiate(&Test{
			Func: TESTINSTANCETEST,
			Attr: attrs,
		}); err == nil {
			t.Errorf("instantiate unexpectedly succeeded with Attr = %q", attrs)
		}
	}
}

func TestInstantiateCompatAttrs(t *gotesting.T) {
	got, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
	})
	if err != nil {
		t.Fatal("Failed to instantiate test: ", err)
	}
	want := []*TestInstance{{
		Name: "testing.TESTINSTANCETEST",
		Pkg:  "chromiumos/tast/internal/testing",
		Attr: []string{
			testNameAttrPrefix + "testing.TESTINSTANCETEST",
			// The bundle name is the second-to-last component in the package's path.
			testBundleAttrPrefix + "internal",
			// This attribute is added for compatibility.
			"disabled",
		},
	}}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(TestInstance{}, "Func", "HardwareDeps", "Pre")); diff != "" {
		t.Errorf("Got unexpected test instances (-got +want):\n%s", diff)
	}
}

func TestSoftwareDeps(t *gotesting.T) {
	test := TestInstance{SoftwareDeps: []string{"dep3", "dep1", "dep2", "depX"}}
	got := test.ShouldRun(features([]string{"dep0", "dep2", "dep4"}, "eve"))
	want := &ShouldRunResult{
		SkipReasons: []string{"missing SoftwareDeps: dep1, dep3, depX"},
		Errors:      []string{"unknown SoftwareDeps: depX"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ShouldRun() = %+v; want %+v", got, want)
	}
}

func TestHardwareDeps(t *gotesting.T) {
	test := TestInstance{HardwareDeps: hwdep.D(hwdep.Model("eve"))}
	got := test.ShouldRun(features(nil, "samus"))
	want := &ShouldRunResult{SkipReasons: []string{"ModelId did not match"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ShouldRun() = %+v; want %+v", got, want)
	}
}

func TestJSON(t *gotesting.T) {
	orig := TestInstance{
		Func: TESTINSTANCETEST,
		Desc: "Description",
		Attr: []string{"attr1", "attr2"},
		Data: []string{"foo.txt"},
		Pkg:  "chromiumos/foo/bar",
	}
	b, err := json.Marshal(&orig)
	if err != nil {
		t.Fatalf("Failed to marshal %v: %v", orig, err)
	}
	loaded := TestInstance{}
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
	softwareDeps := []string{"sw1", "sw2"}
	serviceDeps := []string{"svc1", "svc2"}
	f := func(context.Context, *State) {}

	// Checks that tst's fields still contain the above values.
	checkTest := func(msg string, tst *TestInstance) {
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
		if !reflect.DeepEqual(tst.SoftwareDeps, softwareDeps) {
			t.Errorf("%s set SoftwareDeps to %v; want %v", msg, tst.SoftwareDeps, softwareDeps)
		}
		if !reflect.DeepEqual(tst.ServiceDeps, serviceDeps) {
			t.Errorf("%s set ServiceDeps to %v; want %v", msg, tst.ServiceDeps, serviceDeps)
		}
		if tst.Timeout != timeout {
			t.Errorf("%s set Timeout to %v; want %v", msg, tst.Timeout, timeout)
		}
	}

	// First check that a cloned copy gets the correct values.
	orig := TestInstance{
		Name:         name,
		Func:         f,
		Attr:         append([]string(nil), attr...),
		SoftwareDeps: append([]string(nil), softwareDeps...),
		ServiceDeps:  append([]string(nil), serviceDeps...),
		Timeout:      timeout,
	}
	clone := orig.clone()
	checkTest("clone()", clone)

	// Now update fields in the copy and check that the original is unaffected.
	clone.Name = "NewTest"
	clone.Func = nil
	clone.Attr[0] = "new"
	clone.Timeout = 2 * timeout
	clone.SoftwareDeps[0] = "swnew"
	clone.ServiceDeps[0] = "svcnew"
	checkTest("update after clone()", &orig)
}

func TestSortTests(t *gotesting.T) {
	pre1 := &fakePre{name: "pre1"}
	pre2 := &fakePre{name: "pre2"}

	// Assign names with different leading digits to make sure we don't sort by name primarily.
	t1 := &TestInstance{Name: "3-test1", Pre: nil}
	t2 := &TestInstance{Name: "4-test2", Pre: nil}
	t3 := &TestInstance{Name: "1-test3", Pre: pre1}
	t4 := &TestInstance{Name: "2-test4", Pre: pre1}
	t5 := &TestInstance{Name: "0-test5", Pre: pre2}
	tests := []*TestInstance{t4, t2, t3, t5, t1}

	getNames := func(tests []*TestInstance) (names []string) {
		for _, test := range tests {
			names = append(names, test.Name)
		}
		return names
	}

	in := getNames(tests)
	SortTests(tests)
	actual := getNames(tests)
	expected := getNames([]*TestInstance{t1, t2, t3, t4, t5})
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Sort(%v) = %v; want %v", in, actual, expected)
	}
}

func TestWriteTestsAsProto(t *gotesting.T) {
	in := []*TestInstance{
		{
			Name:         "test001",
			Attr:         []string{"attr1", "attr2"},
			HardwareDeps: hwdep.D(),
			Contacts: []string{
				"someone1@chromium.org",
				"someone2@chromium.org",
			},
		},
	}
	expected := testpb.Specification{
		RemoteTestDrivers: []*testpb.RemoteTestDriver{
			{
				Name: "remoteTestDrivers/tast",
				Tests: []*testpb.Test{
					{
						Name: "remoteTestDrivers/tast/tests/test001",
						Attributes: []*testpb.Attribute{
							{Name: "attr1"},
							{Name: "attr2"},
						},

						// dutconstraint is tested separately in TestHardwareDepsCEL.

						Informational: &testpb.Informational{
							Authors: []*testpb.Contact{
								{Type: &testpb.Contact_Email{Email: "someone1@chromium.org"}},
								{Type: &testpb.Contact_Email{Email: "someone2@chromium.org"}},
							},
						},
					},
				},
			},
		},
	}
	var b bytes.Buffer
	WriteTestsAsProto(&b, in)
	var actual testpb.Specification
	proto.Unmarshal(b.Bytes(), &actual)
	if !cmp.Equal(expected, actual, cmp.Comparer(proto.Equal)) {
		t.Errorf("WriteTestsAsProto(%v): got %v; want %v", in, actual, expected)
	}
}
