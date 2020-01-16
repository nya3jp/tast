// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arc supports interacting with the ARC framework, which is used to run Android applications on Chrome OS.
package arc

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/process"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/testexec"
	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

const (
	// BootTimeout is the maximum amount of time that ARC is expected to take to boot.
	// Tests that call New should declare a timeout that's at least this long.
	BootTimeout = 120 * time.Second

	intentHelperTimeout = 20 * time.Second

	logcatName = "logcat.txt"

	//ARCPath is the path where the container images are installed in the rootfs.
	ARCPath = "/opt/google/containers/android"
	//ARCVMPath is the pather where the VM images are installed in the rootfs.
	ARCVMPath = "/opt/google/vms/android"
)

// InstallType is the type of ARC (Container or VM) available on the device.
type InstallType int

const (
	// Container is for the ARC container install.
	Container InstallType = iota
	// VM is for the ARCVM install.
	VM
)

// locked is set to true while a precondition is active to prevent tests from calling New or Close.
var locked = false

// TODO(b/134144418): Consolidate ARC and ARCVM diverged code once ADB issues are resolved.

// Supported returns true if ARC is supported on the board.
//
// This function must not be used to skip tests entirely; declare the "android"
// software dependency instead. A valid use case would be to change the test
// expectation by whether ARC is supported or not (e.g. existence of mount
// points).
func Supported() bool {
	_, ok := Type()
	return ok
}

// Type detects the type (container or VM) of the ARC installation. As for
// Supported(), it should not be used to skip tests entirely, both fall under
// the "android" software dependency. But it could be used to change the
// behaviour of a test (e.g. check that ARCVM is running or not).
func Type() (t InstallType, ok bool) {
	if _, err := os.Stat(filepath.Join(ARCPath, "system.raw.img")); err == nil {
		return Container, true
	}
	if _, err := os.Stat(filepath.Join(ARCVMPath, "system.raw.img")); err == nil {
		return VM, true
	}
	return 0, false
}

// ARC holds resources related to an active ARC session. Call Close to release
// those resources.
type ARC struct {
	logcatCmd    *testexec.Cmd // process saving Android logs
	logcatWriter dynamicWriter // writes output from logcatCmd to logcatFile
	logcatFile   *os.File      // file currently being written to
}

// Close releases testing-related resources associated with ARC.
// ARC itself is not stopped.
func (a *ARC) Close() error {
	if locked {
		panic("Do not call Close while precondition is being used")
	}
	var err error
	if a.logcatCmd != nil {
		a.logcatCmd.Kill()
		a.logcatCmd.Wait()
	}
	if a.logcatFile != nil {
		a.logcatWriter.setDest(nil)
		err = a.logcatFile.Close()
	}
	return err
}

