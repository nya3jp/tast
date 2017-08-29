// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"chromiumos/tast/common/testing"
)

// waitForAndroidBooted waits for the Android container to report that it's finished booting.
func waitForAndroidBooted(ctx context.Context) error {
	testing.ContextLog(ctx, "Waiting for Android to finish booting")
	f := func() bool {
		// TODO(derat): android-sh introduces a lot of overhead. Instead of calling it repeatedly,
		// make it run a while loop that checks the property.
		b, err := exec.Command("android-sh", "-c", "getprop sys.boot_completed").CombinedOutput()
		return err == nil && strings.TrimRight(string(b), "\n") == "1"
	}
	if err := poll(ctx, f); err != nil {
		return fmt.Errorf("Android didn't boot: %v", err)
	}
	return nil
}

// TODO(derat): Add an enablePlayStore function based on enable_play_store() in
// client/common_lib/cros/arc_util.py.
