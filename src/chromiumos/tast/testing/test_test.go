// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	gotesting "testing"
	"time"
)

// TESTTEST is a public test function with a name that's chosen to be appropriate for this file's
// name (test_test.go). The obvious choice, "TestTest", is unavailable since Go's testing package
// will interpret it as itself being a unit test, so let's just pretend that "test" is an acronym.
func TESTTEST(context.Context, *State) {}

// InvalidTestName is an arbitrary public test function used by unit tests.
func InvalidTestName(context.Context, *State) {}

func TestMissingFunc(t *gotesting.T) {
	if err := validateTest(&Test{}); err == nil {
		t.Error("Didn't get error with missing function")
	}
}

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

// TestAutoName makes sure the validateName runs agains the name delived from
// the Func's function name and its source file name.
func TestFuncName(t *gotesting.T) {
	if err := validateTest(&Test{Func: TESTTEST}); err != nil {
		t.Error("Got error when finalizing test with valid test func name: ", err)
	}
	if err := validateTest(&Test{Func: InvalidTestName}); err == nil {
		t.Error("Didn't get expected error when finalizing test with invalid test func name")
	}
}

func TestValidateDataPath(t *gotesting.T) {
	if err := validateData([]string{"foo", "bar/baz"}); err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}
}

func TestValidateDataPathUnclean(t *gotesting.T) {
	if err := validateData([]string{"foo", "bar/../bar/baz"}); err == nil {
		t.Error("Did not get an error with unclean path")
	}
}

func TestValidateDataPathAbsolutePath(t *gotesting.T) {
	if err := validateData([]string{"foo", "/etc/passwd"}); err == nil {
		t.Error("Did not get an error with absolute path")
	}
}

func TestValidateDataPathRelativePath(t *gotesting.T) {
	if err := validateData([]string{"foo", "../baz"}); err == nil {
		t.Error("Did not get an error with relative path")
	}
}

func TestReservedAttrPrefixes(t *gotesting.T) {
	for _, attr := range []string{
		testNameAttrPrefix + "foo",
		testBundleAttrPrefix + "bar",
		testDepAttrPrefix + "dep",
	} {
		if err := validateAttr([]string{attr}); err == nil {
			t.Errorf("Did not get an error for reserved attribute %q", attr)
		}
	}
}

func TestNegativeTimeout(t *gotesting.T) {
	if err := validateTest(&Test{Func: TESTTEST, Timeout: -1 * time.Second}); err == nil {
		t.Error("Didn't get error with negative timeout")
	}
}
