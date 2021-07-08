// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runnerclient provides test_runner clients.
package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"chromiumos/tast/cmd/tast/internal/run/genericexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

// JSONClient is a JSON-protocol client to test_runner.
type JSONClient struct {
	cmd    genericexec.Cmd
	params *protocol.RunnerInitParams
	hops   int
}

// NewJSONClient creates a new JSONClient.
func NewJSONClient(cmd genericexec.Cmd, params *protocol.RunnerInitParams, hops int) *JSONClient {
	return &JSONClient{
		cmd:    cmd,
		params: params,
		hops:   hops,
	}
}

// GetDUTInfo retrieves various DUT information needed for test execution.
func (c *JSONClient) GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerGetDUTInfoMode,
		GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
			ExtraUSEFlags:       req.GetExtraUseFlags(),
			RequestDeviceConfig: true,
		},
	}

	var res jsonprotocol.RunnerGetDUTInfoResult
	if err := c.runBatch(ctx, args, &res); err != nil {
		return nil, errors.Wrap(err, "getting DUT info")
	}

	// If the software feature is empty, then the DUT doesn't know about its features
	// (e.g. because it's a non-test image and doesn't have a listing of relevant USE flags).
	if res.SoftwareFeatures == nil {
		return nil, errors.New("can't check test deps; no software features reported by DUT")
	}

	for _, warn := range res.Warnings {
		logging.Info(ctx, warn)
	}
	return res.Proto(), nil
}

func (c *JSONClient) runBatch(ctx context.Context, args *jsonprotocol.RunnerArgs, out interface{}) error {
	args.FillDeprecated()

	stdin, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal runner args: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := c.cmd.Run(ctx, nil, bytes.NewBuffer(stdin), &stdout, &stderr); err != nil {
		// Append the first line of stderr, which often contains useful info
		// for debugging to users.
		if split := bytes.SplitN(stderr.Bytes(), []byte(","), 2); len(split) > 0 {
			err = errors.Errorf("%v: %s", err, string(split[0]))
		}
		return err
	}
	if err := json.NewDecoder(&stdout).Decode(out); err != nil {
		return errors.Wrap(err, "failed to unmarshal runner response")
	}
	return nil
}
