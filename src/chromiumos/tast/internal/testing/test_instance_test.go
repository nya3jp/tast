// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
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
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/testing/hwdep"
	"chromiumos/tast/testutil"
)

// TESTINSTANCETEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_instance_test.go). The obvious choice, "TestInstanceTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "instance" and "test" are acronyms.
func TESTINSTANCETEST(context.Context, *State) {}

// InvalidTestName is an arbitrary public test function used by unit tests.
func InvalidTestName(context.Context, *State) {}

// testPre implements both Precondition and preconditionImpl for unit tests.
type testPre struct {
	prepareFunc func(context.Context, *State) interface{}
	closeFunc   func(context.Context, *State)
	name        string // name to return from String
}

func (p *testPre) Prepare(ctx context.Context, s *State) interface{} {
	if p.prepareFunc != nil {
		return p.prepareFunc(ctx, s)
	}
	return nil
}

func (p *testPre) Close(ctx context.Context, s *State) {
	if p.closeFunc != nil {
		p.closeFunc(ctx, s)
	}
}

func (p *testPre) Timeout() time.Duration { return time.Minute }

func (p *testPre) String() string { return p.name }

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
	pre := &testPre{}
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
	pre := &testPre{}

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

func TestHardwareDepsCEL(t *gotesting.T) {
	for i, c := range []struct {
		input    hwdep.Deps
		expected string
	}{
		{hwdep.D(hwdep.Model("model1", "model2")), "not_implemented"},
		{hwdep.D(hwdep.SkipOnModel("model1", "model2")), "not_implemented"},
		{hwdep.D(hwdep.Platform("platform_id1", "platform_id2")), "not_implemented"},
		{hwdep.D(hwdep.SkipOnPlatform("platform_id1", "platform_id2")), "not_implemented"},
		{hwdep.D(hwdep.TouchScreen()), "dut.hardware_features.screen.touch_support == api.HardwareFeatures.Present.PRESENT"},
		{hwdep.D(hwdep.Fingerprint()), "dut.hardware_features.fingerprint.location != api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT"},
		{hwdep.D(hwdep.InternalDisplay()), "dut.hardware_features.screen.milliinch.value != 0U"},
		{hwdep.D(hwdep.Wifi80211ac()), "not_implemented"},

		{hwdep.D(hwdep.TouchScreen(), hwdep.Fingerprint()),
			"dut.hardware_features.screen.touch_support == api.HardwareFeatures.Present.PRESENT && dut.hardware_features.fingerprint.location != api.HardwareFeatures.Fingerprint.Location.NOT_PRESENT"},
		{hwdep.D(hwdep.Model("model1", "model2"), hwdep.SkipOnPlatform("id1", "id2")), "not_implemented && not_implemented"},
	} {
		actual := c.input.CEL()
		if actual != c.expected {
			t.Errorf("TestHardwareDepsCEL[%d]: got %q; want %q", i, actual, c.expected)
		}
	}
}

func TestRunSuccess(t *gotesting.T) {
	test := TestInstance{Func: func(context.Context, *State) {}, Timeout: time.Minute}
	or := newOutputReader()
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	od := filepath.Join(td, "out")
	test.Run(context.Background(), or.ch, &TestConfig{OutDir: od})
	if errs := getOutputErrors(or.read()); len(errs) != 0 {
		t.Errorf("Got unexpected error(s) for test: %v", errs)
	}
	if fi, err := os.Stat(od); err != nil {
		t.Errorf("Out dir %v not accessible after testing: %v", od, err)
	} else if mode := fi.Mode()&os.ModePerm | os.ModeSticky; mode != 0777|os.ModeSticky {
		t.Errorf("Out dir %v has mode 0%o; want 0%o", od, mode, 0777|os.ModeSticky)
	}
}

func TestRunPanic(t *gotesting.T) {
	test := TestInstance{Func: func(context.Context, *State) { panic("intentional panic") }, Timeout: time.Minute}
	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	if errs := getOutputErrors(or.read()); len(errs) != 1 {
		t.Errorf("Got %v errors for panicking test; want 1", errs)
	}
}

