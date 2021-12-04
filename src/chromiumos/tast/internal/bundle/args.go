// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
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
		rpctcp := flags.Bool("rpctcp", false, "run gRPC server listening on TCP. Sample usage:\n"+
			"  cros -rpctcp -port 4444 -handshake [HANDSHAKE_BASE64]")
		port := flags.Int("port", 4444, "port number for gRPC server. Only applicable for rpctcp mode")
		handshakeUsage := "Handshake request for setting up gRPC server. Only applicable for rpctcp mode.\n" +
			"Request should adhere to handshake.proto and be encoded in base64. Example in golang:\n" +
			"  var req *HandshakeRequest = ...\n" +
			"  raw, _ := proto.Marshal(req)\n" +
			"  input = base64.StdEncoding.EncodeToString(raw)"
		handshakeBase64 := flags.String("handshake", "", handshakeUsage)
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
		if *rpctcp {
			var handshakeReq protocol.HandshakeRequest
			if err := decodeBase64Proto(*handshakeBase64, &handshakeReq); err != nil {
				return nil, command.NewStatusErrorf(statusBadArgs, "failed to decode handshake into proto: %v", err)
			}
			return &jsonprotocol.BundleArgs{
				Mode: jsonprotocol.BundleRPCTCPServerMode,
				RPCTCPServer: &jsonprotocol.BundleRPCTCPServerArgs{
					Port:             *port,
					HandshakeRequest: &handshakeReq,
				},
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

// decodeBase64Proto decodes a base64 encoded proto into proto message.
func decodeBase64Proto(protoBase64 string, msg proto.Message) error {
	protoBytes, err := base64.StdEncoding.DecodeString(protoBase64)
	if err != nil {
		return err
	}
	return proto.Unmarshal(protoBytes, msg)
}
