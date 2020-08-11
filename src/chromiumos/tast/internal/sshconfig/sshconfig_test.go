// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"chromiumos/tast/testutil"
)

// testData stores input data and expected output.
type testData struct {
	input    string
	expected string
}

// testForResolveHostFromDefaultConfig runs wiath a default test configuaration that used by most unit tests for ResolveHostFromFiles.
func testForResolveHostFromDefaultConfig(t *testing.T, data *testData) {
	// topConfigFile defines the name of the config file used in most unit tests in this file
	firstConfigFile := "config1_d/config"

	// firstConfigFileContent defines the content of the config file used in most unit tests in this file
	firstConfigFileContent := `
		Include non_existing_file    # Test non-existing include file.
		Include octopus_config       # Test include file at top
		Host test*                   # Test simple wildcard
			HostName %h.google.com   # Test format %h
			Port 2224               
		Host !*google.com eve*       # Test patterns with negate
			Include port_config      # Test include file without Host keyword
			HostName 127.0.0.1       # Use other rule for port
		Host hello*                  # Test nest include
			user goodday
			Include hello_config     # Test nest include
		Host 0:0:0:0:0:ffff:*1
			Port 2222
	`

	// topConfigFile defines the name of the config file used in most unit tests in this file
	secondConfigFile := "config2_d/config"

	// secondConfigFileContent defines the content of the config file used in most unit tests in this file
	// It is to make sure we can get data from second configuration files.
	secondConfigFileContent := `
		Include hana_*                         # Test include file with wildcard
		Host *zone
			HostName fe00::1ff:ffff:7f00:1%%%h # Test format %%%h
			Port 2230
		Host prefix*                           # Test host match two rules
			HostName tworules.google.com
		Host *suffix                           # Test host match two rules
			Port 2231
		Host !*:* *
			Port 22
	`

	// octopusConfigFile defines the name of the config file for an alias name octopus
	octopusConfigFile := "config1_d/octopus_config"

	// octopusConfigContent defines the content of the config file for an alias name octopus
	octopusConfigContent := `
		Host "octopus"
			HostName 127.0.0.1
			Port 2222
	`

	// hanaConfigFile defines the name of the config file for an alias name hana
	hanaConfigFile := "config2_d/hana_config"

	// hanaConfigContent defines the content of the config file for an alias name hana
	hanaConfigContent := `
		Host "hana"
			HostName 127.0.0.1
			Port 2223
	`

	// portConfigFile defines the name of the config file for a port
	portConfigFile := "config1_d/port_config"

	// portConfigContent defines the content of the config file for a port
	portConfigContent := `
		Port 2225
	`

	// helloConfigFile defines the name of the config file for hosts with hello prefix
	helloConfigFile := "config1_d/hello_config"

	// helloConfigContent defines the content of the config file for hosts with hello prefix
	helloConfigContent := `
		Host *google.com
			HostName %h
			Port 2226
		Host "!*googler.com" *.*
			HostName %h
			Port 2227
		Host !"*googler.com" *
			HostName %h.c.googler.com
		Host !hello22* *
			Port 2228
	`
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	firstFullPath := filepath.Join(td, firstConfigFile)
	secondFullPath := filepath.Join(td, secondConfigFile)

	if err := testutil.WriteFiles(td, map[string]string{
		firstConfigFile:   firstConfigFileContent,
		secondConfigFile:  secondConfigFileContent,
		octopusConfigFile: octopusConfigContent,
		hanaConfigFile:    hanaConfigContent,
		portConfigFile:    portConfigContent,
		helloConfigFile:   helloConfigContent,
	}); err != nil {
		t.Fatal(err)
	}
	fileParams := []FileParam{
		{
			Path:    firstFullPath,
			BaseDir: filepath.Dir(firstFullPath),
		},
		{
			Path:    secondFullPath,
			BaseDir: filepath.Dir(secondFullPath),
		},
	}
	// make sure both hana and octopus can be read
	testResolveHostFromFiles(t, data, fileParams)
}

