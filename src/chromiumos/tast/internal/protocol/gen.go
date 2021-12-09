// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc --go_out=plugins=grpc:../../../.. -I . fake_core.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . fake_user.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . -I ../../../../../../../config/proto features.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . file_transfer.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . handshake.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . logging.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . loopback.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . -I ../../../../../../../config/proto remote_fixture.proto
//go:generate protoc --go_out=plugins=grpc:../../../.. -I . -I ../../../../../../../config/proto testing.proto

package protocol

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/trunk/src/platform/tast/tools/go.sh generate chromiumos/tast/internal/protocol
