// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/process"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/testingutil"
)

// PollOptions may be passed to Poll to configure its behavior.
type PollOptions = testingutil.PollOptions

// PollBreak creates an error wrapping err that may be returned from a
// function passed to Poll to terminate polling immediately. For example:
//
//   err := testing.Poll(ctx, func(ctx context.Context) error {
//     if err := mustSucceed(ctx); err != nil {
//       return testing.PollBreak(err)
//     }
//     ...
//   })
func PollBreak(err error) error {
	return testingutil.PollBreak(err)
}

// Poll runs f repeatedly until f returns nil and then itself returns nil.
// If ctx returns an error before then or opts.Timeout is reached, the last error returned by f is returned.
// f should use the context passed to it, as it may have an adjusted deadline if opts.Timeout is set.
// If ctx's deadline has already been reached, f will not be invoked.
// If opts is nil, reasonable defaults are used.
//
// Polling often results in increased load and slower execution (since there's a delay between when something
// happens and when the next polling cycle notices it). It should only be used as a last resort when there's no
// other way to watch for an event. The preferred approach is to watch for events in select{} statements.
// Goroutines can be used to provide notifications over channels.
// If an error wrapped by PollBreak is returned, then it
// immediately terminates the polling, and returns the unwrapped error.
func Poll(ctx context.Context, f func(context.Context) error, opts *PollOptions) error {
	return testingutil.Poll(ctx, f, opts)
}

// Sleep pauses the current goroutine for d or until ctx expires.
//
// Please consider using testing.Poll instead. Sleeping without polling for
// a condition is discouraged, since it makes tests flakier (when the sleep
// duration isn't long enough) or slower (when the duration is too long).
func Sleep(ctx context.Context, d time.Duration) error {
	return testingutil.Sleep(ctx, d)
}

// KillProcessByName sends SIGINT to the first process found with the given
// name, and then waits for the process to exit before returning.
//
// If no process with that name is found, returns nil.
// If killing the process fails, or if the process does not exit within 10
// seconds, returns an error.
func KillProcessByName(ctx context.Context, name string) error {
	ps, err := process.Processes()
	if err != nil {
		return errors.Wrap(err, "failed to list running processes")
	}

	for _, p := range ps {
		if processName, err := p.Name(); err != nil || processName != name {
			continue
		}

		if err := syscall.Kill(int(p.Pid), syscall.SIGINT); err != nil && err != syscall.ESRCH {
			return errors.Wrapf(err, "failed to kill %s", name)
		}

		// Wait for the process to exit.
		if err := Poll(ctx, func(ctx context.Context) error {
			// We need a fresh process.Process since it caches attributes.
			// TODO(crbug.com/1131511): Clean up error handling here when gpsutil has been upreved.
			if _, err := process.NewProcess(p.Pid); err == nil {
				return errors.Errorf("pid %d is still running", p.Pid)
			}
			return nil
		}, &PollOptions{Timeout: 10 * time.Second}); err != nil {
			return errors.Wrapf(err, "failed to wait for %s to exit", name)
		}
	}
	return nil
}
