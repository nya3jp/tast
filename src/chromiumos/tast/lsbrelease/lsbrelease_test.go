// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lsbrelease

import (
	"bytes"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	// Example from https://chromium.googlesource.com/chromiumos/docs/+/master/lsb-release.md
	const data = `# Normal line.
SOME_KEY=value
# Key and value with leading/trailing whitespace.
  WS_KEY  =    value
# Value with whitespace in the middle.
WS_VALUE = v a l u e
# Value with quotes don't get removed.
DOUBLE_QUOTES = "double"
SINGLE_QUOTES = 'sin gle'
RANDOM_QUOTES = '"
`
	exp := map[string]string{
		"SOME_KEY":      "value",
		"WS_KEY":        "value",
		"WS_VALUE":      "v a l u e",
		"DOUBLE_QUOTES": `"double"`,
		"SINGLE_QUOTES": `'sin gle'`,
		"RANDOM_QUOTES": `'"`,
	}
	kvs, err := Parse(bytes.NewBufferString(data))
	if err != nil {
		t.Fatal("Parse failed: ", err)
	}
	if !reflect.DeepEqual(kvs, exp) {
		t.Errorf("Parse returned %+v; want %+v", kvs, exp)
	}
}
