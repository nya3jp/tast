// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/subcommands"
)

func TestRunError(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.cfg.KeyFile = "" // force SSH auth error
	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("Run() = %v; want %v", status, subcommands.ExitFailure)
	}

	const exp = "Failed to connect to" // error message returned by local()
	p := filepath.Join(td.cfg.ResDir, runErrorFilename)
	if b, err := ioutil.ReadFile(p); err != nil {
		t.Errorf("Failed to read %v: %v", p, err)
	} else if !strings.Contains(string(b), exp) {
		t.Errorf("%v contains %q; want substring %q", p, string(b), exp)
	}
}
