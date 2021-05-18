// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"testing"
)

// TestCheckDuplicateVars makes sure that there are no duplicate variable in globalVars
func TestCheckDuplicateVars(t *testing.T) {
	var varsToDescs map[string]string
	varsToDescs = make(map[string]string)
	for _, v := range globalVars {
		if _, found := varsToDescs[v.Name]; found {
			t.Errorf("variable %v is defined more than once", v.Name)
			varsToDescs[v.Name] = v.Desc
		}
	}
}
