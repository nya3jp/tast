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
	"regexp"
	"strings"

	"chromiumos/tast/errors"
)

// sshConfigBlockType represent type of SSH config block.
type sshConfigBlockType int

const (
	notInBlock sshConfigBlockType = iota // not in a block
	hostBlock                            // host block
	matchBlock                           // match block
)

// sshConfig is a hierary representation a SSH configuration.
// The Include parameter is allowed inside a Host or Match block.
// If the pattern is not matched, it will use the parameters defined in the include file.
// The following is an example using Include inside a block
//
type sshConfig struct {
	blockType  sshConfigBlockType  // block type.
	patterns   []string            // pattern to match a block
	parameters map[string][]string // name/value pair for parameters
	subBlocks  []*sshConfig        // sub block under current hierarchy
}

// tokenRegExp is regular expression for a token in ssh configuration file.
// "<string_with_or_without_space>", <string_without_space>, and !"<string_with_or_without_space>".
var tokenRegExp = regexp.MustCompile(`!("([^"]*)")|("([^"]*)")|([^\s"]+)`)

// ResolveHost takes an address and returns a resolved hostname based on ~/.ssh/config and /etc/ssh/ssh_config.
func ResolveHost(addr string) (resolvedHost string, err error) {
	var paths []string
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, homeDir+"/.ssh/config")
	}
	paths = append(paths, "/etc/ssh/ssh_config")
	return ResolveHostFromFiles(addr, paths)
}

// ResolveHostFromFiles takes an address and a list of SSH configuration files and returns the resolved hostname.
func ResolveHostFromFiles(addr string, configFiles []string) (resolvedHost string, err error) {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return "", err
	}
	if host == "" {
		return addr, nil
	}
	sc := sshConfig{
		blockType:  notInBlock,            // top of the hierarchy so it is not in any block.
		parameters: map[string][]string{}, // initialized as an empty map.
	}
	// openedFile is used to maintain current opened files prevent
	// the same file is included recursively.
	openedFiles := map[string]struct{}{}
	for _, f := range configFiles {
		if err = readFiles(f, openedFiles, &sc); err != nil {
			return "", err
		}
	}
	resolvedHostName, resolvedPort := sc.findHostPort(host)
	// Use input host name f we cannot find HostName parameter from the matched Host definition.
	// For example, if we cannot anything Host pattern match 127.0.0.1,
	// the resolvedHostName will be 127.0.0.1
	if resolvedHostName == "" {
		resolvedHostName = host
	}
	// If user does not specify the port number, we will use the found port number.
	// Empty if it is not found.
	if port != "" {
		// If the user specifies the port, we will use the user specified port.
		resolvedPort = port
	}
	return joinHostAndPort(resolvedHostName, resolvedPort), nil
}

// readFiles reads one or more files that match the argument configFileName.
// The argument configFileName can have wildcards.
func readFiles(configFileName string,
	openedFiles map[string]struct{}, sc *sshConfig) error {
	fileNames, err := resolvePathToPaths(configFileName)
	if err != nil {
		return err
	}
	for _, f := range fileNames {
		if err = readFile(f, openedFiles, sc); err != nil {
			return err
		}
	}
	return nil
}

