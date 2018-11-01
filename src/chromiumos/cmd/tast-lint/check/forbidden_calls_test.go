// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestForbiddenCalls(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"time"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	fmt.Errorf("foo")
	errors.Errorf("foo")
	time.Sleep(time.Second)
}
`
	expects := []string{
		"testfile.go:12:2: chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
		"testfile.go:14:2: time.Sleep ignores context deadline; use time.After instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := ForbiddenCalls(fs, f)
	verifyIssues(t, issues, expects)
}
