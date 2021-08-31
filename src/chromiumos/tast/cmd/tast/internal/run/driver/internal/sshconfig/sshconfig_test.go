// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"chromiumos/tast/cmd/tast/internal/run/driver/internal/sshconfig"
	"chromiumos/tast/testutil"
)

// testForResolveHostFromDefaultConfig runs wiath a default test configuration that used by most unit tests for ResolveHostFromFiles.
func testForResolveHostFromDefaultConfig(t *testing.T, input, expected string) {
	// firstConfigFile defines the name of the first config file used in most unit tests in this file.
	firstConfigFile := "testdata/config1_d/config"
	// firstConfigFile defines the name of the second config file used in most unit tests in this file.
	secondConfigFile := "testdata/config2_d/config"
	// fileParams defines which config files to read and which base directory to used for include files.
	fileParams := []sshconfig.FileParam{
		{
			Path:    firstConfigFile,
			BaseDir: filepath.Dir(firstConfigFile),
		},
		{
			Path:    secondConfigFile,
			BaseDir: filepath.Dir(secondConfigFile),
		},
	}
	// testResolveHostFromFiles checks if ResolveHostFromFiles behaves as expected.
	testResolveHostFromFiles(t, input, expected, fileParams)
}

// TestResolveHostFromFilesIncludeTop tests using include file at the top level.
func TestResolveHostFromFilesIncludeTop(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "octopus", "127.0.0.1:2222")
}

// TestResolveHostFromFilesIncludeWildcard tests using include file with wildcard.
func TestResolveHostFromFilesIncludeWildcard(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "hana", "127.0.0.1:2223")
}

// TestResolveHostFromFilesSimpleWildcard tests wildcard pattern.
func TestResolveHostFromFilesSimpleWildcard(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "mytest.com", "mytest.com:22")
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
	fileParams := []sshconfig.FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, "mytest.com", "mytest.com:2222", fileParams)
}

// TestResolveHostFromFilesWithPortOverride tests port number overriding by users.
func TestResolveHostFromFilesWithPortOverride(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "octopus:1", "127.0.0.1:1")
}

// TestResolveHostFromFilesMatchedWithNegateExpr tests a case that host match a pattern with negate.
func TestResolveHostFromFilesMatchedNegateExpr(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "eve.123.com", "127.0.0.1:2225")
}

// TestResolveHostFromFilesNotMatchedNegateExpr tests a case that host match does not a pattern with negate.
func TestResolveHostFromFilesNotMatchedNegateExpr(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "eve.google.com", "eve.google.com:22")
}

// TestResolveHostFromFilesFormatHost tests format character %h.
func TestResolveHostFromFilesFormatHost(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "test1", "test1.google.com:2224")
}

// TestResolveHostFromFilesNotMatchedIpv4 tests a case that a ipv4 address does not match anything.
func TestResolveHostFromFilesNotMatchedIpv4(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "127.0.0.1:2345", "127.0.0.1:2345")
}

// TestResolveHostFromFilesNotMatchedIpv6 tests a case that a ipv6 address does not match anything.
func TestResolveHostFromFilesNotMatchedIpv6(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "0:0:0:0:0:ffff:7f00:2", "0:0:0:0:0:ffff:7f00:2")
}

// TestResolveHostFromFilesMatchedIpv6 tests a case that a ipv6 address matches a pattern.
func TestResolveHostFromFilesMatchedIpv6(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "0:0:0:0:0:ffff:7f00:1", "[0:0:0:0:0:ffff:7f00:1]:2222")
}

// TestResolveHostFromFilesMatchedIpv6WithBracket tests a case that a ipv6 address with bracket matches a pattern.
func TestResolveHostFromFilesMatchedIpv6WithBracket(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "[0:0:0:0:0:ffff:7f00:1]", "[0:0:0:0:0:ffff:7f00:1]:2222")
}

// TestResolveHostFromFilesMatchedIpv6WithOverride tests a case that a ipv6 address with override.
func TestResolveHostFromFilesMatchedIpv6WithOverride(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "[0:0:0:0:0:ffff:7f00:1]:2223", "[0:0:0:0:0:ffff:7f00:1]:2223")
}

// TestResolveHostFromFilesTwoFormat tests a case that hostname has consecutive format characters.
func TestResolveHostFromFilesMatchedIpv6Format(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "zone", "[fe00::1ff:ffff:7f00:1%zone]:2230")
}

// TestResolveHostFromFilesMatchCondIncludeTwoRules tests the case that the
// host name matches two patterns in a conditional Include.
// One for host name and one for port number.
func TestResolveHostFromFilesMatchCondIncludeTwoRules(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "prefix_suffix", "tworules.google.com:2231")
}

// TestResolveHostFromFilesMatchCondIncludeAndOutside tests the case that the
// host name matches pattern in a conditional Include and a pattern outside.
func TestResolveHostFromFilesMatchCondIncludeAndOutside(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "hello22", "hello22.c.googler.com:22")
}

// TestResolveHostFromFilesMatchCondInclude tests the case that the
// host name matches pattern in a conditional Include.
func TestResolveHostFromFilesMatchCondInclude(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "hello1.google.com", "hello1.google.com:2226")
}

// TestResolveHostFromFilesNotMatchCondInclude tests the case that the
// host name does not match pattern for conditional Include and
// the wildcard rule in the include will not have effect.
func TestResolveHostFromFilesNotMatchCondInclude(t *testing.T) {
	testForResolveHostFromDefaultConfig(t, "h.google.com", "h.google.com:22")
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
	fileParams := []sshconfig.FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	if _, err := sshconfig.ResolveHostFromFiles("hana", fileParams); err == nil {
		t.Fatalf("Expecting a loop error in reading %v but get no error", configPath)
	}
}

// testResolveHostFromFiles runs a test that does not expect an error.
func testResolveHostFromFiles(t *testing.T, input, expected string, fileParams []sshconfig.FileParam) {
	resolvedHost, err := sshconfig.ResolveHostFromFiles(input, fileParams)
	if err != nil {
		t.Fatalf("Encounter error while calling ResolveHostFromFiles(%q, %q): %v", input, fileParams, err)
	}
	t.Logf("ResolveHostFromFiles(%q) = %q", input, resolvedHost)
	if resolvedHost != expected {
		t.Fatalf("ResolveHostFromFiles(%q, %q) = %q; want %q", input, fileParams, resolvedHost, expected)
	}
}
