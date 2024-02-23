// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc --go_out=plugins=grpc:../../../../.. -I . reports.proto
//go:generate protoc --go_out=plugins=grpc:../../../../.. -I . -I ../../../../../../../../config/proto dutfeatures.proto
//go:generate protoc --go_out=plugins=grpc:../../../../.. -I . -I ../../../../../../../../config/proto dut_lab_config.proto

package protocol

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast/core/framework/protocol
