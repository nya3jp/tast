// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"
)

const (
	stderrTimeout time.Duration = time.Second // time to wait for stderr on command failure
)

// firstLineReader reads and returns the first line from a reader, discarding the rest.
// It's useful for consuming stderr from asynchronous commands.
type firstLineReader struct {
	sch chan string
	ech chan error
}

// newFirstLineReader returns a new reader for getting the first line from r.
func newFirstLineReader(r io.Reader) *firstLineReader {
	f := &firstLineReader{make(chan string, 1), make(chan error, 1)}
	go func() {
		if ln, err := bufio.NewReader(r).ReadString('\n'); err != nil {
			f.ech <- err
		} else {
			f.sch <- strings.TrimSpace(ln)
		}
		io.Copy(ioutil.Discard, r)
	}()
	return f
}

// getLine returns the first line that was read, waiting up to timeout for the line.
// If an error or timeout is encountered, an error is returned, and for
// convenience the returned string also contains the error message.
func (f *firstLineReader) getLine(timeout time.Duration) (string, error) {
	select {
	case s := <-f.sch:
		return s, nil
	case err := <-f.ech:
		return err.Error(), err
	case <-time.After(timeout):
		err := errors.New("read timed out")
		return err.Error(), err
	}
}

// appendToError waits up to timeout for the first line and returns a new error containing both
// origErr and the line. If no line is available, origErr is returned unchanged.
func (f *firstLineReader) appendToError(origErr error, timeout time.Duration) error {
	s, err := f.getLine(timeout)
	if err != nil || s == "" {
		return origErr
	}
	return fmt.Errorf("%v: %v", origErr, s)
}
