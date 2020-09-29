// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"fmt"
	"net"
	"strconv"

	"chromiumos/tast/errors"
)

func parseIPAddressAndPort(s string) (net.IP, int, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "invalid host/port: %s", s)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, 0, fmt.Errorf("invalid IP address in host: %s", host)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "invalid port number: %s", portStr)
	}
	return ip, p, nil
}