func TestRunDeadline(t *gotesting.T) {
	f := func(ctx context.Context, s *State) {
		// Wait for the context to report that the deadline has been hit.
		<-ctx.Done()
		s.Error("Saw timeout within test")
	}
	test := TestInstance{Func: f, Timeout: time.Millisecond, ExitTimeout: 10 * time.Second}
	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	// The error that was reported by the test after its deadline was hit
	// but within the exit delay should be available.
	if errs := getOutputErrors(or.read()); len(errs) != 1 {
		t.Errorf("Got %v errors for timed-out test; want 1", len(errs))
	}
}

func TestRunLogAfterTimeout(t *gotesting.T) {
	cont := make(chan bool)
	done := make(chan bool)
	f := func(ctx context.Context, s *State) {
		// Report when we're done, either after completing or after panicking before completion.
		completed := false
		defer func() { done <- completed }()

		// Ignore the deadline and wait until we're told to continue.
		<-ctx.Done()
		<-cont
		s.Log("Done waiting")
		completed = true
	}
	test := TestInstance{Func: f, Timeout: time.Millisecond, ExitTimeout: time.Millisecond}

	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})

	// Tell the test to continue even though Run has already returned. The output channel should
	// still be open so as to avoid a panic when the test writes to it: https://crbug.com/853406
	cont <- true
	if completed := <-done; !completed {
		t.Error("Test function didn't complete")
	}
	// No output errors should be written to the channel; reporting timeouts is the caller's job.
	if errs := getOutputErrors(or.read()); len(errs) != 0 {
		t.Errorf("Got %v error(s) for runaway test; want 0", len(errs))
	}
}

func TestRunLateWriteFromGoroutine(t *gotesting.T) {
	// Run a test that calls s.Error from a goroutine after the test has finished.
	start := make(chan struct{}) // tells goroutine to start
	end := make(chan struct{})   // announces goroutine is done
	test := TestInstance{Func: func(ctx context.Context, s *State) {
		go func() {
			<-start
			s.Error("This message should be discarded since the test is done")
			close(end)
		}()
	}, Timeout: time.Minute}
	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})

	// Tell the goroutine to start and wait for it to finish.
	close(start)
	<-end

	// No errors should be reported, and we also shouldn't panic due to
	// the s.Error call trying to write to a closed channel.
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}
}

