// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dbusutil

const (
	BusName      = "org.freedesktop.DBus"  // system bus service name
	BusPath      = "/org/freedesktop/DBus" // system bus service path
	BusInterface = "org.freedesktop.DBus"  // system bus interface

	// TODO(derat): Figure out if there's a way to get constants from system_api's headers.
	SessionManagerName      = "org.chromium.SessionManager"
	SessionManagerPath      = "/org/chromium/SessionManager"
	SessionManagerInterface = "org.chromium.SessionManagerInterface"

	PowerManagerName      = "org.chromium.PowerManager"
	PowerManagerPath      = "/org/chromium/PowerManager"
	PowerManagerInterface = "org.chromium.PowerManager"
)
