// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/rpc"
)

// runRPCServer runs a gRPC server on stdin and stdout.
func runRPCServer(stdin io.Reader, stdout io.Writer) error {
	srv := grpc.NewServer()

	// Register the reflection service for easier debugging.
	reflection.Register(srv)

	if err := srv.Serve(rpc.NewPipeListener(stdin, stdout)); err != nil && err != io.EOF {
		return err
	}
	return nil
}
