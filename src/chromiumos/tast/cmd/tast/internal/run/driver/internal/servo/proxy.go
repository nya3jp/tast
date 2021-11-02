// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/client"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/ssh"
	"chromiumos/tast/ssh/linuxssh"
	"chromiumos/tast/testing"
)

const proxyTimeout = 10 * time.Second // max time for establishing SSH connection

// Proxy wraps a Servo object and forwards connections to the servod instance
// over SSH if needed.
type Proxy struct {
	svo  *Servo
	hst  *ssh.Conn      // nil if servod is running locally
	fwd  *ssh.Forwarder // nil if servod is running locally or inside a docker container
	port int
	dcl  *client.Client // nil if servod is not running inside a docker container
	sdc  string         // empty if servod is not running inside a docker container
}

func isDockerHost(host string) bool {
	return strings.HasSuffix(host, "docker_servod")
}

func splitHostPort(servoHostPort string) (string, int, int, error) {
	host := "localhost"
	port := 9999
	sshPort := 22

	if strings.Contains(servoHostPort, "docker_servod") {
		hostInfo := strings.Split(servoHostPort, ":")
		return hostInfo[0], port, sshPort, nil
	}

	hostport := servoHostPort
	if strings.HasSuffix(hostport, ":nossh") {
		sshPort = 0
		hostport = strings.TrimSuffix(hostport, ":nossh")
	}
	sshParts := strings.SplitN(hostport, ":ssh:", 2)
	if len(sshParts) == 2 {
		hostport = sshParts[0]
		var err error
		if sshPort, err = strconv.Atoi(sshParts[1]); err != nil {
			return "", 0, 0, errors.Wrap(err, "parsing servo host ssh port")
		}
		if sshPort <= 0 {
			return "", 0, 0, errors.New("invalid servo host ssh port")
		}
	}

	// The port starts after the last colon.
	i := strings.LastIndexByte(hostport, ':')
	if i >= 0 {
		if hostport[0] == '[' {
			// Expect the first ']' just before the last ':'.
			end := strings.IndexByte(hostport, ']')
			if end < 0 {
				return "", 0, 0, errors.New("missing ']' in address")
			}
			switch end + 1 {
			case len(hostport): // No port
				if hostport[1:end] != "" {
					host = hostport[1:end]
				}
				return host, port, sshPort, nil
			case i: // ] before :
				if hostport[1:end] != "" {
					host = hostport[1:end]
				}
			default:
				return "", 0, 0, errors.New("servo arg must be of the form hostname:9999 or hostname:9999:ssh:22 or [::1]:9999")
			}
		} else {
			if hostport[:i] != "" {
				host = hostport[:i]
			}
			if strings.IndexByte(host, ':') >= 0 {
				return "", 0, 0, errors.New("unexpected colon in hostname")
			}
		}
		var err error
		if port, err = strconv.Atoi(hostport[i+1:]); err != nil {
			return "", 0, 0, errors.Wrap(err, "parsing servo port")
		}
		if port <= 0 {
			return "", 0, 0, errors.New("invalid servo port")
		}
	} else if hostport != "" {
		host = hostport
	}
	return host, port, sshPort, nil
}

