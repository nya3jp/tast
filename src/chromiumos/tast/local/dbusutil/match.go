// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dbusutil

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus"
)

// MatchSpec specifies messages that should be received by a D-Bus client.
// Empty fields are disregarded.
type MatchSpec struct {
	// Type contains the message type, e.g. "signal".
	Type string
	// Path contains the path that the message is sent to (for method calls)
	// or emitted from (for signals).
	Path dbus.ObjectPath
	// Sender contains the message sender (typically the sender's service name in the
	// case of a signal).
	Sender string
	// Interface contains the interface that the message is sent to (for method calls)
	// or emitted from (for signals).
	Interface string
	// Member contains the method or signal name. It does not include the interface.
	Member string
	// Arg0 contains the first argument in the message. Non-string arguments are unsupported.
	// If empty, the argument is not compared.
	Arg0 string
}

// String returns a match rule that can be passed to the bus's AddMatch or RemoveMatch
// method to start or stop receiving messages described by the spec.
func (ms MatchSpec) String() string {
	parts := make([]string, 0)
	f := func(name, val string) {
		if val != "" {
			parts = append(parts, fmt.Sprintf("%s='%s'", name, val))
		}
	}

	f("type", ms.Type)
	f("path", string(ms.Path))
	f("sender", ms.Sender)
	f("interface", ms.Interface)
	f("member", ms.Member)
	f("arg0", ms.Arg0)

	return strings.Join(parts, ",")
}

// MatchesSignal returns true if sig is matched by the spec.
func (ms MatchSpec) MatchesSignal(sig *dbus.Signal) bool {
	if ms.Type != "" && ms.Type != "signal" {
		return false
	}
	if ms.Path != "" && sig.Path != ms.Path {
		return false
	}
	if ms.Sender != "" && sig.Sender != ms.Sender {
		return false
	}

	parts := strings.Split(sig.Name, ".")
	if ms.Interface != "" && strings.Join(parts[:len(parts)-1], ".") != ms.Interface {
		return false
	}
	if ms.Member != "" && parts[len(parts)-1] != ms.Member {
		return false
	}

	if ms.Arg0 != "" {
		if len(sig.Body) == 0 {
			return false
		}
		if v, ok := sig.Body[0].(string); !ok || v != ms.Arg0 {
			return false
		}
	}

	return true
}
