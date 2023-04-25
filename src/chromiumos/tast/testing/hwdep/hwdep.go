// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hwdep provides the hardware dependency mechanism to select tests to run on
// a DUT based on its hardware features and setup.
package hwdep

import (
	configpb "go.chromium.org/chromiumos/config/go/api"

	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/testing/wlan"
)

// These are form factor values that can be passed to FormFactor and SkipOnFormFactor.
const (
	FormFactorUnknown = hwdep.FormFactorUnknown
	Clamshell         = hwdep.Clamshell
	Convertible       = hwdep.Convertible
	Detachable        = hwdep.Detachable
	Chromebase        = hwdep.Chromebase
	Chromebox         = hwdep.Chromebox
	Chromebit         = hwdep.Chromebit
	Chromeslate       = hwdep.Chromeslate
)

// Deps holds hardware dependencies all of which need to be satisfied to run a test.
type Deps = hwdep.Deps

// Condition represents one condition of hardware dependencies.
type Condition = hwdep.Condition

// D returns hardware dependencies representing the given Conditions.
func D(conds ...Condition) Deps {
	return hwdep.D(conds...)
}

// WLAN device IDs. Convenience wrappers.
const (
	Marvell88w8897SDIO         = hwdep.Marvell88w8897SDIO
	Marvell88w8997PCIE         = hwdep.Marvell88w8997PCIE
	QualcommAtherosQCA6174     = hwdep.QualcommAtherosQCA6174
	QualcommAtherosQCA6174SDIO = hwdep.QualcommAtherosQCA6174SDIO
	QualcommWCN3990            = hwdep.QualcommWCN3990
	QualcommWCN6750            = hwdep.QualcommWCN6750
	QualcommWCN6855            = hwdep.QualcommWCN6855
	Intel7260                  = hwdep.Intel7260
	Intel7265                  = hwdep.Intel7265
	Intel8265                  = hwdep.Intel8265
	Intel9000                  = hwdep.Intel9000
	Intel9260                  = hwdep.Intel9260
	Intel22260                 = hwdep.Intel22260
	Intel22560                 = hwdep.Intel22560
	IntelAX201                 = hwdep.IntelAX201
	IntelAX203                 = hwdep.IntelAX203
	IntelAX211                 = hwdep.IntelAX211
	BroadcomBCM4354SDIO        = hwdep.BroadcomBCM4354SDIO
	BroadcomBCM4356PCIE        = hwdep.BroadcomBCM4356PCIE
	BroadcomBCM4371PCIE        = hwdep.BroadcomBCM4371PCIE
	Realtek8822CPCIE           = hwdep.Realtek8822CPCIE
	Realtek8852APCIE           = hwdep.Realtek8852APCIE
	Realtek8852CPCIE           = hwdep.Realtek8852CPCIE
	MediaTekMT7921PCIE         = hwdep.MediaTekMT7921PCIE
	MediaTekMT7921SDIO         = hwdep.MediaTekMT7921SDIO
	MediaTekMT7922PCIE         = hwdep.MediaTekMT7922PCIE
)

// Model returns a hardware dependency condition that is satisfied if the DUT's model ID is
// one of the given names.
// Practically, this is not recommended to be used in most cases. Please consider again
// if this is the appropriate use, and whether there exists another option, such as
// check whether DUT needs to have touchscreen, some specific SKU, internal display etc.
//
// Expected example use case is; there is a problem in some code where we do not have
// control, such as a device specific driver, or hardware etc., and unfortunately
// it unlikely be fixed for a while.
// Another use case is; a test is stably running on most of models, but failing on some
// specific models. By using Model() and SkipOnModel() combination, the test can be
// promoted to critical on stably running models, while it is still informational
// on other models. Note that, in this case, it is expected that an engineer is
// assigned to stabilize/fix issues of the test on informational models.
func Model(names ...string) Condition {
	return hwdep.Model(names...)
}

// SkipOnModel returns a hardware dependency condition that is satisfied
// iff the DUT's model ID is none of the given names.
// Please find the doc of Model(), too, for details about the expected usage.
func SkipOnModel(names ...string) Condition {
	return hwdep.SkipOnModel(names...)
}

