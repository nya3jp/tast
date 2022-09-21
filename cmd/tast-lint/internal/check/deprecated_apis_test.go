// Copyright 2021 The ChromiumOS Authors
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
	}, {
		pkg:         "syscall",
		alternative: "golang.org/x/sys/unix",
		exclusion:   map[string]struct{}{"stat_t": {}},
		link:        "https://buganizer.corp.google.com/issues/187787902",
	}}
	const code = `package main

import (
	b "chromiumos/tast/bundle"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/local/testexec"
	"syscall"
)

func main() {
	testexec.CommandContext(ctx, "cat")
	f(b.LocalDelegate)
	_ = a.b.LocalDelegate // ok
	f(bundle.LocalDelegate) // ok
	syscall.not_stat_t // not ok
}
`
	want := []string{
		"testfile.go:6:2: package chromiumos/tast/local/testexec is deprecated; use chromiumos/tast/common/testexec instead",
		"testfile.go:12:4: chromiumos/tast/bundle.LocalDelegate is deprecated; use Delegate instead",
		"testfile.go:15:2: syscall.not_stat_t is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := deprecatedAPIs(fs, f, deprecated)
	verifyIssues(t, issues, want)
}

func TestDeprecatedAPIsWithExclusion(t *testing.T) {
	deprecated := []*deprecatedAPI{{
		pkg:         "chromiumos/tast/local/testexec",
		alternative: "chromiumos/tast/common/testexec",
		link:        "https://crbug.com/1119252",
	}, {
		pkg:         "syscall",
		alternative: "golang.org/x/sys/unix",
		exclusion:   map[string]struct{}{"Stat_t": {}},
		link:        "https://buganizer.corp.google.com/issues/187787902",
	}}
	const code = `package main

import (
	"chromiumos/tast/local/testexec"
	"syscall"
)

func main() {
	testexec.CommandContext(ctx, "cat")
	syscall.Stat_t // ok
	syscall.SIGSEGV // not ok
}
`
	want := []string{
		"testfile.go:4:2: package chromiumos/tast/local/testexec is deprecated; use chromiumos/tast/common/testexec instead",
		"testfile.go:11:2: syscall.SIGSEGV is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := deprecatedAPIs(fs, f, deprecated)
	verifyIssues(t, issues, want)
}

func TestDeprecatedAPIsWithExclusionSameName(t *testing.T) {
	deprecated := []*deprecatedAPI{{
		pkg:         "chromiumos/tast/local/testexec",
		alternative: "chromiumos/tast/common/testexec",
		link:        "https://crbug.com/1119252",
	}, {
		pkg:         "syscall",
		alternative: "golang.org/x/sys/unix",
		exclusion:   map[string]struct{}{"Stat_t": {}},
		link:        "https://buganizer.corp.google.com/issues/187787902",
	}, {
		pkg:         "syscall2",
		alternative: "golang.org/x/sys/unix",
		link:        "https://buganizer.corp.google.com/issues/187787902",
	}, {
		pkg:         "syscall3",
		alternative: "golang.org/x/sys/unix",
		exclusion: map[string]struct{}{
			"Stat_t":      {},
			"RawConn":     {},
			"Conn":        {},
			"SysProcAttr": {},
			"WaitStatus":  {},
			"Rusage":      {},
			"Credential":  {},
		},
		link: "https://buganizer.corp.google.com/issues/187787902",
	}}
	const code = `package main

import (
	"chromiumos/tast/local/testexec"
	"syscall"
	"syscall2"
	"syscall3"
)

func main() {
	testexec.CommandContext(ctx, "cat")
	syscall2.stat_t // not ok
	syscall3.stat_t // not ok
	syscall3.Stat_t // ok
	syscall3.rawconn // not ok
	syscall3.RawConn // ok
	syscall3.coNN // not ok
	syscall3.Conn // ok
	syscall3.sysProcAttr // not ok
	syscall3.SysProcAttr // ok
	syscall3.waitStatus // not ok
	syscall3.WaitStatus // ok
	syscall3.rusage // not ok
	syscall3.Rusage // ok
	syscall3.credential // not ok
	syscall3.Credential // ok
}
`
	want := []string{
		"testfile.go:4:2: package chromiumos/tast/local/testexec is deprecated; use chromiumos/tast/common/testexec instead",
		"testfile.go:6:2: package syscall2 is deprecated; use golang.org/x/sys/unix instead",
		"testfile.go:13:2: syscall3.stat_t is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:15:2: syscall3.rawconn is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:17:2: syscall3.coNN is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:19:2: syscall3.sysProcAttr is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:21:2: syscall3.waitStatus is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:23:2: syscall3.rusage is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:25:2: syscall3.credential is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := deprecatedAPIs(fs, f, deprecated)
	verifyIssues(t, issues, want)
}