func TestRunSkipStages(t *gotesting.T) {
	type action int // actions that can be performed by stages
	const (
		pass    action = iota
		doError        // call State.Error
		doFatal        // call State.Fatal
		doPanic        // call panic()
		noCall         // stage should be skipped
	)

	// Define a sequence of tests to run and specify which stages should be executed for each.
	var pre, pre2, pre3, pre4 testPre
	cases := []struct {
		pre                *testPre
		preTestAction      action // TestConfig.PreTestFunc
		prepareAction      action // Precondition.Prepare
		testAction         action // Test.Func
		closeAction        action // Precondition.Close
		postTestAction     action // TestConfig.PostTestFunc
		postTestHookAction action // Return of TestConfig.PreTestFunc
		desc               string
	}{
		{&pre, pass, pass, pass, noCall, pass, pass, "everything passes"},
		{&pre, doError, noCall, noCall, noCall, pass, pass, "pre-test fails"},
		{&pre, doPanic, noCall, noCall, noCall, pass, pass, "pre-test panics"},
		{&pre, pass, doError, noCall, noCall, pass, pass, "prepare fails"},
		{&pre, pass, doPanic, noCall, noCall, pass, pass, "prepare panics"},
		{&pre, pass, pass, doError, noCall, pass, pass, "test fails"},
		{&pre, pass, pass, doPanic, noCall, pass, pass, "test panics"},
		{&pre, pass, pass, pass, pass, pass, pass, "everything passes, next test has different precondition"},
		{&pre2, pass, doError, noCall, pass, pass, pass, "prepare fails, next test has different precondition"},
		{&pre3, pass, pass, doError, pass, pass, pass, "test fails, next test has no precondition"},
		{nil, pass, noCall, pass, noCall, pass, pass, "no precondition"},
		{&pre4, pass, pass, pass, pass, pass, pass, "final test"},
	}

	// Create tests first so we can set TestConfig.NextTest later.
	var tests []*TestInstance
	for _, c := range cases {
		test := &TestInstance{Timeout: time.Minute}
		// We can't just do "test.Pre = c.pre" here. See e.g. https://tour.golang.org/methods/12:
		// "Note that an interface value that holds a nil concrete value is itself non-nil."
		if c.pre != nil {
			test.Pre = c.pre
		}
		tests = append(tests, test)
	}

	// makeFunc returns a function that sets *called to true and performs the action described by a.
	makeFunc := func(a action, called *bool) func(context.Context, *State) {
		return func(ctx context.Context, s *State) {
			*called = true
			switch a {
			case doError:
				s.Error("intentional error")
			case doFatal:
				s.Fatal("intentional fatal")
			case doPanic:
				panic("intentional panic")
			}
		}
	}

	makeFuncWithCallback := func(a action, called *bool, cbA action, cbCalled *bool) func(ctx context.Context, s *State) func(ctx context.Context, s *State) {
		return func(ctx context.Context, s *State) func(ctx context.Context, s *State) {
			*called = true
			switch a {
			case doError:
				s.Error("intentional error")
			case doFatal:
				s.Fatal("intentional fatal")
			case doPanic:
				panic("intentional panic")
			}

			return makeFunc(cbA, cbCalled)
		}
	}

	// Now actually run each test.
	for i, c := range cases {
		var preTestRan, prepareRan, testRan, closeRan, postTestRan, postTestHookRan bool

		test := tests[i]
		test.Func = makeFunc(c.testAction, &testRan)
		if c.pre != nil {
			prepare := makeFunc(c.prepareAction, &prepareRan)
			c.pre.prepareFunc = func(ctx context.Context, s *State) interface{} {
				prepare(ctx, s)
				return nil
			}
			c.pre.closeFunc = makeFunc(c.closeAction, &closeRan)
		}
		cfg := &TestConfig{
			PreTestFunc:  makeFuncWithCallback(c.preTestAction, &preTestRan, c.postTestHookAction, &postTestHookRan),
			PostTestFunc: makeFunc(c.postTestAction, &postTestRan),
		}
		if i < len(tests)-1 {
			cfg.NextTest = tests[i+1]
		}

		or := newOutputReader()
		test.Run(context.Background(), or.ch, cfg)

		// Verify that stages were executed or skipped as expected.
		checkRan := func(name string, ran bool, a action) {
			wantRun := a != noCall
			if !ran && wantRun {
				t.Errorf("Test %d (%s) didn't run %s", i, c.desc, name)
			} else if ran && !wantRun {
				t.Errorf("Test %d (%s) ran %s unexpectedly", i, c.desc, name)
			}
		}
		checkRan("TestConfig.PreTestFunc", preTestRan, c.preTestAction)
		checkRan("Precondition.Prepare", prepareRan, c.prepareAction)
		checkRan("Test.Func", testRan, c.testAction)
		checkRan("Precondition.Close", closeRan, c.closeAction)
		checkRan("TestConfig.PostTestFunc", postTestRan, c.postTestAction)
	}
}

func TestRunMissingData(t *gotesting.T) {
	const (
		existingFile      = "existing.txt"
		missingFile1      = "missing1.txt"
		missingFile2      = "missing2.txt"
		missingErrorFile1 = missingFile1 + ExternalErrorSuffix
	)

	test := TestInstance{
		Func:    func(context.Context, *State) {},
		Data:    []string{existingFile, missingFile1, missingFile2},
		Timeout: time.Minute,
	}

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	if err := ioutil.WriteFile(filepath.Join(td, existingFile), nil, 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", existingFile, err)
	}
	if err := ioutil.WriteFile(filepath.Join(td, missingErrorFile1), []byte("some reason"), 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", missingErrorFile1, err)
	}

	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{DataDir: td})

	expected := []string{
		"Required data file missing1.txt missing: some reason",
		"Required data file missing2.txt missing",
	}
	if errs := getOutputErrors(or.read()); len(errs) != 2 {
		t.Errorf("Got %v errors for missing data test; want 2", errs)
	} else if actual := []string{errs[0].Reason, errs[1].Reason}; !reflect.DeepEqual(actual, expected) {
		t.Errorf("Got errors %q; want %q", actual, expected)
	}
}