// New waits for Android to finish booting.
//
// ARC must be enabled in advance by passing chrome.ARCEnabled or chrome.ARCSupported with
// real user gaia login to chrome.New.
//
// After this function returns successfully, you can assume BOOT_COMPLETED
// intent has been broadcast from Android system, and ADB connection is ready.
// Note that this does not necessarily mean all ARC mojo services are up; call
// WaitIntentHelper() to wait for ArcIntentHelper to be ready, for example.
//
// The returned ARC instance must be closed when the test is finished.
func New(ctx context.Context, outDir string) (*ARC, error) {
	ctx, st := timing.Start(ctx, "arc_new")
	defer st.End()

	if locked {
		panic("Cannot create ARC instance while precondition is being used")
	}

	ctx, cancel := context.WithTimeout(ctx, BootTimeout)
	defer cancel()

	if err := checkSoftwareDeps(ctx); err != nil {
		return nil, err
	}

	if err := ensureARCEnabled(); err != nil {
		return nil, err
	}

	arc := &ARC{}
	toClose := arc
	defer func() {
		if toClose != nil {
			toClose.Close()
		}
	}()

	vm, err := VMEnabled()
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine if ARCVM is enabled")
	}

	testing.ContextLog(ctx, "Waiting for Android boot")

	if vm {
		// When running ARCVM, 'android-sh' runs via ADB. So, the first thing to do is set up ADB.
		// Android should be initialized once a working connection to ADB is made.
		testing.ContextLog(ctx, "Setting up ADB")
		if err := setUpADB(ctx); err != nil {
			return nil, errors.Wrap(err, "failed setting up ADB")
		}

		// Connect to ADB.
		if err := connectADB(ctx); err != nil {
			return nil, errors.Wrap(err, "failed connecting to ADB")
		}

	} else {
		if err := WaitAndroidInit(ctx); err != nil {
			return nil, errors.Wrap(err, "Android failed to boot in very early stage")
		}
	}

	// At this point we can start logcat.
	logcatPath := filepath.Join(outDir, logcatName)
	if err := arc.setLogcatFile(logcatPath); err != nil {
		return nil, errors.Wrap(err, "failed to create logcat output file")
	}
	logcatCmd, err := startLogcat(ctx, &arc.logcatWriter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start logcat")
	}
	arc.logcatCmd = logcatCmd

	if !vm {
		// Wait for internal networking to get ready. This gives better error messages
		// when networking is broken, rather than obscure "failed connecting to ADB" error.
		if err := waitNetworking(ctx); err != nil {
			return nil, diagnose(logcatPath, errors.Wrap(err, "Android network unreachable"))
		}
	}

	// This property is set by the Android system server just before LOCKED_BOOT_COMPLETED is broadcast.
	const androidBootProp = "sys.boot_completed"
	if err := waitProp(ctx, androidBootProp, "1", reportTiming); err != nil {
		return nil, diagnose(logcatPath, errors.Wrapf(err, "%s not set", androidBootProp))
	}

	var ch chan error
	if !vm {
		// Android container is up. Set up ADB auth in parallel to Android boot since
		// ADB local server takes a few seconds to start up.
		testing.ContextLog(ctx, "Setting up ADB auth")
		ch = make(chan error, 1)
		go func() {
			ch <- setUpADBAuth(ctx)
		}()
	}

	// This property is set by ArcAppLauncher when it receives BOOT_COMPLETED.
	const arcBootProp = "ro.arc.boot_completed"
	if err := waitProp(ctx, arcBootProp, "1", reportTiming); err != nil {
		return nil, diagnose(logcatPath, errors.Wrapf(err, "%s not set", arcBootProp))
	}

	if !vm {
		// Android has booted.
		if err := <-ch; err != nil {
			return nil, diagnose(logcatPath, errors.Wrap(err, "failed setting up ADB auth"))
		}

		// Connect to ADB.
		if err := connectADB(ctx); err != nil {
			return nil, diagnose(logcatPath, errors.Wrap(err, "failed connecting to ADB"))
		}
	}

	toClose = nil
	return arc, nil
}

// WaitIntentHelper waits for ArcIntentHelper to get ready.
func (a *ARC) WaitIntentHelper(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, intentHelperTimeout)
	defer cancel()

	testing.ContextLog(ctx, "Waiting for ArcIntentHelper")
	const prop = "ro.arc.intent_helper.ready"
	if err := waitProp(ctx, prop, "1", reportTiming); err != nil {
		return errors.Wrapf(err, "property %s not set", prop)
	}
	return nil
}

// androidDeps contains Android-related software features (see testing.Test.SoftwareDeps).
// At least one of them must be declared to call New.
var androidDeps = []string{
	"android",
	"android_vm",
	"android_both",
	"android_all",
	"android_all_both",
	"android_p",
	"android_p_both",
	"android_r",
}

// checkSoftwareDeps ensures the current test declares Android software dependencies.
func checkSoftwareDeps(ctx context.Context) error {
	deps, ok := testing.ContextSoftwareDeps(ctx)
	if !ok {
		// Test info can be unavailable in unit tests.
		return nil
	}

	for _, dep := range deps {
		for _, adep := range androidDeps {
			if dep == adep {
				return nil
			}
		}
	}
	return errors.Errorf("test must declare at least one of Android software dependencies %v", androidDeps)
}

// setLogcatFile creates a new logcat output file at p and opens it as a.logcatFile.
// a.logcatWriter is updated to write to the new file, and explanatory messages are
// written to both the new file and old file (if there was a previous file).
func (a *ARC) setLogcatFile(p string) error {
	oldFile := a.logcatFile

	var createErr error
	a.logcatFile, createErr = os.Create(p)
	if createErr == nil && oldFile != nil {
		// Make the new file start with a line pointing at the old file.
		if rel, err := filepath.Rel(filepath.Dir(a.logcatFile.Name()), oldFile.Name()); err == nil {
			fmt.Fprintf(a.logcatFile, "[output continued from %v]\n", rel)
		}
	}
	// If the create failed, we'll just drop the new logs.
	a.logcatWriter.setDest(a.logcatFile)

	if oldFile != nil {
		if a.logcatFile != nil {
			// Make the old file end with a line pointing at the new file.
			if rel, err := filepath.Rel(filepath.Dir(oldFile.Name()), a.logcatFile.Name()); err == nil {
				fmt.Fprintf(oldFile, "[output continued in %v]\n", rel)
			}
		}
		oldFile.Close()
	}

	return createErr
}

