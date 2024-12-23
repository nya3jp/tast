// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast executable, used to build and run tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/google/subcommands"
	"golang.org/x/term"

	"go.chromium.org/tast/core/internal/command"
	"go.chromium.org/tast/core/internal/logging"
)

// Version is the version info of this command. It is filled in during emerge.
var Version = "<unknown>"

// newLogger creates a logging.Logger based on the supplied command-line flags.
func newLogger(verbose, logTime bool) *logging.SinkLogger {
	level := logging.LevelInfo
	if verbose {
		level = logging.LevelDebug
	}
	return logging.NewSinkLogger(level, logTime, logging.NewWriterSink(os.Stdout))
}

// installSignalHandler starts a goroutine that attempts to do some minimal
// cleanup when the process is being terminated by a signal (which prevents
// deferred functions from running).
func installSignalHandler(ctx context.Context) {
	var st *term.State
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		var err error
		if st, err = term.GetState(fd); err != nil {
			logging.Info(ctx, "Failed to get terminal state: ", err)
		}
	}

	command.InstallSignalHandler(os.Stderr, func(os.Signal) {
		if st != nil {
			term.Restore(fd, st)
		}
	})
}

// doMain implements the main body of the program. It's a separate function so
// that its deferred functions will run before os.Exit makes the program exit
// immediately.
func doMain() int {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(newListCmd(os.Stdout, trunkDir()), "")
	subcommands.Register(newRunCmd(trunkDir(), Version), "")
	subcommands.Register(&symbolizeCmd{}, "")
	subcommands.Register(newGlobalRuntimeVarsCmd(os.Stdout, trunkDir()), "")

	version := flag.Bool("version", false, "print version and exit")
	verbose := flag.Bool("verbose", false, "use verbose logging")
	logTime := flag.Bool("logtime", true, "include date/time headers in logs")
	flag.Parse()

	if *version {
		fmt.Printf("tast version %s\n", Version)
		return 0
	}

	logger := newLogger(*verbose, *logTime)
	ctx := logging.AttachLogger(context.Background(), logger)

	installSignalHandler(ctx)

	return int(subcommands.Execute(ctx))
}

func main() {
	os.Exit(doMain())
}
