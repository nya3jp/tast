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
func (d *Driver) DownloadPrivateBundles(ctx context.Context) error {
	if !d.cfg.DownloadPrivateBundles() {
		return nil
	}

	ctx, st := timing.Start(ctx, "download_private_bundles")
	defer st.End()

	logging.Debug(ctx, "Downloading private bundles")

	devservers := append([]string(nil), d.cfg.Devservers()...)
	if url, ok := d.conn.Services().EphemeralDevserverURL(); ok {
		devservers = append(devservers, url)
	}

	var tlwServer, tlwSelfName string
	if addr, ok := d.conn.Services().TLWAddr(); ok {
		tlwServer = addr.String()
		tlwSelfName = d.cc.Target()
	}

	req := &protocol.DownloadPrivateBundlesRequest{
		ServiceConfig: &protocol.ServiceConfig{
			Devservers:  devservers,
			TlwServer:   tlwServer,
			TlwSelfName: tlwSelfName,
		},
		BuildArtifactUrl: d.cfg.BuildArtifactsURL(),
	}

	if err := d.localClient().DownloadPrivateBundles(ctx, req); err != nil {
		return errors.Wrap(err, "failed to download private bundles")
	}
	return nil
}