// readFile reads one single file with the resolved abosolute path name.
func readFile(configFileName string,
	openedFiles map[string]struct{}, parentConfig *sshConfig) error {
	// Ignore file that does not exist. This is same behavior as ssh command.
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		return nil
	}
	if _, opened := openedFiles[configFileName]; opened { // ignore opened files.
		return errors.Errorf("there is a loop while trying to include %v", configFileName)
	}
	configFile, err := os.Open(configFileName)
	if err != nil {
		return err
	}
	defer configFile.Close()
	defer delete(openedFiles, configFileName)

	curConfig := parentConfig
	scanner := bufio.NewScanner(configFile)
	for scanner.Scan() {
		strs := extractLine(scanner.Text()) // ignore all comments.
		if len(strs) < 2 {                  // need at least one value to be meaningful.
			continue
		}
		switch {
		case strings.EqualFold(strs[0], "Host"):
			// Ignore Match block in this version.
			curConfig = &sshConfig{
				blockType:  hostBlock,             // it is a host block.
				patterns:   strs[1:],              // patterns for matching later.
				parameters: map[string][]string{}, // no parameters at the top.
			}
			// Add new block to parent block
			parentConfig.subBlocks = append(parentConfig.subBlocks, curConfig)
		case strings.EqualFold(strs[0], "Match"):
			curConfig = &sshConfig{
				blockType:  matchBlock,            // it is a match block.
				patterns:   strs[1:],              // patterns for matching later.
				parameters: map[string][]string{}, // no parameters at the top.
			}
			// Add new block to parent block
			parentConfig.subBlocks = append(parentConfig.subBlocks, curConfig)
		case strings.EqualFold(strs[0], "Include"):
			if err = includeFiles(strs[1:], openedFiles, curConfig); err != nil {
				return err
			}
		default:
			// Save parameters
			curConfig.parameters[strings.ToLower(strs[0])] = strs[1:]
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return nil
}

// includeFiles handle the include parameter and include a list of files
func includeFiles(files []string,
	openedFiles map[string]struct{}, sc *sshConfig) error {
	for _, newFile := range files {
		if !strings.HasPrefix(newFile, "/") && !strings.HasPrefix(newFile, "~") {
			// Files without absolute paths are assumed to be
			// in ~/.ssh if included in a user configuration file or
			// /etc/ssh if included from the system configuration file.

			// first try ~/.ssh/
			firstChoice := filepath.Join("~", ".ssh", newFile)
			if err := readFiles(firstChoice, openedFiles, sc); err != nil {
				return err
			}
			// Then try /etc/ssh
			newFile = filepath.Join("/etc/ssh", newFile)
		}
		if err := readFiles(newFile, openedFiles, sc); err != nil {
			return err
		}
	}
	return nil
}

// extractLine extract a line from SSH configuration for to an array of strings.
// It will remove comments and handle quoted strings
func extractLine(line string) []string {
	strs := tokenRegExp.FindAllString(line, -1)
	for i, s := range strs {
		switch {
		case strings.HasPrefix(s, "#"):
			return strs[:i] // ignore comments
		case strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`):
			strs[i] = s[1 : len(s)-1] // change "host" to host
		case strings.HasPrefix(s, `!"`) && strings.HasSuffix(s, `"`):
			strs[i] = fmt.Sprintf("!%v", s[2:len(s)-1]) // change !"host" to !host
		}
	}
	return strs
}

// resolvePathToPaths expands a path to a list of absolute full paths.
// It will resolve leading tildes and wildcards so that each resolved path can be used by os.Open.
func resolvePathToPaths(f string) ([]string, error) {
	f, err := expandLeadingTilde(f)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(f)
	if err != nil {
		return nil, err
	}
	return filepath.Glob(absPath)
}

// expandLeadingTilde expands a path if its first character is ~.
// If the path is leading with "~/", it will replace leading ~ with the current user's home directory.
// If the path is leading with "~<user>/", it will replace leading ~<user> with the specified user's home directory.
/*
# Content of ~/ssh/config
host 0:0:0:0:0:ffff:7f00:1
    Port 2222
Host "!octopu*" o*
   HostName %h.c.googlers.com
Host "octopus"
    HostName 127.0.0.1
    Port 2222
    # Include ~/test/bad.conf
    Include ~/.ssh/custom_ssh_d/root_config
Host hana
    Include ~/.ssh/custom_ssh_d/hana_config
    HostName 127.0.0.1
    Port 2223
Host *
	Include custom_ssh_d/my_config

# Content of ~/.ssh/custom_ssh_d/root_config
Host *
   User root

# Content of ~/.ssh/custom_ssh_d/hana_config
Host badone
    Port 2226

# Content of ~/.ssh/custom_ssh_d/my_config
Host *
   User user1

When I do "ssh mach1", ssh will not pick up Port 2223 from the "Host *" inside hana_config.
The reason is that the host "mach1" does not match the pattern "hana" so the rules
inside hana_config will not be used. It does match the pattern "Host *" inside ~/.ssh/config.
Therefore, ssh will pick up the user id from  the "Host *" inside my_config.

user1@user1:~/.ssh$ ssh mach1
Welcome to
     _
   ." ".           _     _
  / o o \     __ _| |   (_)_ __  _   ___  __
  |/\v/\|    / _` | |   | | '_ \| | | \ \/ /
 /|     |\  | (_| | |___| | | | | |_| |>  <
 \|_^_^_|/   \__, |_____|_|_| |_|\__,_/_/\_\
             |___/
Linux mach1.c.googlers.com 5.6.14-2rodete5-amd64 #1 SMP Debian 5.6.14-2rodete5 (2020-07-31 > 2018) x86_64
Last login: Mon Aug 17 23:19:44 2020 from 172.253.30.82
user1@mach1:~$ exit

*/
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
	userName := directories[0][1:]
	u, err := user.Lookup(userName)
	if err != nil {
		return "", err
	}
	directories[0] = u.HomeDir
	return filepath.Join(directories...), nil
}

// splitHostPort splits an address to host and port.
// Example 1: "127.0.0.1" -> "127.0.0.1" "".
// Example 2: "[0:0:0:0:0:ffff:7f00:1]:2" -> "0:0:0:0:0:ffff:7f00:1" "2".
func splitHostPort(hostAndPort string) (host, port string, err error) {
	numColons := strings.Count(hostAndPort, ":")
	// case: 127.0.0.1
	host = hostAndPort
	port = ""
	if numColons == 0 {
		// Example: 127.0.0.1
		return hostAndPort, "", nil
	}
	if strings.HasPrefix(hostAndPort, "[") && strings.HasSuffix(hostAndPort, "]") {
		// Example: "[0:0:0:0:0:ffff:7f00:1]".
		// SSH config does not support ipv6 names with [].
		return hostAndPort[1 : len(hostAndPort)-1], "", nil
	}
	if numColons == 1 || strings.HasPrefix(hostAndPort, "[") {
		// Example: "[0:0:0:0:0:ffff:7f00:1]:10" or "test:1".
		if host, port, err = net.SplitHostPort(hostAndPort); err != nil {
			return "", "", err
		}
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		// Since SSH config does not support ipv6 names with [], strip them
		return host[1 : len(host)-1], port, nil
	}
	return host, port, nil
}

// joinHostAndPort joins host and port to a single address.
// Example 1: "127.0.0.1" "" -> "127.0.0.1".
// Example 2: "0:0:0:0:0:ffff:7f00:1" "2" -> "[0:0:0:0:0:ffff:7f00:1]:2".
// Example 3: "0:0:0:0:0:ffff:7f00:1" "" -> 0:0:0:0:0:ffff:7f00:1"
func joinHostAndPort(host, port string) string {
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
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

// findHostPort finds the first matched host and port from sshConfig
func (sc *sshConfig) findHostPort(inputHostName string) (resolvedHostName, resolvedPort string) {
	if sc.blockType == matchBlock ||
		(sc.patterns != nil && !matchHost(inputHostName, sc.patterns)) {
		return "", ""
	}
	values := sc.parameters["hostname"]
	if values != nil && len(values) == 1 {
		// In order to handle the case %%%h, we first replace all %% with tab because
		// tab is not allowed in hostname
		resolvedHostName = strings.ReplaceAll(values[0], "%%", "\t")
		resolvedHostName = strings.ReplaceAll(resolvedHostName, "%h", inputHostName)
		// Now we replace all tabs back to %
		resolvedHostName = strings.ReplaceAll(resolvedHostName, "\t", "%")
	}
	values = sc.parameters["port"]
	if values != nil && len(values) == 1 {
		resolvedPort = values[0]
	}

	for _, subBlock := range sc.subBlocks {
		if resolvedHostName != "" && resolvedPort != "" {
			return resolvedHostName, resolvedPort
		}
		hostname, port := subBlock.findHostPort(inputHostName)
		if resolvedHostName == "" {
			resolvedHostName = hostname
		}
		if resolvedPort == "" {
			resolvedPort = port
		}
	}
	return resolvedHostName, resolvedPort
}

// print is for debugging purpose only.
func (sc *sshConfig) print(tab int) {
	indent := strings.Repeat(" ", tab)
	fmt.Printf("\n%vblockType  %v\n", indent, sc.blockType)
	fmt.Printf("%vpatterns   %v\n", indent, sc.patterns)
	fmt.Printf("%vparameters %v\n", indent, sc.parameters)
	fmt.Printf("%vsubBlocks\n", indent)
	for _, sb := range sc.subBlocks {
		sb.print(tab + 4)
	}
}
