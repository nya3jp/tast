// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"strings"
)

// isUserMethod checks if a gRPC method belongs to a user-defined gRPC service.
func isUserMethod(name string) bool {
	return !strings.HasPrefix(name, "/tast.core.")
}

// isLoggingMethod checks if a gRPC method belongs to the logging gRPC service.
func isLoggingMethod(name string) bool {
	return strings.HasPrefix(name, "/tast.core.Logging/")
}
