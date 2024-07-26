// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wlan provides the information of the wlan device.
package wlan

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// DeviceID is used as a Device ID type.
type DeviceID int32

// DevInfo contains the information of the WLAN device.
type DevInfo struct {
	// Vendor is the vendor ID seen in /sys/class/net/<interface>/vendor.
	Vendor string
	// Device is the product ID seen in /sys/class/net/<interface>/device.
	Device string
	// Compatible is the compatible property.
	// See https://www.kernel.org/doc/Documentation/devicetree/usage-model.txt.
	Compatible string
	// Subsystem is the RF chip's ID. The addition of this property is necessary for
	// device disambiguation (b/129489799).
	Subsystem string
	// Device (enum) ID
	ID DeviceID
	// The device name.
	Name string
}

// WLAN Device IDs.
const (
	UnknownDevice DeviceID = iota
	Marvell88w8897SDIO
	Marvell88w8997PCIE
	QualcommAtherosQCA6174
	QualcommAtherosQCA6174SDIO
	QualcommWCN3990
	QualcommWCN6750
	QualcommWCN6855
	Intel7260
	Intel7265
	Intel8265
	Intel9000
	Intel9260
	Intel22260
	Intel22560
	IntelAX201
	IntelAX203
	IntelAX211
	IntelBE200
	BroadcomBCM4354SDIO
	BroadcomBCM4356PCIE
	BroadcomBCM4371PCIE
	Realtek8822CPCIE
	Realtek8852APCIE
	Realtek8852CPCIE
	Realtek8852BPCIE
	MediaTekMT7921PCIE
	MediaTekMT7921SDIO
	MediaTekMT7922PCIE
	MediaTekMT7925PCIE
)

// DeviceNames map contains WLAN device names.
var DeviceNames = map[DeviceID]string{
	Marvell88w8897SDIO:         "Marvell 88W8897 SDIO",
	Marvell88w8997PCIE:         "Marvell 88W8997 PCIE",
	QualcommAtherosQCA6174:     "Qualcomm Atheros QCA6174",
	QualcommAtherosQCA6174SDIO: "Qualcomm Atheros QCA6174 SDIO",
	QualcommWCN3990:            "Qualcomm WCN3990",
	QualcommWCN6750:            "Qualcomm WCN6750",
	QualcommWCN6855:            "Qualcomm WCN6855",
	Intel7260:                  "Intel 7260",
	Intel7265:                  "Intel 7265",
	Intel8265:                  "Intel 8265",
	Intel9000:                  "Intel 9000",
	Intel9260:                  "Intel 9260",
	Intel22260:                 "Intel 22260",
	Intel22560:                 "Intel 22560",
	IntelAX201:                 "Intel AX 201",
	IntelAX203:                 "Intel AX 203",
	IntelAX211:                 "Intel AX 211",
	IntelBE200:                 "Intel BE 200",
	BroadcomBCM4354SDIO:        "Broadcom BCM4354 SDIO",
	BroadcomBCM4356PCIE:        "Broadcom BCM4356 PCIE",
	BroadcomBCM4371PCIE:        "Broadcom BCM4371 PCIE",
	Realtek8822CPCIE:           "Realtek 8822C PCIE",
	Realtek8852APCIE:           "Realtek 8852A PCIE",
	Realtek8852CPCIE:           "Realtek 8852C PCIE",
	Realtek8852BPCIE:           "Realtek 8852B PCIE",
	MediaTekMT7921PCIE:         "MediaTek MT7921 PCIE",
	MediaTekMT7921SDIO:         "MediaTek MT7921 SDIO",
	MediaTekMT7922PCIE:         "MediaTek MT7922 PCIE",
	MediaTekMT7925PCIE:         "MediaTek MT7925 PCIE",
}