func TestRunPrecondition(t *gotesting.T) {
	type data struct{}
	preData := &data{}

	// The test should be able to access the data via State.PreValue.
	test := &TestInstance{
		// Use a precondition that returns the struct that we declared earlier from its Prepare method.
		Pre: &testPre{
			prepareFunc: func(context.Context, *State) interface{} { return preData },
		},
		Func: func(ctx context.Context, s *State) {
			if s.PreValue() == nil {
				s.Fatal("Precondition value not supplied")
			} else if d, ok := s.PreValue().(*data); !ok {
				s.Fatal("Precondition value didn't have expected type")
			} else if d != preData {
				s.Fatalf("Got precondition value %v; want %v", d, preData)
			}
		},
		Timeout: time.Minute,
	}

	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}
}

func TestRunPreconditionContext(t *gotesting.T) {
	var testCtx context.Context

	prepareFunc := func(ctx context.Context, s *State) interface{} {
		if testCtx == nil {
			testCtx = s.PreCtx()
		}

		if testCtx != s.PreCtx() {
			t.Errorf("Different context in Prepare")
		}

		if _, ok := ContextSoftwareDeps(s.PreCtx()); !ok {
			t.Error("ContextSoftwareDeps unavailable")
		}
		return nil
	}

	closeFunc := func(ctx context.Context, s *State) {
		if testCtx != s.PreCtx() {
			t.Errorf("Different context in Close")
		}
	}

	testFunc := func(ctx context.Context, s *State) {
		defer func() {
			expectedPanic := "PreCtx can only be called in a precondition"

			if r := recover(); r == nil {
				t.Errorf("PreCtx did not panic")
			} else if r != expectedPanic {
				t.Errorf("PreCtx unexpected panic: want %q; got %q", expectedPanic, r)
			}
		}()

		s.PreCtx()
	}

	pre := &testPre{
		prepareFunc: prepareFunc,
		closeFunc:   closeFunc,
	}

	t1 := &TestInstance{Name: "t1", Pre: pre, Timeout: time.Minute, Func: testFunc}
	t2 := &TestInstance{Name: "t2", Pre: pre, Timeout: time.Minute, Func: testFunc}

	or := newOutputReader()
	t1.Run(context.Background(), or.ch, &TestConfig{
		NextTest: t2,
	})
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}

	if t1.PreCtx != t2.PreCtx {
		t.Errorf("PreCtx different between test instances")
	}

	or = newOutputReader()
	t2.Run(context.Background(), or.ch, &TestConfig{
		NextTest: nil,
	})
	for _, err := range getOutputErrors(or.read()) {
		t.Error("Got error: ", err.Reason)
	}

	if t1.PreCtx.Err() == nil {
		t.Errorf("Context not cancelled")
	}
}

func TestAttachStateToContext(t *gotesting.T) {
	test := TestInstance{
		Func: func(ctx context.Context, s *State) {
			logging.ContextLog(ctx, "msg ", 1)
			logging.ContextLogf(ctx, "msg %d", 2)
		},
		Timeout: time.Minute,
	}

	or := newOutputReader()
	test.Run(context.Background(), or.ch, &TestConfig{})
	if out := or.read(); len(out) != 2 || out[0].Msg != "msg 1" || out[1].Msg != "msg 2" {
		t.Errorf("Bad test output: %v", out)
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
	pre1 := &testPre{name: "pre1"}
	pre2 := &testPre{name: "pre2"}

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
