// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package protocol

// Event is a common interface for event messages.
type Event interface {
	isEvent()
}

func (*RunLogEvent) isEvent()           {}
func (*EntityStartEvent) isEvent()      {}
func (*EntityLogEvent) isEvent()        {}
func (*EntityErrorEvent) isEvent()      {}
func (*EntityEndEvent) isEvent()        {}
func (*EntityCopyEndEvent) isEvent()    {}
func (*StackOperationRequest) isEvent() {}
