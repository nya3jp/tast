// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package externalservers provides a utility to parse external servers information.
package externalservers

import (
	"strings"

	"chromiumos/tast/errors"
)

// ParseServerVarValues parse the values of server related run time variables
// and return a role to server host map.
// Example input: ":addr1:22,cd1:addr2:2222"
// Example output: { "": "addr1:22", "cd1": "addr2:2222" }
func ParseServerVarValues(inputValue string) (result map[string]string, err error) {
	result = make(map[string]string)
	if len(inputValue) == 0 {
		return result, nil
	}
	serverInfos := strings.Split(inputValue, ",")
	for _, serverInfo := range serverInfos {
		roleHost := strings.SplitN(serverInfo, ":", 2)
		if len(roleHost) != 2 {
			return nil, errors.Errorf("invalid role/server value %s", serverInfo)
		}
		result[roleHost[0]] = roleHost[1]
	}
	return result, nil
}
