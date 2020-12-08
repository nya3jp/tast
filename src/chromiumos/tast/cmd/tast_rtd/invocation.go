// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast_rtd executable, used to invoke tast in RTD.
package main

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

// unmarshalInvocation unmarshals an invocation request and returns a pointer to rtd.Invocation.
func unmarshalInvocation(req []byte) (*rtd.Invocation, error) {
	inv := &rtd.Invocation{}
	if err := proto.Unmarshal(req, inv); err != nil {
		return nil, fmt.Errorf("fail to unmarshal invocation data: %v", err)
	}
	return inv, nil
}
