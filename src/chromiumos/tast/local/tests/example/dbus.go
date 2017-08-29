// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"chromiumos/tast/common/testing"
	"chromiumos/tast/local/dbusutil"
	"chromiumos/tast/local/upstart"

	"github.com/godbus/dbus"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DBus,
		Desc: "Demonstrates how to use D-Bus",
	})
}

func DBus(s *testing.State) {
	conn, err := dbus.SystemBus()
	if err != nil {
		s.Fatal("failed to connect to system bus: ", err)
	}

	s.Logf("Checking that %s service is already available", dbusutil.PowerManagerName)
	if err = dbusutil.WaitForService(s.Context(), conn, dbusutil.PowerManagerName); err != nil {
		s.Errorf("Failed waiting for %v: %v", dbusutil.PowerManagerName, err)
	}

	const job = "powerd"
	s.Logf("Stopping %s job", job)
	if err = upstart.StopJob(job); err != nil {
		s.Errorf("Failed to stop %s: %v", job, err)
	}

	// Start a goroutine that waits for the service and then writes to channel.
	done := make(chan bool)
	go func() {
		if err = dbusutil.WaitForService(s.Context(), conn, dbusutil.PowerManagerName); err != nil {
			s.Errorf("Failed waiting for %v: %v", dbusutil.PowerManagerName, err)
		}
		done <- true
	}()

	s.Logf("Restarting %s job and waiting for %s service", job, dbusutil.PowerManagerName)
	if err = upstart.RestartJob("powerd"); err != nil {
		s.Errorf("Failed to start %s: %v", job, err)
	}
	<-done

	s.Logf("Asking powerd for screen brightness")
	var pct float64
	obj := conn.Object(dbusutil.PowerManagerName, dbusutil.PowerManagerPath)
	if err = obj.Call(dbusutil.PowerManagerInterface+".GetScreenBrightnessPercent", 0).Store(&pct); err != nil {
		s.Errorf("Failed to get screen brightness: %v", err)
	} else {
		s.Logf("Screen brightness is %.1f%%", pct)
	}
}
