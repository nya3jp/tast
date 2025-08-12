// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// DO NOT USE THIS COPY OF SERVO IN TESTS, USE THE ONE IN platform/tast-tests/src/go.chromium.org/tast-tests/cros/common/servo

package servo

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"google.golang.org/grpc"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/chromiumos/config/go/test/api"
	"go.chromium.org/chromiumos/infra/proto/go/satlabrpcserver"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/xcontext"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
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

// HostInfo Stores servo related host information.
type HostInfo struct {
	Host               string // Host stores the servo host name.
	Port               int    // Port stores the servo port number.
	SSHPort            int    // SSHPort stores servo ssh port number.
	DockerHost         bool   // DockerHost indicates whether the servo host is a docker container.
	MsgLineStart       int64  // MsgLineStart stores the starting line of /var/log/messages.
	ServoInitiated     bool   // ServoInitiated indicate if servod is started by current process.
	CurrentLogFileName string // The real log file name for the current servod session.
}

// StartServo start servod and verify every 2 second until it is ready. Add check to avoid repeated attempting.
func StartServo(parentCtx context.Context, servoHostPort, keyFile, keyDir string, dutTopology *labapi.Dut) (hostInfo *HostInfo, retErr error) {
	// Starting servo should not take more than 5 minutes.
	d := time.Now().Add(time.Minute * 5)
	ctx, cancel := xcontext.WithDeadline(parentCtx, d,
		errors.Errorf("%v: timeout for starting servod reached", context.DeadlineExceeded))
	defer cancel(context.Canceled)

	if servoHostPort == "" {
		return nil, nil
	}
	host, port, sshPort, err := splitHostPort(servoHostPort)
	if err != nil {
		return nil, err
	}
	isDocker := isDockerHost(host)
	// Example servoHostPort: satlab-0wgtfqin20158027-host2-docker_servod:9999.
	if isDocker {
		logging.Infof(ctx, "Start Docker servod container via Satlab RPC server.")
		conn, err := grpc.Dial(testing.SatlabRPCServer, grpc.WithInsecure())
		if err != nil {
			return nil, err
		}
		c := satlabrpcserver.NewSatlabRpcServiceClient(conn)
		if _, err = c.StartServod(ctx,
			&api.StartServodRequest{ServodDockerContainerName: host}); err != nil {
			return nil, err
		}
		return &HostInfo{
			Host:       host,
			Port:       port,
			SSHPort:    sshPort,
			DockerHost: true,
		}, nil
	}

	var msgLineStart int64
	// If the servod instance isn't running locally, assume that we need to connect to it via SSH.
	if sshPort > 0 && !isDocker && ((host != "localhost" && host != "127.0.0.1" && host != "::1") || sshPort != 22) {
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
		hst, err := ssh.New(ctx, &sopt)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open SSH connection to labstation")
		}
		defer hst.Close(ctx)

		defer func() {
			// This code should only run if the function is returning successfully and servod was started by us.
			if retErr == nil && hostInfo != nil {
				logLink := fmt.Sprintf("/var/log/servod_%d/latest.DEBUG", port)
				if realpath, err := hst.CommandContext(ctx, "realpath", logLink).Output(); err != nil {
					logging.Infof(ctx, "failed to get realpath for %s: %v", logLink, err)
					hostInfo.CurrentLogFileName = ""
				} else {
					hostInfo.CurrentLogFileName = filepath.Base(strings.TrimSpace(string(realpath)))
				}
			}
		}()

		wcInfo, err := linuxssh.WordCount(ctx, hst, "/var/log/messages")
		if err != nil {
			logging.Infof(ctx, "Failed to get word count for /var/log/messages of servo host %s: %v", servoHostPort, err)
			wcInfo = &linuxssh.WordCountInfo{}
		}
		msgLineStart = wcInfo.Lines
		hostInfo = &HostInfo{
			Host:         host,
			Port:         port,
			SSHPort:      sshPort,
			DockerHost:   false,
			MsgLineStart: msgLineStart,
		}
		inUseFile := fmt.Sprintf("/var/lib/servod/%d_in_use", int(port))
		if out, err := hst.CommandContext(ctx, "touch", inUseFile).CombinedOutput(); err != nil {
			logging.Infof(ctx, "failed to touch %s: %s: %v", inUseFile, (out), err)
		}
		out, err := hst.CommandContext(ctx, "servodtool", "instance", "show", "-p", strconv.Itoa(port)).Output()
		if err == nil {
			// Since servod has already been started, do not need to start again.
			return hostInfo, nil
		}
		logging.Infof(ctx, "Failed to find running servod instance: %s: %v", string(out), err)
		logging.Info(ctx, "Attempt to start servod")
		args := []string{"servod"}
		args = append(args, fmt.Sprintf("PORT=%d", port))
		if board := dutTopology.GetChromeos().GetDutModel().GetBuildTarget(); board != "" {
			args = append(args, fmt.Sprintf("BOARD=%s", board))
		}
		if model := dutTopology.GetChromeos().GetDutModel().GetModelName(); model != "" {
			args = append(args, fmt.Sprintf("MODEL=%s", model))
		}
		if serial := dutTopology.GetChromeos().GetServo().GetSerial(); serial != "" {
			args = append(args, fmt.Sprintf("SERIAL=%s", serial))
		} else if serial := dutTopology.GetDevboard().GetServo().GetSerial(); serial != "" {
			args = append(args, fmt.Sprintf("SERIAL=%s", serial))
		}
		if pools := dutTopology.GetPools(); len(pools) > 0 {
			for _, p := range pools {
				if strings.Contains(p, "faft-cr50") {
					args = append(args, "CONFIG=cr50.xml")
					break
				}
			}
		}
		if out, err := hst.CommandContext(ctx, "start", args...).CombinedOutput(); err != nil {
			// Don't return error so that we can log servod logs later.
			logging.Infof(ctx, "failed to start servod: %s: %v", string(out), err)
			return hostInfo, nil
		}
		// Save it for clean up later.
		hostInfo.ServoInitiated = true
		logging.Infof(ctx, "Started servod at port %d at servo host %s:%d", port, host, sshPort)
		// Provide servod up to 120 second to prepare, otherwise it will time out.
		logging.Infof(ctx, "Wait for servod to be ready")
		if out, err := hst.CommandContext(ctx, "servodtool", "instance", "wait-for-active", "--timeout", "120", "-p", strconv.Itoa(port)).Output(); err != nil {
			// Don't return error so that we can log servod logs later.
			logging.Infof(ctx, "failed to check if servod is ready: %s: %v", string(out), err)
		}
	}
	return hostInfo, nil
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

