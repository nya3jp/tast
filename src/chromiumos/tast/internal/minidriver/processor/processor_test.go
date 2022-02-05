// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"context"
	"os"
	"time"

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/failfast"
	"chromiumos/tast/internal/minidriver/processor"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/run/reporting"
)

var epoch = time.Unix(0, 0)
var epochpb, _ = ptypes.TimestampProto(epoch)

// runProcessor runs Processor by feeding events.
// If Processor returns an error for any event, its RunEnd method is called
// immediately with the error. Otherwise, the RunEnd method is called with
// endErr.
func runProcessor(ctx context.Context, proc *processor.Processor, events []protocol.Event, endErr error) {
	proc.RunEnd(ctx, func() error {
		if err := proc.RunStart(ctx); err != nil {
			return err
		}
		for _, ev := range events {
			var err error
			switch ev := ev.(type) {
			case *protocol.EntityStartEvent:
				err = proc.EntityStart(ctx, ev)
			case *protocol.EntityLogEvent:
				err = proc.EntityLog(ctx, ev)
			case *protocol.EntityErrorEvent:
				err = proc.EntityError(ctx, ev)
			case *protocol.EntityEndEvent:
				err = proc.EntityEnd(ctx, ev)
			case *protocol.EntityCopyEndEvent:
				err = proc.EntityCopyEnd(ctx, ev)
			case *protocol.RunLogEvent:
				err = proc.RunLog(ctx, ev)
			}
			if err != nil {
				return err
			}
		}
		return endErr
	}())
}

// nopPull is a PullFunc to be used in unit tests not interested in test
// outputs.
func nopPull(src, dst string) error {
	if src != "" {
		return errors.New("nopPull: source directory must be empty")
	}
	return os.MkdirAll(dst, 0755)
}

// nopDiagnose is a DiagnoseFunc that does nothing.
func nopDiagnose(ctx context.Context, outDir string) string {
	return ""
}

func newHandlers(resDir string, multiplexer *logging.MultiLogger, pull processor.PullFunc, counter *failfast.Counter, client *reporting.RPCClient) []processor.Handler {
	return []processor.Handler{
		processor.NewLoggingHandler(resDir, multiplexer, client),
		processor.NewTimingHandler(),
		processor.NewStreamedResultsHandler(resDir),
		processor.NewRPCResultsHandler(client),
		processor.NewFailFastHandler(counter),
		// copyOutputHandler should come last as it can block RunEnd for a while.
		processor.NewCopyOutputHandler(pull),
	}
}
