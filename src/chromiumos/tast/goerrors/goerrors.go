// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package goerror bridges the tast/errors package and the original go errors
// package.

package goerrors

import "github.com/pkg/errors"

// Cause exposes errors.Cause in a program that defines its own "errors"
// package.
func Cause(err error) error {
	return errors.Cause(err)
}
