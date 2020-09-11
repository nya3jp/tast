// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package devserver provides a client for devservers. For more information about
// devservers, see go/devserver-doc.
package devserver

import (
	"context"
	"io"
)

// Client is a client interface to communicate with devservers.
type Client interface {
	// Open opens a file on Google Cloud Storage at gsURL. gsURL must have a "gs://" scheme.
	// Callers are responsible to close the/ returned io.ReadCloser after use.
	// If the file does not exist, os.ErrNotExist is returned.
	Open(ctx context.Context, gsURL string) (io.ReadCloser, error)

	// TearDown should be called once when the Client is destructed,
	// regardless of whether Open was called or not.
	TearDown() error
}
