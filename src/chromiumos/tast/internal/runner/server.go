// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"fmt"
	"io"
	"log"
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
	stream TastCoreService_DelegateServer
}

func (d *delegateSender) Write(b []byte) (int, error) {
	return len(b), d.stream.Send(&DelegateResponse{
		Payload: string(b),
	})
}

func (s *server) Delegate(req *DelegateRequest, stream TastCoreService_DelegateServer) error {
	w := &delegateSender{stream}
	r := strings.NewReader(req.Payload)
	log.Println("hogehoge")
	if status := Run(s.clArgs, r, w, s.stderr, s.args, s.cfg); status != statusSuccess {
		return fmt.Errorf("runner failed with status %d", status)
	}
	return nil
}
