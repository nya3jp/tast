// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"reflect"
	gotesting "testing"
	"time"
)

// TESTTEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_test.go). The obvious choice, "TestTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "test" is an acronym.
func TESTTEST(context.Context, *State) {}

// InvalidTestName is an arbitrary public test function used by unit tests.
func InvalidTestName(context.Context, *State) {}

func TestValidateName(t *gotesting.T) {
	for _, tc := range []struct {
		name, category, fn string
		valid              bool
	}{
		{"Invalid%@!", "testing", "test.go", false},              // Invalid name pattern
		{"Test", "testing", "test.go", true},                     // single word
		{"MyTest", "testing", "my_test.go", true},                // two words separated with underscores
		{"LoadURL", "testing", "load_url.go", true},              // word and acronym
		{"PlayMP3", "testing", "play_mp3.go", true},              // word contains numbers
		{"PlayMP3Song", "testing", "play_mp3_song.go", true},     // acronym followed by word
		{"ConnectToDBus", "testing", "connect_to_dbus.go", true}, // word with multiple leading caps
		{"RestartCrosVM", "testing", "restart_crosvm.go", true},  // word with ending acronym
		{"RestartCrosVM", "testing", "restart_cros_vm.go", true}, // word followed by acronym
		{"Foo123bar", "testing", "foo123bar.go", true},           // word contains digits
		{"Foo123Bar", "testing", "foo123_bar.go", true},          // word with trailing digits
		{"Foo123bar", "testing", "foo_123bar.go", true},          // word with leading digits
		{"Foo123Bar", "testing", "foo_123_bar.go", true},         // word consisting only of digits
		{"foo", "testing", "foo.go", false},                      // lowercase func name
		{"Foobar", "testing", "foo_bar.go", false},               // lowercase word
		{"FirstTest", "testing", "first.go", false},              // func name has word not in filename
		{"Firstblah", "testing", "first.go", false},              // func name has word longer than filename
		{"First", "testing", "firstabc.go", false},               // filename has word longer than func name
		{"First", "testing", "first_test.go", false},             // filename has word not in func name
		{"FooBar", "testing", "foo__bar.go", false},              // empty word in filename
		{"Foo", "testing", "bar.go", false},                      // completely different words
		{"Foo", "testing", "Foo.go", false},                      // non-lowercase filename
		{"Foo", "testing", "foo.txt", false},                     // filename without ".go" extension
	} {
		err := validateName(tc.name, tc.category, tc.fn)
		if err != nil && tc.valid {
			t.Errorf("validateName(%q, %q, %q) failed: %v", tc.name, tc.category, tc.fn, err)
		} else if err == nil && !tc.valid {
			t.Errorf("validateName(%q, %q, %q) didn't return expected error", tc.name, tc.category, tc.fn)
		}
	}
}

func TestSettle(t *gotesting.T) {
	test := &Test{
		Func:         TESTTEST,
		Attr:         []string{"group:mainline", "group:crosbolt"},
		Data:         []string{"data1.txt", "data2.txt"},
		SoftwareDeps: []string{"chrome", "android"},
		Params: []Param{{
			Name:              "p10",
			Val:               10,
			ExtraAttr:         []string{"informational"},
			ExtraData:         []string{"data10.txt"},
			ExtraSoftwareDeps: []string{"tpm"},
		}, {
			Name:              "p20",
			Val:               20,
			ExtraAttr:         []string{"crosbolt_weekly"},
			ExtraData:         []string{"data20.txt"},
			ExtraSoftwareDeps: []string{"wifi"},
		}},
	}
	sts, err := test.settle()
	if err != nil {
		t.Fatal("settle failed: ", err)
	}
	if len(sts) != 2 {
		t.Fatalf("settle returned %d Test(s); want 2", len(sts))
	}
	{
		st := sts[0]
		if got, want := st.suffix, "p10"; got != want {
			t.Errorf("sts[0].suffix = %q; want %q", got, want)
		}
		if got, want := st.val.(int), 10; got != want {
			t.Errorf("sts[0].val = %q; want %q", got, want)
		}
		if got, want := st.Attr, []string{"group:mainline", "group:crosbolt", "informational"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sts[0].Attr = %q; want %q", got, want)
		}
		if got, want := st.Data, []string{"data1.txt", "data2.txt", "data10.txt"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sts[0].Data = %q; want %q", got, want)
		}
		if got, want := st.SoftwareDeps, []string{"chrome", "android", "tpm"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sts[0].SoftwareDeps = %q; want %q", got, want)
		}
	}
	{
		st := sts[1]
		if got, want := st.suffix, "p20"; got != want {
			t.Errorf("sts[1].suffix = %q; want %q", got, want)
		}
		if got, want := st.val.(int), 20; got != want {
			t.Errorf("sts[1].val = %q; want %q", got, want)
		}
		if got, want := st.Attr, []string{"group:mainline", "group:crosbolt", "crosbolt_weekly"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sts[1].Attr = %q; want %q", got, want)
		}
		if got, want := st.Data, []string{"data1.txt", "data2.txt", "data20.txt"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sts[1].Data = %q; want %q", got, want)
		}
		if got, want := st.SoftwareDeps, []string{"chrome", "android", "wifi"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sts[1].SoftwareDeps = %q; want %q", got, want)
		}
	}
}

