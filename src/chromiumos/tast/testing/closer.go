// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import "context"

// Closer defines the type of a generic closer method.
type Closer func(context.Context) error

// Closers is a type of slice of Closer.
type Closers []Closer

// Append takes an interface an append it to the existing Closers slice.
func (c *Closers) Append(cl interface{}) {
	if cl == nil {
		return
	}
	if cls, ok := cl.(Closer); ok {
		*c = append(*c, cls)
	}
}

// CloseAll should be called with defer. It closes all the existing closers
func (c *Closers) CloseAll(ctx context.Context) {
	for i := len(*c) - 1; i >= 0; i-- {
		if err := (*c)[i](ctx); err != nil {
			ContextLog(ctx, "Failed to close the closer: ", err)
		}
	}
}
