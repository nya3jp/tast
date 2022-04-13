// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package command

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"
)

var selfName = filepath.Base(os.Args[0])

// InstallSignalHandler installs a signal handler that calls callback and
// runs common logic (e.g. dumping stack traces). out is the output stream to
// write messages to (typically stderr).
func InstallSignalHandler(out io.Writer, callback func(sig os.Signal)) {
	ch := make(chan os.Signal, 1)
	go func() {
		sig := <-ch
		fmt.Fprintf(out, "\n%s: Caught %v signal; exiting\n", selfName, sig)
		callback(sig)
		if sig == unix.SIGTERM {
			handleSIGTERM(out)
		}
		os.Exit(1)
	}()
	signal.Notify(ch, unix.SIGINT, unix.SIGTERM)
}

func handleSIGTERM(out io.Writer) {
	// SIGTERM is often sent by the parent process on timeout. In this
	// case, print stack traces to help debugging.
	fmt.Fprintf(out, "\n%s: Dumping all goroutines...\n\n", selfName)
	if p := pprof.Lookup("goroutine"); p != nil {
		p.WriteTo(out, 2)
	}
	fmt.Fprintf(out, "\n%s: Finished dumping goroutines\n", selfName)

	// Also terminate all child processes with SIGTERM. This can recursively
	// print stack traces.
	procs, err := process.Processes()
	if err != nil {
		fmt.Fprintf(out, "Failed to terminate subprocesses: %v\n", err)
		return
	}

	selfPid := int32(os.Getpid())

	for _, proc := range procs {
		ppid, err := proc.Ppid()
		if err != nil {
			continue
		}
		if ppid == selfPid {
			proc.Terminate()
		}
	}
}