// Servo returns the proxy's encapsulated Servo object.
func (p *Proxy) Servo() *Servo { return p.svo }

// CleanUpAndCollectLogs clean up servo-in-use file and downloads servo logs
// to the dest directory.
// TODO: Move this code to a default fixture after b/333592531 is implemented.
func CleanUpAndCollectLogs(ctx context.Context, servoHostInfo *HostInfo, keyFile, keyDir, dest string) (retErr error) {
	if servoHostInfo == nil {
		return nil
	}
	host := servoHostInfo.Host
	port := servoHostInfo.Port
	sshPort := servoHostInfo.SSHPort
	isDocker := servoHostInfo.DockerHost
	// Example servoHostPort: satlab-0wgtfqin20158027-host2-docker_servod:9999.
	// TODO: b/333928745 download servod logs from servod containers.
	if isDocker {
		logging.Infof(ctx, "Downloading servod log from servo container is not supported yet")
		return nil
	}
	// If the servod instance isn't running locally, assume that we need to connect to it via SSH.
	shouldDownloadLogs := sshPort > 0 && !isDocker && ((host != "localhost" && host != "127.0.0.1" && host != "::1") || sshPort != 22)
	if !shouldDownloadLogs {
		logging.Infof(ctx, "Downloading servod log is not supported for host %s:%d", host, sshPort)
		return nil
	}
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
	hst, err := ssh.New(ctx, &sopt)
	if err != nil {
		return err
	}
	defer hst.Close(ctx)

	servodLogDir := fmt.Sprintf("/var/log/servod_%d", port)

	logging.Info(ctx, "Collecting servo logs from ", servodLogDir)
	defer logging.Info(ctx, "Done collecting servo logs from ", servodLogDir)

	servoHostDestDir := filepath.Join(dest, fmt.Sprintf("servo_host_%s", sopt.Hostname))

	destDir := filepath.Join(servoHostDestDir, fmt.Sprintf("servod_%d", port))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		logging.Infof(ctx, "Failed to create dir %s for downloading servo logs: %v", destDir, err)
	}

	// Getting servod logs.
	downloadServodLogs(ctx, hst, servodLogDir, destDir, servoHostInfo)

	// Getting servo startup log from servo host.
	const maxLogSize = 20 * 1024 * 1024 //20mb
	downloadServodStartupLogs(ctx, hst, port, servoHostDestDir, maxLogSize)

	// Getting /var/log/messages from servo host.
	downloadServodMessages(ctx, hst, servoHostDestDir, servoHostInfo.MsgLineStart, maxLogSize)

	// Getting dmesg.
	downloadServodDMesgLogs(ctx, hst, servoHostDestDir, sopt.Hostname)

	// Extract MCU logs from latest.DEBUG.
	extractServodMCULogs(ctx, destDir)

	// Remove in-use file
	if servoHostInfo.ServoInitiated {
		cleanupFile(ctx, hst, port, sopt.Hostname)
	}

	return nil
}