// TestResolveHostFromFilesIncludeTop tests using include file at the top level.
func TestResolveHostFromFilesIncludeTop(t *testing.T) {
	data := testData{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesIncludeWildcard tests using include file with wildcard.
func TestResolveHostFromFilesIncludeWildcard(t *testing.T) {
	data := testData{
		input:    "hana",
		expected: "127.0.0.1:2223",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesSimpleWildcard tests wildcard pattern.
func TestResolveHostFromFilesSimpleWildcard(t *testing.T) {
	data := testData{
		input:    "mytest.com",
		expected: "mytest.com:22",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesNoScope tests parameters that are not inside any host statement.
func TestResolveHostFromFilesNoScope(t *testing.T) {
	// This test configuration does not fit default configuration.
	// Therefore, it is going to use its own configuration.
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Port 2222
		Host *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testData{
		input:    "mytest.com",
		expected: "mytest.com:2222",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
}

// TestResolveHostFromFilesWithPortOverride tests port number overriding by users.
func TestResolveHostFromFilesWithPortOverride(t *testing.T) {
	data := testData{
		input:    "octopus:1",
		expected: "127.0.0.1:1",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesMatchedWithNegateExpr tests a case that host match a pattern with negate.
func TestResolveHostFromFilesMatchedNegateExpr(t *testing.T) {
	data := testData{
		input:    "eve.123.com",
		expected: "127.0.0.1:2225",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesNotMatchedNegateExpr tests a case that host match does not a pattern with negate.
func TestResolveHostFromFilesNotMatchedNegateExpr(t *testing.T) {
	data := testData{
		input:    "eve.google.com",
		expected: "eve.google.com:22",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesFormatHost tests format character %h.
func TestResolveHostFromFilesFormatHost(t *testing.T) {
	data := testData{
		input:    "test1",
		expected: "test1.google.com:2224",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesNotMatchedIpv4 tests a case that a ipv4 address does not match anything.
func TestResolveHostFromFilesNotMatchedIpv4(t *testing.T) {
	data := testData{
		input:    "127.0.0.1:2345",
		expected: "127.0.0.1:2345",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesNotMatchedIpv6 tests a case that a ipv6 address does not match anything.
func TestResolveHostFromFilesNotMatchedIpv6(t *testing.T) {
	data := testData{
		input:    "0:0:0:0:0:ffff:7f00:2",
		expected: "0:0:0:0:0:ffff:7f00:2",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesMatchedIpv6 tests a case that a ipv6 address matches a pattern.
func TestResolveHostFromFilesMatchedIpv6(t *testing.T) {
	data := testData{
		input:    "0:0:0:0:0:ffff:7f00:1",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2222",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesMatchedIpv6WithBracket tests a case that a ipv6 address with bracket matches a pattern.
func TestResolveHostFromFilesMatchedIpv6WithBracket(t *testing.T) {
	data := testData{
		input:    "[0:0:0:0:0:ffff:7f00:1]",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2222",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesMatchedIpv6WithOverride tests a case that a ipv6 address with override.
func TestResolveHostFromFilesMatchedIpv6WithOverride(t *testing.T) {
	data := testData{
		input:    "[0:0:0:0:0:ffff:7f00:1]:2223",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2223",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesTwoFormat tests a case that hostname has consecutive format characters.
func TestResolveHostFromFilesMatchedIpv6Format(t *testing.T) {
	data := testData{
		input:    "zone",
		expected: "[fe00::1ff:ffff:7f00:1%zone]:2230",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesMatchCondIncludeTwoRules tests the case that the
// host name matches two patterns in a conditional Include.
// One for host name and one for port number.
func TestResolveHostFromFilesMatchCondIncludeTwoRules(t *testing.T) {
	data := testData{
		input:    "prefix_suffix",
		expected: "tworules.google.com:2231",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesMatchCondIncludeAndOutside tests the case that the
// host name matches pattern in a conditional Include and a pattern outside.
func TestResolveHostFromFilesMatchCondIncludeAndOutside(t *testing.T) {
	data := testData{
		input:    "hello22",
		expected: "hello22.c.googler.com:22",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesMatchCondInclude tests the case that the
// host name matches pattern in a conditional Include.
func TestResolveHostFromFilesMatchCondInclude(t *testing.T) {
	data := testData{
		input:    "hello1.google.com",
		expected: "hello1.google.com:2226",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesNotMatchCondInclude tests the case that the
// host name does not match pattern for conditional Include and
// the wildcard rule in the include will not have effect.
func TestResolveHostFromFilesNotMatchCondInclude(t *testing.T) {
	data := testData{
		input:    "h.google.com",
		expected: "h.google.com:22",
	}
	testForResolveHostFromDefaultConfig(t, &data)
}

// TestResolveHostFromFilesWithLoop tests if ResolveHostFromFiles detect a loop in Include statements.
func TestResolveHostFromFilesWithLoop(t *testing.T) {

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	configPath := filepath.Join(td, "config_loop")
	configContent1 := `Include %v
		Host hana
			HostName 127.0.0.1
			Port 2223
		`
	configContent1 = fmt.Sprintf(configContent1, configPath)

	if err := testutil.WriteFiles(td, map[string]string{
		"config_loop": configContent1,
	}); err != nil {
		t.Fatal(err)
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	if _, err := ResolveHostFromFiles("hana", fileParams); err == nil {
		t.Fatalf("Expecting a loop error in reading %v but get no error", configPath)
	}
}

// testResolveHostFromFiles runs a test that does not expect an error.
func testResolveHostFromFiles(t *testing.T, data *testData, fileParams []FileParam) {
	resolvedHost, err := ResolveHostFromFiles(data.input, fileParams)
	if err != nil {
		t.Fatalf("Encounter error while calling ResolveHostFromFiles(%q, %q): %v", data.input, fileParams, err)
	}
	t.Logf("ResolveHostFromFiles(%q) = %q", data.input, resolvedHost)
	if resolvedHost != data.expected {
		t.Fatalf("ResolveHostFromFiles(%q, %q) = %q; want %q", data.input, fileParams, resolvedHost, data.expected)
	}
}
