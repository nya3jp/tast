// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package upstart supports interacting with the Upstart init daemon on behalf of
// local tests.
package upstart

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var runningRegexp *regexp.Regexp

func init() {
	runningRegexp = regexp.MustCompile("^[^ ]+ start/running, process (\\d+)$")
}

// JobStatus returns the current status of job.
func JobStatus(job string) (running bool, pid int, err error) {
	b, err := exec.Command("initctl", "status", job).CombinedOutput()
	if err != nil {
		return false, 0, err
	}

	out := strings.TrimSpace(string(b))
	if !strings.HasPrefix(out, job+" ") {
		return false, 0, fmt.Errorf("unexpected \"status\" output %q", out)
	}

	m := runningRegexp.FindStringSubmatch(out)
	if m == nil {
		return false, 0, nil
	}

	p, err := strconv.ParseInt(m[1], 10, 32)
	if err != nil {
		return false, 0, fmt.Errorf("unable to parse PID from %q", out)
	}
	return true, int(p), nil
}

// RestartJob restarts job. If the job is currently stopped, it will be started.
func RestartJob(job string) error {
	running, _, err := JobStatus(job)
	if err != nil {
		return err
	}

	var cmd string
	if running {
		cmd = "restart"
	} else {
		cmd = "start"
	}
	out, err := exec.Command("initctl", cmd, job).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restarting job %q failed: %v: %s",
			job, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// StopJob stops job. If it is not currently running, this is a no-op.
func StopJob(job string) error {
	running, _, err := JobStatus(job)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	out, err := exec.Command("initctl", "stop", job).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stopping job %q failed: %v: %s",
			job, err, strings.TrimSpace(string(out)))
	}
	return nil
}
