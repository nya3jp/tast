// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// RunServer runs a gRPC server on r/w channels.
func RunServer(r io.Reader, w io.Writer) error {
	srv := grpc.NewServer()

	// Register the reflection service for easier debugging.
	reflection.Register(srv)

	if err := srv.Serve(newPipeListener(r, w)); err != nil && err != io.EOF {
		return err
	}
	return nil
}