// LookupWLANDev mapping of device identification data to device ID.
var LookupWLANDev = map[DevInfo]DeviceID{
	{Vendor: "0x02df", Device: "0x912d"}: Marvell88w8897SDIO,
	{Vendor: "0x1b4b", Device: "0x2b42"}: Marvell88w8997PCIE,
	{Vendor: "0x168c", Device: "0x003e"}: QualcommAtherosQCA6174,
	{Vendor: "0x0271", Device: "0x050a"}: QualcommAtherosQCA6174SDIO,
	{Vendor: "0x17cb", Device: "0x1103"}: QualcommWCN6855,
	{Vendor: "0x8086", Device: "0x08b1"}: Intel7260,
	{Vendor: "0x8086", Device: "0x08b2"}: Intel7260,
	{Vendor: "0x8086", Device: "0x095a"}: Intel7265,
	{Vendor: "0x8086", Device: "0x095b"}: Intel7265,
	// Note that Intel 9000 is also Intel 9560 aka Jefferson Peak 2.
	{Vendor: "0x8086", Device: "0x9df0"}: Intel9000,
	{Vendor: "0x8086", Device: "0x31dc"}: Intel9000,
	{Vendor: "0x8086", Device: "0x2526"}: Intel9260,
	{Vendor: "0x8086", Device: "0x2723"}: Intel22260,
	// For integrated wifi chips, use device_id and subsystem_id together
	// as an identifier.
	// 0x02f0 is for Quasar on CML; 0x4070, 0x0074, 0x6074 are for HrP2.
	{Vendor: "0x8086", Device: "0x24fd", Subsystem: "0x0010"}: Intel8265,
	{Vendor: "0x8086", Device: "0x02f0", Subsystem: "0x0030"}: Intel9000,
	{Vendor: "0x8086", Device: "0x02f0", Subsystem: "0x0034"}: Intel9000,
	{Vendor: "0x8086", Device: "0x02f0", Subsystem: "0x4070"}: Intel22560,
	{Vendor: "0x8086", Device: "0x02f0", Subsystem: "0x0074"}: Intel22560,
	{Vendor: "0x8086", Device: "0x02f0", Subsystem: "0x6074"}: Intel22560,
	{Vendor: "0x8086", Device: "0x4df0", Subsystem: "0x0070"}: Intel22560,
	{Vendor: "0x8086", Device: "0x4df0", Subsystem: "0x4070"}: Intel22560,
	{Vendor: "0x8086", Device: "0x4df0", Subsystem: "0x0074"}: Intel22560,
	{Vendor: "0x8086", Device: "0x4df0", Subsystem: "0x6074"}: Intel22560,
	{Vendor: "0x8086", Device: "0xa0f0", Subsystem: "0x4070"}: Intel22560,
	{Vendor: "0x8086", Device: "0xa0f0", Subsystem: "0x0074"}: Intel22560,
	{Vendor: "0x8086", Device: "0xa0f0", Subsystem: "0x6074"}: Intel22560,
	{Vendor: "0x8086", Device: "0x02f0", Subsystem: "0x0070"}: IntelAX201,
	{Vendor: "0x8086", Device: "0xa0f0", Subsystem: "0x0070"}: IntelAX201,
	{Vendor: "0x8086", Device: "0x54f0", Subsystem: "0x0274"}: IntelAX203,
	{Vendor: "0x8086", Device: "0x54f0", Subsystem: "0x4274"}: IntelAX203,
	{Vendor: "0x8086", Device: "0x51f0", Subsystem: "0x0090"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x51f1", Subsystem: "0x0090"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x51f1", Subsystem: "0x0094"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x51f0", Subsystem: "0x0094"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x51f0", Subsystem: "0x4090"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x51f0", Subsystem: "0x4094"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x54f0", Subsystem: "0x0090"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x54f0", Subsystem: "0x0094"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x7e40", Subsystem: "0x0090"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x7e40", Subsystem: "0x0094"}: IntelAX211,
	{Vendor: "0x8086", Device: "0x272b"}:                      IntelBE200,
	{Vendor: "0x1a56", Device: "0x272b"}:                      IntelBE200,
	{Vendor: "0x14e4", Device: "0x43ec"}:                      BroadcomBCM4356PCIE,
	{Vendor: "0x10ec", Device: "0xc822"}:                      Realtek8822CPCIE,
	{Vendor: "0x10ec", Device: "0x8852"}:                      Realtek8852APCIE,
	{Vendor: "0x10ec", Device: "0xc852"}:                      Realtek8852CPCIE,
	{Vendor: "0x10ec", Device: "0xb852"}:                      Realtek8852BPCIE,
	{Vendor: "0x14c3", Device: "0x7961"}:                      MediaTekMT7921PCIE,
	{Vendor: "0x037a", Device: "0x7901"}:                      MediaTekMT7921SDIO,
	{Vendor: "0x14c3", Device: "0x7922"}:                      MediaTekMT7922PCIE,
	{Vendor: "0x14c3", Device: "0x0616"}:                      MediaTekMT7922PCIE,
	{Vendor: "0x14c3", Device: "0x0717"}:                      MediaTekMT7925PCIE,
	{Compatible: "qcom,wcn3990-wifi"}:                         QualcommWCN3990,
	{Compatible: "qcom,wcn6750-wifi"}:                         QualcommWCN6750,
}

var compatibleRE = regexp.MustCompile("^OF_COMPATIBLE_[0-9]")
var wlanRE = regexp.MustCompile("DEVTYPE=wlan")

func findWlanIface() (string, error) {
	const baseDir = "/sys/class/net/"

	dirs, err := ioutil.ReadDir(baseDir)
	if err != nil {
		return "", errors.Wrapf(err,
			"failed to read %s directory", baseDir)
	}
	for _, dir := range dirs {
		path := filepath.Join(baseDir, dir.Name(), "uevent")
		bs, err := ioutil.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				// It is perfectly OK if the file does not exist.
				continue
			}
			// Other errors should be reported, though.
			return "", errors.Wrapf(err, "failed to read %s file", path)
		}
		if wlanRE.MatchString(string(bs)) {
			// ChromeOS supports only one wlan device, thus return first dir matched.
			return dir.Name(), nil
		}
	}
	return "", errors.New("Wireless device not found")
}

