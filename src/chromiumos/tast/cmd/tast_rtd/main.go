// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast_rtd executable, used to invoke tast in RTD.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

// Version is the version info of this command. It is filled in during emerge.
var Version = "<unknown>"

// newLogger creates a logger.
func newLogger() (*log.Logger, error) {
	const fullPath = "/tmp/tast_rtd/"
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %v: %v", fullPath, err)
	}
	t := time.Now()
	fullLogName := fmt.Sprintf("%v/tast_rtd_%v.log", fullPath, t.Format("20060102150405"))

	// Log the full output of the command to disk.
	fullLog, err := os.Create(fullLogName)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file %v: %v", fullPath, err)
	}
	return log.New(fullLog, "", log.LstdFlags), nil

}

// readInput reads an invocation protobuf file and returns a pointer to rtd.Invocation.
// TODO: Populate rtd.Invocation.
func readInput(fileName string) (*rtd.Invocation, error) {
	_, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("fail to read file %v: %v", fileName, err)
	}
	return nil, nil
}

// doMain implements the main body of the program.
func doMain() int {

	version := flag.Bool("version", false, "print version and exit")
	input := flag.String("input", "", "specify the test invocation request protobuf input file")
	flag.Parse()

	if *version {
		fmt.Printf("tast_rtd version %s\n", Version)
		return 0
	}

	logger, err := newLogger()
	if err != nil {
		log.Fatalf("Failed to create log file: %v", err)
	}

	logger.Println("Input File: ", *input)

	_, err = readInput(*input)
	if err != nil {
		log.Fatalf("Failed to read invocation protobuf file %v: %v", *input, err)
	}

	return 0
}

func main() {
	os.Exit(doMain())
}
