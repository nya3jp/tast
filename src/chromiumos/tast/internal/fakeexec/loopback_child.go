// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakeexec

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
)

const portEnvName = "FAKEEXEC_LOOPBACK_PORT"

func init() {
	port, err := strconv.Atoi(os.Getenv(portEnvName))
	if err != nil {
		return
	}

	if err := runLoopback(port); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(254)
	}
	panic("BUG: runLoopback returned successfully")
}

func runLoopback(port int) error {
	ctx := context.Background()

	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()

	cl := protocol.NewLoopbackExecServiceClient(conn)

	stream, err := cl.Exec(ctx)
	if err != nil {
		return err
	}

	// First send an initialization event.
	if err := stream.Send(&protocol.ExecRequest{Type: &protocol.ExecRequest_Init{Init: &protocol.InitEvent{Args: os.Args}}}); err != nil {
		return err
	}

	// Start a goroutine that sends stdin data to the server.
	go func() {
		// Ignore all errors on this goroutine. The main goroutine should
		// handle a gone server.
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				break
			}
			stream.Send(&protocol.ExecRequest{Type: &protocol.ExecRequest_Stdin{Stdin: &protocol.PipeEvent{Data: buf[:n], Close: false}}})
		}
		stream.Send(&protocol.ExecRequest{Type: &protocol.ExecRequest_Stdin{Stdin: &protocol.PipeEvent{Close: true}}})
	}()

	// On the main goroutine, process events from the server.
	for {
		res, err := stream.Recv()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
			os.Exit(254)
		}
		switch t := res.Type.(type) {
		case *protocol.ExecResponse_Exit:
			os.Exit(int(t.Exit.Code))
		case *protocol.ExecResponse_Stdout:
			os.Stdout.Write(t.Stdout.Data)
			if t.Stdout.Close {
				os.Stdout.Close()
			}
		case *protocol.ExecResponse_Stderr:
			os.Stderr.Write(t.Stderr.Data)
			if t.Stderr.Close {
				os.Stderr.Close()
			}
		}
	}
}
