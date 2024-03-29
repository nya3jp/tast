// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"io"
	"strings"

	"google.golang.org/grpc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/rpc"
	"go.chromium.org/tast/core/internal/testing"
)

// RunRPCServer runs the bundle as an RPC server.
func RunRPCServer(r io.Reader, w io.Writer, scfg *StaticConfig) error {
	reg := scfg.registry
	return rpc.RunServer(r, w, reg.AllServices(), func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
		if err := checkRegistrationErrors(reg); err != nil {
			return err
		}
		registerFixtureService(srv, reg)
		protocol.RegisterTestServiceServer(srv, newTestServer(scfg, req.GetBundleInitParams()))
		// TODO(b/187793617): Remove this check once we fully migrate to gRPC-based protocol.
		// The check is currently needed because BundleInitParams is not available for some JSON-based protocol methods.
		if req.GetBundleInitParams() != nil {
			if err := reg.InitializeVars(req.GetBundleInitParams().GetVars()); err != nil {
				return err
			}
		}
		return nil
	})
}

// RunRPCServerTCP runs the bundle as an RPC server listening on TCP.
func RunRPCServerTCP(port int, handshakeReq *protocol.HandshakeRequest, stdin io.Reader, stdout, stderr io.Writer, scfg *StaticConfig) error {
	reg := scfg.registry
	return rpc.RunTCPServer(port, handshakeReq, stdin, stdout, stderr, reg.AllServices(), func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
		if err := checkRegistrationErrors(reg); err != nil {
			return err
		}
		if err := reg.InitializeVars(req.GetBundleInitParams().GetVars()); err != nil {
			return err
		}
		return nil
	})
}

func checkRegistrationErrors(reg *testing.Registry) error {
	if errs := reg.Errors(); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return errors.Errorf("bundle initialization failed: %s", strings.Join(msgs, "; "))
	}
	return nil
}
