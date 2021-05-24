// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtest

import (
	"strings"
	"testing"
)

type testLogWriter testing.T

func (w *testLogWriter) Write(p []byte) (int, error) {
	(*testing.T)(w).Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func newTestLogWriter(t *testing.T) *testLogWriter {
	return (*testLogWriter)(t)
}