// NewProxy returns a Proxy object for communicating with the servod instance at spec,
// which can be blank (defaults to localhost:9999:ssh:22) or a hostname (defaults to hostname:9999:ssh:22)
// or a host:port (ssh port defaults to 22) or to fully qualify everything host:port:ssh:sshport.
//
// Use hostname:9999:nossh to prevent the use of ssh at all. You probably don't ever want to use this.
//
// You can also use IPv4 addresses as the hostnames, or IPv6 addresses in square brackets [::1].
//
// If you are using ssh port forwarding, please note that the host and ssh port will be evaluated locally,
// but the servo port should be the real servo port on the servo host.
// So if you used the ssh command `ssh -L 2223:localhost:22 -L 2222:${DUT_HOSTNAME?}:22 root@${SERVO_HOSTNAME?}`
// then you would start tast with `tast run --var=servo=localhost:${SERVO_PORT?}:ssh:2223 localhost:2222 firmware.Config*`
//
// If the instance is not running on the local system, an SSH connection will be opened
// to the host running servod and servod connections will be forwarded through it.
// keyFile and keyDir are used for establishing the SSH connection and should
// typically come from dut.DUT's KeyFile and KeyDir methods.
//
// If the servod is running in a docker container, the serverHostPort expected to be in form "${CONTAINER_NAME}:9999:docker:".
// The port of the servod host is defaulted to 9999, user only needs to provide the container name.
// CONTAINER_NAME must end with docker_servod.
func NewProxy(ctx context.Context, servoHostPort, keyFile, keyDir string) (newProxy *Proxy, retErr error) {
	var pxy Proxy
	toClose := &pxy
	defer func() {
		if toClose != nil {
			toClose.Close(ctx)
		}
	}()

	host, port, sshPort, err := splitHostPort(servoHostPort)
	if err != nil {
		return nil, err
	}
	pxy.port = port
	// If the servod instance isn't running locally, assume that we need to connect to it via SSH.
	if sshPort > 0 && !isDockerHost(host) && ((host != "localhost" && host != "127.0.0.1" && host != "::1") || sshPort != 22) {
		// First, create an SSH connection to the remote system running servod.
		sopt := ssh.Options{
			KeyFile:        keyFile,
			KeyDir:         keyDir,
			ConnectTimeout: proxyTimeout,
			WarnFunc:       func(msg string) { logging.Info(ctx, msg) },
			Hostname:       net.JoinHostPort(host, fmt.Sprint(sshPort)),
			User:           "root",
		}
		logging.Infof(ctx, "Opening Servo SSH connection to %s", sopt.Hostname)
		var err error
		if pxy.hst, err = ssh.New(ctx, &sopt); err != nil {
			return nil, err
		}

		defer func() {
			if retErr != nil {
				logServoStatus(ctx, pxy.hst, port)
			}
		}()

		logging.Infof(ctx, "Attempt to start servod at port %d at servo host %s:%d just in case that it has not been started", port, host, sshPort)
		output, err := pxy.hst.CommandContext(ctx, "start", "servod", fmt.Sprintf("PORT=%d", port)).Output()
		if err != nil {
			logging.Infof(ctx, "Warning in running servod: %s: %v", string(output), err)
		} else {
			logging.Infof(ctx, "Started servod at port %d", port)
			// Sleep for a minute to make sure servod is ready.
			testing.Sleep(ctx, time.Minute)
		}

		// Next, forward a local port over the SSH connection to the servod port.
		logging.Info(ctx, "Creating forwarded connection to port ", port)
		pxy.fwd, err = pxy.hst.NewForwarder("localhost:0", fmt.Sprintf("localhost:%d", port),
			func(err error) { logging.Info(ctx, "Got servo forwarding error: ", err) })
		if err != nil {
			return nil, err
		}
		var portstr string
		if host, portstr, err = net.SplitHostPort(pxy.fwd.ListenAddr().String()); err != nil {
			return nil, err
		}
		if port, err = strconv.Atoi(portstr); err != nil {
			return nil, errors.Wrap(err, "parsing forwarded servo port")
		}
	}

	logging.Infof(ctx, "Connecting to servod at %s:%d", host, port)
	pxy.svo, err = New(ctx, host, port)
	if err != nil {
		return nil, err
	}
	if strings.Contains(host, "docker_servod") {
		pxy.dcl, err = client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			return nil, err
		}
		pxy.sdc = host
	}
	toClose = nil // disarm cleanup
	return &pxy, nil
}

// logServoStatus logs the current servo status from the servo host.
func logServoStatus(ctx context.Context, hst *ssh.Conn, port int) {
	// Check if servod is running of the servo host.
	out, err := hst.CommandContext(ctx, "servodtool", "instance", "show", "-p", fmt.Sprint(port)).CombinedOutput()
	if err != nil {
		logging.Infof(ctx, "Servod process is not initialized on the servo-host: %v: %v", err, string(out))
		return
	}
	logging.Infof(ctx, "Servod instance is running on port %v of the servo host", port)
	// Check if servod is busy.
	if out, err = hst.CommandContext(ctx, "dut-control", "-p", fmt.Sprint(port), "serialname").CombinedOutput(); err != nil {
		logging.Infof(ctx, "The servod is not responsive or busy: %v: %v", err, string(out))
		return
	}
	logging.Info(ctx, "Servod is responsive on the host and can provide information about serialname: ", string(out))
}