// Platform returns a hardware dependency condition that is satisfied
// iff the DUT's platform ID is one of the give names.
// Please find the doc of Model(), too, for details about the expected usage.
// Deprecated. Use Model() or "board:*" software dependency.
func Platform(names ...string) Condition {
	return hwdep.Platform(names...)
}

// SkipOnPlatform returns a hardware dependency condition that is satisfied
// iff the DUT's platform ID is none of the give names.
// Please find the doc of Model(), too, for details about the expected usage.
// Deprecated. Use SkipOnModel() or "board:*" software dependency.
func SkipOnPlatform(names ...string) Condition {
	return hwdep.SkipOnPlatform(names...)
}

// WifiDevice returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi device is one of the given names.
// Please find the doc of Model(), too, for details about the expected usage.
func WifiDevice(devices ...wlan.DeviceID) Condition {
	return hwdep.WifiDevice(devices...)
}

// SkipOnWifiDevice returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi device is none of the given names.
// Please find the doc of Model(), too, for details about the expected usage.
func SkipOnWifiDevice(devices ...wlan.DeviceID) Condition {
	return hwdep.SkipOnWifiDevice(devices...)
}

// TouchScreen returns a hardware dependency condition that is satisfied
// iff the DUT has touchscreen.
func TouchScreen() Condition {
	return hwdep.TouchScreen()
}

// NoTouchScreen returns a hardware dependency condition that is satisfied
// if the DUT doesn't have a touchscreen.
func NoTouchScreen() Condition {
	return hwdep.NoTouchScreen()
}

// ChromeEC returns a hardware dependency condition that is satisfied
// iff the DUT has a present EC of the "Chrome EC" type.
func ChromeEC() Condition {
	return hwdep.ChromeEC()
}

// ECFeatureTypecCmd returns a hardware dependency condition that is satisfied
// iff the DUT has an EC which supports the EC_FEATURE_TYPEC_CMD feature flag.
func ECFeatureTypecCmd() Condition {
	return hwdep.ECFeatureTypecCmd()
}

// ECFeatureCBI returns a hardware dependency condition that
// is satisfied iff the DUT has an EC which supports CBI.
func ECFeatureCBI() Condition {
	return hwdep.ECFeatureCBI()
}

// ECFeatureDetachableBase returns a hardware dependency condition that is
// satisfied iff the DUT has the detachable base attached.
func ECFeatureDetachableBase() Condition {
	return hwdep.ECFeatureDetachableBase()
}

// ECFeatureChargeControlV2 returns a hardware dependency condition that is
// satisfied iff the DUT supports version 2 of the EC_CMD_CHARGE_CONTROL feature
// (which adds battery sustain).
func ECFeatureChargeControlV2() Condition {
	return hwdep.ECFeatureChargeControlV2()
}

// Cellular returns a hardware dependency condition that
// is satisfied iff the DUT has a cellular modem.
func Cellular() Condition {
	return hwdep.Cellular()
}

// CellularSoftwareDynamicSar returns a hardware dependency condition that
// is satisfied iff the DUT has enabled software dynamic sar.
func CellularSoftwareDynamicSar() Condition {
	return hwdep.CellularSoftwareDynamicSar()
}

// NoCellular returns a hardware dependency condition that
// is satisfied iff the DUT does not have a cellular modem.
func NoCellular() Condition {
	return hwdep.NoCellular()
}

// Bluetooth returns a hardware dependency condition that
// is satisfied iff the DUT has a bluetooth adapter.
func Bluetooth() Condition {
	return hwdep.Bluetooth()
}

// GSCUART returns a hardware dependency condition that is satisfied iff the DUT has a GSC and that GSC has a working UART.
// TODO(b/224608005): Add a cros_config for this and use that instead.
func GSCUART() Condition {
	return hwdep.GSCUART()
}

// GSCRWKeyIDProd returns a hardware dependency condition that
// is satisfied iff the DUT does have a GSC RW image signed with prod key.
func GSCRWKeyIDProd() Condition {
	return hwdep.GSCRWKeyIDProd()
}

