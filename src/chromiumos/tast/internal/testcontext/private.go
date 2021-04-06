// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext

import (
	"context"
)

// privateDataKey is the type of the key used for attaching a PrivateData
// to a context.Context.
type privateDataKey struct{}

// PrivateData contains information private to the framework.
type PrivateData struct {
	// WaitUntilReady carries the value of -waituntilready flag.
	WaitUntilReady bool
}

// WithPrivateData attaches PrivateData to context.Context.
// This function must not be exposed to user code.
func WithPrivateData(ctx context.Context, pd PrivateData) context.Context {
	return context.WithValue(ctx, privateDataKey{}, pd)
}

// PrivateDataFromContext extracts PrivateData from a context.Context.
// This function must not be exposed to user code.
func PrivateDataFromContext(ctx context.Context) (pd PrivateData, ok bool) {
	pd, ok = ctx.Value(privateDataKey{}).(PrivateData)
	return pd, ok
}
