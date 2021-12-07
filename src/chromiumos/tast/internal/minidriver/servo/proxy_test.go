// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"testing"
)

func TestSplitHostPort(t *testing.T) {
	for _, tc := range []struct {
		input           string
		expectedHost    string
		expectedPort    int
		expectedSSHPort int
		expectErr       bool
	}{
		{"", "localhost", 9999, 22, false},
		{":ssh:", "", 0, 0, true},
		{":ssh:33", "localhost", 9999, 33, false},
		{"rutabaga", "rutabaga", 9999, 22, false},
		{"rutabaga:ssh:33", "rutabaga", 9999, 33, false},
		{"rutabaga:1234", "rutabaga", 1234, 22, false},
		{"rutabaga:1234:ssh:33", "rutabaga", 1234, 33, false},
		{"rutabaga:localhost:1234", "", 0, 0, true},
		{":1234", "localhost", 1234, 22, false},
		{":1234:ssh:", "", 0, 0, true},
		{":1234:ssh:33", "localhost", 1234, 33, false},
		{"[::2]", "::2", 9999, 22, false},
		{"[::2]:ssh:33", "::2", 9999, 33, false},
		{"[::2]:1234", "::2", 1234, 22, false},
		{"[::2]:1234:ssh:33", "::2", 1234, 33, false},
		{"[::2]:localhost:1234", "", 0, 0, true},
		{"::2", "", 0, 0, true},
		{"::2:1234", "", 0, 0, true},
		{"dut1-docker_servod", "dut1-docker_servod", 9999, 22, false},
		{"dut1-docker_servod:9998", "dut1-docker_servod", 9999, 22, false},
		{"dut1-docker_servod:9998::", "dut1-docker_servod", 9999, 22, false},
	} {
		actualHost, actualPort, actualSSHPort, err := splitHostPort(tc.input)
		if err != nil && !tc.expectErr {
			t.Errorf("splitHostPort(%q) returned unexpected error: %v", tc.input, err)
			continue
		}
		if err == nil && tc.expectErr {
			t.Errorf("splitHostPort(%q) unexpectedly succeeded %s %d %d", tc.input, actualHost, actualPort, actualSSHPort)
			continue
		}
		if actualHost != tc.expectedHost {
			t.Errorf("splitHostPort(%q) returned host %q; want %q", tc.input, actualHost, tc.expectedHost)
		}
		if actualPort != tc.expectedPort {
			t.Errorf("splitHostPort(%q) returned port %d; want %d", tc.input, actualPort, tc.expectedPort)
		}
		if actualSSHPort != tc.expectedSSHPort {
			t.Errorf("splitHostPort(%q) returned port %d; want %d", tc.input, actualSSHPort, tc.expectedSSHPort)
		}
	}
}
