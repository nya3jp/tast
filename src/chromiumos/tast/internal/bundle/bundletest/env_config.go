// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundletest

import (
	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/testing"
)

type config struct {
	localBundles  []*testing.Registry
	remoteBundles []*testing.Registry
	localData     map[string]string
	remoteData    map[string]string
	primaryDUT    *DUTConfig
	companionDUTs map[string]*DUTConfig
}

// DUTConfig contains configurations of a fake DUT.
type DUTConfig struct {
	ExtraSSHHandlers []fakesshserver.Handler
}

// Option can be passed to SetUp to customize the testing environment.
type Option func(*config)

// WithLocalBundles specifies fake local test bundles to be installed.
func WithLocalBundles(reg ...*testing.Registry) Option {
	return func(cfg *config) {
		cfg.localBundles = reg
	}
}

// WithRemoteBundles specifies fake remote test bundles to be installed.
func WithRemoteBundles(reg ...*testing.Registry) Option {
	return func(cfg *config) {
		cfg.remoteBundles = reg
	}
}

// WithLocalData specifies local data. m should be a mapping from filepath to
// file content, where filepath is relative to the local data directory.
func WithLocalData(m map[string]string) Option {
	return func(cfg *config) {
		cfg.localData = m
	}
}

// WithRemoteData specifies remote data. m should be a mapping from filepath to
// file content, where filepath is relative to the remote data directory.
func WithRemoteData(m map[string]string) Option {
	return func(cfg *config) {
		cfg.remoteData = m
	}
}

// WithPrimaryDUT specifies fake primary DUT configuration.
func WithPrimaryDUT(d *DUTConfig) Option {
	return func(cfg *config) {
		cfg.primaryDUT = d
	}
}

// WithCompanionDUTs specifies fake companion DUTs.
func WithCompanionDUTs(ds map[string]*DUTConfig) Option {
	return func(cfg *config) {
		cfg.companionDUTs = ds
	}
}
