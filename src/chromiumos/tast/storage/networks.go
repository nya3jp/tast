// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"net"
)

// networks is a set of IP networks.
type networks []net.IPNet

// localNetworks returns the set of local IP networks.
func localNetworks() (networks, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var ns networks
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			return nil, err
		}

		for _, a := range addrs {
			_, n, err := net.ParseCIDR(a.String())
			if err != nil {
				return nil, err
			}
			ns = append(ns, *n)
		}
	}
	return ns, nil
}

// Contains checks if ip is within any of the networks.
func (ns networks) Contains(ip net.IP) bool {
	for _, n := range ns {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
