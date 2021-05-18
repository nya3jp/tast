// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// Var define an interface for global runtime variable types.
type Var interface {
	Unmarshal(data string) error
	Name() string
}
