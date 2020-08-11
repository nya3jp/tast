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

// testDataType stores input data and expected output.
type testDataType struct {
	input    string
	expected string
}

// TestResolveHostFromFilesIncludeTop tests using include file at the top level.
func TestResolveHostFromFilesIncludeTop(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath := filepath.Join(td, "octopus_config")

	configContent := `Include %v
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		`
	configContent = fmt.Sprintf(configContent, includePath)
	includePathContent := `
		Host "octopus"
			HostName 127.0.0.1
			Port 2222
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":         configContent,
		"octopus_config": includePathContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesTwoFiles tests reading two configuration files in one call.
func TestResolveHostFromFilesTwoFiles(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath1 := filepath.Join(td, "config")
	configPath2 := filepath.Join(td, "octopus_config")

	configContent1 := `
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		`
	configContent2 := `
		Host "octopus"
			HostName 127.0.0.1
			Port 2222
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":         configContent1,
		"octopus_config": configContent2,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	configPaths := []string{configPath1, configPath2}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesIncludeWildcard tests using include file with wildcard.
func TestResolveHostFromFilesIncludeWildcard(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath := filepath.Join(td, "octopus_*")

	configContent := `Include %v
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		`
	configContent = fmt.Sprintf(configContent, includePath)
	includePathContent := `
		Host "octopus"
			HostName 127.0.0.1
			Port 2222
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":         configContent,
		"octopus_config": includePathContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesIncludeNonExistingFile tests using include a non-exiting file without getting errors.
func TestResolveHostFromFilesIncludeNonExistingFile(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")

	configContent := `Include NonExistingFile
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "hana",
		expected: "127.0.0.1:2223",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesAfterIncludeTop make sure included data does not cause problem in other configuration.
func TestResolveHostFromFilesAfterIncludeTop(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath := filepath.Join(td, "octopus_config")

	configContent := `Include %v
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		`
	configContent = fmt.Sprintf(configContent, includePath)
	includePathContent := `
		Host "octopus"
			HostName 127.0.0.1
			Port 2222
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":           configContent,
		"octopus_config.d": includePathContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "hana",
		expected: "127.0.0.1:2223",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesSimpleWildcard tests wildcard pattern.
func TestResolveHostFromFilesSimpleWildcard(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host hana
		    HostName 127.0.0.1
			Port 2223
		Host *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "mytest.com",
		expected: "mytest.com:22",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesNoScope tests parameters that are not inside any host statement.
func TestResolveHostFromFilesNoScope(t *testing.T) {
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
	data := testDataType{
		input:    "mytest.com",
		expected: "mytest.com:2222",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesWithPortOverride tests port number overriding by users.
func TestResolveHostFromFilesWithPortOverride(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host octopus
		    HostName 127.0.0.1
			Port 2222
		Host *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "octopus:1",
		expected: "127.0.0.1:1",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesMatchedWithNegateExpr tests a case that host match a pattern with negate.
func TestResolveHostFromFilesMatchedNegateExpr(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host !*google.com  eve*
			Port 2225
			HostName 127.0.0.1
		Host *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "eve.123.com",
		expected: "127.0.0.1:2225",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesNotMatchedNegateExpr tests a case that host match does not a pattern with negate.
func TestResolveHostFromFilesNotMatchedNegateExpr(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host !*google.com  eve*
			Port 2225
			HostName 127.0.0.1
		Host *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "eve.google.com",
		expected: "eve.google.com:22",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesFormatHost tests format character %h.
func TestResolveHostFromFilesFormatHost(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host hana
			HostName 127.0.0.1
			Port 2223
		Host test*
			HostName %h.google.com
			Port 2224
		Host !*:* *
			Port 22
	`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "test1",
		expected: "test1.google.com:2224",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesNotMatchedIpv4 tests a case that a ipv4 address does not match anything.
func TestResolveHostFromFilesNotMatchedIpv4(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host !*google.com  eve*
			Port 2225
			HostName 127.0.0.1
		Host !"*:" *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "127.0.0.1:2345",
		expected: "127.0.0.1:2345",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesNotMatchedIpv6 tests a case that a ipv6 address does not match anything.
func TestResolveHostFromFilesNotMatchedIpv6(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host !*google.com  eve*
			Port 2225
			HostName 127.0.0.1
		Host !*:* *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "0:0:0:0:0:ffff:7f00:2",
		expected: "0:0:0:0:0:ffff:7f00:2",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesMatchedIpv6 tests a case that a ipv6 address matches a pattern.
func TestResolveHostFromFilesMatchedIpv6(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host 0:0:0:0:0:*:1
			Port 2222
		Host *zone
			HostName fe00::1ff:ffff:7f00:1%%%h
			Port 2230
		Host !*:* *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "0:0:0:0:0:ffff:7f00:1",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2222",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesMatchedIpv6WithBracket tests a case that a ipv6 address with bracket matches a pattern.
func TestResolveHostFromFilesMatchedIpv6WithBracket(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host 0:0:0:0:0:ffff:*1
			Port 2222
		Host *zone
			HostName fe00::1ff:ffff:7f00:1%%%h
			Port 2230
		Host !*:* *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "[0:0:0:0:0:ffff:7f00:1]",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2222",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesMatchedIpv6WithOverride tests a case that a ipv6 address with override.
func TestResolveHostFromFilesMatchedIpv6WithOverride(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host 0:0:0:0:0:ffff:*1
			Port 2222
		Host *zone
			HostName fe00::1ff:ffff:7f00:1%%%h
			Port 2230
		Host !*:* *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "[0:0:0:0:0:ffff:7f00:1]:2223",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2223",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesTwoFormat tests a case that hostname has consecutive format characters.
func TestResolveHostFromFilesMatchedIpv6Format(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	configContent := `
		Host 0:0:0:0:0:ffff:*1
			Port 2222
		Host *zone
			HostName fe00::1ff:ffff:7f00:1%%%h
			Port 2230
		Host !*:* *
			Port 22
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config": configContent,
	}); err != nil {
		t.Fatal(err)
	}
	data := testDataType{
		input:    "zone",
		expected: "[fe00::1ff:ffff:7f00:1%zone]:2230",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesMatchCondIncludeTwoRules tests the case that the
// host name matches two patterns in a conditional Include.
// One for host name and one for port number.
func TestResolveHostFromFilesMatchCondIncludeTwoRules(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath := filepath.Join(td, "hello_config")

	configContent := `
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		Host test*
		    HostName %%h.google.com
		    Port 2224
		Host !*google.com eve*
		    Port 2225
		    HostName 127.0.0.1
		Host hello*
		    user goodday
			Include %v
		Host !*:* *
			Port 22
		`
	configContent = fmt.Sprintf(configContent, includePath)

	includePathContent := `
		Host *google.com
			HostName %h
			Port 2226
		Host "!*googler.com" *.*
			HostName %h
			Port 2227
		Host !"*googler.com" *
			HostName %h.c.googler.com
		Host *
			Port 2228
	`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":       configContent,
		"hello_config": includePathContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "hello1",
		expected: "hello1.c.googler.com:2228",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesMatchCondIncludeAndOutside tests the case that the
// host name matches pattern in a conditional Include and a pattern outside.
func TestResolveHostFromFilesMatchCondIncludeAndOutside(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath := filepath.Join(td, "hello_config")

	configContent := `
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		Host test*
		    HostName %%h.google.com
		    Port 2224
		Host !*google.com eve*
		    Port 2225
		    HostName 127.0.0.1
		Host hello*
		    user goodday
			Include %v
		Host !*:* *
			Port 22
		`
	configContent = fmt.Sprintf(configContent, includePath)

	includePathContent := `
		Host *google.com
			HostName %h
			Port 2226
		Host "!*googler.com" *.*
			HostName %h
			Port 2227
		Host !"*googler.com" *
			HostName %h.c.googler.com
	`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":       configContent,
		"hello_config": includePathContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "hello1",
		expected: "hello1.c.googler.com:22",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesMatchCondInclude tests the case that the
// host name matches pattern in a conditional Include.
func TestResolveHostFromFilesMatchCondInclude(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath := filepath.Join(td, "hello_config")

	configContent := `
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		Host test*
		    HostName %%h.google.com
		    Port 2224
		Host !*google.com eve*
		    Port 2225
		    HostName 127.0.0.1
		Host hello*
		    user goodday
			Include %v
		Host !*:* *
			Port 22
		`
	configContent = fmt.Sprintf(configContent, includePath)

	includePathContent := `
		Host *google.com
			HostName %h
			Port 2226
		Host "!*googler.com" *.*
			HostName %h
			Port 2227
		Host !"*googler.com" *
			HostName %h.c.googler.com
		Host *
			Port 2228
	`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":       configContent,
		"hello_config": includePathContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "hello1.google.com",
		expected: "hello1.google.com:2226",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
}

// TestResolveHostFromFilesNotMatchCondInclude tests the case that the
// host name does not match pattern for conditional Include and
// the wildcard rule in the include will not have effect.
func TestResolveHostFromFilesNotMatchCondInclude(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath := filepath.Join(td, "hello_config")

	configContent := `
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		Host !*google.com eve*
		    Port 2225
		    HostName 127.0.0.1
		Host hello*
		    user goodday
			Include %v
		Host !*:* *
			Port 22
		`
	configContent = fmt.Sprintf(configContent, includePath)

	includePathContent := `
		Host *google.com
			HostName %h
			Port 2226
		Host "!*googler.com" *.*
			HostName %h
			Port 2227
		Host !"*googler.com" *
			HostName %h.c.googler.com
		Host *
			Port 2228
	`
	if err := testutil.WriteFiles(td, map[string]string{
		"config":       configContent,
		"hello_config": includePathContent,
	}); err != nil {
		t.Fatal(err)
	}

	data := testDataType{
		input:    "h.google.com",
		expected: "h.google.com:22",
	}
	configPaths := []string{configPath}
	testResolveHostFromFiles(&data, configPaths, t)
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
	configPaths := []string{configPath}
	if _, err := ResolveHost("hana", configPaths...); err == nil {
		t.Errorf("expecting a loop error in reading %v but get no error", configPath)
	}
}

// testResolveHostFromFiles runs a test that does not expect an error.
func testResolveHostFromFiles(data *testDataType, configPaths []string, t *testing.T) {
	resolvedHost, err := ResolveHost(data.input, configPaths...)
	if err != nil {
		t.Errorf("encounter error while reading %v: %v", configPaths, err)
	}
	t.Logf("%v -- %v\n", data.input, resolvedHost)
	if resolvedHost != data.expected {
		t.Errorf("expected to get %v but get %v", data.expected, resolvedHost)
	}
}