// HasTpm returns a hardware dependency condition that is satisfied iff the DUT
// does have an enabled TPM.
func HasTpm() Condition {
	return hwdep.HasTpm()
}

// HasTpm1 returns a hardware dependency condition that is satisfied iff the DUT
// does have an enabled TPM1.2.
func HasTpm1() Condition {
	return hwdep.HasTpm1()
}

// HasTpm2 returns a hardware dependency condition that is satisfied iff the DUT
// does have an enabled TPM2.0.
func HasTpm2() Condition {
	return hwdep.HasTpm2()
}

// CPUNotNeedsCoreScheduling returns a hardware dependency condition that is satisfied iff the DUT's
// CPU is does not need to use core scheduling to mitigate hardware vulnerabilities.
func CPUNotNeedsCoreScheduling() Condition {
	return hwdep.CPUNotNeedsCoreScheduling()
}

// CPUNeedsCoreScheduling returns a hardware dependency condition that is satisfied iff the DUT's
// CPU needs to use core scheduling to mitigate hardware vulnerabilities.
func CPUNeedsCoreScheduling() Condition {
	return hwdep.CPUNeedsCoreScheduling()
}

// CPUSupportsSMT returns a hardware dependency condition that is satisfied iff the DUT supports
// Symmetric Multi-Threading.
func CPUSupportsSMT() Condition {
	return hwdep.CPUSupportsSMT()
}

// CPUSupportsSHANI returns a hardware dependency condition that is satisfied iff the DUT supports
// SHA-NI instruction extension.
func CPUSupportsSHANI() Condition {
	return hwdep.CPUSupportsSHANI()
}

// Fingerprint returns a hardware dependency condition that is satisfied
// iff the DUT has fingerprint sensor.
func Fingerprint() Condition {
	return hwdep.Fingerprint()
}

// NoFingerprint returns a hardware dependency condition that is satisfied
// if the DUT doesn't have fingerprint sensor.
func NoFingerprint() Condition {
	return hwdep.NoFingerprint()
}

// InternalDisplay returns a hardware dependency condition that is satisfied
// iff the DUT has an internal display, e.g. Chromeboxes and Chromebits don't.
func InternalDisplay() Condition {
	return hwdep.InternalDisplay()
}

// NoInternalDisplay returns a hardware dependency condition that is satisfied
// iff the DUT does not have an internal display.
func NoInternalDisplay() Condition {
	return hwdep.NoInternalDisplay()
}

// Keyboard returns a hardware dependency condition that is satisfied
// iff the DUT has an keyboard, e.g. Chromeboxes and Chromebits don't.
// Tablets might have a removable keyboard.
func Keyboard() Condition {
	return hwdep.Keyboard()
}

// KeyboardBacklight returns a hardware dependency condition that is satified
// if the DUT supports keyboard backlight functionality.
func KeyboardBacklight() Condition {
	return hwdep.KeyboardBacklight()
}

// Touchpad returns a hardware dependency condition that is satisfied
// iff the DUT has a touchpad.
func Touchpad() Condition {
	return hwdep.Touchpad()
}

// WifiWEP returns a hardware dependency condition that is satisfied
// if the DUT's WiFi module supports WEP.
// New generation of Qcom chipsets do not support WEP security protocols.
func WifiWEP() Condition {
	return hwdep.WifiWEP()
}

// Wifi80211ax returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports 802.11ax.
func Wifi80211ax() Condition {
	return hwdep.Wifi80211ax()
}

// Wifi80211ax6E returns a hardware dependency condition that is satisfied
// iff the DUT's WiFi module supports WiFi 6E.
func Wifi80211ax6E() Condition {
	return hwdep.Wifi80211ax6E()
}

// WifiMACAddrRandomize returns a hardware dependency condition that is satisfied
// iff the DUT supports WiFi MAC Address Randomization.
func WifiMACAddrRandomize() Condition {
	return hwdep.WifiMACAddrRandomize()
}

// WifiTDLS returns a hardware dependency condition that is satisfied
// iff the DUT fully supports TDLS MGMT and OPER.
func WifiTDLS() Condition {
	return hwdep.WifiTDLS()
}