// DeviceInfo returns a public struct (DevInfo) containing the WLAN device information.
func DeviceInfo() (*DevInfo, error) {
	readInfo := func(netIf, x string) (string, error) {
		bs, err := ioutil.ReadFile(filepath.Join("/sys/class/net/", netIf, "device", x))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(bs)), nil
	}

	netIf, err := findWlanIface()
	if err != nil {
		// Nothing important to add to the error description.
		return nil, err
	}
	uevent, err := readInfo(netIf, "uevent")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get uevent")
	}

	// Support for (qcom,wcnXXXX-wifi) chips.
	for _, line := range strings.Split(uevent, "\n") {
		if kv := compatibleRE.FindStringSubmatch(line); kv != nil {
			if wifiSnoc := strings.SplitN(line, "=", 2); len(wifiSnoc) == 2 {
				if d, ok := LookupWLANDev[DevInfo{Compatible: wifiSnoc[1]}]; ok {
					// Found the matching device.
					return &DevInfo{ID: d, Name: DeviceNames[d]}, nil
				}
			}
		}
	}

	vendorID, err := readInfo(netIf, "vendor")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get vendor ID at device %q", netIf)
	}

	productID, err := readInfo(netIf, "device")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get product ID at device %q", netIf)
	}

	subsystemID, err := readInfo(netIf, "subsystem_device")
	// DUTs that use SDIO as the bus technology may not have subsystem_device at all.
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "failed to get subsystem ID at device %q", netIf)
	}
	if d, ok := LookupWLANDev[DevInfo{Vendor: vendorID, Device: productID, Subsystem: subsystemID}]; ok {
		return &DevInfo{Vendor: vendorID, Device: productID, Subsystem: subsystemID, ID: d, Name: DeviceNames[d]}, nil
	}

	if d, ok := LookupWLANDev[DevInfo{Vendor: vendorID, Device: productID}]; ok {
		return &DevInfo{Vendor: vendorID, Device: productID, ID: d, Name: DeviceNames[d]}, nil
	}

	return nil, errors.Errorf("unknown %s device with vendorID=%s, productID=%s, subsystemID=%s",
		netIf, vendorID, productID, subsystemID)
}

// List of WLAN devices that don't support MU-MIMO.
var denyListMUMIMO = []DeviceID{
	Marvell88w8897SDIO,  // Tested a DUT.
	Intel7260,           // (WP2) according to datasheet.
	Intel7265,           // (StP2) tested a DUT.
	BroadcomBCM4354SDIO, // Tested a DUT.
	BroadcomBCM4356PCIE, // According to datasheet.
}

// SupportMUMIMO return true if the WLAN device support MU-MIMO.
func (dev *DevInfo) SupportMUMIMO() bool {
	// Checking if the tested WLAN device does not support MU-MIMO.
	for _, id := range denyListMUMIMO {
		if id == dev.ID {
			return false
		}
	}
	return true
}
