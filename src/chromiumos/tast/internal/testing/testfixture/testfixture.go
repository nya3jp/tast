// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testfixture provides an implementation of testing.FixtureImpl to be
// used in unit tests.
package testfixture

import (
	"context"

	"chromiumos/tast/internal/testing"
)

// Fixture is a customizable implementation of testing.FixtureImpl.
type Fixture struct {
	setUpFunc    func(ctx context.Context, s *testing.FixtState) interface{}
	resetFunc    func(ctx context.Context) error
	preTestFunc  func(ctx context.Context, s *testing.FixtTestState)
	postTestFunc func(ctx context.Context, s *testing.FixtTestState)
	tearDownFunc func(ctx context.Context, s *testing.FixtState)
}

// Option is an option passed to New to customize
// a constructed Fixture.
type Option func(tf *Fixture)

// WithSetUp returns an option to set a function called back on SetUp.
func WithSetUp(f func(ctx context.Context, s *testing.FixtState) interface{}) Option {
	return func(tf *Fixture) {
		tf.setUpFunc = f
	}
}

// WithReset returns an option to set a function called back on Reset.
func WithReset(f func(ctx context.Context) error) Option {
	return func(tf *Fixture) {
		tf.resetFunc = f
	}
}

// WithPreTest returns an option to set a function called back on PreTest.
func WithPreTest(f func(ctx context.Context, s *testing.FixtTestState)) Option {
	return func(tf *Fixture) {
		tf.preTestFunc = f
	}
}

// WithPostTest returns an option to set a function called back on PostTest.
func WithPostTest(f func(ctx context.Context, s *testing.FixtTestState)) Option {
	return func(tf *Fixture) {
		tf.postTestFunc = f
	}
}

// WithTearDown returns an option to set a function called back on TearDown.
func WithTearDown(f func(ctx context.Context, s *testing.FixtState)) Option {
	return func(tf *Fixture) {
		tf.tearDownFunc = f
	}
}

// New creates a new fake fixture.
func New(opts ...Option) *Fixture {
	f := &Fixture{
		setUpFunc:    func(ctx context.Context, s *testing.FixtState) interface{} { return nil },
		resetFunc:    func(ctx context.Context) error { return nil },
		preTestFunc:  func(ctx context.Context, s *testing.FixtTestState) {},
		postTestFunc: func(ctx context.Context, s *testing.FixtTestState) {},
		tearDownFunc: func(ctx context.Context, s *testing.FixtState) {},
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// SetUp implements the fixture lifecycle method by a user-supplied callback.
func (f *Fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return f.setUpFunc(ctx, s)
}

// Reset implements the fixture lifecycle method by a user-supplied callback.
func (f *Fixture) Reset(ctx context.Context) error {
	return f.resetFunc(ctx)
}

// PreTest implements the fixture lifecycle method by a user-supplied callback.
func (f *Fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.preTestFunc(ctx, s)
}

// PostTest implements the fixture lifecycle method by a user-supplied callback.
func (f *Fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	f.postTestFunc(ctx, s)
}

// TearDown implements the fixture lifecycle method by a user-supplied callback.
func (f *Fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	f.tearDownFunc(ctx, s)
}
