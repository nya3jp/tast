// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"io/ioutil"
	"strings"

	"chromiumos/tast/common/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DataFiles,
		Desc: "Demonstrates how to use data files",
		Data: []string{
			"data_files_common.txt",
			"data_files_{arch}.txt",
		},
	})
}

func DataFiles(s *testing.State) {
	p := func(p string) {
		b, err := ioutil.ReadFile(s.DataPath(p))
		if err != nil {
			s.Error(err)
		} else {
			s.Logf("Read data file %s: %s", p, strings.TrimRight(string(b), "\n"))
		}
	}
	p("data_files_common.txt")
	p("data_files_{arch}.txt")
}
