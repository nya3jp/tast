// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides fixture stack data structure.
package fixture

import (
	"fmt"

	"chromiumos/tast/internal/protocol"
)

// Status represents a status of a fixture, as well as that of a fixture
// stack. See comments around InternalStack for details.
type Status int

const (
	// StatusRed means fixture is not set up or torn down.
	StatusRed Status = iota
	// StatusGreen means fixture is set up.
	StatusGreen
	// StatusYellow means fixture is set up but last reset failed
	StatusYellow
)

// String converts fixtureStatus to a string for debugging.
func (s Status) String() string {
	switch s {
	case StatusRed:
		return "red"
	case StatusGreen:
		return "green"
	case StatusYellow:
		return "yellow"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

func (s Status) proto() protocol.StackStatus {
	switch s {
	case StatusGreen:
		return protocol.StackStatus_GREEN
	case StatusRed:
		return protocol.StackStatus_RED
	case StatusYellow:
		return protocol.StackStatus_YELLOW
	default:
		panic(fmt.Sprintf("BUG: unknown status %v", s))
	}
}

func statusFromProto(s protocol.StackStatus) Status {
	switch s {
	case protocol.StackStatus_GREEN:
		return StatusGreen
	case protocol.StackStatus_RED:
		return StatusRed
	case protocol.StackStatus_YELLOW:
		return StatusYellow
	default:
		panic(fmt.Sprintf("BUG: unknown status %v", s))
	}
}
