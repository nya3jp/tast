// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshconfig

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"chromiumos/tast/errors"
)

// hostConfig store input target host, resolved host and port.
type hostConfig struct {
	resolvedHostName string // resolved hostname to be used
	resolvedPort     string // resolved port to be used
}

// matchCallback check if the host address matches any member of an array of patterns.
type matchCallback func(configType string, patterns []string) bool

// doneCallback will decide whether we have all the information we needed.
type doneCallback func() bool

// setValueCallback set incoming value to appropriate member.
type setValueCallback func(key string, value []string)

// configParser maintains the internal status of SSH Config Parser.
type configParser struct {
	openedFiles   map[string]struct{} // prevent include the same file recursively
	needCallback  bool                // flag indicates if a call back is needed
	acceptInclude bool                // flag indicates if Include should be processed
	match         matchCallback       // callback to check if block is a match
	done          doneCallback        // callback to check if all needed information was restrieved
	setValue      setValueCallback    // callback to setValue for a keyword
}

// GetResolvedHost takes an address and returns a resolved hostname based on ~/.ssh/config and /etc/ssh/ssh_config.
func GetResolvedHost(addr string) (resolvedHost string, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return GetResolvedHostFromFiles(addr, []string{"/etc/ssh/ssh_config"})
	}
	return GetResolvedHostFromFiles(addr,
		[]string{homeDir + "/.ssh/config", "/etc/ssh/ssh_config"})
}

// GetResolvedHostFromFiles takes an address and a list of SSH configuration files and returns the resolved hostname.
// Reading of files will stop once both the hostname and port are found in the matched.
// If the first file already has a hostname and port, the rest of the file will not be read.
func GetResolvedHostFromFiles(addr string, configFiles []string) (resolvedHost string, err error) {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return "", err
	}
	if host == "" {
		return addr, nil
	}
	hc := hostConfig{
		resolvedHostName: "",   // Initialized as empty string.
		resolvedPort:     port, // Initialized as input if it is specified.
	}
	for _, f := range configFiles {
		// Ignore the files that do not exist.
		if _, err := os.Stat(f); os.IsNotExist(err) {
			continue
		}
		err = readHostConfig(f, host, &hc)
		if err == nil && hc.done() {
			break
		}
	}
	if err != nil {
		return "", err
	}
	if hc.resolvedHostName == "" {
		hc.resolvedHostName = host
	}
	return joinHostAndPort(hc.resolvedHostName, hc.resolvedPort), nil
}

// readHostConfig takes SSH configuration file name and returns a list of host blocks defined in the file.
func readHostConfig(configFileName, inputHostName string, hc *hostConfig) error {
	parser := configParser{
		openedFiles:   map[string]struct{}{}, // Initialize it ot an empty map.
		needCallback:  false,                 // Not in any scope at the beginning of file.
		acceptInclude: true,                  // Include is always ok at the beginning.
	}
	return parser.readFiles(configFileName, inputHostName, hc)
}

// readFiles reads one or more files that match the argument configFileName.
// The argument configFileName can have wildcards.
func (parser *configParser) readFiles(configFileName, inputHostName string, hc *hostConfig) error {

	fileNames, err := resolvePath(configFileName)
	if err != nil {
		return err
	}
	for _, f := range fileNames {
		err = parser.readFile(f, inputHostName, hc)
		if err != nil {
			return err
		}
	}
	return nil
}

// readFile reads one single file with the resolved abosolute path name.
func (parser *configParser) readFile(configFileName, inputHostName string, hc *hostConfig) error {
	_, opened := parser.openedFiles[configFileName]
	if opened { // Ignore opened files.
		return errors.Errorf("there is a loop while trying to include %v", configFileName)
	}
	configFile, err := os.Open(configFileName)
	if err != nil {
		return err
	}
	defer configFile.Close()
	parser.openedFiles[configFileName] = struct{}{}
	defer delete(parser.openedFiles, configFileName)

	sc := bufio.NewScanner(configFile)
	for sc.Scan() {
		strs := strings.Split(sc.Text(), "#") // Ignore all comments.
		if len(strs) == 0 {
			continue
		}
		strs = strings.Fields(strs[0])
		if len(strs) < 2 { // Need at least one value to be meaningful.
			continue
		}
		switch {
		case strings.EqualFold(strs[0], "Host"):
			// We support only HOST type in this version.
			// Host keyword start a new block.
			parser.needCallback = matchHost(inputHostName, strs[1:])
			// Include statement is allowed inside a matched block
			if parser.needCallback {
				parser.acceptInclude = true
			}
		case strings.EqualFold(strs[0], "Match"):
			// Ignore Match block in this version.
			parser.needCallback = false
			parser.acceptInclude = false
		case strings.EqualFold(strs[0], "Include"):
			if !parser.acceptInclude {
				break
			}
			for _, newFile := range strs[1:] {
				err = parser.readFiles(newFile, inputHostName, hc)
				if err != nil {
					return err
				}
			}
		case parser.needCallback:
			// Only save data if it is in a matched block.
			hc.setValue(inputHostName, strs[0], strs[1:])
		}
	}
	if err = sc.Err(); err != nil {
		return err
	}
	return nil
}

