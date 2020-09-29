// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"net"
	"testing"
)

func TestParseIPAddressAndPort(t *testing.T) {
	testData := []struct {
		input string
		ip    net.IP
		port  int
	}{
		{"127.0.0.1:0", net.IP{127, 0, 0, 1}, 0},
		{"[::ffff]:12345", net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, 12345},
		{"127.0.0:2", nil, 0},
		{"127.0.0.1", nil, 0},
	}
	for _, td := range testData {
		ip, port, err := parseIPAddressAndPort(td.input)
		expectFail := td.ip == nil
		if expectFail {
			if err == nil {
				t.Errorf("%q succeeded unexpectedly", td.input)
			}
			continue
		}
		if !ip.Equal(td.ip) {
			t.Errorf("%q got %s want %s", td.input, ip, td.ip)
		}
		if port != td.port {
			t.Errorf("%q got %d want %d", td.input, port, td.port)
		}
		if err != nil {
			t.Errorf("%q failed unexpectedly: %s", td.input, err)
		}
	}
}
