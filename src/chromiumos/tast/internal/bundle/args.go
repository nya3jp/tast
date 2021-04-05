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

// bundleType describes the type of tests contained in a test bundle (i.e. local or remote).
type bundleType int

const (
	localBundle bundleType = iota
	remoteBundle
)

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is further updated by decoding a JSON-marshaled BundleArgs struct from stdin.
// The caller is responsible for performing the requested action.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer, args *jsonprotocol.BundleArgs, bt bundleType) error {
	if len(clArgs) != 0 {
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.SetOutput(stderr)
		flags.Usage = func() {
			runner := "local_test_runner"
			if bt == remoteBundle {
				runner = "remote_test_runner"
			}
			fmt.Fprintf(stderr, "Usage: %s [flag]...\n\n"+
				"Tast test bundle containing integration tests.\n\n"+
				"This is typically executed by %s.\n\n",
				filepath.Base(os.Args[0]), runner)
			flags.PrintDefaults()
		}

		dump := flags.Bool("dumptests", false, "dump all tests as a JSON-marshaled array of testing.Test structs")
		exportMetadata := flags.Bool("exportmetadata", false, "export all test metadata as a protobuf-marshaled message")
		rpc := flags.Bool("rpc", false, "run gRPC server")
		if err := flags.Parse(clArgs); err != nil {
			return command.NewStatusErrorf(statusBadArgs, "%v", err)
		}
		if *dump {
			args.Mode = jsonprotocol.BundleListTestsMode
			args.ListTests = &jsonprotocol.BundleListTestsArgs{}
			return nil
		}
		if *exportMetadata {
			args.Mode = jsonprotocol.BundleExportMetadataMode
			return nil
		}
		if *rpc {
			args.Mode = jsonprotocol.BundleRPCMode
			return nil
		}
	}

	if err := json.NewDecoder(stdin).Decode(args); err != nil {
		return command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
	}

	if (args.Mode == jsonprotocol.BundleRunTestsMode && args.RunTests == nil) ||
		(args.Mode == jsonprotocol.BundleListTestsMode && args.ListTests == nil) {
		return command.NewStatusErrorf(statusBadArgs, "args not set for mode %v", args.Mode)
	}

	// Use non-zero-valued deprecated fields if they were supplied by an old test runner.
	args.PromoteDeprecated()

	return nil
}
