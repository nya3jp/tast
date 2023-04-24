// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"context"
	"net"

	"go.chromium.org/tast/core/ssh"
)

// DialProxyCommand creates a new connection using the specified proxy command.
func DialProxyCommand(ctx context.Context, hostPort, proxyCommand string) (net.Conn, error) {
	return ssh.DialProxyCommand(ctx, hostPort, proxyCommand)
}
