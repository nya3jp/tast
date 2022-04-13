// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"
)

// killSession makes a best-effort attempt to kill all processes in session sid.
// It makes several passes over the list of running processes, sending sig to any
// that are part of the session. After it doesn't find any new processes, it returns.
// Note that this is racy: it's possible (but hopefully unlikely) that continually-forking
// processes could spawn children that don't get killed.
func killSession(sid int, sig unix.Signal) {
	const maxPasses = 3
	for i := 0; i < maxPasses; i++ {
		pids, err := process.Pids()
		if err != nil {
			return
		}
		n := 0
		for _, pid := range pids {
			pid := int(pid)
			if s, err := unix.Getsid(pid); err == nil && s == sid {
				unix.Kill(pid, sig)
				n++
			}
		}
		// If we didn't find any processes in the session, we're done.
		if n == 0 {
			return
		}
	}
}
