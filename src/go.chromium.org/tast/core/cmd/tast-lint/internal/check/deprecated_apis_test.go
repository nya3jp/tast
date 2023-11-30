// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestDeprecatedAPIs(t *testing.T) {
	const code = `package main

import (
	"golang.org/x/net/context"
	"go.chromium.org/tast-tests/cros/local/testexec"
	"syscall"
)

func main() {
	testexec.CommandContext(ctx, "cat") // not ok
	context.Context // not ok
	syscall.stat_t // not ok
	syscall.Stat_t // ok
	syscall.rawconn // not ok
	syscall.RawConn // ok
	syscall.coNN // not ok
	syscall.Conn // ok
	syscall.sysProcAttr // not ok
	syscall.SysProcAttr // ok
	syscall.waitStatus // not ok
	syscall.WaitStatus // ok
	syscall.rusage // not ok
	syscall.Rusage // ok
	syscall.credential // not ok
	syscall.Credential // ok
	syscall.seteuid // not ok
	syscall.Seteuid // ok
}
`
	want := []string{
		"testfile.go:4:2: package golang.org/x/net/context is deprecated; use context instead",
		"testfile.go:5:2: package go.chromium.org/tast-tests/cros/local/testexec is deprecated; use go.chromium.org/tast-tests/cros/common/testexec instead",
		"testfile.go:12:2: syscall.stat_t is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:14:2: syscall.rawconn is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:16:2: syscall.coNN is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:18:2: syscall.sysProcAttr is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:20:2: syscall.waitStatus is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:22:2: syscall.rusage is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:24:2: syscall.credential is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
		"testfile.go:26:2: syscall.seteuid is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := DeprecatedAPIs(fs, f)
	verifyIssues(t, issues, want)
}

func TestDeprecatedAPIsInternal(t *testing.T) {
	deprecated := []*deprecatedAPI{{
		pkg:         "go.chromium.org/tast-tests/cros/local/testexec",
		alternative: "go.chromium.org/tast-tests/cros/common/testexec",
		link:        "https://crbug.com/1119252",
	}, {
		pkg:         "go.chromium.org/tast/core/bundle",
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
	b "go.chromium.org/tast/core/bundle"
	"go.chromium.org/tast/core/internal/bundle"
	"go.chromium.org/tast-tests/cros/local/testexec"
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
		"testfile.go:6:2: package go.chromium.org/tast-tests/cros/local/testexec is deprecated; use go.chromium.org/tast-tests/cros/common/testexec instead",
		"testfile.go:12:4: go.chromium.org/tast/core/bundle.LocalDelegate is deprecated; use Delegate instead",
		"testfile.go:15:2: syscall.not_stat_t is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := deprecatedAPIs(fs, f, deprecated)
	verifyIssues(t, issues, want)
}

func TestDeprecatedAPIsWithExclusion(t *testing.T) {
	deprecated := []*deprecatedAPI{{
		pkg:         "go.chromium.org/tast-tests/cros/local/testexec",
		alternative: "go.chromium.org/tast-tests/cros/common/testexec",
		link:        "https://crbug.com/1119252",
	}, {
		pkg:         "syscall",
		alternative: "golang.org/x/sys/unix",
		exclusion:   map[string]struct{}{"Stat_t": {}},
		link:        "https://buganizer.corp.google.com/issues/187787902",
	}}
	const code = `package main

import (
	"go.chromium.org/tast-tests/cros/local/testexec"
	"syscall"
)

func main() {
	testexec.CommandContext(ctx, "cat")
	syscall.Stat_t // ok
	syscall.SIGSEGV // not ok
}
`
	want := []string{
		"testfile.go:4:2: package go.chromium.org/tast-tests/cros/local/testexec is deprecated; use go.chromium.org/tast-tests/cros/common/testexec instead",
		"testfile.go:11:2: syscall.SIGSEGV is from a deprecated package; use corresponding API in golang.org/x/sys/unix instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := deprecatedAPIs(fs, f, deprecated)
	verifyIssues(t, issues, want)
}

func TestDeprecatedAPIsWithExclusionSameName(t *testing.T) {
	deprecated := []*deprecatedAPI{{
		pkg:         "go.chromium.org/tast-tests/cros/local/testexec",
		alternative: "go.chromium.org/tast-tests/cros/common/testexec",
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
	"go.chromium.org/tast-tests/cros/local/testexec"
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
		"testfile.go:4:2: package go.chromium.org/tast-tests/cros/local/testexec is deprecated; use go.chromium.org/tast-tests/cros/common/testexec instead",
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
