// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package breakpad

import (
	"bytes"
	"context"
	"testing"
)

func TestPostMinidumpToCrashServer(t *testing.T) {
	const (
		crash  = "/tmp/crash/powerd.20171207.121018.1162.dmp"
		config = "/home/derat/trunk/src/third_party/autotest/files/global_config.ini"
	)
	servers, err := GetCrashServerURLs(config)
	if err != nil {
		t.Fatal(err)
	} else if len(servers) == 0 {
		t.Fatal("no servers")
	}
	symbols := GetSymbolsURL("reef-release/R64-10176.7.0")

	b := bytes.Buffer{}
	if err := PostMinidumpToCrashServer(context.Background(), servers[0], crash, symbols, &b); err != nil {
		t.Fatal(err)
	}
	println(b.String())
}