// matchHost check if inputHostName matches the patterns specified in the Host statement.
func matchHost(inputHostName string, patterns []string) bool {
	matched := false
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "!") {
			// If the inputHostName match one of the negate patterns, return false immediately.
			if ok, err := filepath.Match(pattern[1:], inputHostName); err == nil && ok {
				return false
			}
		}
		if ok, err := filepath.Match(pattern, inputHostName); err == nil && ok {
			// Need at least one match for non-negate patterns.
			matched = ok
		}
	}
	return matched
}

// done decides whether we have all the information we needed.
// We only need hostname and port for now. Ignore other information.
func (hc *hostConfig) done() bool {
	return hc.resolvedHostName != "" && hc.resolvedPort != ""
}

// setValue sets the value of a hostConfig attribute base on input host name and a key/value pair.
func (hc *hostConfig) setValue(inputHostName, key string, value []string) {
	if len(value) == 0 {
		return
	}
	// We only need hostname and port for now. Ignore other information.
	switch strings.ToLower(key) {
	case "hostname":
		// If it is already set, we will not set it again.
		if hc.resolvedHostName != "" {
			return
		}
		hc.resolvedHostName = strings.ReplaceAll(value[0], "%h", inputHostName)
		hc.resolvedHostName = strings.ReplaceAll(hc.resolvedHostName, "%%", "%")
	case "port":
		// If it is already set, we will not set it again.
		if hc.resolvedPort == "" {
			hc.resolvedPort = value[0]
		}
	}
}

// resolvePath expands a path to a list of absolute full paths.
// It will resolve leading tildes and wildcards so that each resolved path can be used by os.Open.
func resolvePath(f string) ([]string, error) {
	f, err := expandLeadingTilde(f)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(f)
	if err != nil {
		return nil, err
	}
	return filepath.Glob(filepath.Clean(absPath))
}

// expandLeadingTilde expands a path if its first character is ~.
// If the path is leading with "~/", it will replace leading ~ with the current user's home directory.
// If the path is leading with "~<user>/", it will replace leading ~<user> with the specified user's home directory.
func expandLeadingTilde(f string) (string, error) {
	if !strings.HasPrefix(f, "~") {
		return f, nil
	}
	if strings.HasPrefix(f, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return strings.Replace(f, "~", homeDir, 1), nil
	}
	directories := strings.Split(f, "/")
	userName := directories[0]
	u, err := user.Lookup(userName)
	if err != nil {
		return "", err
	}
	directories[0] = u.HomeDir
	return strings.Join(directories, "/"), nil
}

// splitHostPort splits an address to host and port.
// Example 1: "octopus" -> "octopus" "".
// Example 2: "[0:0:0:0:0:ffff:7f00:1]:2" -> "0:0:0:0:0:ffff:7f00:1" "2".
func splitHostPort(hostAndPort string) (host, port string, err error) {
	numColons := strings.Count(hostAndPort, ":")
	// case: octopus
	host = hostAndPort
	port = ""
	switch {
	case numColons == 0:
		// Example: octopus
		return hostAndPort, "", nil
	case strings.HasPrefix(hostAndPort, "[") && strings.HasSuffix(hostAndPort, "]"):
		// Example: "[0:0:0:0:0:ffff:7f00:1]".
		// SSH config does not support ipv6 names with [].
		return hostAndPort[1 : len(hostAndPort)-1], "", nil
	case numColons == 1 || strings.HasPrefix(hostAndPort, "["):
		// Example: "[0:0:0:0:0:ffff:7f00:1]:10" or "test:1".
		host, port, err = net.SplitHostPort(hostAndPort)
		if err != nil {
			return "", "", err
		}
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		// SSH config does not support ipv6 names with [].
		return host[1 : len(host)-1], port, nil
	}
	return host, port, nil
}

// joinHostAndPort joins host and port to a single address.
// Example 1: "octopus" "" -> "octopus".
// Example 2: "0:0:0:0:0:ffff:7f00:1" "2" -> "[0:0:0:0:0:ffff:7f00:1]:2".
func joinHostAndPort(host, port string) string {
	if strings.Contains(host, ":") {
		// ipv6 hostname
		host = fmt.Sprintf("[%v]", host)
	}
	if port == "" {
		return host
	}
	return fmt.Sprintf("%v:%v", host, port)
}
