// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package jsonprotocol

import (
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/internal/protocol"
)

func fromDeviceConfigSoc(soc device.Config_SOC) protocol.DeviceInfo_SOC {
	switch soc {
	case device.Config_SOC_UNSPECIFIED:
		return protocol.DeviceInfo_SOC_UNSPECIFIED
	case device.Config_SOC_AMBERLAKE_Y:
		return protocol.DeviceInfo_SOC_AMBERLAKE_Y
	case device.Config_SOC_APOLLO_LAKE:
		return protocol.DeviceInfo_SOC_APOLLO_LAKE
	case device.Config_SOC_BAY_TRAIL:
		return protocol.DeviceInfo_SOC_BAY_TRAIL
	case device.Config_SOC_BRASWELL:
		return protocol.DeviceInfo_SOC_BRASWELL
	case device.Config_SOC_BROADWELL:
		return protocol.DeviceInfo_SOC_BROADWELL
	case device.Config_SOC_CANNON_LAKE_Y:
		return protocol.DeviceInfo_SOC_CANNON_LAKE_Y
	case device.Config_SOC_COMET_LAKE_U:
		return protocol.DeviceInfo_SOC_COMET_LAKE_U
	case device.Config_SOC_EXYNOS_5250:
		return protocol.DeviceInfo_SOC_EXYNOS_5250
	case device.Config_SOC_EXYNOS_5420:
		return protocol.DeviceInfo_SOC_EXYNOS_5420
	case device.Config_SOC_GEMINI_LAKE:
		return protocol.DeviceInfo_SOC_GEMINI_LAKE
	case device.Config_SOC_HASWELL:
		return protocol.DeviceInfo_SOC_HASWELL
	case device.Config_SOC_ICE_LAKE_Y:
		return protocol.DeviceInfo_SOC_ICE_LAKE_Y
	case device.Config_SOC_IVY_BRIDGE:
		return protocol.DeviceInfo_SOC_IVY_BRIDGE
	case device.Config_SOC_KABYLAKE_U:
		return protocol.DeviceInfo_SOC_KABYLAKE_U
	case device.Config_SOC_KABYLAKE_U_R:
		return protocol.DeviceInfo_SOC_KABYLAKE_U_R
	case device.Config_SOC_KABYLAKE_Y:
		return protocol.DeviceInfo_SOC_KABYLAKE_Y
	case device.Config_SOC_MT8173:
		return protocol.DeviceInfo_SOC_MT8173
	case device.Config_SOC_MT8176:
		return protocol.DeviceInfo_SOC_MT8176
	case device.Config_SOC_MT8183:
		return protocol.DeviceInfo_SOC_MT8183
	case device.Config_SOC_PICASSO:
		return protocol.DeviceInfo_SOC_PICASSO
	case device.Config_SOC_PINE_TRAIL:
		return protocol.DeviceInfo_SOC_PINE_TRAIL
	case device.Config_SOC_RK3288:
		return protocol.DeviceInfo_SOC_RK3288
	case device.Config_SOC_RK3399:
		return protocol.DeviceInfo_SOC_RK3399
	case device.Config_SOC_SANDY_BRIDGE:
		return protocol.DeviceInfo_SOC_SANDY_BRIDGE
	case device.Config_SOC_SDM845:
		return protocol.DeviceInfo_SOC_SDM845
	case device.Config_SOC_SKYLAKE_U:
		return protocol.DeviceInfo_SOC_SKYLAKE_U
	case device.Config_SOC_SKYLAKE_Y:
		return protocol.DeviceInfo_SOC_SKYLAKE_Y
	case device.Config_SOC_STONEY_RIDGE:
		return protocol.DeviceInfo_SOC_STONEY_RIDGE
	case device.Config_SOC_TEGRA_K1:
		return protocol.DeviceInfo_SOC_TEGRA_K1
	case device.Config_SOC_WHISKEY_LAKE_U:
		return protocol.DeviceInfo_SOC_WHISKEY_LAKE_U
	case device.Config_SOC_SC7180:
		return protocol.DeviceInfo_SOC_SC7180
	case device.Config_SOC_JASPER_LAKE:
		return protocol.DeviceInfo_SOC_JASPER_LAKE
	case device.Config_SOC_TIGER_LAKE:
		return protocol.DeviceInfo_SOC_TIGER_LAKE
	case device.Config_SOC_MT8192:
		return protocol.DeviceInfo_SOC_MT8192
	}
	return protocol.DeviceInfo_SOC_UNSPECIFIED
}

func fromDeviceConfigCPUArch(cpuArch device.Config_Architecture) protocol.DeviceInfo_Architecture {
	switch cpuArch {
	case device.Config_ARCHITECTURE_UNDEFINED:
		return protocol.DeviceInfo_ARCHITECTURE_UNDEFINED
	case device.Config_X86:
		return protocol.DeviceInfo_X86
	case device.Config_X86_64:
		return protocol.DeviceInfo_X86_64
	case device.Config_ARM:
		return protocol.DeviceInfo_ARM
	case device.Config_ARM64:
		return protocol.DeviceInfo_ARM64
	}
	return protocol.DeviceInfo_ARCHITECTURE_UNDEFINED
}

func fromDeviceConfigPowerSupply(power device.Config_PowerSupply) protocol.DeviceInfo_PowerSupply {
	switch power {
	case device.Config_POWER_SUPPLY_UNSPECIFIED:
		return protocol.DeviceInfo_POWER_SUPPLY_UNSPECIFIED
	case device.Config_POWER_SUPPLY_BATTERY:
		return protocol.DeviceInfo_POWER_SUPPLY_BATTERY
	case device.Config_POWER_SUPPLY_AC_ONLY:
		return protocol.DeviceInfo_POWER_SUPPLY_AC_ONLY
	}
	return protocol.DeviceInfo_POWER_SUPPLY_UNSPECIFIED
}
