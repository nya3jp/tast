// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package debugger provides the ability to start binaries under a debugger.
package debugger

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/ssh"
)

// A DebugTarget represents a go binary that can be debugged.
type DebugTarget string

// Valid DebugTargets are listed below.
const (
	LocalBundle      DebugTarget = "local"
	RemoteBundle     DebugTarget = "remote"
	LocalTestRunner  DebugTarget = "local-test-runner"
	RemoteTestRunner DebugTarget = "remote-test-runner"
)

// FindPreemptiveDebuggerErrors pre-emptively checks potential errors, to ensure better error messages for users.
func FindPreemptiveDebuggerErrors(port int) error {
	if port == 0 {
		return nil
	}
	// We want to change the error messages based on whether this is running on the DUT or the host.
	// We can't necessarily know in the caller because it doesn't distinguish between remote bundles and local bundles.
	lsb, err := ioutil.ReadFile("/etc/lsb-release")
	isDUT := err == nil && strings.Contains(string(lsb), "CHROMEOS_RELEASE_BOARD")

	if _, err := exec.LookPath("dlv"); err != nil {
		if runtime.GOARCH == "arm" {
			return errors.New("delve isn't supported for arm32 (https://github.com/go-delve/delve/issues/2051). If possible, try installing a 64 bit OS onto your dut (eg. hana64 instead of hana)")
		} else if isDUT {
			return errors.New(`dlv doesn't exist on your DUT. To install on supported architectures (x86, amd64, arm64), run "emerge-<board> dev-go/delve" and then cros deploy it`)
		} else {
			return errors.New(`dlv doesn't exist on your host machine. To install, run "sudo emerge dev-go/delve"`)
		}
	}

	machine := "host"
	if isDUT {
		machine = "DUT"
	}

	// If there is no debugger, then we'll return a pid of -1.
	getCurrentDebugger := func() (pid int, err error) {
		cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port))
		out, err := cmd.Output()
		// Status code 1 indicates no process found.
		if err != nil {
			return -1, nil
		}
		// The start of the line is process name, then PID.
		match := regexp.MustCompile(`(?m)^([^\s]+)\s*([0-9]+)\b`).FindStringSubmatch(string(out))

		pname := match[1]
		pid, err = strconv.Atoi(match[2])
		if err != nil {
			return 0, err
		}
		if pname != "dlv" {
			return 0, errors.Errorf("port %d in use by process %s with pid %d on %s machine", port, pname, pid, machine)
		}
		return pid, err
	}

	pid, err := getCurrentDebugger()
	if err != nil || pid == -1 {
		return err
	}

	// When you control-c an ongoing test, the test continues until completion.
	// Thus, the common scenario here may occur if we don't kill the debugger:
	// 1) Start a test.
	// 2) Before connecting to the debugger, you realise your code had a mistake.
	// 3) Fix your code.
	// 4) Control-C the current test, and rerun
	// 5) Since the test is considered started (the binary was executed), it runs
	//    until completion. Since it's waiting for a debugger, this will be until
	//    it times out.
	// 6) Tast attempts to start a debugger, but the port is already in use.
	// Since ensuring that the debugger is running correctly is within the scope
	// of tast, and not the end user, we should kill the process for them
	// (especially since finding the pid and killing it on a remote machine is a
	// pain).
	if err := unix.Kill(pid, unix.SIGKILL); err != nil {
		return errors.Wrapf(err, "port %d already in use by debugger on %s. Attempted to kill the existing debugger, but failed: ", port, machine)
	}
	// Unfortunately unix only allows you to wait on child processes, so we need to busy wait here.
	// Although this is an infinite loop, SIGKILL should ensure that the process cannot save itself.
	// If sigkill succeeds, the process will die in a timely manner.
	for {
		pid, err := getCurrentDebugger()
		if err != nil || pid == -1 {
			return err
		}
	}
}

// ForwardPort forwards a port from port to the ssh'd machine on the same port for the debugger.
// The existing SSHConn.ForwardLocalToRemote is unsuitable for our use case because it assumes
// that both channels will stop writing, and also because it attempts to accept multiple connections.
func ForwardPort(ctx context.Context, sshConn *ssh.Conn, port int) error {
	ctx, cancel := context.WithCancel(ctx)
	onError := func(err error) {
		logging.Infof(ctx, "Error while port forwarding: %s", err.Error())
		cancel()
	}

	localAddress := fmt.Sprintf(":%d", port)
	remoteAddress, err := sshConn.GenerateRemoteAddress(port)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", localAddress)
	if err != nil {
		return err
	}

	go func() {
		defer listener.Close()
		client, err := listener.Accept()
		if err != nil {
			onError(err)
			return
		}
		defer client.Close()

		server, err := sshConn.Dial("tcp", remoteAddress)
		if err != nil {
			onError(err)
			return
		}
		defer server.Close()

		ch := make(chan error)
		go func() {
			_, err := io.Copy(client, server)
			ch <- err
		}()
		go func() {
			_, err := io.Copy(server, client)
			ch <- err
		}()

		// When detaching a debugger, only the server -> client copy returns early,
		// so we only wait for one of them to close.
		if err := <-ch; err != nil {
			onError(err)
		}

	}()
	return nil
}

// RewriteDebugCommand takes a go binary and a set of arguments it takes,
// and if a debug port was provided, rewrites it as a command that instead
// runs a debugger and waits on that port before running the binary.
func RewriteDebugCommand(debugPort int, cmd string, args ...string) (newCmd string, newArgs []string) {
	if debugPort == 0 {
		return cmd, args
	}
	var prefix []string
	var binary string
	var binaryArgs []string
	if cmd == "env" {
		prefix = append(prefix, "env")
		for i, arg := range args {
			if !strings.Contains(arg, "=") {
				prefix = append(prefix, args[:i]...)
				binary = args[i]
				binaryArgs = args[i+1:]
			}
		}
	} else {
		binary = cmd
		binaryArgs = args
	}

	// Tast uses stdout to interact with the binary. Remove all delve output to
	// stdout with --log-dest=/dev/null, since critical things go to stderr anyway.
	result := append(append(prefix,
		[]string{"dlv", "exec", binary,
			"--api-version=2",
			"--headless",
			fmt.Sprintf("--listen=:%d", debugPort),
			"--log-dest=/dev/null",
			"--"}...), binaryArgs...)

	return result[0], result[1:]
}

// PrintWaitingMessage outputs a "Waiting for debugger" message, if required.
func PrintWaitingMessage(ctx context.Context, debugPort int) {
	if debugPort != 0 {
		logging.Infof(ctx, "Waiting for debugger on port %d", debugPort)
	}
}
