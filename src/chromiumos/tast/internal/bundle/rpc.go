// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"io"
	"strings"

	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

// RunRPCServer runs the bundle as an RPC server.
func RunRPCServer(r io.Reader, w io.Writer, scfg *StaticConfig) error {
	reg := scfg.registry
	return rpc.RunServer(r, w, reg.AllServices(), func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
		if err := checkRegistrationErrors(reg); err != nil {
			return err
		}
		registerFixtureService(srv, reg)
		protocol.RegisterTestServiceServer(srv, newTestServer(scfg))
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
