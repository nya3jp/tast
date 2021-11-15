// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Build this file only when cgo is enabled.
//go:build cgo
// +build cgo

package testing

func init() {
	// Cgo must be disabled on building Tast binaries (crbug.com/976196).
	// The following line will give a build error if cgo is enabled.
	var cgoMustBeDisabledToBuildTast struct{}
}