// Close closes the proxy's SSH connection if present.
func (p *Proxy) Close(ctx context.Context) {
	logging.Info(ctx, "Closing Servo Proxy")
	if p.svo != nil {
		p.svo.Close(ctx)
		p.svo = nil
	}
	if p.fwd != nil {
		p.fwd.Close()
		p.fwd = nil
	}
	if p.hst != nil {
		p.hst.Close(ctx)
		p.hst = nil
	}
	if p.dcl != nil {
		p.dcl.Close()
		p.dcl = nil
	}
}

func (p *Proxy) isLocal() bool {
	return p.hst == nil || p.isDockerized()
}

func (p *Proxy) isClosed() bool {
	return p.svo == nil
}

func (p *Proxy) isDockerized() bool {
	return p.sdc != ""
}

// Servo returns the proxy's encapsulated Servo object.
func (p *Proxy) Servo() *Servo { return p.svo }

func (p *Proxy) runCommandImpl(ctx context.Context, dumpLogOnError, asRoot bool, name string, args ...string) error {
	var sshOpts []ssh.RunOption
	if dumpLogOnError {
		sshOpts = append(sshOpts, ssh.DumpLogOnError)
	}
	if p.isClosed() {
		return errors.New("connection to servo is closed")
	}
	if p.isLocal() {
		return errors.New("local servo is not supported")
	}
	return p.hst.CommandContext(ctx, name, args...).Run(sshOpts...)
}

// RunCommand execs a command on the servo host, optionally as root.
func (p *Proxy) RunCommand(ctx context.Context, asRoot bool, name string, args ...string) error {
	return p.runCommandImpl(ctx /*dumpLogOnError=*/, true, asRoot, name, args...)
}

// RunCommandQuiet execs a command on the servo host, optionally as root, does not log output.
func (p *Proxy) RunCommandQuiet(ctx context.Context, asRoot bool, name string, args ...string) error {
	return p.runCommandImpl(ctx /*dumpLogOnError=*/, false, asRoot, name, args...)
}

// OutputCommand execs a command as the root user and returns stdout.
func (p *Proxy) OutputCommand(ctx context.Context, asRoot bool, name string, args ...string) ([]byte, error) {
	if p.isClosed() {
		return nil, errors.New("connection to servo is closed")
	}
	if p.isLocal() {
		return nil, errors.New("local servo is not supported")
	}
	return p.hst.CommandContext(ctx, name, args...).Output(ssh.DumpLogOnError)
}

// InputCommand execs a command and redirects stdin.
func (p *Proxy) InputCommand(ctx context.Context, asRoot bool, stdin io.Reader, name string, args ...string) error {
	if p.isClosed() {
		return errors.New("connection to servo is closed")
	}
	if p.isLocal() {
		return errors.New("local servo is not supported")
	}
	cmd := p.hst.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	return cmd.Run(ssh.DumpLogOnError)
}

// GetFile copies a servo host file to a local file.
func (p *Proxy) GetFile(ctx context.Context, asRoot bool, remoteFile, localFile string) error {
	if p.isClosed() {
		return errors.New("connection to servo is closed")
	}
	if p.isLocal() {
		return errors.New("local servo is not supported")
	}
	return linuxssh.GetFile(ctx, p.hst, remoteFile, localFile, linuxssh.DereferenceSymlinks)
}

// PutFiles copies a local file to a servo host file.
func (p *Proxy) PutFiles(ctx context.Context, asRoot bool, fileMap map[string]string) error {
	if p.isClosed() {
		return errors.New("connection to servo is closed")
	}
	if p.isLocal() {
		return errors.New("local servo is not supported")
	}
	_, err := linuxssh.PutFiles(ctx, p.hst, fileMap, linuxssh.DereferenceSymlinks)
	return err
}

// GetPort returns the port where servod is running on the server.
func (p *Proxy) GetPort() int { return p.port }

// Proxied returns true if the servo host is connected via ssh proxy.
func (p *Proxy) Proxied() bool {
	return p.hst != nil
}

// NewForwarder forwards a local port to a remote port on the servo host.
func (p *Proxy) NewForwarder(ctx context.Context, hostPort string) (*ssh.Forwarder, error) {
	if !p.Proxied() {
		return nil, errors.New("servo host is not proxied via ssh")
	}
	fwd, err := p.hst.NewForwarder("localhost:0", hostPort,
		func(err error) { logging.Info(ctx, "Got forwarding error: ", err) })
	if err != nil {
		return nil, errors.Wrap(err, "creating ssh forwarder")
	}
	return fwd, nil
}
