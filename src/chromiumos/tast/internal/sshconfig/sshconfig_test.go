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

/*
 ** config file for testing

     # Rule 0
    Host 0:0:0:0:0:ffff:7f00:1
		Port 2222

    # Rule 1
    Host "octopus"
        HostName 127.0.0.1
        Port 2222

    # Rule 2
    Host hana
        HostName 127.0.0.1
        Port 2223

    # Rule 3
    Host test*
        HostName %h.google.com
        Port 2224

    # Rule 4
    Host !*google.com  eve*
        Port 2225
        HostName 127.0.0.1

    # Rule 5
    Host hello*
		user goodday
		# Rules 5.* are Include Contents
		# Rule 5.1
		Host *google.com
			HostName %h
			Port 2226
		# Rule 5.2
		Host "!*googler.com" *.*
			HostName %h
			Port 2227
		# Rule 5.3
		Host !"*googler.com" *
			HostName %h.c.googler.com
		# Rule 5.4
		Host *
			Port 2228

	# Line to be ignored
	Match host=""

    # Rule 6
    Host *
        Port 22

*/
func TestGetRealHostFromFiles(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	configPath := filepath.Join(td, "config")
	includePath1 := filepath.Join(td, "octopus_config.d")
	includePath2 := filepath.Join(td, "port_config.d")
	includePath3 := filepath.Join(td, "hello_config.d")
	configPathOther := filepath.Join(td, "config_other")

	configContent1 := `Include %v
		Host hana
		    HostName 127.0.0.1
		    Port 2223
		Host test*
		    HostName %%h.google.com
		    Port 2224
		Host !*google.com eve*
		    Include %v
		    HostName 127.0.0.1
		Host hello*
		    user goodday
		    Include %v
		`
	configContent1 = fmt.Sprintf(configContent1, includePath1, includePath2, includePath3)
	includePathContent1 := `
		Host 0:0:0:0:0:ffff:7f00:1
    		Port 2222
		Host "octopus"
			HostName 127.0.0.1
			Port 2222
		`
	includePathContent2 := "Port 2225\n"

	configContent2 := `
		Host *
			Port 22
	`
	includePathContent3 := `
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
		"config":           configContent1,
		"octopus_config.d": includePathContent1,
		"port_config.d":    includePathContent2,
		"hello_config.d":   includePathContent3,
		"config_other":     configContent2,
	}); err != nil {
		t.Fatal(err)
	}

	inputs := [...]string{
		"octopus",
		"hana",
		"mytest.com",
		"octopus:1",
		"hello1",
		"eve.123.com",
		"eve.google.com",
		"mytest.google.com",
		"127.0.0.1:2345",
		"test:3",
		"test1",
		"0:0:0:0:0:ffff:7f00:1",
		"[0:0:0:0:0:ffff:7f00:1]",
		"[0:0:0:0:0:ffff:7f00:1]:2233",
		"hello1.google.com",
		"hello.c.googler.com",
		"hello.mytest.com",
	}
	expected := [...]string{
		"127.0.0.1:2222",               // rule 1
		"127.0.0.1:2223",               // rule 2
		"mytest.com:22",                // rule 6
		"127.0.0.1:1",                  // rule 1 + override
		"hello1.c.googler.com:2228",    // rule 5.3
		"127.0.0.1:2225",               // rule 4
		"eve.google.com:22",            // rule 6
		"mytest.google.com:22",         // rule 6
		"127.0.0.1:2345",               // use as is because no rule matched
		"test.google.com:3",            // rule 3 + override
		"test1.google.com:2224",        // rule 3
		"[0:0:0:0:0:ffff:7f00:1]:2222", // rule 0
		"[0:0:0:0:0:ffff:7f00:1]:2222", // rule 0
		"[0:0:0:0:0:ffff:7f00:1]:2233", // rule 0 + override
		"hello1.google.com:2226",       // rule 5.1
		"hello.c.googler.com:2228",     // rule 5.3 and 5.4
		"hello.mytest.com:2227",        // rule 5.2
	}
	configPaths := []string{configPath, configPathOther}
	for i, h := range inputs {
		resolvedHost, err := ResolveHostFromFiles(h, configPaths)
		if err != nil {
			t.Errorf("encounter error while reading %v: %v", configPaths, err)
		}
		t.Logf("%v -- %v\n", h, resolvedHost)
		if resolvedHost != expected[i] {
			t.Errorf("expected to get %v but get %v", expected[i], resolvedHost)
		}
	}
}