func downloadServodLogs(ctx context.Context, hst *ssh.Conn, servodLogDir, destDir string, hostInfo *HostInfo) {
	if hostInfo == nil || hostInfo.CurrentLogFileName == "" {
		logging.Info(ctx, "CurrentLogFileName is empty, falling back to downloading latest.DEBUG with GetFileTail")
		const maxLogSize = 20 * 1024 * 1024 // 20mb
		src := filepath.Join(servodLogDir, "latest.DEBUG")
		dest := filepath.Join(destDir, "latest.DEBUG")
		logging.Info(ctx, "Saving servo log ", dest)
		if err := linuxssh.GetFileTail(ctx, hst, src, dest, 0, maxLogSize); err != nil {
			logging.Infof(ctx, "Failed to get servod log %s: %v", src, err)
		}
		return
	}

	// Find all files in the servod log directory.
	cmd := hst.CommandContext(ctx, "find", servodLogDir, "-maxdepth", "1", "-type", "f", "-printf", "%f\\n")
	out, err := cmd.Output()
	if err != nil {
		logging.Infof(ctx, "Failed to list files in %s: %v", servodLogDir, err)
		return
	}
	allFiles := strings.Split(strings.TrimSpace(string(out)), "\n")

	var filesToDownload []string
	startFile := hostInfo.CurrentLogFileName
	// Verify CurrentLogFileName exists on the DUT.
	found := false
	for _, f := range allFiles {
		if f == startFile {
			found = true
			break
		}
	}
	if found {
		filesToDownload = append(filesToDownload, startFile)
	} else {
		logging.Infof(ctx, "Initial log file %s not found in %s", startFile, servodLogDir)
	}

	// Find all newer log files.
	logPattern := regexp.MustCompile(`^log\.\d{4}-\d{2}-\d{2}--\d{2}-\d{2}-\d{2}\.\d{3}\.DEBUG$`)
	var newerFiles []string
	for _, f := range allFiles {
		if logPattern.MatchString(f) && f > startFile {
			newerFiles = append(newerFiles, f)
		}
	}
	sort.Strings(newerFiles)
	filesToDownload = append(filesToDownload, newerFiles...)

	if len(filesToDownload) == 0 {
		logging.Info(ctx, "No specific log files found to process, falling back to downloading latest.DEBUG")
		src := filepath.Join(servodLogDir, "latest.DEBUG")
		dest := filepath.Join(destDir, "latest.DEBUG")
		if err := linuxssh.GetFile(ctx, hst, src, dest, linuxssh.PreserveSymlinks); err != nil {
			logging.Infof(ctx, "Failed to get servod log %s: %v", src, err)
		}
		return
	}

	logging.Infof(ctx, "Concatenating the following servo log files: %s", strings.Join(filesToDownload, ", "))
	var remotePaths []string
	for _, f := range filesToDownload {
		remotePaths = append(remotePaths, shutil.Escape(filepath.Join(servodLogDir, f)))
	}

	remoteTempFile, err := hst.CommandContext(ctx, "mktemp").Output()
	if err != nil {
		logging.Infof(ctx, "Failed to create remote temp file: %v", err)
		return
	}
	remoteTempPath := strings.TrimSpace(string(remoteTempFile))
	defer hst.CommandContext(ctx, "rm", remoteTempPath).Run()

	catCmdStr := fmt.Sprintf("cat %s > %s", strings.Join(remotePaths, " "), shutil.Escape(remoteTempPath))
	if out, err := hst.CommandContext(ctx, "sh", "-c", catCmdStr).CombinedOutput(); err != nil {
		// Log error but continue, as some logs might have been concatenated successfully.
		logging.Infof(ctx, "Concatenating remote log files with command `%s` failed with output %q: %v", catCmdStr, string(out), err)
	}

	destPath := filepath.Join(destDir, "latest.DEBUG")
	logging.Info(ctx, "Saving concatenated servo log to ", destPath)
	if err := linuxssh.GetFile(ctx, hst, remoteTempPath, destPath, linuxssh.PreserveSymlinks); err != nil {
		logging.Infof(ctx, "Failed to get concatenated servod log from %s: %v", remoteTempPath, err)
	}
}

