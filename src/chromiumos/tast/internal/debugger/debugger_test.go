// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debugger_test

import (
	"reflect"
	"testing"

	"chromiumos/tast/internal/debugger"
)

func dutEnv(args ...string) []string {
	return append(append([]string{"env"}, debugger.DlvDUTEnv...), args...)
}

func hostEnv(args ...string) []string {
	return append(append([]string{"env"}, debugger.DlvHostEnv...), args...)
}

func TestRewriteDebugCommand(t *testing.T) {
	for _, tc := range []struct {
		debugPort int
		env       []string
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
			env:       debugger.DlvHostEnv,
			args:      []string{"binary"},
			want:      hostEnv("dlv", "exec", "binary", "--api-version=2", "--headless", "--listen=:2345", "--log-dest=/dev/null", "--"),
		}, {
			debugPort: 2345,
			env:       debugger.DlvDUTEnv,
			args:      []string{"binary"},
			want:      dutEnv("dlv", "exec", "binary", "--api-version=2", "--headless", "--listen=:2345", "--log-dest=/dev/null", "--"),
		}, {
			debugPort: 2345,
			env:       debugger.DlvDUTEnv,
			args:      []string{"env", "a=b", "binary", "arg"},
			want:      dutEnv("a=b", "dlv", "exec", "binary", "--api-version=2", "--headless", "--listen=:2345", "--log-dest=/dev/null", "--", "arg"),
		}, {
			debugPort: 2345,
			env:       debugger.DlvDUTEnv,
			args:      []string{"binary", "arg1", "arg2"},
			want:      dutEnv("dlv", "exec", "binary", "--api-version=2", "--headless", "--listen=:2345", "--log-dest=/dev/null", "--", "arg1", "arg2"),
		}, {
			debugPort: 2345,
			env:       debugger.DlvHostEnv,
			args:      []string{"binary", "arg1", "arg2"},
			want:      hostEnv("dlv", "exec", "binary", "--api-version=2", "--headless", "--listen=:2345", "--log-dest=/dev/null", "--", "arg1", "arg2"),
		},
	} {
		name, args := debugger.RewriteDebugCommand(tc.debugPort, tc.env, tc.args[0], tc.args[1:]...)
		got := append([]string{name}, args...)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("cfg.DebuggerPorts(%d, %q) = %q; want %q", tc.debugPort, tc.args, got, tc.want)
		}
	}
}
