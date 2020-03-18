// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

// The model values of intel family 6 CPUs.
// This table is taken from kernel/v5.4/arch/x86/include/asm/intel-family.h.
// See the original header file for detail.
const (
	IntelFam6CoreYonah = 0x0E

	IntelFam6Core2Merom      = 0x0F
	IntelFam6Core2MeromL     = 0x16
	IntelFam6Core2Penryn     = 0x17
	IntelFam6Core2Dunnington = 0x1D

	IntelFam6Nehalem   = 0x1E
	IntelFam6NehalemG  = 0x1F
	IntelFam6NehalemEP = 0x1A
	IntelFam6NehalemEX = 0x2E

	IntelFam6Westmere   = 0x25
	IntelFam6WestmereEP = 0x2C
	IntelFam6WestmereEX = 0x2F

	IntelFam6Sandybridge  = 0x2A
	IntelFam6SandybridgeX = 0x2D
	IntelFam6Ivybridge    = 0x3A
	IntelFam6IvybridgeX   = 0x3E

	IntelFam6Haswell  = 0x3C
	IntelFam6HaswellX = 0x3F
	IntelFam6HaswellL = 0x45
	IntelFam6HaswellG = 0x46

	IntelFam6Broadwell  = 0x3D
	IntelFam6BroadwellG = 0x47
	IntelFam6BroadwellX = 0x4F
	IntelFam6BroadwellD = 0x56

	IntelFam6SkylakeL  = 0x4E
	IntelFam6Skylake   = 0x5E
	IntelFam6SkylakeX  = 0x55
	IntelFam6KabylakeL = 0x8E
	IntelFam6Kabylake  = 0x9E

	IntelFam6Cannonlake = 0x66

	IntelFam6IcelakeX    = 0x6A
	IntelFam6IcelakeD    = 0x6C
	IntelFam6Icelake     = 0x7D
	IntelFam6IcelakeL    = 0x7E
	IntelFam6IcelakeNNPI = 0x9D

	IntelFam6TigerlakeL = 0x8C
	IntelFam6Tigerlake  = 0x8D

	IntelFam6Cometlake  = 0xA5
	IntelFam6CometlakeL = 0xA6

	IntelFam6AtomBonnell   = 0x1C
	IntelFam6AtomBonnelMid = 0x26

	IntelFam6AtomSaltwell       = 0x36
	IntelFam6AtomSaltwellMid    = 0x27
	IntelFam6AtomSaltwellTablet = 0x35

	IntelFam6AtomSilvermont    = 0x37
	IntelFam6AtomSilvermontD   = 0x4D
	IntelFam6AtomSilvermontMid = 0x4A

	IntelFam6AtomAirmont    = 0x4C
	IntelFam6AtomAirmontMid = 0x5A
	IntelFam6AtomAirmontNP  = 0x75

	IntelFam6AtomGoldmont  = 0x5C
	IntelFam6AtomGoldmontD = 0x5F

	IntelFam6AtomGoldmontPlus = 0x7A

	IntelFam6AtomTremontD = 0x86
	IntelFam6AtomTremont  = 0x96
	IntelFam6AtomTremontL = 0x9C

	IntelFam6XeonPhiKNL = 0x57
	IntelFam6XeonPhiKNM = 0x85
)
