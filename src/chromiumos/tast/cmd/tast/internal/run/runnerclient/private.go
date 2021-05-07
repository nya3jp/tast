// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/timing"
)

// DownloadPrivateBundles executes local_test_runner on hst to download and unpack
// a private test bundles archive corresponding to the Chrome OS version of hst
// if it has not been done yet.
// An archive contains Go executables of local test bundles and their associated
// internal data files and external data link files. Note that remote test
// bundles are not included in archives.
func DownloadPrivateBundles(ctx context.Context, cfg *config.Config, conn *target.Conn, target string) error {
	ctx, st := timing.Start(ctx, "download_private_bundles")
	defer st.End()

	localDevservers := append([]string(nil), cfg.Devservers...)
	if url, ok := conn.Services().EphemeralDevserverURL(); ok {
		localDevservers = append(localDevservers, url)
	}

	var tlwServer string
	if addr, ok := conn.Services().TLWAddr(); ok {
		tlwServer = addr.String()
	}

	var res jsonprotocol.RunnerDownloadPrivateBundlesResult
	if err := runTestRunnerCommand(
		ctx,
		localRunnerCommand(cfg, conn.SSHConn()),
		&jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerDownloadPrivateBundlesMode,
			DownloadPrivateBundles: &jsonprotocol.RunnerDownloadPrivateBundlesArgs{
				Devservers:        localDevservers,
				TLWServer:         tlwServer,
				DUTName:           target,
				BuildArtifactsURL: cfg.BuildArtifactsURL,
			},
		},
		&res,
	); err != nil {
		return err
	}

	for _, msg := range res.Messages {
		cfg.Logger.Log(msg)
	}
	return nil
}
