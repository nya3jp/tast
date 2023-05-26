// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestForbiddenFlashromSubprocess_CommandContext(t *testing.T) {
	const code = `package main

import "chromiumos/tast/local/bundles/cros/example/util"

func init() {
	testing.AddTest(&testing.Test{
		Func: Test,
		Desc: "It test flashrom",
		Contacts: []string{
			"flashrom@google.com",
		},SoftwareDeps: []string{"flashrom"},
	})
}

func Test() {
	h.DUT.Conn().CommandContext(ctx, "flashrom", "-p", "host", "-r").Output(ssh.DumpLogOnError)
	testexec.CommandContext(ctx, "flashrom", "-p", "host", "-r")
	h.DUT.Conn().CommandContext(ctx, "/usr/sbin/flashrom", "-p", "host", "-r").Output(ssh.DumpLogOnError)
	testexec.CommandContext(ctx, "/usr/sbin/flashrom", "-p", "host", "-r")
}
`

	for _, tc := range []struct {
		filepath string
		want     []string
	}{
		{
			filepath: "src/chromiumos/tast/local/testfile.go",
			want: []string{
				"src/chromiumos/tast/local/testfile.go:16:2: Please don't use flashrom subprocess but use flashrom_library instead.",
				"src/chromiumos/tast/local/testfile.go:17:2: Please don't use flashrom subprocess but use flashrom_library instead.",
				"src/chromiumos/tast/local/testfile.go:18:2: Please don't use flashrom subprocess but use flashrom_library instead.",
				"src/chromiumos/tast/local/testfile.go:19:2: Please don't use flashrom subprocess but use flashrom_library instead.",
			},
		},
		{
			filepath: "src/chromiumos/tast/remote/testfile.go",
			want: []string{
				"src/chromiumos/tast/remote/testfile.go:16:2: Please don't use flashrom subprocess but use flashrom_library instead.",
				"src/chromiumos/tast/remote/testfile.go:17:2: Please don't use flashrom subprocess but use flashrom_library instead.",
				"src/chromiumos/tast/remote/testfile.go:18:2: Please don't use flashrom subprocess but use flashrom_library instead.",
				"src/chromiumos/tast/remote/testfile.go:19:2: Please don't use flashrom subprocess but use flashrom_library instead.",
			},
		},
		{
			filepath: "src/go.chromium.org/tast-tests/cros/common/testfile.go",
			want:     []string{},
		},
	} {
		f, fs := parse(code, tc.filepath)
		issues := ForbiddenFlashromSubprocess(fs, f)
		verifyIssues(t, issues, tc.want)
	}
}
