// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:build linux
// +build linux

package runner

import (
	"os"
	"testing"
	"time"
)

func TestPipeWatcher(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	defer w.Close()

	pw, err := newPipeWatcher(int(w.Fd()))
	if err != nil {
		t.Fatalf("Failed to create watcher for FD %d: %v", w.Fd(), err)
	}

	// The watcher shouldn't initially report closure.
	select {
	case <-time.After(10 * time.Millisecond):
	case <-pw.readClosed:
		t.Fatalf("FD %d initially reported as having closed read end", w.Fd())
	}

	// After we close the read end of the pipe, closure should be reported.
	r.Close()
	r = nil // disarm cleanup
	select {
	case <-time.After(time.Minute):
		t.Errorf("FD %d not reported as having closed read end", w.Fd())
	case <-pw.readClosed:
	}

	if err := pw.close(); err != nil {
		t.Error("close() failed: ", err)
	}
}
