// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"chromiumos/tast/internal/minidriver/processor"
)

// NewHandler creates a handler which handles stack operation.
func NewHandler(server *StackServer) processor.Handler {
	return processor.NewStackOperationHandler(server.Handle)
}
