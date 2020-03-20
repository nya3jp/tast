// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"io"

	"chromiumos/tast/rpc"
)

func gRPCParams(r io.Reader, w io.Writer) (*rpc.BundleParamsReq, error) {
	params, err := readRPCParams(r)
	if err != nil {
		sendRPCParamsRsp(w, err)
		return nil, err
	}
	// Send seccess response with err set to nil.
	if err := sendRPCParamsRsp(w, nil); err != nil {
		return nil, err
	}
	return params, nil
}

func readRPCParams(r io.Reader) (*rpc.BundleParamsReq, error) {
	params := &rpc.BundleParamsReq{}
	if err := rpc.ReceiveRawMessage(r, params); err != nil {
		return nil, err
	}

	return params, nil
}

func sendRPCParamsRsp(w io.Writer, err error) error {
	rsp := &rpc.BundleParamsRsp{
		Success: err == nil,
	}
	if err != nil {
		rsp.ErrorMessage = err.Error()
	}

	return rpc.SendRawMessage(w, rsp)
}
