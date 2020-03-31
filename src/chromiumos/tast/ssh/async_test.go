// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoAsyncSuccess(t *testing.T) {
	ctx := context.Background()

	bodyCh := make(chan struct{})

	err := doAsync(ctx, func() error {
		// Sleep for short time to make sure doAsync is blocked until body returns.
		time.Sleep(10 * time.Millisecond)
		close(bodyCh)
		return nil
	}, func() {
		t.Error("clean was run despite body succeeded")
	})

	if err != nil {
		t.Error("doAsync failed: ", err)
	}

	select {
	case <-bodyCh:
	default:
		t.Error("doAsync returned before body finishes")
	}
}

func TestDoAsyncFailure(t *testing.T) {
	ctx := context.Background()

	myErr := errors.New("some failure")
	cleanCh := make(chan struct{})

	err := doAsync(ctx, func() error {
		return myErr
	}, func() {
		// Sleep for short time to make sure doAsync is blocked until clean returns.
		time.Sleep(10 * time.Millisecond)
		close(cleanCh)
	})

	if err != myErr {
		t.Errorf("doAsync returned %v; want %v", err, myErr)
	}

	select {
	case <-cleanCh:
	default:
		t.Error("doAsync returned before clean finishes")
	}
}

func TestDoAsyncCanceledPrior(t *testing.T) {
	// Create a canceled context.
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	cancel()

	bodyCh := make(chan struct{})
	cleanCh := make(chan struct{})

	err := doAsync(ctx, func() error {
		close(bodyCh)
		return nil
	}, func() {
		close(cleanCh)
	})

	if err != ctx.Err() {
		t.Errorf("doAsync returned %v; want %v", err, ctx.Err())
	}

	timeout := time.After(5 * time.Second)

	select {
	case <-bodyCh:
	case <-timeout:
		t.Error("body was not run")
	}

	select {
	case <-cleanCh:
	case <-timeout:
		t.Error("clean was not run")
	}
}

func TestDoAsyncCanceledInMiddle(t *testing.T) {
	// Create a context with some timeout so that the test does not block
	// forever on failures.
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	contCh := make(chan struct{})
	cleanCh := make(chan struct{})

	err := doAsync(ctx, func() error {
		cancel() // cancel the context while body is running
		<-contCh
		return nil
	}, func() {
		close(cleanCh)
	})

	if ctx.Err() == nil {
		t.Error("doAsync returned before the context is canceled")
	} else if err != ctx.Err() {
		t.Errorf("doAsync returned %v; want %v", err, ctx.Err())
	}

	select {
	case <-cleanCh:
		t.Error("clean was run before body finishes")
	default:
	}

	close(contCh)

	select {
	case <-cleanCh:
	case <-time.After(5 * time.Second):
		t.Error("clean was not run")
	}
}

func TestDoAsyncNilClean(t *testing.T) {
	ctx := context.Background()

	myErr := errors.New("some failure")

	err := doAsync(ctx, func() error {
		return myErr
	}, nil)

	if err != myErr {
		t.Errorf("doAsync returned %v; want %v", err, myErr)
	}
}
