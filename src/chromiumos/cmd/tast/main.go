// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast executable, used to build and run tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"chromiumos/cmd/tast/logging"

	"github.com/google/subcommands"

	"golang.org/x/crypto/ssh/terminal"
)

const (
	signalChannelSize = 3  // capacity of channel used to intercept signals
	fancyVerboseLines = 30 // verbose log lines to display in "fancy" mode
)

var Version = "<unknown>" // Version info is filled in during emerge.

// lg should be used throughout the tast executable to log informative messages.
var lg logging.Logger

// newLogger creates a logging.Logger based on the supplied command-line flags.
func newLogger(fancy, verbose, logTime bool) (logging.Logger, error) {
	if fancy {
		l, err := logging.NewFancy(fancyVerboseLines)
		if err != nil {
			err = fmt.Errorf("-fancy unsupported: %v", err)
		}
		return l, err
	}

	var flags int
	if logTime {
		flags = log.LstdFlags
	}
	return logging.NewSimple(os.Stdout, flags, verbose), nil
}

// installSignalHandler starts a goroutine that attempts to do some minimal
// cleanup when the process is being terminated by a signal (which prevents
// deferred functions from running).
func installSignalHandler() {
	var st *terminal.State
	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		var err error
		if st, err = terminal.GetState(fd); err != nil {
			lg.Log("Failed to get terminal state: ", err)
		}
	}

	sc := make(chan os.Signal, signalChannelSize)
	go func() {
		for sig := range sc {
			lg.Close()
			if st != nil {
				terminal.Restore(fd, st)
			}
			fmt.Fprintf(os.Stdout, "\nCaught %v signal; exiting\n", sig)
			os.Exit(1)
		}
	}()
	signal.Notify(sc, syscall.SIGINT, syscall.SIGKILL)
}

// doMain implements the main body of the program. It's a separate function so
// that its deferred functions will run before os.Exit makes the program exit
// immediately.
func doMain() int {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&buildCmd{}, "")
	subcommands.Register(newListCmd(), "")
	subcommands.Register(newRunCmd(), "")
	subcommands.Register(&symbolizeCmd{}, "")

	version := flag.Bool("version", false, "print version and exit")
	fancy := flag.Bool("fancy", false, "use fancy logging")
	verbose := flag.Bool("verbose", false, "use verbose logging")
	logTime := flag.Bool("logtime", true, "include date/time headers in logs")
	flag.Parse()

	if *version {
		fmt.Printf("tast version %s\n", Version)
		return 0
	}

	var err error
	if lg, err = newLogger(*fancy, *verbose, *logTime); err != nil {
		log.Fatal("Failed to initialize logging: ", err)
	}
	defer lg.Close()

	installSignalHandler()

	return int(subcommands.Execute(context.Background()))
}

func main() {
	os.Exit(doMain())
}
