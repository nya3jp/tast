// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestMessages(t *testing.T) {
	const code = `package pkg

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

func Test(ctx context.Context, s *testing.State) {
	var err error

	// Bad messages:
	s.Logf("Should use Log for single string arg")
	s.Logf("Should use Log for single trailing %v", err)
	s.Log("Shouldn't use trailing period.")
	s.Log("Shouldn't contain\na newline")
	s.Log("Should end with colon and space ", err)
	s.Log("Should end with colon and space:", err)
	s.Log(err)

	// Other functions should also be checked:
	s.Error(err)
	s.Fatal(err)
	s.Errorf("Just a string")
	s.Fatalf("Just a string")
	testing.ContextLog(ctx, err)
	testing.ContextLogf(ctx, "Should've used ContextLog: %v", err)

	// Good messages:
	s.Log("Good single-arg message")
	s.Log("Good message with error: ", err)
	s.Logf("Good message with custom-formatted value %d", 3)
	s.Logf("Good message with %v default-formatted values: %v", 2, err)

	// Bad errors:
	errors.New("shouldn't use trailing period.")
	errors.New("shouldn't contain\na newline")
	errors.Errorf("should use New for single string arg")
	errors.Errorf("should use Wrap: %v", err)
	errors.Errorf("should use Wrapf %s: %v", "here", err)

	// Good errors:
	errors.New("normal message")
	errors.Errorf("need Errorf for multiple values: %v", true)
	errors.Errorf("also okay for custom formatting: %d", 1)
	errors.Wrapf(err, "this is okay: %v", 3)

	// Bad quoting:
	s.Logf("Read value '%s'", "blah")
	s.Errorf("Read value \"%v\"", "blah")

	// Good quoting:
	s.Logf("Read value %q", "blah")
	s.Errorf("Read value '%d'", 123)

	// Bad formatting. (Using Errorf/Fatalf/Logf instead of
	// Error/Fatal/Log mistakenly).
	s.Errorf("Some error: ", err)
	s.Fatalf("Some error: ", err)
	s.Logf("Some log: ", text)

	// Verb counts.
	s.Errorf("Got %d; want %d", actual, expect)
	s.Errorf("Got %d %%; want 100 %%", actual)
	s.Errorf("%%%%%d", actual)  // Test for an edge case about %%.

	// Bad error construction with empty string.
	errors.New("")
	errors.Wrap(err, "")
	s.Error("")
	s.Fatal("")

	// Good messages. (Allowed for line breaking)
	s.Log("")
	testing.ContextLog(ctx, "")

	// Messages should start with correct case.
	s.Error("Failed to start ARC: ", err)
	s.Error("failed to start ARC: ", err)
	s.Fatalf("Unexpected string %q received", "blah")
	s.Fatalf("unexpected string %q received", "blah")
	s.Log("Got messages")
	s.Log("got messages")
	testing.ContextLogf(ctx, "Found a file %q", "blah")
	testing.ContextLogf(ctx, "found a file %q", "blah")
	errors.New("could not start ARC")
	errors.New("Could not start ARC")
	errors.Wrapf(err, "too many (%d) files open", 28)
	errors.Wrapf(err, "Too many (%d) files open", 28)

	// Proper nouns are not subject to case rules.
	s.Error("testSomething failed: ", err)
	errors.New("ARC failed to start")

	// Bad combinations substitutable with *f family.
	s.Error(fmt.Sprintf("Foo (%d)", 42))
	errors.New(fmt.Sprintf("foo (%d)", 42))
	errors.Wrap(err, fmt.Sprintf("foo (%d)", 42))

	// Allowed combinations for Sprintf
	s.Error(fmt.Sprintf("Foo (%d)", 42), "bar")

	// Bad usage of verbs in non-format logging
	s.Error("Foo (%d)", 42)
	errors.Wrap(err, "foo (%1.2f)", 4.2)
	testing.ContextLog(ctx, "Foo: %#v", foo)

	// Good usage of % in non-format logging
	testing.ContextLog(ctx, "Waiting for 100% progress")
}`

	f, fs := parse(code, "test.go")
	issues := Messages(fs, f)
	expects := []string{
		`test.go:14:2: Use s.Log("<msg>") instead of s.Logf("<msg>")`,
		`test.go:15:2: Use s.Log("<msg> ", val) instead of s.Logf("<msg> %v", val)`,
		`test.go:16:2: s.Log string arg should not contain trailing punctuation`,
		`test.go:17:2: s.Log string arg should not contain embedded newlines`,
		`test.go:18:2: s.Log string arg should end with ": " when followed by error`,
		`test.go:19:2: s.Log string arg should end with ": " when followed by error`,
		`test.go:20:2: Use s.Log("Something failed: ", err) instead of s.Log(err)`,
		`test.go:23:2: Use s.Error("Something failed: ", err) instead of s.Error(err)`,
		`test.go:24:2: Use s.Fatal("Something failed: ", err) instead of s.Fatal(err)`,
		`test.go:25:2: Use s.Error("<msg>") instead of s.Errorf("<msg>")`,
		`test.go:26:2: Use s.Fatal("<msg>") instead of s.Fatalf("<msg>")`,
		`test.go:27:2: Use testing.ContextLog(ctx, "Something failed: ", err) instead of testing.ContextLog(ctx, err)`,
		`test.go:28:2: Use testing.ContextLog(ctx, "<msg> ", val) instead of testing.ContextLogf(ctx, "<msg> %v", val)`,
		`test.go:37:2: errors.New string arg should not contain trailing punctuation`,
		`test.go:38:2: errors.New string arg should not contain embedded newlines`,
		`test.go:39:2: Use errors.New("<msg>") instead of errors.Errorf("<msg>")`,
		`test.go:40:2: Use errors.Wrap(err, "<msg>") instead of errors.Errorf("<msg>: %v", err)`,
		`test.go:41:2: Use errors.Wrapf(err, "<msg>", ...) instead of errors.Errorf("<msg>: %v", ..., err)`,
		`test.go:50:2: Use %q to quote values instead of manually quoting them`,
		`test.go:51:2: Use %q to quote values instead of manually quoting them`,
		`test.go:59:2: The number of verbs in format literal mismatches with the number of arguments`,
		`test.go:60:2: The number of verbs in format literal mismatches with the number of arguments`,
		`test.go:61:2: The number of verbs in format literal mismatches with the number of arguments`,
		`test.go:69:2: Error message should have some surrounding context, so must not empty`,
		`test.go:70:2: Error message should have some surrounding context, so must not empty`,
		`test.go:71:2: Error message should have some surrounding context, so must not empty`,
		`test.go:72:2: Error message should have some surrounding context, so must not empty`,
		`test.go:80:2: Test failure messages should be capitalized`,
		`test.go:82:2: Test failure messages should be capitalized`,
		`test.go:84:2: Log messages should be capitalized`,
		`test.go:86:2: Log messages should be capitalized`,
		`test.go:88:2: Messages of the error type should not be capitalized`,
		`test.go:90:2: Messages of the error type should not be capitalized`,
		`test.go:97:2: Use s.Errorf(...) instead of s.Error(fmt.Sprintf(...))`,
		`test.go:98:2: Use errors.Errorf(...) instead of errors.New(fmt.Sprintf(...))`,
		`test.go:99:2: Use errors.Wrapf(err, ...) instead of errors.Wrap(err, fmt.Sprintf(...))`,
		`test.go:105:2: s.Error has verbs in the first string (do you mean s.Errorf?)`,
		`test.go:106:2: errors.Wrap has verbs in the first string (do you mean errors.Wrapf?)`,
		`test.go:107:2: testing.ContextLog has verbs in the first string (do you mean testing.ContextLogf?)`,
	}
	verifyIssues(t, issues, expects)
}
