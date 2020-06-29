// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . -I ../../../../../proto/infra --go_out=plugins=grpc:../../../.. tastcore.proto local_runner.proto

package runner

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/trunk/src/platform/tast/tools/go.sh generate ~/trunk/src/platform/tast/src/chromiumos/tast/internal/runner/gen.go
