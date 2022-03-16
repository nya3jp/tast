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
	errors.Wrap(err, "shouldn't use trailing colon:")
	errors.Wrap(err, "shouldn't use trailing colon: ")
	errors.Wrap(err, "shouldn't use trailing colon:  ")
	errors.Wrapf(err, "shouldn't wrap err twice: %v", err)

	// Good errors:
	errors.New("normal message")
	errors.Errorf("need Errorf for multiple values: %v", true)
	errors.Errorf("also okay for custom formatting: %d", 1)
	errors.Wrapf(err, "this is okay: %v", 3)
	errors.Wrap(err, "this is okay")

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
	issues := Messages(fs, f, false)
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
		`test.go:42:2: Use errors.Wrap(err, "shouldn't use trailing colon") instead of errors.Wrap(err, "shouldn't use trailing colon:")`,
		`test.go:43:2: Use errors.Wrap(err, "shouldn't use trailing colon") instead of errors.Wrap(err, "shouldn't use trailing colon: ")`,
		`test.go:44:2: Use errors.Wrap(err, "shouldn't use trailing colon") instead of errors.Wrap(err, "shouldn't use trailing colon:  ")`,
		`test.go:45:2: Use errors.Wrap(err, "<msg>") instead of errors.Wrapf(err, "<msg>: %v", err)`,
		`test.go:55:2: Use %q to quote values instead of manually quoting them`,
		`test.go:56:2: Use %q to quote values instead of manually quoting them`,
		`test.go:64:2: The number of verbs in format literal mismatches with the number of arguments`,
		`test.go:65:2: The number of verbs in format literal mismatches with the number of arguments`,
		`test.go:66:2: The number of verbs in format literal mismatches with the number of arguments`,
		`test.go:74:2: Error message should have some surrounding context, so must not empty`,
		`test.go:75:2: Error message should have some surrounding context, so must not empty`,
		`test.go:76:2: Error message should have some surrounding context, so must not empty`,
		`test.go:77:2: Error message should have some surrounding context, so must not empty`,
		`test.go:85:2: Test failure messages should be capitalized`,
		`test.go:87:2: Test failure messages should be capitalized`,
		`test.go:89:2: Log messages should be capitalized`,
		`test.go:91:2: Log messages should be capitalized`,
		`test.go:93:2: Messages of the error type should not be capitalized`,
		`test.go:95:2: Messages of the error type should not be capitalized`,
		`test.go:102:2: Use s.Errorf(...) instead of s.Error(fmt.Sprintf(...))`,
		`test.go:103:2: Use errors.Errorf(...) instead of errors.New(fmt.Sprintf(...))`,
		`test.go:104:2: Use errors.Wrapf(err, ...) instead of errors.Wrap(err, fmt.Sprintf(...))`,
		`test.go:110:2: s.Error has verbs in the first string (do you mean s.Errorf?)`,
		`test.go:111:2: errors.Wrap has verbs in the first string (do you mean errors.Wrapf?)`,
		`test.go:112:2: testing.ContextLog has verbs in the first string (do you mean testing.ContextLogf?)`,
	}
	verifyIssues(t, issues, expects)
}

