// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestEmptySlice(t *testing.T) {
	const code = `package main
func main(){
  a := []string{}
  var b []string
  c := []int{}
  d := [2]float{}
  e := []bool{true, true, false,}
  for _, f := range e {
    if e == false {
      g := []newtype.new{}
      h := []*star{}
    }
  }
  i = []string{}
  same := [][2]string{}
  lst := []struct {
		ID string
	}{}
}`
	const path = "/src/chromiumos/tast/local/foo.go"
	f, fs := parse(code, path)
	issues := EmptySlice(fs, f)
	expects := []string{
		path + ":3:3: Use 'var' statement when you declare empty slice 'a'",
		path + ":5:3: Use 'var' statement when you declare empty slice 'c'",
		path + ":10:7: Use 'var' statement when you declare empty slice 'g'",
		path + ":11:7: Use 'var' statement when you declare empty slice 'h'",
		path + ":15:3: Use 'var' statement when you declare empty slice 'same'",
		path + ":16:3: Use 'var' statement when you declare empty slice 'lst'",
	}
	verifyIssues(t, issues, expects)
}
