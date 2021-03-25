// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"chromiumos/tast/internal/protocol"
)

// OutputStream is an interface to report streamed outputs of an entity.
// Note that planner.OutputStream is for multiple entities in contrast.
type OutputStream interface {
	// Log reports an informational log message from an entity.
	Log(msg string) error

	// Error reports an error from by an entity. An entity that reported one or
	// more errors should be considered failure.
	Error(e *protocol.Error) error
}
