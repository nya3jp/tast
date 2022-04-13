// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"
)

func TestKillSession(t *testing.T) {
	// Start a shell in a new session that runs sleep.
	// We can't tell the shell to run "sleep 60" directly since it execs sleep then.
	cmd := exec.Command("/bin/sh", "-c", "true && sleep 60; true")
	cmd.SysProcAttr = &unix.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal("Failed to start command: ", err)
	}
	sid := cmd.Process.Pid // session ID matches PID

	// Waits up to maxTime for there to be num processes in session sid.
	waitForProcs := func(num int, maxTime time.Duration) error {
		start := time.Now()
		for {
			all, err := process.Processes()
			if err != nil {
				return err
			}

			matched := make(map[int]string) // keys are PIDs, values are command lines
			for _, p := range all {
				if s, err := unix.Getsid(int(p.Pid)); err == nil && s == sid {
					cl, _ := p.Cmdline()
					matched[int(p.Pid)] = cl
				}
			}

			if len(matched) == num {
				return nil
			} else if time.Now().Sub(start) > maxTime {
				return fmt.Errorf("got %v proc(s): %v", len(matched), matched)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Wait for the sh and sleep processes to show up.
	if err := waitForProcs(2, 10*time.Second); err != nil {
		t.Errorf("Didn't get 2 initial procs: %v", err)
	}

	// After killing the session and calling wait() on sh (to remove its process entry),
	// both processes should disappear.
	killSession(sid, unix.SIGKILL)
	go cmd.Wait() // avoid blocking forever if killSession is broken
	if err := waitForProcs(0, 10*time.Second); err != nil {
		t.Errorf("Didn't get 0 procs after calling killSession: %v", err)
	}
}
