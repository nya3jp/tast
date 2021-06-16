// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"os"
	"path/filepath"
)

var globalRegistry *Registry // singleton, initialized on first use

// GlobalRegistry returns a global registry containing tests
// registered by calls to AddTest.
func GlobalRegistry() *Registry {
	if globalRegistry == nil {
		globalRegistry = NewRegistry(filepath.Base(os.Args[0]))
	}
	return globalRegistry
}
