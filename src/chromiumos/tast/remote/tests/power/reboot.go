// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"chromiumos/tast/common/testing"
	"chromiumos/tast/remote/dut"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Reboot,
		Desc: "Verifies that system comes back after rebooting",
	})
}

func Reboot(s *testing.State) {
	d, ok := dut.FromContext(s.Context())
	if !ok {
		s.Fatal("Failed to get DUT")
	}

	s.Log("Rebooting DUT")
	if _, err := d.Run(s.Context(), "(sleep 1; reboot) &"); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	s.Log("Waiting for DUT to become unreachable")
	if err := d.WaitUnreachable(s.Context()); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable: ", err)
	}
	s.Log("DUT became unreachable (as expected)")

	s.Log("Reconnecting to DUT")
	if err := d.WaitReconnect(s.Context()); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
	s.Log("Reconnected to DUT")
}
