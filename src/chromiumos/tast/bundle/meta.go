// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"reflect"
	"strings"

	"chromiumos/tast/testing"
)

type metaKeyType string

var metaKey metaKeyType = "meta" // key used to attach Meta to a context

const metaCategory = "meta" // category for remote tests exercising Tast, as in "meta.TestName"

// Meta contains information about how the "tast" process used to initiate testing was run.
// It is used by remote tests in the "meta" category that run the tast executable to test Tast's behavior.
type Meta struct {
	// TastPath contains the absolute path to the tast executable.
	TastPath string
	// Target contains information about the DUT as "[<user>@]host[:<port>]".
	Target string
	// Flags contains flags that should be passed to the tast command's "list" and "run" subcommands.
	RunFlags []string
}

// metaContext returns a new context derived from ctx with a Meta struct derived from args attached,
// if appropriate for t.
func metaContext(ctx context.Context, t *testing.Test, args *Args) context.Context {
	// Only tests in the "meta" category have access to this information.
	parts := strings.SplitN(t.Name, ".", 2)
	if len(parts) != 2 || parts[0] != metaCategory {
		return ctx
	}
	if reflect.DeepEqual(args.RemoteArgs, RemoteArgs{}) {
		return ctx
	}
	return context.WithValue(ctx, metaKey, &Meta{
		TastPath: args.RemoteArgs.TastPath,
		Target:   args.RemoteArgs.Target,
		RunFlags: args.RemoteArgs.RunFlags,
	})
}

// MetaFromContext may be called by remote tests in the "meta" category to get
// the Meta struct attached to ctx. The returned pointer may be nil if information
// is unavailable to the test.
func MetaFromContext(ctx context.Context) *Meta {
	m, _ := ctx.Value(metaKey).(*Meta)
	return m
}
