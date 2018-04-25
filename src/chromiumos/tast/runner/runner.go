// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"chromiumos/tast/bundle"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
)

// TODO(derat): Split this into separate args.go and run.go files.

const (
	statusSuccess      = 0 // runner was successful
	statusError        = 1 // unspecified error was encountered
	statusBadArgs      = 2 // bad arguments were passed to the runner
	statusNoBundles    = 3 // glob passed to runner didn't match any bundles
	statusNoTests      = 4 // pattern(s) passed to runner didn't match any tests
	statusBundleFailed = 5 // test bundle exited with nonzero status
)

// logger is used to write messages to stdout when the runner is executed manually
// rather than by the tast command.
var logger *log.Logger = log.New(os.Stdout, "", log.LstdFlags)

// writeError writes a RunError control message to mw if non-nil or writes the message
// directly to stderr otherwise. After calling this function, the runner should pass
// the returned status code (which may or may not be equal to the status arg) to os.Exit.
func writeError(mw *control.MessageWriter, msg string, status int) int {
	if mw == nil {
		fmt.Fprintln(os.Stderr, msg)
		return status
	}

	_, fn, ln, _ := runtime.Caller(1)
	mw.WriteMessage(&control.RunError{Time: time.Now(), Error: testing.Error{
		Reason: msg,
		File:   fn,
		Line:   ln,
		Stack:  string(debug.Stack()),
	}})
	// Exit with success when reporting progress via control messages.
	// The tast command will know that the run failed because of the RunError message.
	return statusSuccess
}

// runBundle runs the bundle at path to completion, passing args.
// The bundle's stdout is copied to the stdout arg.
func runBundle(path string, args *bundle.Args, stdout io.Writer) error {
	cmd := exec.Command(path)
	cmd.Stdout = stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	// Save stderr so we can return it to aid in debugging.
	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr

	if err = cmd.Start(); err != nil {
		return err
	}

	jerr := json.NewEncoder(stdin).Encode(args)
	stdin.Close()
	err = cmd.Wait()

	if jerr != nil {
		return jerr
	}
	if err != nil {
		// Pass back stderr if the bundle wrote anything to it.
		if msg := strings.TrimSpace(stderr.String()); len(msg) > 0 {
			return fmt.Errorf("%v (%v)", err, msg)
		}
		return err
	}
	return nil
}

// RunTests runs tests across multiple bundles as described by cfg.
// The returned status code should be passed to os.Exit.
func RunTests(cfg *RunConfig) int {
	if len(cfg.tests) == 0 {
		// If the runner was executed manually, report an error if no tests were matched.
		if cfg.mw == nil {
			return writeError(nil, fmt.Sprintf("No tests matched by %v", cfg.bundleArgs.Patterns), statusNoTests)
		}

		// Otherwise, just report an empty run. It's expected to not match any tests if
		// both local and remote tests are being run but the user specified a pattern that
		// matched only local or only remote tests rather than tests of both types.
		cfg.mw.WriteMessage(&control.RunStart{Time: time.Now()})
		cfg.mw.WriteMessage(&control.RunEnd{Time: time.Now()})
		return statusSuccess
	}

	if cfg.bundleArgs.OutDir == "" {
		var err error
		if cfg.bundleArgs.OutDir, err = ioutil.TempDir("", "tast_out."); err != nil {
			return writeError(cfg.mw, fmt.Sprintf("Failed to create out dir: %v", err), statusError)
		}
		// If we were run by the tast command, it should clean up the output dir after it copies it over.
		// Otherwise, we should clean it up ourselves.
		if cfg.mw == nil {
			defer os.RemoveAll(cfg.bundleArgs.OutDir)
		}
	}

	if cfg.mw != nil {
		cfg.mw.WriteMessage(&control.RunStart{Time: time.Now(), NumTests: len(cfg.tests)})
	}

	// Execute bundles serially to run tests.
	cfg.bundleArgs.Mode = bundle.RunTestsMode
	for _, bundle := range cfg.bundles {
		if err := runBundleTests(bundle, cfg); err != nil {
			return writeError(cfg.mw, fmt.Sprintf("Bundle %v failed: %v", bundle, err), statusBundleFailed)
		}
	}

	if cfg.mw != nil {
		cfg.mw.WriteMessage(&control.RunEnd{Time: time.Now(), OutDir: cfg.bundleArgs.OutDir})
	}

	return statusSuccess
}

// runBundleTests executes tests in the bundle at path bundle as instructed by cfg.
func runBundleTests(bundle string, cfg *RunConfig) error {
	// When we were run by the tast command, just copy stdout over directly.
	if cfg.mw != nil {
		return runBundle(bundle, &cfg.bundleArgs, cfg.stdout)
	}

	// When we were run manually, read control messages from stdout so we can log
	// them in a human-readable form.

	// First, start a goroutine to log messages as they're produced by the bundle.
	pr, pw := io.Pipe()
	ch := make(chan error, 1)
	go func() { ch <- logBundleOutputForManualRun(pr) }()

	// Run the bundle to completion, copying its output to the goroutine over the pipe.
	if err := runBundle(bundle, &cfg.bundleArgs, pw); err != nil {
		pw.Close()
		return err
	}
	pw.Close()

	// Finally, wait for the goroutine to finish and return its result.
	return <-ch
}

// logBundleOutputForManualRun reads control messages from src and logs them to stdout.
// It is used to print human-readable test output when the runner is executed manually rather
// than via the tast command. An error is returned if any TestError messages are read from src.
func logBundleOutputForManualRun(src io.Reader) error {
	failed := false
	mr := control.NewMessageReader(src)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			return err
		}
		switch v := msg.(type) {
		case *control.TestStart:
			logger.Print("Running ", v.Test.Name)
		case *control.TestLog:
			logger.Print(v.Text)
		case *control.TestError:
			logger.Printf("Error: [%s:%d] %v", v.Error.File, v.Error.Line, v.Error.Reason)
			failed = true
		case *control.TestEnd:
			logger.Print("Finished ", v.Name)
		}
	}

	if failed {
		return errors.New("Test(s) failed")
	}
	return nil
}
