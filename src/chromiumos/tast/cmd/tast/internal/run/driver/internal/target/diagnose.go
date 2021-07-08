// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/ssh"
)

// diagnoseNetwork runs network diagnosis to debug SSH connection issues.
// It returns a message string describing possible network problems if it finds
// any.
func diagnoseNetwork(ctx context.Context, target string) (string, error) {
	opts := &ssh.Options{}
	if err := ssh.ParseTarget(target, opts); err != nil {
		return "", err
	}
	host, port, err := net.SplitHostPort(opts.Hostname)
	if err != nil {
		return "", err
	}

	logging.Info(ctx, "Failed to connect to ", host)
	logging.Info(ctx, "Running network diagnosis...")

	if err := tryResolveDNS(ctx, host); err != nil {
		msg := fmt.Sprintf("%v", err)
		if errors.Is(err, context.DeadlineExceeded) {
			msg = fmt.Sprintf("%v (is the DNS down? see b/191721645)", err)
		}
		logging.Info(ctx, "* DNS resolution: FAIL: ", msg)
		return fmt.Sprintf("DNS resolution failed: %s", msg), nil
	}
	logging.Info(ctx, "* DNS resolution: OK")

	if err := tryPing(ctx, host); err != nil {
		msg := fmt.Sprintf("%v (is the DUT down?)", err)
		logging.Info(ctx, "* Ping: FAIL: ", msg)
		return fmt.Sprintf("ping failed: %s", msg), nil
	}
	logging.Info(ctx, "* Ping: OK")

	if err := tryRawConnect(ctx, host, port); err != nil {
		msg := fmt.Sprintf("%v (is the SSH daemon down?)", err)
		logging.Info(ctx, "* Connect: FAIL: ", msg)
		return fmt.Sprintf("connect failed: %s", msg), nil
	}
	logging.Info(ctx, "* Connect: OK")

	logging.Info(ctx, "No issue found on network diagnosis")
	return "", nil
}

func tryResolveDNS(ctx context.Context, host string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	return err
}

func tryPing(ctx context.Context, host string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-n", "-v", "-w", "5", host)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logging.Info(ctx, "Ping failed!")
		for _, line := range strings.Split(string(out), "\n") {
			logging.Info(ctx, line)
		}
	}
	return err
}

func tryRawConnect(ctx context.Context, host, port string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