// WifiFT returns a hardware dependency condition that is satisfied
// iff the DUT supports Fast Transition roaming mode.
func WifiFT() Condition {
	return hwdep.WifiFT()
}

// WifiNotMarvell returns a hardware dependency condition that is satisfied iff
// the DUT's not using a Marvell WiFi chip.
func WifiNotMarvell() Condition {
	return hwdep.WifiNotMarvell()
}

// WifiNotMarvell8997 returns a hardware dependency condition that is satisfied if
// the DUT is not using Marvell 8997 chipsets.
func WifiNotMarvell8997() Condition {
	return hwdep.WifiNotMarvell8997()
}

// WifiMarvell returns a hardware dependency condition that is satisfied if the
// the DUT is using a Marvell WiFi chip.
func WifiMarvell() Condition {
	return hwdep.WifiMarvell()
}

// WifiIntel returns a hardware dependency condition that if satisfied, indicates
// that a device uses Intel WiFi. It is not guaranteed that the condition will be
// satisfied for all devices with Intel WiFi.
func WifiIntel() Condition {
	return hwdep.WifiIntel()
}

// WifiQualcomm returns a hardware dependency condition that if satisfied, indicates
// that a device uses Qualcomm WiFi.
func WifiQualcomm() Condition {
	return hwdep.WifiQualcomm()
}

// WifiNotQualcomm returns a hardware dependency condition that if satisfied, indicates
// that a device doesn't use Qualcomm WiFi.
func WifiNotQualcomm() Condition {
	return hwdep.WifiNotQualcomm()
}

// WifiSAP returns a hardware dependency condition that if satisfied, indicates
// that a device supports SoftAP.
func WifiSAP() Condition {
	return hwdep.WifiSAP()
}

// WifiVpdSar returns a hardware dependency condition that if satisfied, indicates
// that a device supports VPD SAR tables, and the device actually has such tables
// in VPD.
func WifiVpdSar() Condition {
	return hwdep.WifiVpdSar()
}

// WifiNoVpdSar returns a hardware dependency condition that if satisfied, indicates
// that the device does not support VPD SAR tables.
func WifiNoVpdSar() Condition {
	return hwdep.WifiNoVpdSar()
}

// Battery returns a hardware dependency condition that is satisfied iff the DUT
// has a battery, e.g. Chromeboxes and Chromebits don't.
func Battery() Condition {
	return hwdep.Battery()
}

// NoBatteryBootSupported returns a hardware dependency condition that is satisfied iff the DUT
// supports booting without a battery.
func NoBatteryBootSupported() Condition {
	return hwdep.NoBatteryBootSupported()
}

// SupportsNV12Overlays says true if the SoC supports NV12 hardware overlays,
// which are commonly used for video overlays. SoCs with Intel Gen 7.5 (Haswell,
// BayTrail) and Gen 8 GPUs (Broadwell, Braswell) for example, don't support
// those.
func SupportsNV12Overlays() Condition {
	return hwdep.SupportsNV12Overlays()
}

// SupportsVideoOverlays says true if the SoC supports some type of YUV
// hardware overlay. This includes NV12, I420, and YUY2.
func SupportsVideoOverlays() Condition {
	return hwdep.SupportsVideoOverlays()
}

// Supports30bppFramebuffer says true if the SoC supports 30bpp color depth
// primary plane scanout. This is: Intel SOCs Kabylake and onwards, AMD SOCs
// from Zork onwards (codified Picasso), and not ARM SOCs.
func Supports30bppFramebuffer() Condition {
	return hwdep.Supports30bppFramebuffer()
}

// ForceDischarge returns a hardware dependency condition that is satisfied iff the DUT
// has a battery and it supports force discharge through `ectool chargecontrol`.
// The devices listed in modelsWithoutForceDischargeSupport do not satisfy this condition
// even though they have a battery since they does not support force discharge via ectool.
// This is a complementary condition of NoForceDischarge.
func ForceDischarge() Condition {
	return hwdep.ForceDischarge()
}

// NoForceDischarge is a complementary condition of ForceDischarge.
func NoForceDischarge() Condition {
	return hwdep.NoForceDischarge()
}

