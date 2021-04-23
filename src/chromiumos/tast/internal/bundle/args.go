// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/jsonprotocol"
)

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// The caller is responsible for performing the requested action.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer) (*jsonprotocol.BundleArgs, error) {
	if len(clArgs) != 0 {
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.SetOutput(stderr)
		flags.Usage = func() {
			fmt.Fprintf(stderr, "Usage: %s [flag]...\n\n"+
				"Tast test bundle containing integration tests.\n\n",
				filepath.Base(os.Args[0]))
			flags.PrintDefaults()
		}

		dump := flags.Bool("dumptests", false, "dump all tests as a JSON-marshaled array of testing.Test structs")
		exportMetadata := flags.Bool("exportmetadata", false, "export all test metadata as a protobuf-marshaled message")
		rpc := flags.Bool("rpc", false, "run gRPC server")
		if err := flags.Parse(clArgs); err != nil {
			return nil, command.NewStatusErrorf(statusBadArgs, "%v", err)
		}
		if *dump {
			return &jsonprotocol.BundleArgs{
				Mode:      jsonprotocol.BundleListTestsMode,
				ListTests: &jsonprotocol.BundleListTestsArgs{},
			}, nil
		}
		if *exportMetadata {
			return &jsonprotocol.BundleArgs{
				Mode: jsonprotocol.BundleExportMetadataMode,
			}, nil
		}
		if *rpc {
			return &jsonprotocol.BundleArgs{
				Mode: jsonprotocol.BundleRPCMode,
			}, nil
		}
	}

	var args jsonprotocol.BundleArgs
	if err := json.NewDecoder(stdin).Decode(&args); err != nil {
		return nil, command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
	}

	if args.Mode == jsonprotocol.BundleListTestsMode && args.ListTests == nil {
		return nil, command.NewStatusErrorf(statusBadArgs, "args not set for mode %v", args.Mode)
	}

	// Use non-zero-valued deprecated fields if they were supplied by an old test runner.
	args.PromoteDeprecated()

	return &args, nil
}
