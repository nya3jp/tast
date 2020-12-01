// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"io"
	"os"
	"os/exec"

	"chromiumos/tast/internal/protocol"
)

// fileTransferServer is an implementation of FileTransfer gRPC service.
type fileTransferServer struct {
	protocol.UnimplementedFileTransferServer
}

func newFileTransferServer() *fileTransferServer {
	return &fileTransferServer{}
}

func (s *fileTransferServer) PullDirectory(req *protocol.PullDirectoryRequest, srv protocol.FileTransfer_PullDirectoryServer) error {
	ctx := srv.Context()
	path := req.Path

	// Remove the directory on completion, regardless of errors.
	defer os.RemoveAll(path)

	cmd := exec.CommandContext(ctx, "tar", "-cz", "-C", path, ".")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	const bufSize = 65536
	buf := make([]byte, bufSize)
	for {
		n, err := stdout.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := srv.Send(&protocol.PullDirectoryResponse{Data: buf[:n]}); err != nil {
			return err
		}
	}
	return nil
}

// pullDirectory pulls a directory on the DUT to the local disk by calling
// FileTransfer.PullDirectory gRPC method.
// src is a source directory path on the DUT, and dst is a destination directory
// path on the host. Both src and dst must be existing directories.
func pullDirectory(ctx context.Context, cl protocol.FileTransferClient, src, dst string) (retErr error) {
	cmd := exec.CommandContext(ctx, "tar", "-xz", "-C", dst)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		stdin.Close()
		if err := cmd.Wait(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	h, err := cl.PullDirectory(ctx, &protocol.PullDirectoryRequest{Path: src})
	if err != nil {
		return err
	}
	for {
		res, err := h.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if _, err := stdin.Write(res.Data); err != nil {
			return err
		}
	}
	return nil
}
