// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"go.chromium.org/tast/core/internal/minidriver/processor"
)

// NewHandler creates a handler which handles stack operation.
func NewHandler(server *StackServer) processor.Handler {
	return processor.NewStackOperationHandler(server.Handle)
}
