// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/run/devserver"
	"chromiumos/tast/ssh"
)

// Services owns services exposed to a target by SSH port forwarding.
// Services is owned by Conn because its lifetime is tied to a corresponding
// SSH connection.
type Services struct {
	tlwForwarder          *ssh.Forwarder
	dutServerForwarder    *ssh.Forwarder
	ephemeralDevserver    *devserver.Ephemeral
	ephemeralDevserverURL string
}

// ServiceConfig contains configs for creating a service.
type ServiceConfig struct {
	TLWServer             string
	UseEphemeralDevserver bool
	Devservers            []string
	TastDir               string
	ExtraAllowedBuckets   []string
	DebuggerPorts         []int
}

func startServices(ctx context.Context, cfg *ServiceConfig, conn *ssh.Conn, dutServer string) (svcs *Services, retErr error) {
	var tlwForwarder *ssh.Forwarder
	var dutServerForwarder *ssh.Forwarder
	var ephemeralDevserver *devserver.Ephemeral
	var ephemeralDevserverURL string

	if dutServer != "" {
		var err error
		dutServerForwarder, err = conn.ForwardRemoteToLocal("tcp", "127.0.0.1:0", dutServer, func(e error) {
			logging.Infof(ctx, "DUT server port forwarding failed: %v", e)
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to set up remote-to-local port forwarding for DUT server")
		}
		defer func() {
			if retErr != nil {
				dutServerForwarder.Close()
			}
		}()
	} else if cfg.TLWServer != "" {
		// TODO: remove TLW after we make sure we don't need TLW server anymore.
		var err error
		tlwForwarder, err = conn.ForwardRemoteToLocal("tcp", "127.0.0.1:0", cfg.TLWServer, func(e error) {
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
	} else if cfg.UseEphemeralDevserver && len(cfg.Devservers) == 0 {
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

		for _, debugPort := range cfg.DebuggerPorts {
			if err := debugger.ForwardPort(ctx, conn, debugPort); err != nil {
				return nil, err
			}
		}
	}

	return &Services{
		tlwForwarder:          tlwForwarder,
		dutServerForwarder:    dutServerForwarder,
		ephemeralDevserver:    ephemeralDevserver,
		ephemeralDevserverURL: ephemeralDevserverURL,
	}, nil
}

func (s *Services) close() error {
	var firstErr error
	if s.dutServerForwarder != nil {
		if err := s.dutServerForwarder.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
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

// DUTServerAddr returns the address of port-forwarded TLW service accessible from the
// target, if available.
func (s *Services) DUTServerAddr() (addr net.Addr, ok bool) {
	if s.dutServerForwarder == nil {
		return nil, false
	}
	return s.dutServerForwarder.ListenAddr(), true
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

func startEphemeralDevserver(cfg *ServiceConfig, conn *ssh.Conn) (ds *devserver.Ephemeral, url string, retErr error) {
	lis, err := conn.ListenTCP(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, "", fmt.Errorf("failed to reverse-forward a port: %v", err)
	}
	defer func() {
		if retErr != nil {
			lis.Close()
		}
	}()

	cacheDir := filepath.Join(cfg.TastDir, "devserver", "static")
	ds, err = devserver.NewEphemeral(lis, cacheDir, cfg.ExtraAllowedBuckets)
	if err != nil {
		return nil, "", err
	}

	url = fmt.Sprintf("http://%s", lis.Addr())
	return ds, url, nil
}
