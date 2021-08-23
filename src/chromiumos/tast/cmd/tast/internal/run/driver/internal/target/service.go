// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/devserver"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/ssh"
)

// Services owns services exposed to a target by SSH port forwarding.
// Services is owned by Conn because its lifetime is tied to a corresponding
// SSH connection.
type Services struct {
	tlwForwarder          *ssh.Forwarder
	ephemeralDevserver    *devserver.Ephemeral
	ephemeralDevserverURL string
}

func startServices(ctx context.Context, cfg *config.Config, conn *ssh.Conn) (svcs *Services, retErr error) {
	var tlwForwarder *ssh.Forwarder
	var ephemeralDevserver *devserver.Ephemeral
	var ephemeralDevserverURL string

	if cfg.TLWServer() != "" {
		var err error
		tlwForwarder, err = conn.ForwardRemoteToLocal("tcp", "127.0.0.1:0", cfg.TLWServer(), func(e error) {
			logging.Infof(ctx, "TLW server port forwarding failed: %v", e)
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to set up remote-to-local port forwarding for TLW server")
		}
		defer func() {
			if retErr != nil {
				tlwForwarder.Close()
			}
		}()
	} else if cfg.UseEphemeralDevserver() && len(cfg.Devservers()) == 0 {
		var err error
		ephemeralDevserver, ephemeralDevserverURL, err = startEphemeralDevserver(cfg, conn)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start ephemeral devserver for local tests")
		}
		defer func() {
			if retErr != nil {
				ephemeralDevserver.Close()
			}
		}()
	}

	return &Services{
		tlwForwarder:          tlwForwarder,
		ephemeralDevserver:    ephemeralDevserver,
		ephemeralDevserverURL: ephemeralDevserverURL,
	}, nil
}

func (s *Services) close() error {
	var firstErr error
	if s.tlwForwarder != nil {
		if err := s.tlwForwarder.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.ephemeralDevserver != nil {
		if err := s.ephemeralDevserver.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// TLWAddr returns the address of port-forwarded TLW service accessible from the
// target, if available.
func (s *Services) TLWAddr() (addr net.Addr, ok bool) {
	if s.tlwForwarder == nil {
		return nil, false
	}
	return s.tlwForwarder.ListenAddr(), true
}

// EphemeralDevserverURL returns the URL of port-forwarded ephemeral devserver
// accessible from the target, if available.
func (s *Services) EphemeralDevserverURL() (url string, ok bool) {
	if s.ephemeralDevserver == nil {
		return "", false
	}
	return s.ephemeralDevserverURL, true
}

func startEphemeralDevserver(cfg *config.Config, conn *ssh.Conn) (ds *devserver.Ephemeral, url string, retErr error) {
	lis, err := conn.ListenTCP(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, "", fmt.Errorf("failed to reverse-forward a port: %v", err)
	}
	defer func() {
		if retErr != nil {
			lis.Close()
		}
	}()

	cacheDir := filepath.Join(cfg.TastDir(), "devserver", "static")
	ds, err = devserver.NewEphemeral(lis, cacheDir, cfg.ExtraAllowedBuckets())
	if err != nil {
		return nil, "", err
	}

	url = fmt.Sprintf("http://%s", lis.Addr())
	return ds, url, nil
}
