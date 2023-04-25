// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wlan provides the information of the wlan device.
package wlan

import (
	"go.chromium.org/tast/core/testing/wlan"
)

// DeviceID is used as a Device ID type.
type DeviceID = wlan.DeviceID

// DevInfo contains the information of the WLAN device.
type DevInfo = wlan.DevInfo

// WLAN Device IDs.
const (
	UnknownDevice              = wlan.UnknownDevice
	Marvell88w8897SDIO         = wlan.Marvell88w8897SDIO
	Marvell88w8997PCIE         = wlan.Marvell88w8997PCIE
	QualcommAtherosQCA6174     = wlan.QualcommAtherosQCA6174
	QualcommAtherosQCA6174SDIO = wlan.QualcommAtherosQCA6174SDIO
	QualcommWCN3990            = wlan.QualcommWCN3990
	QualcommWCN6750            = wlan.QualcommWCN6750
	QualcommWCN6855            = wlan.QualcommWCN6855
	Intel7260                  = wlan.Intel7260
	Intel7265                  = wlan.Intel7265
	Intel8265                  = wlan.Intel8265
	Intel9000                  = wlan.Intel9000
	Intel9260                  = wlan.Intel9260
	Intel22260                 = wlan.Intel22260
	Intel22560                 = wlan.Intel22560
	IntelAX201                 = wlan.IntelAX201
	IntelAX203                 = wlan.IntelAX203
	IntelAX211                 = wlan.IntelAX211
	BroadcomBCM4354SDIO        = wlan.BroadcomBCM4354SDIO
	BroadcomBCM4356PCIE        = wlan.BroadcomBCM4356PCIE
	BroadcomBCM4371PCIE        = wlan.BroadcomBCM4371PCIE
	Realtek8822CPCIE           = wlan.Realtek8822CPCIE
	Realtek8852APCIE           = wlan.Realtek8852APCIE
	Realtek8852CPCIE           = wlan.Realtek8852CPCIE
	MediaTekMT7921PCIE         = wlan.MediaTekMT7921PCIE
	MediaTekMT7921SDIO         = wlan.MediaTekMT7921SDIO
	MediaTekMT7922PCIE         = wlan.MediaTekMT7922PCIE
)

// DeviceNames map contains WLAN device names.
var DeviceNames = wlan.DeviceNames

// DeviceInfo returns a public struct (DevInfo) containing the WLAN device information.
func DeviceInfo() (*DevInfo, error) {
	return wlan.DeviceInfo()
}
