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
//		EntityStart (first test started)
//			EntityLog (first test logged a message)
//		EntityEnd (first test ended)
//		EntityStart (second test started)
//			EntityLog (second test logged a message)
//			EntityError (second test encountered an error)
//			EntityError (second test encountered another error)
//			EntityLog (second test logged another message)
//		EntityEnd (second test ended)
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

	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/timing"
)

// Msg is an interface implemented by all message types.
type Msg interface {
	// isMsg indicates that a type is a message type. It is not intended to be called.
	// Since this method is unexported, no other packages can define message types.
	isMsg()
}

// RunStart describes the start of a run (consisting of one or more tests).
type RunStart struct {
	// Time is the device-local time at which the run started.
	Time time.Time `json:"runStartTime"`
	// TestNames contains the names of tests to run, in the order in which they'll be executed.
	// Note that some of these tests may later be skipped (see EntityEnd).
	TestNames []string `json:"runStartTestNames"`
	// NumTests is the number of tests that will be run.
	// TODO(derat): Delete this after 20190715; the tast command now uses TestNames instead: https://crbug.com/889119
	NumTests int `json:"runStartNumTests"`
}

func (*RunStart) isMsg() {}

// RunLog contains an informative, high-level logging message produced by a run.
type RunLog struct {
	// Time is the device-local time at which the message was logged.
	Time time.Time `json:"runLogTime"`
	// Text is the actual message.
	Text string `json:"runLogText"`
}

func (*RunLog) isMsg() {}

// RunError describes a fatal, high-level error encountered during the run.
// This may be encountered at any time (including before RunStart) and
// indicates that the run has been aborted.
type RunError struct {
	// Time is the device-local time at which the error occurred.
	Time time.Time `json:"runErrorTime"`
	// Error describes the error that occurred.
	Error jsonprotocol.Error `json:"runErrorError"`
	// Status is the exit status code of the test runner if it is run directly.
	Status int `json:"runErrorStatus"`
}

func (*RunError) isMsg() {}

// RunEnd describes the completion of a run.
type RunEnd struct {
	// Time is the device-local time at which the run ended.
	Time time.Time `json:"runEndTime"`
	// OutDir is the base directory under which tests wrote output files.
	// DEPRECATED: Client should always set OutDir in the request.
	// TODO(crbug.com/1000549): Remove this field after 20191201.
	OutDir string `json:"runEndOutDir"`
}

func (*RunEnd) isMsg() {}

// EntityStart describes the start of an individual entity.
type EntityStart struct {
	// Time is the device-local time at which the entity started.
	Time time.Time `json:"testStartTime"`
	// Info contains details about the entity.
	Info jsonprotocol.EntityInfo `json:"testStartTest"`
	// OutDir is a directory path where output files for the entity is written.
	// OutDir can be empty if the entity is being skipped.
	// When set, OutDir is a direct subdirectory of bundle.RunTestsArgs.OutDir.
	OutDir string `json:"testStartOutDir"`
}

func (*EntityStart) isMsg() {}

// EntityLog contains an informative logging message produced by an entity.
type EntityLog struct {
	// Time is the device-local time at which the message was logged.
	Time time.Time `json:"testLogTime"`
	// Text is the actual message.
	Text string `json:"testLogText"`
	// Name is the name of the entity, matching the earlier EntityStart.Test.Name.
	Name string `json:"testLogName"`
}

func (*EntityLog) isMsg() {}

// EntityError contains an error produced by an entity.
type EntityError struct {
	// Time is the device-local time at which the error occurred.
	Time time.Time `json:"testErrorTime"`
	// Error describes the error that occurred.
	Error jsonprotocol.Error `json:"testErrorError"`
	// Name is the name of the entity, matching the earlier EntityStart.Test.Name.
	Name string `json:"testErrorName"`
}

func (*EntityError) isMsg() {}

// EntityEnd describes the end of an individual entity.
type EntityEnd struct {
	// Time is the device-local time at which the entity ended.
	Time time.Time `json:"testEndTime"`
	// Name is the name of the entity, matching the earlier EntityStart.Test.Name.
	Name string `json:"testEndName"`
	// DeprecatedMissingSoftwareDeps contains software dependencies declared by the test that were
	// not present on the DUT. If non-empty, the test was skipped.
	// DEPRECATED: This field is no longer filled by newer bundles.
	DeprecatedMissingSoftwareDeps []string `json:"testEndMissingSoftwareDeps"`
	// SkipReasons contains messages describing why the test was skipped.
	// If non-empty, the test was skipped.
	// In the case of non-test entities, this field is always empty.
	SkipReasons []string `json:"testEndHardwareDepsUnsatisfiedReasons"`
	// TimingLog contains entity-reported timing information to be incorporated into the main timing.json file.
	TimingLog *timing.Log `json:"testEndTimingLog"`
}

func (*EntityEnd) isMsg() {}

// Heartbeat is sent periodically to assert that the bundle is alive.
type Heartbeat struct {
	// Time is the device-local time at which this message was generated.
	Time time.Time `json:"heartbeatTime"`
}

func (*Heartbeat) isMsg() {}

// messageUnion contains all message types. It aids in marshaling and unmarshaling heterogeneous messages.
type messageUnion struct {
	*RunStart
	*RunLog
	*RunError
	*RunEnd
	*EntityStart
	*EntityLog
	*EntityError
	*EntityEnd
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
func (mw *MessageWriter) WriteMessage(msg Msg) error {
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
	case *EntityStart:
		return mw.enc.Encode(&messageUnion{EntityStart: v})
	case *EntityLog:
		return mw.enc.Encode(&messageUnion{EntityLog: v})
	case *EntityError:
		return mw.enc.Encode(&messageUnion{EntityError: v})
	case *EntityEnd:
		return mw.enc.Encode(&messageUnion{EntityEnd: v})
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
func (mr *MessageReader) ReadMessage() (Msg, error) {
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
	case mu.EntityStart != nil:
		return mu.EntityStart, nil
	case mu.EntityLog != nil:
		return mu.EntityLog, nil
	case mu.EntityError != nil:
		return mu.EntityError, nil
	case mu.EntityEnd != nil:
		return mu.EntityEnd, nil
	case mu.Heartbeat != nil:
		return mu.Heartbeat, nil
	default:
		return nil, errors.New("unable to decode message of unknown type")
	}
}
