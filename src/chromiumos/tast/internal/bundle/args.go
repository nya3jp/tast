// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/protocol"
)

type mode int

const (
	modeRPC mode = iota
	modeDumpTests
	modeRPCTCP
)

type dumpFormat int

const (
	dumpFormatLegacyJSON dumpFormat = iota
	dumpFormatProto
)

type parsedArgs struct {
	mode       mode
	dumpFormat dumpFormat
	// rpctcp mode only
	port      int
	handshake *protocol.HandshakeRequest
}

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// The caller is responsible for performing the requested action.
func readArgs(clArgs []string, stderr io.Writer) (*parsedArgs, error) {
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
		return &parsedArgs{mode: modeDumpTests, dumpFormat: dumpFormatLegacyJSON}, nil
	}
	if *exportMetadata {
		return &parsedArgs{mode: modeDumpTests, dumpFormat: dumpFormatProto}, nil
	}
	if *rpc {
		return &parsedArgs{mode: modeRPC}, nil
	}
	if *rpctcp {
		var handshakeReq protocol.HandshakeRequest
		if err := decodeBase64Proto(*handshakeBase64, &handshakeReq); err != nil {
			return nil, command.NewStatusErrorf(statusBadArgs, "failed to decode handshake into proto: %v", err)
		}
		return &parsedArgs{
			mode:      modeRPCTCP,
			port:      *port,
			handshake: &handshakeReq,
		}, nil
	}
	flags.Usage()
	return nil, errors.New("no mode flag is set")
}

// decodeBase64Proto decodes a base64 encoded proto into proto message.
func decodeBase64Proto(protoBase64 string, msg proto.Message) error {
	protoBytes, err := base64.StdEncoding.DecodeString(protoBase64)
	if err != nil {
		return err
	}
	return proto.Unmarshal(protoBytes, msg)
}
