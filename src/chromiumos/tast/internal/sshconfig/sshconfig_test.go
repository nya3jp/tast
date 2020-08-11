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

    # Rule 1
    Host octopus
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

	configContent := `Include %v
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
		    HostName %%h.c.googlers.com
		Host *
		    Port 22
		`
	configContent = fmt.Sprintf(configContent, includePath1, includePath2)
	includePathContent1 := `Host octopus
		HostName 127.0.0.1
		Port 2222
		`
	includePathContent2 := "Port 2225\n"

	if err := testutil.WriteFiles(td, map[string]string{
		"config":           configContent,
		"octopus_config.d": includePathContent1,
		"port_config.d":    includePathContent2,
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
	}
	expected := [...]string{
		"127.0.0.1:2222",           // rule 1
		"127.0.0.1:2223",           // rule 2
		"mytest.com:22",            // rule 6
		"127.0.0.1:1",              // rule 1 + override
		"hello1.c.googlers.com:22", // rule 5
		"127.0.0.1:2225",           // rule 4
		"eve.google.com:22",        // rule 6
		"mytest.google.com:22",     // rule 6
		"127.0.0.1:2345",           // use as is because no rule matched
		"test.google.com:3",        // rule 3 + override
		"test1.google.com:2224",    // rule 3
	}
	for i, h := range inputs {
		realHost := GetRealHostFromFiles(h, []string{configPath})
		t.Logf("%v -- %v\n", h, realHost)
		if realHost != expected[i] {
			t.Errorf("Error: expected to get %v but get %v\n", expected[i], realHost)
		}
	}
}
