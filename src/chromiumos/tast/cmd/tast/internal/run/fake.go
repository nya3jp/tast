// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import "chromiumos/tast/testing"

type fakeOption func()

type fake struct {
	localTest  map[string][]*testing.Test
	remoteTest []*testing.Test
}

func setUpFake() {

}
