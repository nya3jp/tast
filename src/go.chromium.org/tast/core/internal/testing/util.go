// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"runtime"
	"strings"

	"go.chromium.org/tast/core/errors/stack"
	"go.chromium.org/tast/core/internal/protocol"
)

// NewError returns a new Error object containing reason rsn.
// skipFrames contains the number of frames to skip to get the code that's reporting
// the error: the caller should pass 0 to report its own frame, 1 to skip just its own frame,
// 2 to additionally skip the frame that called it, and so on.
func NewError(err error, fullMsg, lastMsg string, skipFrames int) *protocol.Error {
	// Also skip the NewError frame.
	skipFrames++

	// runtime.Caller starts counting stack frames at the point of the code that
	// invoked Caller.
	_, fn, ln, _ := runtime.Caller(skipFrames)

	trace := fmt.Sprintf("%s\n%s", lastMsg, stack.New(skipFrames))
	if err != nil {
		trace += fmt.Sprintf("\n%+v", err)
	}

	return &protocol.Error{
		Reason: strings.ToValidUTF8(fullMsg, ""),
		Location: &protocol.ErrorLocation{
			File:  fn,
			Line:  int64(ln),
			Stack: strings.ToValidUTF8(trace, ""),
		},
	}
}
