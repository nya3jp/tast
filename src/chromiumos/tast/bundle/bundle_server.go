// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"sync"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/testing"
)

type bundleServer struct {
	mu sync.Mutex
}

func newBundleServer() *bundleServer {
	return &bundleServer{}
}

func convertPre(pres map[string]testing.Precondition, p testing.Precondition) *Pre {
	name := p.String()
	res := &Pre{Name: name}

	p2, ok := p.(testing.PreV2)
	if !ok {
		return res
	}
	pname := p2.Parent()
	pp, ok := pres[pname]
	if !ok {
		// Parent is remote pre.
		res.ExternalParent = pname
		return res
	}
	res.Parent = convertPre(pres, pp)
	return res
}

func (s *bundleServer) List(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	ts, err := testsToRun(&runConfig{defaultTestTimeout: ctxutil.MaxTimeout}, req.Expr)
	if err != nil {
		return nil, err
	}
	pres := testing.GlobalRegistry().AllPreconditions()

	var res ListResponse
	for _, t := range ts {
		// TODO(oka): fill SkipReason.
		res.Test = append(res.Test, &TestInfo{
			Name:         t.Name,
			Precondition: convertPre(pres, t.Pre),
		})
	}
	return &res, nil
}

func (s *bundleServer) Run(req *RunRequest, srv BundleService_RunServer) error {
	ctx := srv.Context()
	_ = ctx
	return nil
}