// X86 returns a hardware dependency condition matching x86 ABI compatible platform.
func X86() Condition {
	return hwdep.X86()
}

// NoX86 returns a hardware dependency condition matching non-x86 ABI compatible platform.
func NoX86() Condition {
	return hwdep.NoX86()
}

// Emmc returns a hardware dependency condition if the device has an eMMC
// storage device.
func Emmc() Condition {
	return hwdep.Emmc()
}

// Nvme returns a hardware dependency condition if the device has an NVMe
// storage device.
func Nvme() Condition {
	return hwdep.Nvme()
}

// NvmeSelfTest returns a dependency condition if the device has an NVMe storage device which supported NVMe self-test.
func NvmeSelfTest() Condition {
	return hwdep.NvmeSelfTest()
}

// MinStorage returns a hardware dependency condition requiring the minimum size of the storage in gigabytes.
func MinStorage(reqGigabytes int) Condition {
	return hwdep.MinStorage(reqGigabytes)
}

// MinMemory returns a hardware dependency condition requiring the minimum size of the memory in megabytes.
func MinMemory(reqMegabytes int) Condition {
	return hwdep.MinMemory(reqMegabytes)
}

// MaxMemory returns a hardware dependency condition requiring no more than the
// maximum size of the memory in megabytes.
func MaxMemory(reqMegabytes int) Condition {
	return hwdep.MaxMemory(reqMegabytes)
}

// Speaker returns a hardware dependency condition that is satisfied iff the DUT has a speaker.
func Speaker() Condition {
	return hwdep.Speaker()
}

// Microphone returns a hardware dependency condition that is satisfied iff the DUT has a microphone.
func Microphone() Condition {
	return hwdep.Microphone()
}

// PrivacyScreen returns a hardware dependency condition that is satisfied iff the DUT has a privacy screen.
func PrivacyScreen() Condition {
	return hwdep.PrivacyScreen()
}

// NoPrivacyScreen returns a hardware dependency condition that is satisfied if the DUT
// does not have a privacy screen.
func NoPrivacyScreen() Condition {
	return hwdep.NoPrivacyScreen()
}

// SmartAmp returns a hardware dependency condition that is satisfied iff the DUT
// has smart amplifier.
func SmartAmp() Condition {
	return hwdep.SmartAmp()
}

// SmartAmpBootTimeCalibration returns a hardware dependency condition that is satisfied iff
// the DUT enables boot time calibration for smart amplifier.
func SmartAmpBootTimeCalibration() Condition {
	return hwdep.SmartAmpBootTimeCalibration()
}

// FormFactor returns a hardware dependency condition that is satisfied
// iff the DUT's form factor is one of the given values.
func FormFactor(ffList ...configpb.HardwareFeatures_FormFactor_FormFactorType) Condition {
	return hwdep.FormFactor(ffList...)
}

// SkipOnFormFactor returns a hardware dependency condition that is satisfied
// iff the DUT's form factor is none of the give values.
func SkipOnFormFactor(ffList ...configpb.HardwareFeatures_FormFactor_FormFactorType) Condition {
	return hwdep.SkipOnFormFactor(ffList...)
}

// SupportsV4L2StatefulVideoDecoding says true if the SoC supports the V4L2
// stateful video decoding kernel API. Examples of this are MTK8173 and
// Qualcomm devices (7180, etc). In general, we prefer to use stateless
// decoding APIs, so listing them individually makes sense.
func SupportsV4L2StatefulVideoDecoding() Condition {
	return hwdep.SupportsV4L2StatefulVideoDecoding()
}

// SupportsV4L2StatelessVideoDecoding says true if the SoC supports the V4L2
// stateless video decoding kernel API. Examples of this are MTK8192 (Asurada),
// MTK8195 (Cherry), MTK8186 (Corsola), and RK3399 (scarlet/kevin/bob).
func SupportsV4L2StatelessVideoDecoding() Condition {
	return hwdep.SupportsV4L2StatelessVideoDecoding()
}

// Lid returns a hardware dependency condition that is satisfied iff the DUT's form factor has a lid.
func Lid() Condition {
	return hwdep.Lid()
}

