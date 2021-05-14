// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/internal/protocol"
)

func toDeviceConfigSoc(soc protocol.DeviceInfo_SOC) device.Config_SOC {
	switch soc {
	case protocol.DeviceInfo_SOC_UNSPECIFIED:
		return device.Config_SOC_UNSPECIFIED
	case protocol.DeviceInfo_SOC_AMBERLAKE_Y:
		return device.Config_SOC_AMBERLAKE_Y
	case protocol.DeviceInfo_SOC_APOLLO_LAKE:
		return device.Config_SOC_APOLLO_LAKE
	case protocol.DeviceInfo_SOC_BAY_TRAIL:
		return device.Config_SOC_BAY_TRAIL
	case protocol.DeviceInfo_SOC_BRASWELL:
		return device.Config_SOC_BRASWELL
	case protocol.DeviceInfo_SOC_BROADWELL:
		return device.Config_SOC_BROADWELL
	case protocol.DeviceInfo_SOC_CANNON_LAKE_Y:
		return device.Config_SOC_CANNON_LAKE_Y
	case protocol.DeviceInfo_SOC_COMET_LAKE_U:
		return device.Config_SOC_COMET_LAKE_U
	case protocol.DeviceInfo_SOC_EXYNOS_5250:
		return device.Config_SOC_EXYNOS_5250
	case protocol.DeviceInfo_SOC_EXYNOS_5420:
		return device.Config_SOC_EXYNOS_5420
	case protocol.DeviceInfo_SOC_GEMINI_LAKE:
		return device.Config_SOC_GEMINI_LAKE
	case protocol.DeviceInfo_SOC_HASWELL:
		return device.Config_SOC_HASWELL
	case protocol.DeviceInfo_SOC_ICE_LAKE_Y:
		return device.Config_SOC_ICE_LAKE_Y
	case protocol.DeviceInfo_SOC_IVY_BRIDGE:
		return device.Config_SOC_IVY_BRIDGE
	case protocol.DeviceInfo_SOC_KABYLAKE_U:
		return device.Config_SOC_KABYLAKE_U
	case protocol.DeviceInfo_SOC_KABYLAKE_U_R:
		return device.Config_SOC_KABYLAKE_U_R
	case protocol.DeviceInfo_SOC_KABYLAKE_Y:
		return device.Config_SOC_KABYLAKE_Y
	case protocol.DeviceInfo_SOC_MT8173:
		return device.Config_SOC_MT8173
	case protocol.DeviceInfo_SOC_MT8176:
		return device.Config_SOC_MT8176
	case protocol.DeviceInfo_SOC_MT8183:
		return device.Config_SOC_MT8183
	case protocol.DeviceInfo_SOC_PICASSO:
		return device.Config_SOC_PICASSO
	case protocol.DeviceInfo_SOC_PINE_TRAIL:
		return device.Config_SOC_PINE_TRAIL
	case protocol.DeviceInfo_SOC_RK3288:
		return device.Config_SOC_RK3288
	case protocol.DeviceInfo_SOC_RK3399:
		return device.Config_SOC_RK3399
	case protocol.DeviceInfo_SOC_SANDY_BRIDGE:
		return device.Config_SOC_SANDY_BRIDGE
	case protocol.DeviceInfo_SOC_SDM845:
		return device.Config_SOC_SDM845
	case protocol.DeviceInfo_SOC_SKYLAKE_U:
		return device.Config_SOC_SKYLAKE_U
	case protocol.DeviceInfo_SOC_SKYLAKE_Y:
		return device.Config_SOC_SKYLAKE_Y
	case protocol.DeviceInfo_SOC_STONEY_RIDGE:
		return device.Config_SOC_STONEY_RIDGE
	case protocol.DeviceInfo_SOC_TEGRA_K1:
		return device.Config_SOC_TEGRA_K1
	case protocol.DeviceInfo_SOC_WHISKEY_LAKE_U:
		return device.Config_SOC_WHISKEY_LAKE_U
	case protocol.DeviceInfo_SOC_SC7180:
		return device.Config_SOC_SC7180
	case protocol.DeviceInfo_SOC_JASPER_LAKE:
		return device.Config_SOC_JASPER_LAKE
	case protocol.DeviceInfo_SOC_TIGER_LAKE:
		return device.Config_SOC_TIGER_LAKE
	case protocol.DeviceInfo_SOC_MT8192:
		return device.Config_SOC_MT8192
	}
	return device.Config_SOC_UNSPECIFIED
}

func toDeviceConfigCPUArch(cpuArch protocol.DeviceInfo_Architecture) device.Config_Architecture {
	switch cpuArch {
	case protocol.DeviceInfo_ARCHITECTURE_UNDEFINED:
		return device.Config_ARCHITECTURE_UNDEFINED
	case protocol.DeviceInfo_X86:
		return device.Config_X86
	case protocol.DeviceInfo_X86_64:
		return device.Config_X86_64
	case protocol.DeviceInfo_ARM:
		return device.Config_ARM
	case protocol.DeviceInfo_ARM64:
		return device.Config_ARM64
	}
	return device.Config_ARCHITECTURE_UNDEFINED
}
