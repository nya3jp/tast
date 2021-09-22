// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debugger_test

import (
	"reflect"
	"testing"

	"chromiumos/tast/internal/debugger"
)

func TestRewriteDebugCommand(t *testing.T) {
	for _, tc := range []struct {
		debugPort int
		args      []string
		want      []string
	}{
		{
			debugPort: 0,
			args:      []string{"binary"},
			want:      []string{"binary"},
		}, {
			debugPort: 0,
			args:      []string{"binary", "arg1", "arg2"},
			want:      []string{"binary", "arg1", "arg2"},
		}, {
			debugPort: 2345,
			args:      []string{"binary"},
			want:      []string{"dlv", "exec", "binary", "--api-version=2", "--headless", "--listen=:2345", "--log-dest=/dev/null", "--"},
		}, {
			debugPort: 2345,
			args:      []string{"binary", "arg1", "arg2"},
			want:      []string{"dlv", "exec", "binary", "--api-version=2", "--headless", "--listen=:2345", "--log-dest=/dev/null", "--", "arg1", "arg2"},
		},
	} {
		name, args := debugger.RewriteDebugCommand(tc.debugPort, tc.args[0], tc.args[1:]...)
		got := append([]string{name}, args...)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("cfg.DebuggerPorts(%d, %q) = %q; want %q", tc.debugPort, tc.args, got, tc.want)
		}
	}
}
