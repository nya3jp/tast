// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements a script for writing a Go source file containing intel family constants.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"

	"chromiumos/tast/genutil"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <intel-family.h> <out.go>\n", os.Args[0])
		os.Exit(1)
	}

	inputFile := args[0]
	outputFile := args[1]

	const (
		// Relative path to go.sh from the generated file.
		goSh = "../../../../../tools/go.sh"

		// Relative path to this file from the generated file.
		thisFile = "gen/gen_intel_family.go"
	)

	params := genutil.Params{
		PackageName: "crosbundle",
		RepoName:    "Linux kernel",
		PreludeCode: `//go:generate ` + goSh + ` run ` + thisFile + ` ../../../../../../../third_party/kernel/v5.4/arch/x86/include/asm/intel-family.h hardware_intel_family.go
//go:generate ` + goSh + ` fmt hardware_intel_family.go`,
		CopyrightYear:  2020,
		MainGoFilePath: thisFile,

		Groups: []genutil.GroupSpec{
			{Prefix: "INTEL_FAM6_", TypeName: "", Desc: "Intel family"},
		},

		LineParser: func() genutil.LineParser {
			// Reads inputFile, a kernel input-event-codes.h. Looking for lines like:
			//   #define EV_SYN 0x00
			re := regexp.MustCompile(`^#define\s+([A-Z][_A-Z0-9]+)\s+(0x[0-9a-fA-F]+|\d+)`)
			return func(line string) (name, sval string, ok bool) {
				m := re.FindStringSubmatch(line)
				if m == nil {
					return "", "", false
				}
				return m[1], m[2], true
			}
		}(),
	}

	if err := genutil.GenerateConstants(inputFile, outputFile, params); err != nil {
		log.Fatalf("Failed to generate %v: %v", outputFile, err)
	}
}
