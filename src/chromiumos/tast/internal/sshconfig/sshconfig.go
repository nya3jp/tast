// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshconfig

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// HostConfig store input target host, real host and port.
// *HostConfig will be used to communication with SSH Config Parser.
type HostConfig struct {
	inputHostName string // input hostname
	realHostName  string // real hostname to be used
	realPort      string // real port to be used
}

// ConfigParser maintains the internal status of SSH Config Parser.
type ConfigParser struct {
	openedFiles   map[string]struct{} // prevent include the same file recursively
	needCallback  bool                // flag indicates if a call back is needed
	acceptInclude bool                // flag indicates if Include should be processed
	callback      *HostConfig         // callback object
}

// GetRealHost takes an address and returns real hostname based on ~/.ssh/config and /etc/ssh/ssh_config.
func GetRealHost(addr string) (realHost string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return GetRealHostFromFiles(addr, []string{"/etc/ssh/ssh_config"})
	}
	return GetRealHostFromFiles(addr,
		[]string{homeDir + "/.ssh/config", "/etc/ssh/ssh_config"})
}

// GetRealHostFromFiles takes an address and a list of SSH configuration files.
// It will return the real hostname.
func GetRealHostFromFiles(addr string, configFiles []string) (realHost string) {
	hostnameAndPort := strings.Split(addr, ":")
	if len(hostnameAndPort) == 0 {
		return addr
	}
	host := hostnameAndPort[0]
	port := ""
	if len(hostnameAndPort) > 1 {
		port = hostnameAndPort[1]
	}
	sshHost := HostConfig{
		inputHostName: host,
		realHostName:  "",
		realPort:      port,
	}
	for _, f := range configFiles {
		readHostConfig(f, &sshHost)
		if sshHost.Done() {
			break
		}
	}
	if sshHost.realHostName == "" {
		sshHost.realHostName = host
	}
	if sshHost.realPort == "" {
		return sshHost.realHostName
	}
	return fmt.Sprintf("%v:%v", sshHost.realHostName, sshHost.realPort)
}

// readHostConfig takes SSH configuration file name and returns a list of host blocks defined in the file.
func readHostConfig(configFileName string, cb *HostConfig) {
	parser := makeConfigParser(cb)
	parser.readFiles(configFileName)
}

// readFiles reads one or more files that match the argument configFileName.
// The argument configFileName can have wildcards.
func (parser *ConfigParser) readFiles(configFileName string) {

	fileNames := resolvePath(configFileName)
	for _, f := range fileNames {
		_, opened := parser.openedFiles[f]
		if opened { // ignore opened files
			continue
		}
		configFile, err := os.Open(f)
		if err != nil {
			return
		}
		defer configFile.Close()
		parser.openedFiles[f] = struct{}{}
		defer delete(parser.openedFiles, f)

		sc := bufio.NewScanner(configFile)
		for sc.Scan() {
			strs := strings.Split(sc.Text(), "#") // ignore all comments
			if len(strs) == 0 {
				continue
			}
			strs = strings.Fields(strs[0])
			if len(strs) < 2 { // Need at least one value to be meaningful
				continue
			}
			switch {
			case strings.EqualFold(strs[0], "Host"), strings.EqualFold(strs[0], "Match"):
				// Host and Match keyword start a new block
				parser.needCallback = parser.callback.match(strs[0], strs[1:])
				// Include statement is allowed inside a matched block
				if parser.needCallback {
					parser.acceptInclude = true
				}
			case strings.EqualFold(strs[0], "Include"):
				if parser.acceptInclude {
					for _, newFile := range strs[1:] {
						parser.readFiles(newFile)
					}
				}
			case parser.needCallback:
				// only save data if it is in a matched block
				parser.callback.setValue(strs[0], strs[1:])
				if parser.callback.Done() {
					return
				}
			}
		}
	}
	return
}

// makeConfigParser creates a new SHConfigParser.
func makeConfigParser(cb *HostConfig) *ConfigParser {
	return &ConfigParser{
		openedFiles:   map[string]struct{}{},
		needCallback:  false,
		acceptInclude: true,
		callback:      cb,
	}
}

// match check if the host address matches any member of an array of patterns.
func (sshHost *HostConfig) match(configType string, patterns []string) bool {
	if !strings.EqualFold(configType, "HOST") {
		// we support only HOST type in this version
		return false
	}
	matched := false
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "!") {
			ok, _ := filepath.Match(pattern[1:], sshHost.inputHostName)
			if ok {
				return false
			}
		} else {
			ok, _ := filepath.Match(pattern, sshHost.inputHostName)
			if ok {
				matched = true
			}
		}
	}
	return matched
}

// Done will decide whether we have all the information we needed.
// We only need hostname and port for now. Ignore other information.
func (sshHost *HostConfig) Done() bool {
	return sshHost.realHostName != "" && sshHost.realPort != ""
}

// setValue set incoming value to appropriate member.
func (sshHost *HostConfig) setValue(key string, value []string) {
	if len(value) == 0 {
		return
	}
	switch strings.ToLower(key) {
	case "hostname":
		sshHost.setHostName(value[0])
	case "port":
		sshHost.setPort(value[0])
	}
}

// setHostName set the real hostname.
func (sshHost *HostConfig) setHostName(hostName string) {
	// If it is already set, we will not set it again
	if sshHost.realHostName == "" {
		sshHost.realHostName = strings.ReplaceAll(hostName, "%h",
			sshHost.inputHostName)
		sshHost.realHostName = strings.ReplaceAll(sshHost.realHostName, "%%", "%")
	}
}

// setPort set the real port.
func (sshHost *HostConfig) setPort(port string) {
	// If it is already set, we will not set it again
	if sshHost.realPort == "" {
		sshHost.realPort = port
	}
}

// resolvePath expands a path to a list of absolute full paths.
// It will resolve leading tildes and wildcards so that each resolved path can be used by os.Open.
func resolvePath(f string) []string {
	f = expandLeadingTilde(f)
	absPath, err := filepath.Abs(f)
	if err != nil {
		return []string{f}
	}
	f = filepath.Clean(absPath)
	fileNames, err := filepath.Glob(absPath)
	if err != nil {
		return []string{f}
	}
	return fileNames
}

// expandLeadingTilde expands a path if its first character is ~.
// If the path is leading with "~/", it will replace leading ~ with the current user's home directory.
// If the path is leading with "~<user>/", it will replace leading ~<user> with the specified user's home directory.
func expandLeadingTilde(f string) string {
	if strings.HasPrefix(f, "~") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			if strings.HasPrefix(f, "~/") {
				f = strings.Replace(f, "~", homeDir, 1)
			} else {
				directories := strings.Split(f, "/")
				userName := directories[0]
				u, err := user.Lookup(userName)
				if err == nil {
					directories[0] = u.HomeDir
					f = strings.Join(directories, "/")
				}
			}
		}
	}
	return f
}