// InternalKeyboard returns a hardware dependency condition that is satisfied iff the DUT's form factor has a fixed undettachable keyboard.
func InternalKeyboard() Condition {
	return hwdep.InternalKeyboard()
}

// DisplayPortConverter is satisfied if a DP converter with one of the given names
// is present.
func DisplayPortConverter(names ...string) Condition {
	return hwdep.DisplayPortConverter(names...)
}

// Vboot2 is satisfied iff crossystem param 'fw_vboot2' indicates that DUT uses vboot2.
func Vboot2() Condition {
	return hwdep.Vboot2()
}

// SupportsVP9KSVCHWDecoding is satisfied if the SoC supports VP9 k-SVC
// hardware decoding. They are x86 devices that are capable of VP9 hardware
// decoding and Qualcomm7180/7280.
// VP9 k-SVC is a SVC stream in which a frame only on keyframe can refer frames
// in a different spatial layer. See https://www.w3.org/TR/webrtc-svc/#dependencydiagrams* for detail.
func SupportsVP9KSVCHWDecoding() Condition {
	return hwdep.SupportsVP9KSVCHWDecoding()
}

// AssistantKey is satisfied if a model has an assistant key.
func AssistantKey() Condition {
	return hwdep.AssistantKey()
}

// NoAssistantKey is satisfied if a model does not have an assistant key.
func NoAssistantKey() Condition {
	return hwdep.NoAssistantKey()
}

// HPS is satisfied if the HPS peripheral (go/cros-hps) is present in the DUT.
func HPS() Condition {
	return hwdep.HPS()
}

// CameraFeature is satisfied if all the features listed in |names| are enabled on the DUT.
func CameraFeature(names ...string) Condition {
	return hwdep.CameraFeature(names...)
}

// MainboardHasEarlyLibgfxinit is satisfied if the BIOS was built with Kconfig CONFIG_MAINBOARD_HAS_EARLY_LIBGFXINIT
func MainboardHasEarlyLibgfxinit() Condition {
	return hwdep.MainboardHasEarlyLibgfxinit()
}

// VbootCbfsIntegration is satisfied if the BIOS was built with Kconfig CONFIG_VBOOT_CBFS_INTEGRATION
func VbootCbfsIntegration() Condition {
	return hwdep.VbootCbfsIntegration()
}

// RuntimeProbeConfig is satisfied if the probe config of the model exists.
func RuntimeProbeConfig() Condition {
	return hwdep.RuntimeProbeConfig()
}

// SeamlessRefreshRate is satisfied if the device supports changing refresh rates without modeset.
func SeamlessRefreshRate() Condition {
	return hwdep.SeamlessRefreshRate()
}

// GPUFamily is satisfied if the devices GPU family is categoried as one of the families specified.
// For a complete list of values or to add new ones please check the pciid maps at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func GPUFamily(families []string) Condition {
	return hwdep.GPUFamily(families)
}

// SkipGPUFamily is satisfied if the devices GPU family is none of the families specified.
// For a complete list of values or to add new ones please check the pciid maps at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func SkipGPUFamily(families []string) Condition {
	return hwdep.SkipGPUFamily(families)
}

// GPUVendor is satisfied if the devices GPU vendor is categoried as one of the vendors specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func GPUVendor(vendors []string) Condition {
	return hwdep.GPUVendor(vendors)
}

// SkipGPUVendor is satisfied if the devices GPU vendor is categoried as none of the vendors specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func SkipGPUVendor(vendors []string) Condition {
	return hwdep.SkipGPUVendor(vendors)
}

// CPUSocFamily is satisfied if the devices CPU SOC family is categoried as one of the families specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func CPUSocFamily(families []string) Condition {
	return hwdep.CPUSocFamily(families)
}

// SkipCPUSocFamily is satisfied if the devies CPU SOC family is none of the families specified.
// For a complete list of values or to add new ones please check the files at
// https://chromium.googlesource.com/chromiumos/platform/graphics/+/refs/heads/main/src/go.chromium.org/chromiumos/graphics-utils-go/hardware_probe/cmd/hardware_probe
func SkipCPUSocFamily(families []string) Condition {
	return hwdep.SkipCPUSocFamily(families)
}
