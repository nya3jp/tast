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
	}
	verifyIssues(t, issues, expects)
}
