// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"io"

	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

// handleGetSysInfoState gets information about the system's current state (e.g. log files
// and crash reports) and writes a JSON-marshaled RunnerGetSysInfoStateResult struct to w.
func handleGetSysInfoState(ctx context.Context, scfg *StaticConfig, w io.Writer) error {
	logger := newArrayLogger()
	ctx = logging.AttachLogger(ctx, logger)

	compat, err := startCompatServer(ctx, scfg, &protocol.HandshakeRequest{})
	if err != nil {
		return err
	}
	defer compat.Close()

	res, err := compat.Client().GetSysInfoState(ctx, &protocol.GetSysInfoStateRequest{})
	if err != nil {
		return err
	}

	jres := jsonprotocol.RunnerGetSysInfoStateResultFromProto(res)
	jres.Warnings = logger.Logs()

	return json.NewEncoder(w).Encode(jres)
}

// handleCollectSysInfo copies system information that was written after args.CollectSysInfo.InitialState
// was generated into temporary directories and writes a JSON-marshaled RunnerCollectSysInfoResult struct to w.
func handleCollectSysInfo(ctx context.Context, args *jsonprotocol.RunnerArgs, scfg *StaticConfig, w io.Writer) error {
	logger := newArrayLogger()
	ctx = logging.AttachLogger(ctx, logger)

	compat, err := startCompatServer(ctx, scfg, &protocol.HandshakeRequest{})
	if err != nil {
		return err
	}
	defer compat.Close()

	res, err := compat.Client().CollectSysInfo(ctx, args.CollectSysInfo.Proto())
	if err != nil {
		return err
	}

	jres := jsonprotocol.RunnerCollectSysInfoResultFromProto(res)
	jres.Warnings = logger.Logs()

	return json.NewEncoder(w).Encode(jres)
}
