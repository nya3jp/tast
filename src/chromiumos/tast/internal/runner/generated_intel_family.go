// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

// Code generated by gen/gen_intel_family.go. DO NOT EDIT.
//
// Do not change the above line; see https://golang.org/pkg/cmd/go/internal/generate/
//
// This file contains constants from arch/x86/include/asm/intel-family.h
// in the Linux kernel repository at revision 4e8ae0c6a8aa41670b817bf02036cbfff2854227.
// Run "go generate" to regenerate it.

//go:generate ../../../../../tools/go.sh run gen/gen_intel_family.go ../../../../../../../third_party/kernel/v5.4/arch/x86/include/asm/intel-family.h generated_intel_family.go
//go:generate ../../../../../tools/go.sh fmt generated_intel_family.go

const (
	// Intel family
	INTEL_FAM6_CORE_YONAH           = 0xe
	INTEL_FAM6_CORE2_MEROM          = 0xf
	INTEL_FAM6_CORE2_MEROM_L        = 0x16
	INTEL_FAM6_CORE2_PENRYN         = 0x17
	INTEL_FAM6_NEHALEM_EP           = 0x1a
	INTEL_FAM6_ATOM_BONNELL         = 0x1c
	INTEL_FAM6_CORE2_DUNNINGTON     = 0x1d
	INTEL_FAM6_NEHALEM              = 0x1e
	INTEL_FAM6_NEHALEM_G            = 0x1f
	INTEL_FAM6_WESTMERE             = 0x25
	INTEL_FAM6_ATOM_BONNELL_MID     = 0x26
	INTEL_FAM6_ATOM_SALTWELL_MID    = 0x27
	INTEL_FAM6_SANDYBRIDGE          = 0x2a
	INTEL_FAM6_WESTMERE_EP          = 0x2c
	INTEL_FAM6_SANDYBRIDGE_X        = 0x2d
	INTEL_FAM6_NEHALEM_EX           = 0x2e
	INTEL_FAM6_WESTMERE_EX          = 0x2f
	INTEL_FAM6_ATOM_SALTWELL_TABLET = 0x35
	INTEL_FAM6_ATOM_SALTWELL        = 0x36
	INTEL_FAM6_ATOM_SILVERMONT      = 0x37
	INTEL_FAM6_IVYBRIDGE            = 0x3a
	INTEL_FAM6_HASWELL              = 0x3c
	INTEL_FAM6_BROADWELL            = 0x3d
	INTEL_FAM6_IVYBRIDGE_X          = 0x3e
	INTEL_FAM6_HASWELL_X            = 0x3f
	INTEL_FAM6_HASWELL_L            = 0x45
	INTEL_FAM6_HASWELL_G            = 0x46
	INTEL_FAM6_BROADWELL_G          = 0x47
	INTEL_FAM6_ATOM_SILVERMONT_MID  = 0x4a
	INTEL_FAM6_ATOM_AIRMONT         = 0x4c
	INTEL_FAM6_ATOM_SILVERMONT_D    = 0x4d
	INTEL_FAM6_SKYLAKE_L            = 0x4e
	INTEL_FAM6_BROADWELL_X          = 0x4f
	INTEL_FAM6_SKYLAKE_X            = 0x55
	INTEL_FAM6_BROADWELL_D          = 0x56
	INTEL_FAM6_XEON_PHI_KNL         = 0x57
	INTEL_FAM6_ATOM_AIRMONT_MID     = 0x5a
	INTEL_FAM6_ATOM_GOLDMONT        = 0x5c
	INTEL_FAM6_SKYLAKE              = 0x5e
	INTEL_FAM6_ATOM_GOLDMONT_D      = 0x5f
	INTEL_FAM6_CANNONLAKE_L         = 0x66
	INTEL_FAM6_ICELAKE_X            = 0x6a
	INTEL_FAM6_ICELAKE_D            = 0x6c
	INTEL_FAM6_ATOM_AIRMONT_NP      = 0x75
	INTEL_FAM6_ATOM_GOLDMONT_PLUS   = 0x7a
	INTEL_FAM6_ICELAKE              = 0x7d
	INTEL_FAM6_ICELAKE_L            = 0x7e
	INTEL_FAM6_XEON_PHI_KNM         = 0x85
	INTEL_FAM6_ATOM_TREMONT_D       = 0x86
	INTEL_FAM6_TIGERLAKE_L          = 0x8c
	INTEL_FAM6_TIGERLAKE            = 0x8d
	INTEL_FAM6_KABYLAKE_L           = 0x8e
	INTEL_FAM6_ATOM_TREMONT         = 0x96
	INTEL_FAM6_ATOM_TREMONT_L       = 0x9c
	INTEL_FAM6_ICELAKE_NNPI         = 0x9d
	INTEL_FAM6_KABYLAKE             = 0x9e
	INTEL_FAM6_COMETLAKE            = 0xa5
	INTEL_FAM6_COMETLAKE_L          = 0xa6
)
