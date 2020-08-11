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

// topConfigFile defines the name of the config file used in most unit tests in this file
var topConfigFile = "config"

// topConfigFileContent defines the content of the config file used in most unit tests in this file
var topConfigFileContent = `
	Include non_existing_file
	Include octopus_config
	Include hana_config
	Host test*
		HostName %%h.google.com
		Port 2224
	Host !*google.com eve*
		Include port_config
		HostName 127.0.0.1
	Host hello*
		user goodday
		Include hello_config
`

// octopusConfigFile defines the name of the config file for an alias name octopus
var octopusConfigFile = "octopus_config"

// octopusConfigContent defines the content of the config file for an alias name octopus
var octopusConfigContent = `
	Host "octopus"
		HostName 127.0.0.1
		Port 2222
`

// hanaConfigFile defines the name of the config file for an alias name hana
var hanaConfigFile = "hana_config"

// hanaConfigContent defines the content of the config file for an alias name hana
var hanaConfigContent = `
	Host "hana"
		HostName 127.0.0.1
		Port 2223
`

// portConfigFile defines the name of the config file for a port
var portConfigFile = "port_config"

// portConfigContent defines the content of the config file for a port
var portConfigContent = `
	Port 2225
`

// helloConfigFile defines the name of the config file for hosts with hello prefix
var helloConfigFile = "hello_config"

// helloConfigContent defines the content of the config file for hosts with hello prefix
var helloConfigContent = `
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

	data := testData{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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

	data := testData{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	fileParams := []FileParam{
		{
			Path:    configPath1,
			BaseDir: td,
		},
		{
			Path:    configPath2,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
}

// TestResolveHostFromFilesTwoBaseDirs tests reading two configuration files with two different base dir in one call.
func TestResolveHostFromFilesTwoBaseDirs(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configDir1 := filepath.Join(td, "config1_d")
	configDir2 := filepath.Join(td, "config2_d")
	configPath1 := filepath.Join(configDir1, "config")
	configPath2 := filepath.Join(configDir2, "config")

	configContent1 := fmt.Sprint("Include hana_config")
	configContent2 := fmt.Sprint("Include octopus_config")
	includeContent1 := `
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		`
	includeContent2 := `
		Host "octopus"
			HostName 127.0.0.1
			Port 2222
		`
	if err := testutil.WriteFiles(td, map[string]string{
		"config1_d/config":         configContent1,
		"config2_d/config":         configContent2,
		"config1_d/hana_config":    includeContent1,
		"config2_d/octopus_config": includeContent2,
	}); err != nil {
		t.Fatal(err)
	}

	data1 := testData{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	data2 := testData{
		input:    "hana",
		expected: "127.0.0.1:2223",
	}
	fileParams := []FileParam{
		{
			Path:    configPath1,
			BaseDir: configDir1,
		},
		{
			Path:    configPath2,
			BaseDir: configDir2,
		},
	}
	// make sure both hana and octopus can be read
	testResolveHostFromFiles(t, &data1, fileParams)
	testResolveHostFromFiles(t, &data2, fileParams)
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

	data := testData{
		input:    "octopus",
		expected: "127.0.0.1:2222",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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

	data := testData{
		input:    "hana",
		expected: "127.0.0.1:2223",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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

	data := testData{
		input:    "hana",
		expected: "127.0.0.1:2223",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "mytest.com",
		expected: "mytest.com:22",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "octopus:1",
		expected: "127.0.0.1:1",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "eve.123.com",
		expected: "127.0.0.1:2225",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "eve.google.com",
		expected: "eve.google.com:22",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "test1",
		expected: "test1.google.com:2224",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "127.0.0.1:2345",
		expected: "127.0.0.1:2345",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "0:0:0:0:0:ffff:7f00:2",
		expected: "0:0:0:0:0:ffff:7f00:2",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "0:0:0:0:0:ffff:7f00:1",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2222",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "[0:0:0:0:0:ffff:7f00:1]",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2222",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "[0:0:0:0:0:ffff:7f00:1]:2223",
		expected: "[0:0:0:0:0:ffff:7f00:1]:2223",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
	data := testData{
		input:    "zone",
		expected: "[fe00::1ff:ffff:7f00:1%zone]:2230",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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

	data := testData{
		input:    "hello1",
		expected: "hello1.c.googler.com:2228",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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

	data := testData{
		input:    "hello1",
		expected: "hello1.c.googler.com:22",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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

	data := testData{
		input:    "hello1.google.com",
		expected: "hello1.google.com:2226",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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

	data := testData{
		input:    "h.google.com",
		expected: "h.google.com:22",
	}
	fileParams := []FileParam{
		{
			Path:    configPath,
			BaseDir: td,
		},
	}
	testResolveHostFromFiles(t, &data, fileParams)
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
