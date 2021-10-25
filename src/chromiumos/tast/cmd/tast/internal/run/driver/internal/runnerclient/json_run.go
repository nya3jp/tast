// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
)

// RunTests requests to run tests according to the given RunConfig.
// Test execution events are streamed back via out. See RunTestsOutput for
// details.
func (c *JSONClient) RunTests(ctx context.Context, bcfg *protocol.BundleConfig, rcfg *protocol.RunConfig, out RunTestsOutput) {
	// Call RunEnd exactly once on returning from this method.
	out.RunEnd(ctx, func() (runErr error) {
		// Make sure all subprocesses and goroutines exit upon returning from
		// this function.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// This must come first since we have to call RunStart even if we
		// fail to start running tests.
		if err := out.RunStart(ctx); err != nil {
			return err
		}

		bundleArgs, err := jsonprotocol.BundleRunTestsArgsFromProto(bcfg, rcfg)
		if err != nil {
			return err
		}

		args := &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerRunTestsMode,
			RunTests: &jsonprotocol.RunnerRunTestsArgs{
				BundleArgs: *bundleArgs,
				BundleGlob: c.params.GetBundleGlob(),
			},
		}

		proc, err := c.cmd.Interact(ctx, nil)
		if err != nil {
			return err
		}

		// Read stderr in the background so that it can be included in error
		// messages.
		stderrReader := newFirstLineReader(proc.Stderr())

		defer func() {
			// In case the runner exits abnormally, return an error from Wait
			// with additional information from stderrReader so that we don't
			// give a useless error about incorrectly-formed output instead of
			// e.g. an error about the runner being missing.
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := proc.Wait(ctx); err != nil {
				runErr = stderrReader.appendToError(err, stderrTimeout)
			}
		}()

		// Send the request.
		if err := json.NewEncoder(proc.Stdin()).Encode(args); err != nil {
			return errors.Wrap(err, "failed to send RunTests request")
		}

		// Read responses and send them to out.
		msgs := make(chan control.Msg)
		errs := make(chan error)
		go readControlMessages(ctx, proc.Stdout(), msgs, errs)

		for {
			err := processControlMessage(ctx, msgs, errs, c.msgTimeout, out)
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}
	}())
}

// readControlMessages reads serialized control messages from r and sends them
// to msgs.
// On successful completion, msgs is closed and errs is kept open.
// On encountering an error, it is sent to errs and msgs is kept open.
func readControlMessages(ctx context.Context, r io.Reader, msgs chan<- control.Msg, errs chan<- error) {
	mr := control.NewMessageReader(r)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			errs <- err
			return
		}
		select {
		case msgs <- msg:
		case <-ctx.Done():
			return
		}
	}
	close(msgs)
}

// processControlMessage waits and processes the next message from msgs.
// It returns io.EOF if msgs is closed and there is no message to read.
func processControlMessage(ctx context.Context, msgs <-chan control.Msg, errs <-chan error, msgTimeout time.Duration, out RunTestsOutput) error {
	timer := time.NewTimer(msgTimeout)
	defer timer.Stop()

	select {
	case msg := <-msgs:
		if msg == nil {
			// If the channel is closed, we'll read the zero value.
			return io.EOF
		}
		return handleControlMessage(ctx, msg, out)
	case err := <-errs:
		return err
	case <-timer.C:
		return errors.Errorf("timed out after waiting %v for next message (probably lost SSH connection to DUT)", msgTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleControlMessage calls RunTestsOutput methods corresponding to msg.
func handleControlMessage(ctx context.Context, msg control.Msg, out RunTestsOutput) error {
	switch msg := msg.(type) {
	case *control.RunStart, *control.RunEnd, *control.Heartbeat:
		// Ignored.
		return nil
	case *control.RunLog:
		ts, err := ptypes.TimestampProto(msg.Time)
		if err != nil {
			return errors.Wrapf(err, "control message %T", msg)
		}
		return out.RunLog(ctx, &protocol.RunLogEvent{
			Time: ts,
			Text: msg.Text,
		})
	case *control.RunError:
		return errors.New(msg.Error.Reason)
	case *control.EntityStart:
		ts, err := ptypes.TimestampProto(msg.Time)
		if err != nil {
			return errors.Wrapf(err, "control message %T", msg)
		}
		e, err := msg.Info.Proto()
		if err != nil {
			return errors.Wrapf(err, "control message %T", msg)
		}
		return out.EntityStart(ctx, &protocol.EntityStartEvent{
			Time:   ts,
			Entity: e,
			OutDir: msg.OutDir,
		})
	case *control.EntityLog:
		ts, err := ptypes.TimestampProto(msg.Time)
		if err != nil {
			return errors.Wrapf(err, "control message %T", msg)
		}
		return out.EntityLog(ctx, &protocol.EntityLogEvent{
			Time:       ts,
			EntityName: msg.Name,
			Text:       msg.Text,
		})
	case *control.EntityError:
		ts, err := ptypes.TimestampProto(msg.Time)
		if err != nil {
			return errors.Wrapf(err, "control message %T", msg)
		}
		return out.EntityError(ctx, &protocol.EntityErrorEvent{
			Time:       ts,
			EntityName: msg.Name,
			Error: &protocol.Error{
				Reason: msg.Error.Reason,
				Location: &protocol.ErrorLocation{
					File:  msg.Error.File,
					Line:  int64(msg.Error.Line),
					Stack: msg.Error.Stack,
				},
			},
		})
	case *control.EntityEnd:
		ts, err := ptypes.TimestampProto(msg.Time)
		if err != nil {
			return errors.Wrapf(err, "control message %T", msg)
		}
		tl, err := msg.TimingLog.Proto()
		if err != nil {
			return errors.Wrapf(err, "control message %T", msg)
		}
		var skip *protocol.Skip
		if len(msg.SkipReasons) > 0 {
			skip = &protocol.Skip{Reasons: msg.SkipReasons}
		}
		return out.EntityEnd(ctx, &protocol.EntityEndEvent{
			Time:       ts,
			EntityName: msg.Name,
			Skip:       skip,
			TimingLog:  tl,
		})
	default:
		return errors.Errorf("unknown control message type %T", msg)
	}
}
