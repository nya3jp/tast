// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/internal/logging"
)

// NewClient creates a Client from a list of devservers, DUT server or a TLW server.
// If dutServer is non-empty, DUTServiceClient is returned.
// If tlwServer is non-empty, TLWClient is returned.
// If devserver contains 1 or more element, RealClient is returned.
// If the oth are empty, PseudoClient is returned.
func NewClient(ctx context.Context, devservers []string, tlwServer, dutName, dutServer string) (Client, error) {
	if dutServer != "" {
		if len(devservers) > 0 {
			return nil, fmt.Errorf("both dutServer (%q) and devservers (%v) are set", dutServer, devservers)
		}
		cl, err := NewDUTServiceClient(ctx, dutServer)
		if err != nil {
			return nil, err
		}
		logging.Info(ctx, "Devserver status: using DUT service client")
		return cl, nil
	}
	if tlwServer != "" {
		if len(devservers) > 0 {
			return nil, fmt.Errorf("both tlwServer (%q) and devservers (%v) are set", tlwServer, devservers)
		}
		cl, err := NewTLWClient(ctx, tlwServer, dutName)
		if err != nil {
			return nil, err
		}
		logging.Info(ctx, "Devserver status: using TLW client")
		return cl, nil
	}
	if len(devservers) == 0 {
		logging.Info(ctx, "Devserver status: using pseudo client")
		return NewPseudoClient(), nil
	}

	const timeout = 3 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cl := NewRealClient(ctx, devservers, nil)
	logging.Infof(ctx, "Devserver status: %s", cl.Status())
	return cl, nil
}
