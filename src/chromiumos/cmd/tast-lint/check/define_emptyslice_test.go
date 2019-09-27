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
  a := []string{} // error
  var b []string // valid case
  c := []int{} // error
  d := [2]float{} // valid (not enpty)
  e := []bool{true, true, false,} // valid (not empty)
  for _, f := range e {
    if !f {
      g := []newtype.new{} // error
      h := []*star{} // error
    }
  }
  i = []string{} // valid (not define-declaration)
  same := [][2]string{} // error
  lst := []struct { // error
  	ID string
  }{}
  j = []struct { // valid (not define-declaration)
  	ID string
  }{}
  k, l := int, []float{} // error: "l" should be warned
  m, n = string, []string{} // valid (not define-declaration)
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
		path + ":22:6: Use 'var' statement when you declare empty slice 'l'",
	}
	verifyIssues(t, issues, expects)
}