// VMEnabled returns true if Chrome OS is running ARCVM.
// This is done by checking "/run/chrome/is_arcvm" content equal to "1".
// Reference: chrome/browser/chromeos/arc/arc_service_launcher.cc
func VMEnabled() (bool, error) {
	b, err := ioutil.ReadFile("/run/chrome/is_arcvm")
	if err != nil {
		return false, err
	}
	return string(b) == "1", nil
}

// ensureARCEnabled makes sure ARC is enabled by a command line flag to Chrome.
func ensureARCEnabled() error {
	args, err := getChromeArgs()
	if err != nil {
		return errors.Wrap(err, "failed getting Chrome args")
	}

	for _, a := range args {
		if a == "--arc-start-mode=always-start" || a == "--arc-start-mode=always-start-with-no-play-store" || a == "--arc-availability=officially-supported" {
			return nil
		}
	}
	return errors.New("ARC is not enabled; pass chrome.ARCEnabled or chrome.ARCSupported to chrome.New")
}

// getChromeArgs returns command line arguments of the Chrome browser process.
func getChromeArgs() ([]string, error) {
	pid, err := chrome.GetRootPID()
	if err != nil {
		return nil, err
	}
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil, err
	}
	return proc.CmdlineSlice()
}

// WaitAndroidInit waits for Android init process to start.
//
// It is very rare you want to call this function from your test; to wait for
// the Android system to start and become ready, call New instead. A valid use
// case is when you want to interact with Android mini container.
//
// It is fine to call BootstrapCommand after this function successfully returns.
func WaitAndroidInit(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, BootTimeout)
	defer cancel()

	// service.adb.tcp.port is an arbitrary property set by Android init very
	// early in boot process. Wait for it to ensure Android init process started.
	// Note that existence of this property has nothing to do with the status of
	// Android adb daemon.
	const prop = "service.adb.tcp.port"
	if err := waitProp(ctx, prop, "5555", reportTiming); err != nil {
		return errors.Wrapf(err, "Android container did not come up: %s not set", prop)
	}
	return nil
}

// startLogcat starts a logcat process with its stdout redirected to w.
func startLogcat(ctx context.Context, w io.Writer) (*testexec.Cmd, error) {
	// Wait for logd to start by polling logcat.
	cmd := BootstrapCommand(ctx, "/system/bin/sh", "-c", "while ! /system/bin/logcat -ds; do sleep 0.1; done")
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return nil, errors.Wrap(err, "logcat failed")
	}

	// The logcat process may need to span multiple tests if we're being used by a precondition,
	// so use context.Background instead of ctx to make sure it isn't killed prematurely.
	cmd = BootstrapCommand(context.Background(), "/system/bin/logcat") // NOLINT: process may need to persist across multiple tests
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// waitNetworking waits for the internal networking to get ready.
func waitNetworking(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "wait_networking")
	defer st.End()

	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := testexec.CommandContext(ctx, "ping", "-c1", "-w1", "-n", "100.115.92.2").Run(); err != nil {
			return errors.Wrap(err, "ping 100.115.92.2 failed")
		}
		return nil
	}, nil)
}

// timingMode describes whether timing information should be reported.
type timingMode int

const (
	reportTiming   timingMode = iota // create a timing stage
	noReportTiming                   // don't create a timing stage
)

// waitProp waits for Android prop name is set to value.
func waitProp(ctx context.Context, name, value string, tm timingMode) error {
	if tm == reportTiming {
		var st *timing.Stage
		ctx, st = timing.Start(ctx, fmt.Sprintf("wait_prop_%s=%s", name, value))
		defer st.End()
	}

	const loop = `while [ "$(/system/bin/getprop "$1")" != "$2" ]; do sleep 0.1; done`
	return testing.Poll(ctx, func(ctx context.Context) error {
		return BootstrapCommand(ctx, "/system/bin/sh", "-c", loop, "-", name, value).Run()
	}, &testing.PollOptions{Interval: time.Second})
}

// APKPath returns the absolute path to a helper APK.
func APKPath(value string) string {
	return filepath.Join("/usr/local/libexec/tast/apks/local/cros", value)
}
