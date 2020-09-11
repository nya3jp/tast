// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"errors"
	"time"

	"chromiumos/tast/internal/logging"
)

// NewClient creates a Client from a list of devservers. If the list is empty,
// PseudoClient is returned. Otherwise RealClient is returned.
func NewClient(ctx context.Context, devservers []string, tlwServer, dutName string) (Client, error) {
	if len(devservers) == 0 {
		if tlwServer == "" {
			logging.ContextLog(ctx, "Devserver status: using pseudo client")
			return NewPseudoClient(), nil
		}
		if dutName == "" {
			return nil, errors.New("dutName should be set when TLW server is used")
		}
		cl, err := NewTLWClient(ctx, tlwServer, WithDutName(dutName))
		if err == nil {
			logging.ContextLog(ctx, "Devserver status: using TLW client")
		}
		return cl, err
	}

	const timeout = 3 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cl := NewRealClient(ctx, devservers, nil)
	logging.ContextLogf(ctx, "Devserver status: %s", cl.Status())
	return cl, nil
}
