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
	// DownloadGS downloads a file on Google Cloud Storage at gsURL.
	// gsURL must have a "gs://" scheme.
	DownloadGS(ctx context.Context, w io.Writer, gsURL string) (size int64, err error)
}
