// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// This file contains code shared by unit tests that read Output messages produced by tests.

// OutputReader implements an infinitely-buffered chan Output.
// This is useful for unit tests that check tests' output.
// After passing ch to newState, they can call methods on State that write
// to ch without worrying about blocking due to the channel being full.
type OutputReader struct {
	Ch   chan Output   // test output is written here
	done chan struct{} // used to signal that out is complete
	out  []Output      // contains output read from Ch
}

// NewOutputReader constructs a new OutputReader.
func NewOutputReader() *OutputReader {
	or := &OutputReader{
		Ch:   make(chan Output),
		done: make(chan struct{}, 1),
	}
	// Start a goroutine that drains or.ch to or.out.
	go func() {
		for o := range or.Ch {
			or.out = append(or.out, o)
		}
		or.done <- struct{}{}
	}()
	return or
}

// Read blocks until or.ch is closed and returns all messages that were written to it.
func (or *OutputReader) Read() []Output {
	<-or.done
	return or.out
}

// GetOutputErrors returns all errors from out.
func GetOutputErrors(out []Output) []*Error {
	errs := make([]*Error, 0)
	for _, o := range out {
		if o.Err != nil {
			errs = append(errs, o.Err)
		}
	}
	return errs
}
