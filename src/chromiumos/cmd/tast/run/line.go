// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"fmt"
	"sync"
)

// firstLineBuffer is like bytes.Buffer, but it saves only the first line of the written data
// and discards everything else.
// It is safe to call its methods concurrently from multiple goroutines.
// The zero value for firstLineBuffer is ready to use.
// It's useful for consuming stderr from asynchronous commands.
type firstLineBuffer struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done bool
}

// Write implements io.Writer.
func (f *firstLineBuffer) Write(p []byte) (int, error) {
	if f.done {
		return len(p), nil
	}

	var i int
	for i = 0; i < len(p); i++ {
		if p[i] == '\n' {
			f.done = true
			break
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.buf.Write(p[:i])
	return len(p), nil
}

// FirstLine returns the first line that was written.
func (f *firstLineBuffer) FirstLine() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.String()
}

// AppendToError waits up to timeout for the first line and returns a new error containing both
// origErr and the line. If no line is available, origErr is returned unchanged.
func (f *firstLineBuffer) AppendToError(origErr error) error {
	s := f.FirstLine()
	if s == "" {
		return origErr
	}
	return fmt.Errorf("%v: %v", origErr, s)
}
