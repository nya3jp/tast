// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestDeprecatedAPIs(t *testing.T) {
	deprecated := []*deprecatedAPI{{
		pkg:         "chromiumos/tast/local/testexec",
		alternative: "chromiumos/tast/common/testexec",
		link:        "https://crbug.com/1119252",
	}, {
		pkg:         "chromiumos/tast/bundle",
		ident:       "LocalDelegate",
		alternative: "Delegate",
		link:        "https://crbug.com/1134060",
	}}
	const code = `package main

import (
	b "chromiumos/tast/bundle"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/local/testexec"
)

func main() {
	testexec.CommandContext(ctx, "cat")
	f(b.LocalDelegate)
	_ = a.b.LocalDelegate // ok
	f(bundle.LocalDelegate) // ok
}
`
	want := []string{
		"testfile.go:6:2: package chromiumos/tast/local/testexec is deprecated; use chromiumos/tast/common/testexec instead",
		"testfile.go:11:4: chromiumos/tast/bundle.LocalDelegate is deprecated; use Delegate instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := deprecatedAPIs(fs, f, deprecated)
	verifyIssues(t, issues, want)
}
