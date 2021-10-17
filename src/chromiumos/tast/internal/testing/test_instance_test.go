// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"bytes"
	"context"
	gotesting "testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.chromium.org/chromiumos/config/go/test/api"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/testing/hwdep"
)

// TESTINSTANCETEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_instance_test.go). The obvious choice, "TestInstanceTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "instance" and "test" are acronyms.
func TESTINSTANCETEST(context.Context, *State) {}

// InvalidTestName is an arbitrary public test function used by unit tests.
func InvalidTestName(context.Context, *State) {}

// fakePre implements both Precondition for unit tests.
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

func features(availableSWs []string, model string) *protocol.Features {
	availableSWSet := make(map[string]struct{})
	for _, dep := range availableSWs {
		availableSWSet[dep] = struct{}{}
	}

	var unavailableSWs []string
	for _, dep := range []string{"dep0", "dep1", "dep2", "dep3"} {
		if _, ok := availableSWSet[dep]; !ok {
			unavailableSWs = append(unavailableSWs, dep)
		}
	}

	return &protocol.Features{
		CheckDeps: true,
		Dut: &protocol.DUTFeatures{
			Software: &protocol.SoftwareFeatures{
				Available:   availableSWs,
				Unavailable: unavailableSWs,
			},
			Hardware: &protocol.HardwareFeatures{
				DeprecatedDeviceConfig: &protocol.DeprecatedDeviceConfig{
					Id: &protocol.DeprecatedConfigId{
						Model: model,
					},
				},
			},
		},
	}
}

func TestInstantiate(t *gotesting.T) {
	got, err := instantiate(&Test{
		Func:         TESTINSTANCETEST,
		Desc:         "hello",
		Contacts:     []string{"a@example.com", "b@example.com"},
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{"data1.txt", "data2.txt"},
		Vars:         []string{"var1"},
		VarDeps:      []string{"vardep1"},
		SoftwareDeps: []string{"dep1", "dep2"},
		HardwareDeps: hwdep.D(hwdep.Model("model1", "model2")),
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
		Vars:         []string{"var1"},
		VarDeps:      []string{"vardep1"},
		SoftwareDeps: []string{"dep1", "dep2"},
		Timeout:      123 * time.Second,
		ServiceDeps:  []string{"svc1", "svc2"},
	}}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(TestInstance{}, "Func", "HardwareDeps")); diff != "" {
		t.Errorf("Got unexpected test instances (-got +want):\n%s", diff)
	}
	if got[0].Func == nil {
		t.Error("Got nil Func")
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
		reasons, err := got[0].Deps().Check(features([]string{"dep0", "dep1"}, "model1"))
		if err != nil {
			t.Fatal("Check failed: ", err)
		}
		if len(reasons) > 0 {
			t.Error("Got unexpected HardwareDeps for first test instance: Check returned skip reasons for model1: ", reasons)
		}
		reasons, err = got[0].Deps().Check(features([]string{"dep0", "dep1"}, "model2"))
		if len(reasons) == 0 {
			t.Error("Got unexpected HardwareDeps for first test instance: Check returned no skip reason for model2")
		}
		if got[1].Func == nil {
			t.Error("Got nil Func for the second test instance")
		}
		reasons, err = got[1].Deps().Check(features([]string{"dep0", "dep2"}, "model2"))
		if len(reasons) > 0 {
			t.Error("Got unexpected HardwareDeps for second test instance: Check returned skip reasons for model2: ", reasons)
		}
		reasons, err = got[1].Deps().Check(features([]string{"dep0", "dep2"}, "model1"))
		if len(reasons) == 0 {
			t.Error("Got unexpected HardwareDeps for second test instance: Check returned no skip reason for model1")
		}
	}
}

func TestInstantiateFixture(t *gotesting.T) {
	// Registration without params should succeed.
	got, err := instantiate(&Test{
		Func:    TESTINSTANCETEST,
		Fixture: "fixt1",
	})
	if err != nil {
		t.Fatal("Failed to instantiate test: ", err)
	}
	if len(got) != 1 {
		t.Fatalf("Got %d test instances; want 1", len(got))
	}
	if got[0].Fixture != "fixt1" {
		t.Fatalf("TestInstance.Fixture = %q; want %q", got[0].Fixture, "fixt1")
	}

	// Duplicated fields should be rejected.
	if _, err := instantiate(&Test{
		Func:    TESTINSTANCETEST,
		Fixture: "fixt1",
		Params: []Param{{
			Fixture: "fixt2",
		}},
	}); err == nil {
		t.Error("instantiate succeeded unexpectedly for duplicated Pre")
	}

	// OK if the field in the base test is unset.
	got, err = instantiate(&Test{
		Func: TESTINSTANCETEST,
		Params: []Param{{
			Fixture: "fixt2",
		}},
	})
	if err != nil {
		t.Fatal("Failed to instantiate test: ", err)
	}
	if len(got) != 1 {
		t.Fatalf("Got %d test instances; want 1", len(got))
	}
	if got[0].Fixture != "fixt2" {
		t.Fatalf("TestInstance.Fixture = %q; want %q", got[0].Fixture, "fixt2")
	}
}

