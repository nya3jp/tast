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
	notInBlock sshConfigBlockType = iota // Not in a block
	hostBlock                            // Host Block
	matchBlock                           // Match block
)

// sshConfig is a hierary representation a SSH configuration.
type sshConfig struct {
	blockType  sshConfigBlockType  // Block type.
	patterns   []string            // Pattern to match a block
	parameters map[string][]string // Name/Value pair for parameters
	subBlocks  []*sshConfig        // sub block under current hierarchy
}

// print is for debugging purpose only
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

// configParser maintains the internal status of SSH Config Parser.
type configParser struct {
	openedFiles map[string]struct{} // prevent include the same file recursively.
	re          *regexp.Regexp      // reg expression to be used in splitting a line to strings.
}

// ResolveHost takes an address and returns a resolved hostname based on ~/.ssh/config and /etc/ssh/ssh_config.
func ResolveHost(addr string) (resolvedHost string, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ResolveHostFromFiles(addr, []string{"/etc/ssh/ssh_config"})
	}
	return ResolveHostFromFiles(addr,
		[]string{homeDir + "/.ssh/config", "/etc/ssh/ssh_config"})
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
		blockType:  notInBlock,     // Top of the hierarchy so it is not in any block.
		patterns:   nil,            // No patterns at the top.
		parameters: nil,            // No parameters at the top.
		subBlocks:  []*sshConfig{}, // Initialized as an empty array
	}
	parser := makeConfigParser()
	for _, f := range configFiles {

		// Ignore the files that do not exist.
		if _, err := os.Stat(f); os.IsNotExist(err) {
			continue
		}
		err = parser.readFiles(f, &sc)
		if err != nil {
			return "", err
		}
	}
	resolvedHostName, resolvedPort := sc.findHostPort(host)
	if resolvedHostName == "" {
		resolvedHostName = host
	}
	if port != "" {
		resolvedPort = port
	}
	return joinHostAndPort(resolvedHostName, resolvedPort), nil
}

// makeConfigParser creates and initialize a configParser
func makeConfigParser() *configParser {
	parser := configParser{
		openedFiles: map[string]struct{}{}, // Initialize it ot an empty map.
	}
	tokenPattern := `!("([^"]*)")|("([^"]*)")|([^\s"]+)`
	parser.re, _ = regexp.Compile(tokenPattern)
	return &parser
}

// readFiles reads one or more files that match the argument configFileName.
// The argument configFileName can have wildcards.
func (parser *configParser) readFiles(configFileName string, sc *sshConfig) error {
	fileNames, err := resolvePath(configFileName)
	if err != nil {
		return err
	}
	for _, f := range fileNames {
		if err = parser.readFile(f, sc); err != nil {
			return err
		}
	}
	return nil
}

// readFile reads one single file with the resolved abosolute path name.
func (parser *configParser) readFile(configFileName string, parentConfig *sshConfig) error {
	// Ignore file that does not exist. This is same behavior as ssh command.
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		return nil
	}
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

	curConfig := parentConfig
	scanner := bufio.NewScanner(configFile)
	for scanner.Scan() {
		strs := parser.extractLine(scanner.Text()) // Ignore all comments.
		if len(strs) < 2 {                         // Need at least one value to be meaningful.
			continue
		}
		switch {
		case strings.EqualFold(strs[0], "Host"):
			// Ignore Match block in this version.
			curConfig = &sshConfig{
				blockType:  hostBlock,             // It is a host block.
				patterns:   strs[1:],              // Patterns for matching later.
				parameters: map[string][]string{}, // No parameters at the top.
				subBlocks:  []*sshConfig{},        // Initialized as an empty array
			}
			parentConfig.subBlocks = append(parentConfig.subBlocks, curConfig)
		case strings.EqualFold(strs[0], "Match"):
			// Ignore Match block in this version.
			curConfig = &sshConfig{
				blockType:  matchBlock,            // It is a match block.
				patterns:   strs[1:],              // Patterns for matching later.
				parameters: map[string][]string{}, // No parameters at the top.
				subBlocks:  []*sshConfig{},        // Initialized as an empty array
			}
			parentConfig.subBlocks = append(parentConfig.subBlocks, curConfig)
		case strings.EqualFold(strs[0], "Include"):
			for _, newFile := range strs[1:] {
				if err = parser.readFiles(newFile, curConfig); err != nil {
					return err
				}
			}
		case curConfig.blockType != notInBlock:
			// Only save data if it is in a block.
			curConfig.parameters[strings.ToLower(strs[0])] = strs[1:]
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return nil
}

// extractLine extract a line from SSH configuration for to an array of strings.
// It will remove comments and handle quoted strings
func (parser *configParser) extractLine(line string) []string {
	strs := parser.re.FindAllString(line, -1)
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
	if sc.blockType != notInBlock {
		if !matchHost(inputHostName, sc.patterns) {
			return "", ""
		}
		values := sc.parameters["hostname"]
		if values != nil && len(values) == 1 {
			resolvedHostName = strings.ReplaceAll(values[0], "%%", "%")
			resolvedHostName = strings.ReplaceAll(resolvedHostName, "%h", inputHostName)
		}
		values = sc.parameters["port"]
		if values != nil && len(values) == 1 {
			resolvedPort = values[0]
		}
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
