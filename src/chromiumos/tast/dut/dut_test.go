// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dut

import (
	"testing"

	"chromiumos/tast/ssh"
)

func TestCompanionDeviceHostnames(t *testing.T) {
	testcases := []struct {
		Host, Suffix, Result string
		ShouldFail           bool
	}{
		{
			Host:       "dut",
			Suffix:     "-suffix",
			Result:     "dut-suffix",
			ShouldFail: false,
		},
		{
			Host:       "dut.cros",
			Suffix:     "-suffix",
			Result:     "dut-suffix.cros",
			ShouldFail: false,
		},
		{
			Host:       "dut.domain.cros",
			Suffix:     "-s",
			Result:     "dut-s.domain.cros",
			ShouldFail: false,
		},
		// Check failure on IP.
		{
			Host:       "192.168.0.1",
			Suffix:     "-suffix",
			Result:     "",
			ShouldFail: true,
		},
		// With port.
		{
			Host:       "dut:1234",
			Suffix:     "-suffix",
			Result:     "dut-suffix",
			ShouldFail: false,
		},
		{
			Host:       "192.168.0.1:123",
			Suffix:     "-suffix",
			Result:     "",
			ShouldFail: true,
		},
		{
			Host:       "[2001:db8::1]:123",
			Suffix:     "-suffix",
			Result:     "",
			ShouldFail: true,
		},
	}
	for _, tc := range testcases {
		dut := DUT{sopt: ssh.Options{Hostname: tc.Host}}
		ret, err := dut.CompanionDeviceHostname(tc.Suffix)
		if tc.ShouldFail {
			if err == nil {
				t.Errorf("companionDeviceHostname(%q, %q) succeeded, which should fail",
					tc.Host, tc.Suffix)
			}
		} else if err != nil {
			t.Errorf("companionDeviceHostname(%q, %q) failed with err = %q",
				tc.Host, tc.Suffix, err.Error())
		} else if ret != tc.Result {
			t.Errorf("companionDeviceHostname(%q, %q) got %q, want %q",
				tc.Host, tc.Suffix, ret, tc.Result)
		}
	}
}