func TestAutoFixMessages(t *testing.T) {
	files := make(map[string]string)
	expects := make(map[string]string)
	const filename1 = "foo.go"
	files[filename1] = `package pkg

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

func Test(ctx context.Context, s *testing.State) {
	var err error

	s.Logf("Should use Log for single string arg")
	s.Errorf("Just a string")
	s.Fatalf("Just a string")
	errors.Errorf("got just a string with punctuation!")
	testing.ContextLogf(ctx, "got just a small string")
	s.Errorf("found case 1 but ignore case 1 if there is format identifier '%s'.")
	s.Fatalf("got just a small string with punctuation.")

	s.Error(fmt.Sprintf("Foo (%d)", 42))
	errors.New(fmt.Sprintf("foo (%d)", 42))
	errors.Wrap(err, fmt.Sprintf("foo (%d)", 42))
	s.Logf(fmt.Sprintf("something %s", foo))

	s.Logf("Should use Log for single trailing %v", err) // CASE 3->4
	testing.ContextLogf(ctx, "Should've used ContextLog: %v", err)
	s.Errorf("unexpected usage Errorf for single trailing %v", err) // CASE 3+4->10

	s.Log("Should end with colon and space ", err)
	s.Error("Should end with colon and space:", err)
	s.Fatal("Should end with colon and space.", err)
	testing.ContextLog(ctx, "found lower letter and end with ! instead of colon!", err)

	errors.Errorf("should use Wrap: %v", err)
	errors.Errorf("should use Wrapf %s%s: %v", "h", "ere", err)
	errors.Errorf("Found use Errorf and upper letter: %v", err)
	errors.Errorf("Found use Errorf, \"%s\" letter and %s: %v", "upper", "invalid quate", err)

	s.Log("Shouldn't use trailing period.")
	errors.New("shouldn't use trailing period.")
	testing.ContextLogf(ctx, "\"%s\" should be %q not the \"%s\".", "\"%%s\"", "%%q", "\"%%s\"")

	s.Logf("Read value '%s' \"%s\"", "blah", "blah")
	s.Errorf("can read value \"%v\" '%v'", "blah", "blah")

	errors.New("Could not start ARC")
	errors.Wrapf(err, "Too many (%d) files open", 28)

	s.Log("got messages")
	testing.ContextLogf(ctx, "found a file %q", "blah")

	s.Error("failed to start ARC: ", err)
	s.Fatalf("unexpected string %q received", "blah")

	s.Errorf(fmt.Sprintf("bar"))
	s.Error(fmt.Sprintf("bar"))

	// Won't change below.
	s.Log("Hello\x20world")

}
`
	expects[filename1] = `package pkg

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

func Test(ctx context.Context, s *testing.State) {
	var err error

	s.Log("Should use Log for single string arg")
	s.Error("Just a string")
	s.Fatal("Just a string")
	errors.New("got just a string with punctuation")
	testing.ContextLog(ctx, "Got just a small string")
	s.Errorf("Found case 1 but ignore case 1 if there is format identifier %q")
	s.Fatal("Got just a small string with punctuation")

	s.Errorf("Foo (%d)", 42)
	errors.Errorf("foo (%d)", 42)
	errors.Wrapf(err, "foo (%d)", 42)
	s.Logf("something %s", foo)

	s.Log("Should use Log for single trailing: ", err) // CASE 3->4
	testing.ContextLog(ctx, "Should've used ContextLog: ", err)
	s.Error("Unexpected usage Errorf for single trailing: ", err) // CASE 3+4->10

	s.Log("Should end with colon and space: ", err)
	s.Error("Should end with colon and space: ", err)
	s.Fatal("Should end with colon and space: ", err)
	testing.ContextLog(ctx, "Found lower letter and end with ! instead of colon: ", err)

	errors.Wrap(err, "should use Wrap")
	errors.Wrapf(err, "should use Wrapf %s%s", "h", "ere")
	errors.Wrap(err, "found use Errorf and upper letter")
	errors.Wrapf(err, "found use Errorf, %q letter and %s", "upper", "invalid quate")

	s.Log("Shouldn't use trailing period")
	errors.New("shouldn't use trailing period")
	testing.ContextLogf(ctx, "%q should be %q not the %q", "\"%%s\"", "%%q", "\"%%s\"")

	s.Logf("Read value %q %q", "blah", "blah")
	s.Errorf("Can read value %q %q", "blah", "blah")

	errors.New("could not start ARC")
	errors.Wrapf(err, "too many (%d) files open", 28)

	s.Log("Got messages")
	testing.ContextLogf(ctx, "Found a file %q", "blah")

	s.Error("Failed to start ARC: ", err)
	s.Fatalf("Unexpected string %q received", "blah")

	s.Error("bar")
	s.Error("bar")

	// Won't change below.
	s.Log("Hello\x20world")

}
`
	const filename2 = "bar.go"
	files[filename2] = `package new

import "chromiumos/tast/errors"

func main() {
	return errors.Errorf("should use Wrapf %s%s: %v", "h", "ere", err)
}
`
	expects[filename2] = `package new

import "chromiumos/tast/errors"

func main() {
	return errors.Wrapf(err, "should use Wrapf %s%s", "h", "ere")
}
`
	verifyAutoFix(t, Messages, files, expects)
}
