// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast/core/internal/logging"
)

// NewClient creates a Client from a list of devservers, DUT server or a TLW server.
// If dutServer is non-empty, DUTServiceClient is returned.
// If tlwServer is non-empty, TLWClient is returned.
// If devserver contains 1 or more element, RealClient is returned.
// If the oth are empty, PseudoClient is returned.
func NewClient(ctx context.Context, devservers []string,
	tlwServer, dutName, dutServer, swarmingTaskID, buildBucketID string) (Client, error) {
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

	// On CloudBots, lab devservers from -devservers are reachable from the DUT
	// and servo host (so they must remain in CloudStorage.Devservers()), but
	// are unreachable from the host container. When a local ephemeral devserver
	// is present, probe only loopback URLs so NewRealClient does not wait the
	// full 10s /check_health timeout.
	selected := selectDevserversForClient(devservers)
	if len(selected) < len(devservers) {
		logging.Infof(ctx, "CLOUDBOTS_LAB_DOMAIN is set; probing only loopback devservers %v of %v", selected, devservers)
	}
	devservers = selected

	const timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cl := NewRealClient(ctx, devservers,
		&RealClientOptions{
			SwarmingTaskID: swarmingTaskID,
			BuildBucketID:  buildBucketID,
		})

	if exe, err := os.Executable(); err != nil {
		logging.Info(ctx, "Failed to get name of the running executable: ", err)
		logging.Infof(ctx, "Devserver status: %s", cl.Status())
	} else {
		logging.Infof(ctx, "Devserver connection status from %s: %s", exe, cl.Status())
	}
	return cl, nil
}

func selectDevserversForClient(devservers []string) []string {
	if os.Getenv("CLOUDBOTS_LAB_DOMAIN") == "" {
		return devservers
	}
	var loopback []string
	for _, rawURL := range devservers {
		if isLoopbackURL(rawURL) {
			loopback = append(loopback, rawURL)
		}
	}
	if len(loopback) == 0 {
		return devservers
	}
	return loopback
}

func isLoopbackURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
