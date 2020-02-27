// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	gotesting "testing"
	"time"
)

// TODO(before submission): Merge this file to test_instance_test.go.

// TESTTEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_test.go). The obvious choice, "TestTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "test" is an acronym.
func TESTTEST(context.Context, *State) {}

// InvalidTestName is an arbitrary public test function used by unit tests.
func InvalidTestName(context.Context, *State) {}

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
	if _, err := instantiate(&Test{Func: TESTTEST}); err != nil {
		t.Error("instantiate failed: ", err)
	}
	if _, err := instantiate(&Test{Func: InvalidTestName}); err == nil {
		t.Error("instantiate succeeded unexpectedly for wrongly named test func")
	}
}

func TestInstantiateDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTTEST,
		Data: []string{"foo", "bar/baz"},
	}); err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}
}

func TestInstantiateUncleanDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTTEST,
		Data: []string{"foo", "bar/../bar/baz"},
	}); err == nil {
		t.Error("Did not get an error with unclean path")
	}
}

func TestInstantiateAbsoluteDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTTEST,
		Data: []string{"foo", "/etc/passwd"},
	}); err == nil {
		t.Error("Did not get an error with absolute path")
	}
}

func TestInstantiateRelativeDataPath(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTTEST,
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
			Func: TESTTEST,
			Attr: []string{attr},
		}); err == nil {
			t.Errorf("Did not get an error for reserved attribute %q", attr)
		}
	}
}

func TestInstantiateNegativeTimeout(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func:    TESTTEST,
		Timeout: -1 * time.Second,
	}); err == nil {
		t.Error("Didn't get error with negative timeout")
	}
}

func TestInstantiateDuplicatedParamName(t *gotesting.T) {
	if _, err := instantiate(&Test{
		Func: TESTTEST,
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
		Func: TESTTEST,
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
