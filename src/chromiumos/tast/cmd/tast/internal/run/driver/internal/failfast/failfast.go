// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package failfast provides a utility to track test failures and fail fast.
package failfast

import (
	"chromiumos/tast/errors"
)

// Counter counts test failures and aborts test execution if it passes a given
// threshold.
// nil is a valid Counter that never aborts test execution just as if it has
// infinite threshold.
type Counter struct {
	threshold int
	fails     int
}

// NewCounter constructs a Counter. If threshold is not positive, it returns
// nil, which is a valid Counter that never aborts test execution just as if it
// has infinite threshold.
func NewCounter(threshold int) *Counter {
	if threshold <= 0 {
		return nil
	}
	return &Counter{threshold: threshold, fails: 0}
}

// Increment increments the number of test failures.
func (c *Counter) Increment() {
	if c == nil {
		return
	}
	c.fails++
}

// Check checks the current number of test failures against the threshold. If
// it is no less than the threshold, Check returns an error.
func (c *Counter) Check() error {
	if c == nil {
		return nil
	}
	if c.fails >= c.threshold {
		return errors.Errorf("aborting due to too many failures (%d)", c.fails)
	}
	return nil
}
