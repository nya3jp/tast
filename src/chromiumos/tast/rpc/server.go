// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/testing"
)

// RunServer runs a gRPC server providing svcs on r/w channels.
// It blocks until the client connection is closed or it encounters an error.
func RunServer(r io.Reader, w io.Writer, svcs []*testing.Service) error {
	srv := grpc.NewServer()

	// Register the reflection service for easier debugging.
	reflection.Register(srv)

	for _, svc := range svcs {
		svc.Register(srv, &testing.ServiceState{})
	}

	if err := srv.Serve(newPipeListener(r, w)); err != nil && err != io.EOF {
		return err
	}
	return nil
}