func downloadServodStartupLogs(ctx context.Context, hst *ssh.Conn, port int,
	servoHostDestDir string, maxLogSize int64) {
	// Getting servo start log from servo host.
	servodStartup := fmt.Sprintf("servod_%d.STARTUP.log", port)
	src := filepath.Join("/var/log", servodStartup)
	if err := linuxssh.GetFileTail(ctx, hst, src,
		filepath.Join(servoHostDestDir, servodStartup),
		0, maxLogSize); err != nil {
		logging.Infof(ctx, "Failed to download servod log %s: %v", servodStartup, err)
	}
}

func downloadServodMessages(ctx context.Context, hst *ssh.Conn,
	servoHostDestDir string, startLine, maxLogSize int64) {
	// Getting servo start log from servo host.
	msg := "messages"
	src := filepath.Join("/var/log", msg)
	if err := linuxssh.GetFileTail(ctx, hst, src,
		filepath.Join(servoHostDestDir, msg),
		startLine, maxLogSize); err != nil {
		logging.Infof(ctx, "Failed to download servod log %s: %v", src, err)
	}
}

func downloadServodDMesgLogs(ctx context.Context, hst *ssh.Conn,
	servoHostDestDir, hostname string) {
	cmd := hst.CommandContext(ctx, "dmesg", "-H")
	dmesgFile := filepath.Join(servoHostDestDir, "dmesg")
	dmesgOut, err := os.Create(dmesgFile)
	if err != nil {
		logging.Infof(ctx, "Failed to create servo log dmesg: %v", err)
		return
	}
	defer dmesgOut.Close()
	cmd.Stdout = dmesgOut
	if err := cmd.Start(); err != nil {
		logging.Infof(ctx, "Failed to start dmesg for host %s: %v", hostname, err)
		return
	}
	defer cmd.Wait()
}

// extractServodMCULogs extract MCU logs from latest.DEBUG.
func extractServodMCULogs(ctx context.Context, destDir string) {
	var mcuFiles map[string]*os.File = make(map[string]*os.File)

	src := filepath.Join(destDir, "latest.DEBUG")
	f, err := os.Open(src)
	if err != nil {
		logging.Infof(ctx, "Failed to open %s: %v", src, err)
		return
	}
	defer f.Close()

	regExpr := `(?P<time>[\d\-]+(( [\d:,]+ )|(T[\d:.+]+ )))` +
		`- (?P<mcu>[\w/]+) - ` +
		`EC3PO\.Console[\s\-\w\d:.]+LogConsoleOutput - /dev/pts/\d+ - ` +
		`(?P<line>.+$)`

	re, err := regexp.Compile(regExpr)
	if err != nil {
		fmt.Printf("Fail in compiling expression %v\n", err)
		return
	}

	sc := bufio.NewScanner(f)
	sc.Split(bufio.ScanLines)
	for sc.Scan() {
		text := sc.Text()
		matches := re.FindStringSubmatch(text)
		timeIndex := re.SubexpIndex("time")
		if timeIndex < 0 || timeIndex >= len(matches) {
			continue
		}
		mcuIndex := re.SubexpIndex("mcu")
		if mcuIndex < 0 || mcuIndex >= len(matches) {
			continue
		}
		lineIndex := re.SubexpIndex("line")
		if lineIndex < 0 || lineIndex >= len(matches) {
			continue
		}
		timestamp := matches[timeIndex]
		mcu := strings.ToLower(matches[mcuIndex])
		line := matches[lineIndex]
		mcuFile, ok := mcuFiles[mcu]
		if !ok {
			mcuFile, err = os.Create(filepath.Join(destDir, fmt.Sprintf("%s.txt", mcu)))
			if err != nil {
				logging.Infof(ctx, "Failed to create servo log %s.txt: %v", mcu, err)
				mcuFiles[mcu] = nil
				continue
			}
			mcuFiles[mcu] = mcuFile
			defer mcuFile.Close()
		}
		if mcuFile == nil {
			continue
		}
		fmt.Fprintln(mcuFile, timestamp, "- ", line)
	}
}

func cleanupFile(ctx context.Context, hst *ssh.Conn, port int, hostname string) {
	inUseFile := fmt.Sprintf("/var/lib/servod/%d_in_use", port)
	cmd := hst.CommandContext(ctx, "rm", inUseFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		logging.Infof(ctx, "Failed to remove %s from %s: %s: %v", inUseFile, hostname, out, err)
	}
}
