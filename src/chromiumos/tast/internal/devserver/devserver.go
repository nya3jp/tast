// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"chromiumos/tast/internal/testcontext"
)

// NewClient creates a Client from a list of devservers or a TLW server.
// If tlwServer is non-empty, TLWClient is returned.
// If devserver contains 1 or more element, RealClient is returned.
// If the oth are empty, PseudoClient is returned.
func NewClient(ctx context.Context, devservers []string, tlwServer, dutName string) (Client, error) {
	if tlwServer != "" {
		if len(devservers) > 0 {
			return nil, fmt.Errorf("both tlwServer (%q) and devservers (%v) are set", tlwServer, devservers)
		}
		if dutName == "" {
			return nil, errors.New("dutName should be set when TLW server is used")
		}
		cl, err := NewTLWClient(ctx, tlwServer, dutName)
		if err != nil {
			return nil, err
		}
		testcontext.Log(ctx, "Devserver status: using TLW client")
		return cl, nil
	}
	if len(devservers) == 0 {
		testcontext.Log(ctx, "Devserver status: using pseudo client")
		return NewPseudoClient(), nil
	}

	const timeout = 3 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cl := NewRealClient(ctx, devservers, nil)
	testcontext.Logf(ctx, "Devserver status: %s", cl.Status())
	return cl, nil
}
