// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"fmt"
	"io"
	"strings"
)

type server struct {
	clArgs []string
	args   *Args
	cfg    *Config
	stderr io.Writer
}

var _ TastCoreServiceServer = (*server)(nil)

type delegateSender struct {
	res TastCoreService_DelegateServer
}

func (d *delegateSender) Write(b []byte) (int, error) {
	return len(b), d.res.Send(&DelegateResponse{
		PayloadP: string(b),
	})
}

type delegateReqWrap struct {
	req *DelegateRequest
}

func (d *delegateReqWrap) Read() ([]byte, error) {
	return []byte(d.req.Payload), nil
}

func (s *server) Delegate(req *DelegateRequest, res TastCoreService_DelegateServer) error {
	w := &delegateSender{res}
	r := strings.NewReader(req.Payload)
	status := Run(s.clArgs, r, w, s.stderr, s.args, s.cfg)
	if status != statusSuccess {
		return fmt.Errorf("runner failed with status %d", status)
	}
	return nil
}
