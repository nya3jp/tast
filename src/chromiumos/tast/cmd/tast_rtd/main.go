// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast_rtd executable, used to invoke tast in RTD.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast_rtd/internal/rpc"
)

// Version is the version info of this command. It is filled in during emerge.
var Version = "<unknown>"

// createLogFile creates a file and its parent directory for logging purpose.
func createLogFile() (*os.File, error) {
	t := time.Now()
	fullPath := filepath.Join("/tmp/tast_rtd/results", t.Format("20060102-150405"))
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %v: %v", fullPath, err)
	}

	logFullPathName := filepath.Join(fullPath, "log.txt")

	// Log the full output of the command to disk.
	logFile, err := os.Create(logFullPathName)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file %v: %v", fullPath, err)
	}
	return logFile, nil
}

// newLogger creates a logger.
func newLogger(logFile *os.File) *log.Logger {
	mw := io.MultiWriter(logFile, os.Stderr)
	return log.New(mw, "", log.LstdFlags)
}

// readInput reads an invocation protobuf file and returns a pointer to rtd.Invocation.
// TODO: Populate rtd.Invocation.
func readInput(fileName string) (*rtd.Invocation, error) {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("fail to read file %v: %v", fileName, err)
	}
	inv, err := unmarshalInvocation(data)
	if err != nil {
		return nil, fmt.Errorf("fail to unmarshal file %v: %v", fileName, err)
	}
	return inv, nil
}

func main() {
	os.Exit(func() int {
		ctx := context.Background()
		version := flag.Bool("version", false, "print version and exit")
		input := flag.String("input", "", "specify the test invocation request protobuf input file")
		rtdPath := flag.String("rtd", "/usr/src/rtd", "specify the root directory of rtd files and executables.")
		serverPort := flag.Int("reports_port", 0, "specify the port number to start Reports server.")
		flag.Parse()

		if *version {
			fmt.Printf("tast_rtd version %s\n", Version)
			return 0
		}

		logFile, err := createLogFile()
		if err != nil {
			log.Fatalf("Failed to create log file: %v", err)
		}
		defer logFile.Close()

		logger := newLogger(logFile)

		logger.Println("Input File:", *input)

		inv, err := readInput(*input)
		if err != nil {
			logger.Printf("Failed to read invocation protobuf file %v: %v", *input, err)
			return 1
		}

		testsToRequests := make(map[string]string)
		for _, request := range inv.Requests {
			testsToRequests[request.Test] = request.Name
		}

		// Set up connection with ProgressSink
		psAddr := fmt.Sprintf(":%d", inv.GetProgressSinkClientConfig().GetPort())
		conn, err := grpc.DialContext(ctx, psAddr, grpc.WithBlock(), grpc.WithInsecure())
		if err != nil {
			logger.Printf("Failed to connect to progress sink %v: %v", psAddr, err)
			return 1
		}
		defer conn.Close()
		psClient := rtd.NewProgressSinkClient(conn)

		srv, err := rpc.NewReportsServer(*serverPort, psClient, testsToRequests)
		if err != nil {
			log.Fatalf("Failed to start Reports server: %v", err)
		}
		defer srv.Stop()

		if _, err := invokeTast(logger, inv, *rtdPath); err != nil {
			logger.Printf("Failed to invoke tast: %v", err)
			return 1
		}

		if err := srv.SendMissingTestsReports(ctx); err != nil {
			logger.Printf("Failed to send reports for missing tests.")
			return 1
		}

		return 0
	}())
}