func TestSettleMissingFunc(t *gotesting.T) {
	test := &Test{}
	if _, err := test.settle(); err == nil {
		t.Error("Didn't get error with missing function")
	}
}

func TestSettleFuncName(t *gotesting.T) {
	test := &Test{
		Func: TESTTEST,
	}
	if _, err := test.settle(); err != nil {
		t.Error("Got error when finalizing test with valid test func name: ", err)
	}
	test = &Test{
		Func: InvalidTestName,
	}
	if _, err := test.settle(); err == nil {
		t.Error("Didn't get expected error when finalizing test with invalid test func name")
	}
}

func TestSettleInvalidDataPaths(t *gotesting.T) {
	for _, path := range []string{
		"/etc/passwd",
		"foo/../foo/bar",
		"../baz",
	} {
		test := &Test{
			Func: TESTTEST,
			Data: []string{path},
		}
		if _, err := test.settle(); err == nil {
			t.Errorf("settle unexpectedly succeeded for data path %q", path)
		}
	}
}

func TestSettleReservedAttrPrefixes(t *gotesting.T) {
	for _, attr := range []string{
		testNameAttrPrefix + "foo",
		testBundleAttrPrefix + "bar",
		testDepAttrPrefix + "dep",
	} {
		test := &Test{
			Func: TESTTEST,
			Attr: []string{attr},
		}
		if _, err := test.settle(); err == nil {
			t.Errorf("settle unexpectedly succeeded for attr %q", attr)
		}
	}
}

func TestSettleNegativeTimeout(t *gotesting.T) {
	test := &Test{
		Func:    TESTTEST,
		Timeout: -1 * time.Second,
	}
	if _, err := test.settle(); err == nil {
		t.Error("settle unexpectedly succeeded with negative timeout")
	}
}

func TestSettleUniqueParamNames(t *gotesting.T) {
	test := &Test{
		Func: TESTTEST,
		Params: []Param{{
			Name: "abc",
		}, {
			Name: "abc",
		}},
	}
	if _, err := test.settle(); err == nil {
		t.Error("settle unexpectedly succeeded with duplicated parameter names")
	}
}

func TestSettleValTypes(t *gotesting.T) {
	test := &Test{
		Func: TESTTEST,
		Params: []Param{{
			Name: "case1",
			Val:  1,
		}, {
			Name: "case2",
			Val:  "string",
		}},
	}
	if _, err := test.settle(); err == nil {
		t.Error("settle unexpectedly succeeded with inconsistent val types")
	}
}

func TestSettleSuffix(t *gotesting.T) {
	for _, tc := range []struct {
		suffix  string
		success bool
	}{
		{"", true},
		{"word1_word2", true},
		{"CapitalName", false},
		{"!#$%&'()", false},
	} {
		test := &Test{
			Func:   TESTTEST,
			Params: []Param{{Name: tc.suffix}},
		}
		_, err := test.settle()
		if tc.success {
			if err != nil {
				t.Errorf("settle failed for suffix %q: %v", tc.suffix, err)
			}
		} else {
			if err == nil {
				t.Errorf("settle unexpectedly succeeded for suffix %q", tc.suffix)
			}
		}
	}
}

func TestSettleNoParams(t *gotesting.T) {
	test := &Test{
		Func:         TESTTEST,
		Attr:         []string{"group:mainline", "group:crosbolt"},
		Data:         []string{"data1.txt", "data2.txt"},
		SoftwareDeps: []string{"chrome", "android"},
	}
	sts, err := test.settle()
	if err != nil {
		t.Fatal("settle failed: ", err)
	}
	if len(sts) != 1 {
		t.Fatalf("settle returned %d Test(s); want 1", len(sts))
	}
}

func TestSettlePreConflict(t *gotesting.T) {
	pre := &testPre{}
	if _, err := (&Test{
		Func:   TESTTEST,
		Pre:    pre,
		Params: []Param{{}},
	}).settle(); err != nil {
		t.Error("settle failed: ", err)
	}

	if _, err := (&Test{
		Func: TESTTEST,
		Params: []Param{{
			Pre: pre,
		}},
	}).settle(); err != nil {
		t.Error("settle failed: ", err)
	}

	if _, err := (&Test{
		Func: TESTTEST,
		Pre:  pre,
		Params: []Param{{
			Pre: pre,
		}},
	}).settle(); err == nil {
		t.Error("settle unexpectedly succeeded")
	}
}

func TestSettleTimeoutConflict(t *gotesting.T) {
	if _, err := (&Test{
		Func:    TESTTEST,
		Timeout: time.Hour,
		Params:  []Param{{}},
	}).settle(); err != nil {
		t.Error("settle failed: ", err)
	}

	if _, err := (&Test{
		Func: TESTTEST,
		Params: []Param{{
			Timeout: time.Hour,
		}},
	}).settle(); err != nil {
		t.Error("settle failed: ", err)
	}

	if _, err := (&Test{
		Func:    TESTTEST,
		Timeout: time.Hour,
		Params: []Param{{
			Timeout: time.Hour,
		}},
	}).settle(); err == nil {
		t.Error("settle unexpectedly succeeded")
	}
}