func TestInstantiatePre(t *gotesting.T) {
	pre := &fakePre{}

	// Registration without params should succeed.
	got, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Pre:  pre,
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
	got, err = instantiate(&Test{
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

func TestRelativeDataDir(t *gotesting.T) {
	const pkg = "a/b/c"
	got := RelativeDataDir(pkg)
	want := "a/b/c/data"
	if got != want {
		t.Errorf("RelativeDataDir(%q) = %q; want %q", pkg, got, want)
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
		{"x.c"},
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
		{"a.X.c"},
		{"a.B.c."},
		{"a.B._"},
		{"a", "a"},
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

type nonPointerPre struct{}

func (p nonPointerPre) Prepare(ctx context.Context, s *PreState) interface{} { return nil }
func (p nonPointerPre) Close(ctx context.Context, s *PreState)               {}
func (p nonPointerPre) Timeout() time.Duration                               { return time.Minute }
func (p nonPointerPre) String() string                                       { return "nonPointerPre" }

func TestInstantiateNonPointerPrecondition(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Pre:  &nonPointerPre{},
	}); err != nil {
		t.Fatal("Instanciate failed for pointer pre: ", err)
	}

	if _, err := instantiate(&Test{
		Func: TESTINSTANCETEST,
		Pre:  nonPointerPre{},
	}); err == nil {
		t.Fatal("Instanciate unexpectedly succeeded for non-pointer pre")
	}
}

func TestSoftwareDeps(t *gotesting.T) {
	fs := &protocol.Features{
		CheckDeps: true,
		Dut: &protocol.DUTFeatures{
			Software: &protocol.SoftwareFeatures{
				Available:   []string{"dep1", "dep2"},
				Unavailable: []string{"dep0", "dep3"},
			},
		},
	}

	for _, tc := range []struct {
		name        string
		testDeps    []string
		wantReasons []string
		wantErr     string
	}{{
		name:     "error",
		testDeps: []string{"dep3", "dep1", "dep2", "depX"},
		wantErr:  "unknown SoftwareDeps: depX",
	}, {
		name:     "pass",
		testDeps: []string{"dep1", "dep2"},
	}, {
		name:        "skip",
		testDeps:    []string{"dep0", "dep1"},
		wantReasons: []string{"missing SoftwareDeps: dep0"},
	}} {
		t.Run(tc.name, func(t *gotesting.T) {
			reasons, err := (&TestInstance{SoftwareDeps: tc.testDeps}).Deps().Check(fs)
			if tc.wantErr == "" && err != nil {
				t.Errorf("Check() failed: %v", err)
			}
			if tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr) {
				t.Errorf("Check() returned error %v; want %q", err, tc.wantErr)
			}
			if diff := cmp.Diff(reasons, tc.wantReasons); diff != "" {
				t.Errorf("Reasons unmatch (-got +want):\n%v", diff)
			}
		})
	}
}

func TestHardwareDeps(t *gotesting.T) {
	fs := &protocol.Features{
		CheckDeps: true,
		Dut: &protocol.DUTFeatures{
			Hardware: &protocol.HardwareFeatures{
				DeprecatedDeviceConfig: &protocol.DeprecatedDeviceConfig{
					Id: &protocol.DeprecatedConfigId{
						Model: "samus",
					},
				},
			},
		},
	}
	for _, tc := range []struct {
		modelDep    string
		wantReasons []string
	}{{
		modelDep:    "eve",
		wantReasons: []string{"ModelId did not match"},
	}, {
		modelDep: "samus",
	}} {
		t.Run(tc.modelDep, func(t *gotesting.T) {
			reasons, err := (&TestInstance{HardwareDeps: hwdep.D(hwdep.Model(tc.modelDep))}).Deps().Check(fs)
			if err != nil {
				t.Fatal("Check failed: ", err)
			}
			if diff := cmp.Diff(reasons, tc.wantReasons); diff != "" {
				t.Errorf("Reasons unmatch (-got +want):\n%v", diff)
			}
		})
	}
}

func TestVarDeps(t *gotesting.T) {
	test := TestInstance{VarDeps: []string{"foo"}}
	for _, tc := range []struct {
		name        string
		features    *protocol.Features
		wantReasons []string
		wantErr     bool
	}{{
		name: "pass",
		features: &protocol.Features{
			CheckDeps: true,
			Infra: &protocol.InfraFeatures{
				Vars: map[string]string{"foo": "val"},
			},
		},
		wantReasons: nil,
	}, {
		name: "fail",
		features: &protocol.Features{
			CheckDeps: true,
			Infra: &protocol.InfraFeatures{
				Vars: map[string]string{},
			},
		},
		wantErr: true,
	}, {
		name: "skip",
		features: &protocol.Features{
			CheckDeps: true,
			Infra: &protocol.InfraFeatures{
				Vars:             map[string]string{},
				MaybeMissingVars: `foo`,
			},
		},
		wantReasons: []string{"runtime variable foo is missing and matches with ^foo$"},
	}} {
		t.Run(tc.name, func(t *gotesting.T) {
			reasons, err := test.Deps().Check(tc.features)
			if diff := cmp.Diff(reasons, tc.wantReasons); diff != "" {
				t.Errorf("Reasons mismatch (-got +want):\n%v", diff)
			}
			if err != nil && !tc.wantErr {
				t.Errorf("Got error unexpectedly: %v", err)
			}
			if err == nil && tc.wantErr {
				t.Errorf("Got no error, want error")
			}
		})
	}
}

func TestTestInstanceEntityProto(t *gotesting.T) {
	test := &TestInstance{
		Name:         "pkg.Test",
		Pkg:          "chromiumos/foo/bar",
		Val:          "somevalue",
		Func:         TESTINSTANCETEST,
		Desc:         "Description",
		Contacts:     []string{"me@example.com"},
		Attr:         []string{"attr1", "attr2"},
		Data:         []string{"foo.txt"},
		Vars:         []string{"var1", "var2"},
		VarDeps:      []string{"vardep1"},
		SoftwareDeps: []string{"dep1", "dep2"},
		ServiceDeps:  []string{"svc1", "svc2"},
		Fixture:      "fixt",
		Timeout:      time.Hour,
		Bundle:       "bundle",
	}

	got := test.EntityProto()
	want := &protocol.Entity{
		Name:        "pkg.Test",
		Package:     "chromiumos/foo/bar",
		Attributes:  []string{"attr1", "attr2"},
		Description: "Description",
		Fixture:     "fixt",
		Dependencies: &protocol.EntityDependencies{
			DataFiles: []string{"foo.txt"},
			Services:  []string{"svc1", "svc2"},
		},
		Contacts: &protocol.EntityContacts{
			Emails: []string{"me@example.com"},
		},
		LegacyData: &protocol.EntityLegacyData{
			Variables:    []string{"var1", "var2"},
			VariableDeps: []string{"vardep1"},
			SoftwareDeps: []string{"dep1", "dep2"},
			Timeout:      ptypes.DurationProto(time.Hour),
			Bundle:       "bundle",
		},
	}
	if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
		t.Errorf("Got unexpected Entity (-got +want):\n%s", diff)
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
		// Go doesn't allow checking functions for equality.
		if tst.Func == nil {
			t.Errorf("%s set Func to nil; want %p", msg, f)
		}
		want := &TestInstance{
			Name:         name,
			Attr:         attr,
			SoftwareDeps: softwareDeps,
			ServiceDeps:  serviceDeps,
			Timeout:      timeout,
		}
		if diff := cmp.Diff(tst, want, cmpopts.IgnoreFields(TestInstance{}, "Func", "HardwareDeps")); diff != "" {
			t.Errorf("Unexpected instance after %s; (-got +want):\n%v", msg, diff)
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
	expected := api.TestCaseMetadataList{
		Values: []*api.TestCaseMetadata{
			{
				TestCase: &api.TestCase{
					Id: &api.TestCase_Id{
						Value: testHarnessPrefix + "." + "test001",
					},
					Name: "test001",
					Tags: []*api.TestCase_Tag{
						{Value: "attr1"},
						{Value: "attr2"},
					},
				},
				TestCaseExec: &api.TestCaseExec{
					TestHarness: &api.TestHarness{
						TestHarnessType: &api.TestHarness_Tast_{
							Tast: &api.TestHarness_Tast{},
						},
					},
				},
				TestCaseInfo: &api.TestCaseInfo{
					Owners: []*api.Contact{
						{Email: "someone1@chromium.org"},
						{Email: "someone2@chromium.org"},
					},
				},
			},
		},
	}

	var b bytes.Buffer
	WriteTestsAsProto(&b, in)
	var actual api.TestCaseMetadataList
	proto.Unmarshal(b.Bytes(), &actual)
	if !cmp.Equal(expected, actual, cmp.Comparer(proto.Equal)) {
		t.Errorf("WriteTestsAsProto(%v): got %v; want %v", in, actual, expected)
	}
}
