// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package control writes and reads control messages describing the state of a test run.
//
// Control messages are JSON-marshaled and used for communication from test
// executables to the main tast binary. A typical sequence is as follows:
//
//	RunStart (run started)
//		RunLog (run logged a message)
//		TestStart (first test started)
//			TestLog (first test logged a message)
//		TestEnd (first test ended)
//		TestStart (second test started)
//			TestLog (second test logged a message)
//			TestError (second test encountered an error)
//			TestError (second test encountered another error)
//			TestLog (second test logged another message)
//		TestEnd (second test ended)
//	RunEnd (run ended)
//
// Control messages of different types are unmarshaled into a single messageUnion
// struct. To be able to infer a message's type, each message struct must contain
// a Time field with a message-type-prefixed JSON name (e.g. "runStartTime" for
// RunStart.Time), and all other fields must be similarly namespaced.
package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

// RunStart describes the start of a run (consisting of one or more tests).
type RunStart struct {
	// Time is the device-local time at which the run started.
	Time time.Time `json:"runStartTime"`
	// TestNames contains the names of tests to run, in the order in which they'll be executed.
	// Note that some of these tests may later be skipped (see TestEnd.MissingSoftwareDeps).
	TestNames []string `json:"runStartTestNames"`
	// NumTests is the number of tests that will be run.
	// TODO(derat): Delete this after 20190715; the tast command now uses TestNames instead: https://crbug.com/889119
	NumTests int `json:"runStartNumTests"`
}

// RunLog contains an informative, high-level logging message produced by a run.
type RunLog struct {
	// Time is the device-local time at which the message was logged.
	Time time.Time `json:"runLogTime"`
	// Text is the actual message.
	Text string `json:"runLogText"`
}

// RunError describes a fatal, high-level error encountered during the run.
// This may be encountered at any time (including before RunStart) and
// indicates that the run has been aborted.
type RunError struct {
	// Time is the device-local time at which the error occurred.
	Time time.Time `json:"runErrorTime"`
	// Error describes the error that occurred.
	Error testing.Error `json:"runErrorError"`
	// Status is the exit status code of the test runner if it is run directly.
	Status int `json:"runErrorStatus"`
}

// RunEnd describes the completion of a run.
type RunEnd struct {
	// Time is the device-local time at which the run ended.
	Time time.Time `json:"runEndTime"`
	// OutDir is the base directory under which tests wrote output files.
	OutDir string `json:"runEndOutDir"`
}

// TestStart describes the start of an individual test.
type TestStart struct {
	// Time is the device-local time at which the test started.
	Time time.Time `json:"testStartTime"`
	// Test contains details about the test.
	// Some fields, e.g. Func (containing the test function), are dropped during marshaling.
	Test testing.TestCase `json:"testStartTest"`
}

// TestLog contains an informative logging message produced by a test.
type TestLog struct {
	// Time is the device-local time at which the message was logged.
	Time time.Time `json:"testLogTime"`
	// Text is the actual message.
	Text string `json:"testLogText"`
}

// TestError contains an error produced by a test.
type TestError struct {
	// Time is the device-local time at which the error occurred.
	Time time.Time `json:"testErrorTime"`
	// Error describes the error that occurred.
	Error testing.Error `json:"testErrorError"`
}

// TestEnd describes the end of an individual test.
type TestEnd struct {
	// Time is the device-local time at which the test ended.
	Time time.Time `json:"testEndTime"`
	// Name is the name of the test, matching the earlier TestStart.Test.Name.
	Name string `json:"testEndName"`
	// MissingSoftwareDeps contains software dependencies declared by the test that were
	// not present on the DUT. If non-empty, the test was skipped.
	MissingSoftwareDeps []string `json:"testEndMissingSoftwareDeps"`
	// TimingLog contains test-reported timing information to be incorporated into the main timing.json file.
	TimingLog *timing.Log `json:"testEndTimingLog"`
}

// Heartbeat is sent periodically to assert that the bundle is alive.
type Heartbeat struct {
	// Time is the device-local time at which this message was generated.
	Time time.Time `json:"heartbeatTime"`
}

// messageUnion contains all message types. It aids in marshaling and unmarshaling heterogeneous messages.
type messageUnion struct {
	*RunStart
	*RunLog
	*RunError
	*RunEnd
	*TestStart
	*TestLog
	*TestError
	*TestEnd
	*Heartbeat
}

// MessageWriter is used by executables containing tests to write messages describing the state of testing.
// It is safe to call its methods concurrently from multiple goroutines.
type MessageWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// NewMessageWriter returns a new MessageWriter for writing to w.
func NewMessageWriter(w io.Writer) *MessageWriter {
	return &MessageWriter{enc: json.NewEncoder(w)}
}

// WriteMessage writes msg.
func (mw *MessageWriter) WriteMessage(msg interface{}) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	switch v := msg.(type) {
	case *RunStart:
		return mw.enc.Encode(&messageUnion{RunStart: v})
	case *RunLog:
		return mw.enc.Encode(&messageUnion{RunLog: v})
	case *RunError:
		return mw.enc.Encode(&messageUnion{RunError: v})
	case *RunEnd:
		return mw.enc.Encode(&messageUnion{RunEnd: v})
	case *TestStart:
		return mw.enc.Encode(&messageUnion{TestStart: v})
	case *TestLog:
		return mw.enc.Encode(&messageUnion{TestLog: v})
	case *TestError:
		return mw.enc.Encode(&messageUnion{TestError: v})
	case *TestEnd:
		return mw.enc.Encode(&messageUnion{TestEnd: v})
	case *Heartbeat:
		return mw.enc.Encode(&messageUnion{Heartbeat: v})
	default:
		return errors.New("unable to encode message of unknown type")
	}
}

// MessageReader is used by the tast executable to interpret output from tests.
type MessageReader json.Decoder

// NewMessageReader returns a new MessageReader for reading from r.
func NewMessageReader(r io.Reader) *MessageReader {
	return (*MessageReader)(json.NewDecoder(r))
}

// More returns true if more messages are available.
func (mr *MessageReader) More() bool {
	return (*json.Decoder)(mr).More()
}

// ReadMessage reads and returns the next message.
func (mr *MessageReader) ReadMessage() (interface{}, error) {
	dec := (*json.Decoder)(mr)
	var mu messageUnion
	if err := dec.Decode(&mu); err != nil {
		return nil, fmt.Errorf("unable to decode message: %v", err)
	}
	switch {
	case mu.RunStart != nil:
		return mu.RunStart, nil
	case mu.RunLog != nil:
		return mu.RunLog, nil
	case mu.RunError != nil:
		return mu.RunError, nil
	case mu.RunEnd != nil:
		return mu.RunEnd, nil
	case mu.TestStart != nil:
		return mu.TestStart, nil
	case mu.TestLog != nil:
		return mu.TestLog, nil
	case mu.TestError != nil:
		return mu.TestError, nil
	case mu.TestEnd != nil:
		return mu.TestEnd, nil
	case mu.Heartbeat != nil:
		return mu.Heartbeat, nil
	default:
		return nil, errors.New("unable to decode message of unknown type")
	}
}
