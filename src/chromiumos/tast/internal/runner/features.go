// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"io"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

// handleGetDUTInfo handles a RunnerGetDUTInfoMode request from args
// and JSON-marshals a RunnerGetDUTInfoResult struct to w.
func handleGetDUTInfo(ctx context.Context, args *jsonprotocol.RunnerArgs, scfg *StaticConfig, w io.Writer) error {
	logger := newArrayLogger()
	ctx = logging.AttachLogger(ctx, logger)

	compat, err := startCompatServer(ctx, scfg, &protocol.HandshakeRequest{})
	if err != nil {
		return err
	}
	defer compat.Close()

	res, err := compat.Client().GetDUTInfo(ctx, args.GetDUTInfo.Proto())
	if err != nil {
		return err
	}

	jres := jsonprotocol.RunnerGetDUTInfoResultFromProto(res)
	jres.Warnings = logger.Logs()

	if err := json.NewEncoder(w).Encode(jres); err != nil {
		return command.NewStatusErrorf(statusError, "failed to serialize into JSON: %v", err)
	}
	return nil
}
