// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

// DownloadPrivateBundles downloads and installs a private test bundle archive
// corresponding to the target version, if one has not been installed yet.
func (d *Driver) DownloadPrivateBundles(ctx context.Context, dutInfo *protocol.DUTInfo) error {
	if !d.cfg.DownloadPrivateBundles() {
		return nil
	}

	ctx, st := timing.Start(ctx, "download_private_bundles")
	defer st.End()

	logging.Debug(ctx, "Downloading private bundles")

	devservers := append([]string(nil), d.cfg.Devservers()...)
	if url, ok := d.cc.Conn().Services().EphemeralDevserverURL(); ok {
		devservers = append(devservers, url)
	}

	var tlwServer, tlwSelfName string
	if addr, ok := d.cc.Conn().Services().TLWAddr(); ok {
		tlwServer = addr.String()
		// TODO: Fix TLW name. Connection spec is not a right choice.
		tlwSelfName = d.cc.ConnectionSpec()
	}

	var dutServer string
	if addr, ok := d.cc.Conn().Services().DUTServerAddr(); ok {
		dutServer = addr.String()
	}

	buildArtifactsURL := d.cfg.BuildArtifactsURLOverride()
	if buildArtifactsURL == "" {
		buildArtifactsURL = dutInfo.GetDefaultBuildArtifactsUrl()
	}

	req := &protocol.DownloadPrivateBundlesRequest{
		ServiceConfig: &protocol.ServiceConfig{
			Devservers:  devservers,
			DutServer:   dutServer,
			TlwServer:   tlwServer,
			TlwSelfName: tlwSelfName,
		},
		BuildArtifactUrl: buildArtifactsURL,
	}

	if err := d.localRunnerClient().DownloadPrivateBundles(ctx, req); err != nil {
		return errors.Wrap(err, "failed to download private bundles")
	}
	return nil
}
