// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"time"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
)

// OutputStream is an interface to report streamed outputs of an entity.
// Note that planner.OutputStream is for multiple entities in contrast.
type OutputStream interface {
	// Log reports an informational log message from an entity.
	Log(level logging.Level, ts time.Time, msg string) error

	// Error reports an error from by an entity. An entity that reported one or
	// more errors should be considered failure.
	Error(e *protocol.Error) error
}
