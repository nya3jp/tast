// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"os"
	"os/user"
	"strconv"
	"syscall"

	"chromiumos/tast/common/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LogPerms,
		Desc: "Checks permissions of logging-related files",
	})
}

func LogPerms(s *testing.State) {
	u, err := user.Lookup("syslog")
	if err != nil {
		s.Fatal("No syslog user:", err)
	}
	g, err := user.LookupGroup("syslog")
	if err != nil {
		s.Fatal("No syslog group:", err)
	}

	if u.Gid != g.Gid {
		s.Error("syslog user's primary group (%s) isn't syslog (%s)", u.Gid, g.Gid)
	}

	if fi, err := os.Stat("/var/log"); err != nil {
		s.Error(err)
	} else {
		if fi.Mode()&os.ModeSticky == 0 {
			s.Error("/var/log doesn't have sticky bit set")
		}
		if gid := fi.Sys().(*syscall.Stat_t).Gid; strconv.Itoa(int(gid)) != g.Gid {
			s.Error("/var/log not owned by syslog group (%d vs. %s)", gid, g.Gid)
		}
	}

	if fi, err := os.Stat("/var/log/messages"); err != nil {
		s.Error(err)
	} else {
		if uid := fi.Sys().(*syscall.Stat_t).Uid; strconv.Itoa(int(uid)) != u.Uid {
			s.Error("/var/log/messages not owned by syslog user (%d vs. %s)", uid, u.Uid)
		}
	}
}
