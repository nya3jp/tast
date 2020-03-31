// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import "context"

// doAsync runs a function in a goroutine asynchronously.
//
// Firstly, body is called regardless of whether ctx is already canceled or not.
//
// If body returns non-nil or ctx is canceled before body finishes, clean is
// called in the same goroutine as body after body finishes. clean can be used
// to clean up the effect of body. clean is not run if it is nil.
//
// The return value of doAsync is that of body if body finishes before ctx is
// canceled. Otherwise, ctx.Err() is returned.
func doAsync(ctx context.Context, body func() error, clean func()) (retErr error) {
	bodyCh := make(chan error, 1) // result of body is sent
	retCh := make(chan error, 1)  // result of doAsync is sent
	doneCh := make(chan struct{}) // closed when the goroutine finishes

	go func() {
		defer close(doneCh)
		bodyCh <- body()
		if err := <-retCh; err != nil && clean != nil {
			clean()
		}
	}()

	// Do not return until the goroutine finishes or ctx is canceled.
	defer func() {
		retCh <- retErr
		select {
		case <-doneCh:
		case <-ctx.Done():
		}
	}()

	// If ctx is already canceled, always return ctx.Err().
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	select {
	case err := <-bodyCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
