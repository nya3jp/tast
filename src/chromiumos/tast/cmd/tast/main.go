// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast executable, used to build and run tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/subcommands"
	"golang.org/x/crypto/ssh/terminal"

	"chromiumos/tast/cmd/tast/internal/logging"
)

const (
	signalChannelSize = 3 // capacity of channel used to intercept signals
)

// Version is the version info of this command. It is filled in during emerge.
var Version = "<unknown>"

// newLogger creates a logging.Logger based on the supplied command-line flags.
func newLogger(verbose, logTime bool) logging.Logger {
	return logging.NewSimple(os.Stdout, logTime, verbose)
}

// installSignalHandler starts a goroutine that attempts to do some minimal
// cleanup when the process is being terminated by a signal (which prevents
// deferred functions from running).
func installSignalHandler(lg logging.Logger) {
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
	subcommands.Register(newListCmd(os.Stdout, trunkDir()), "")
	subcommands.Register(newRunCmd(trunkDir()), "")
	subcommands.Register(&symbolizeCmd{}, "")

	version := flag.Bool("version", false, "print version and exit")
	verbose := flag.Bool("verbose", false, "use verbose logging")
	logTime := flag.Bool("logtime", true, "include date/time headers in logs")
	flag.Parse()

	if *version {
		fmt.Printf("tast version %s\n", Version)
		return 0
	}

	lg := newLogger(*verbose, *logTime)
	defer lg.Close()
	ctx := logging.NewContext(context.Background(), lg)

	installSignalHandler(lg)

	return int(subcommands.Execute(ctx))
}

func main() {
	os.Exit(doMain())
}
