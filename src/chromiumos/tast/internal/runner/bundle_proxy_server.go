// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"

	"chromiumos/tast/bundle"
)

type bundleServer struct {
	args *Args
	f    func(args *Args, stdout io.Writer) int
}

func newBundleProxyServer(args *Args, f func(*Args, io.Writer) int) *bundleServer {
	return &bundleServer{args: args, f: f}
}

func (s *bundleServer) List(ctx context.Context, req *bundle.ListRequest) (*bundle.ListResponse, error) {
	return nil, errors.New("unimplemented")
}

func (s *bundleServer) Run(req *bundle.RunRequest, srv bundle.BundleService_RunServer) error {
	if err := json.Unmarshal([]byte(req.Args), &s.args); err != nil {
		return err
	}
	r, w := io.Pipe()
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			msg := sc.Text()
			srv.Send(&bundle.RunResponse{
				Control: msg,
			})
		}
		if err := sc.Err(); err != nil {
			log.Fatalf("run pipe goroutine fail: %v", err)
		}
	}()
	s.args.report = true
	res := s.f(s.args, w)
	if res != 0 {
		return fmt.Errorf("run: error status = %d", res)
	}
	return nil
}
