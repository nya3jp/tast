// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// This file contains code shared by unit tests that read Output messages produced by tests.

// outputReader implements an infinitely-buffered chan Output.
// This is useful for unit tests that check tests' output.
// After passing ch to newState, they can call methods on State that write
// to ch without worrying about blocking due to the channel being full.
type outputReader struct {
	ch   chan Output   // test output is written here
	done chan struct{} // used to signal that out is complete
	out  []Output      // contains output read from ch
}

func newOutputReader() *outputReader {
	or := &outputReader{
		ch:   make(chan Output),
		done: make(chan struct{}, 1),
	}
	// Start a goroutine that drains or.ch to or.out.
	go func() {
		for o := range or.ch {
			or.out = append(or.out, o)
		}
		or.done <- struct{}{}
	}()
	return or
}

// read blocks until or.ch is closed and returns all messages that were written to it.
func (or *outputReader) read() []Output {
	<-or.done
	return or.out
}

// getOutputErrors returns all errors from out.
func getOutputErrors(out []Output) []*Error {
	errs := make([]*Error, 0)
	for _, o := range out {
		if o.Err != nil {
			errs = append(errs, o.Err)
		}
	}
	return errs
}

// findLog checks if out contains the specified log message.
func findLog(out []Output, msg string) bool {
	for _, o := range out {
		if o.Msg == msg {
			return true
		}
	}
	return false
}
