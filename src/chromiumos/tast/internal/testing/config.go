// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"

	"chromiumos/tast/dut"
)

// RuntimeConfig contains details about how an individual test should be run.
type RuntimeConfig struct {
	// DataDir is the directory in which the test's data files are located.
	DataDir string
	// OutDir is the directory to which the test will write output files.
	OutDir string
	// Vars contains names and values of out-of-band variables passed to tests at runtime.
	// Names must be registered in Test.Vars and values may be accessed using State.Var.
	Vars map[string]string
	// CloudStorage is a client to read files on Google Cloud Storage.
	CloudStorage *CloudStorage
	// RemoteData contains information relevant to remote tests.
	// This is nil for local tests.
	RemoteData *RemoteData
	// FixtValue is a value returned by a parent fixture.
	// It is nil if not available.
	FixtValue interface{}
	// FixtCtx is the context that lives as long as the fixture.
	// It can be accessed only from testing.FixtState.
	FixtCtx context.Context
	// PreCtx is the context that lives as long as the precondition.
	// It can be accessed only from testing.PreState.
	PreCtx context.Context
	// Purgeable is a list of file paths which are not used for now and thus
	// can be deleted if the disk space is low.
	Purgeable []string
}

// RemoteData contains information relevant to remote entities.
type RemoteData struct {
	// Meta contains information about how the tast process was run.
	Meta *Meta
	// RPCHint contains information needed to establish gRPC connections.
	RPCHint *RPCHint
	// DUT is an SSH connection shared among remote entities.
	DUT *dut.DUT
	// CompanionDUTs are other DUTs that can be used in remote test.
	CompanionDUTs map[string]*dut.DUT
}

// Meta contains information about how the "tast" process used to initiate testing was run.
// It is used by remote tests in the "meta" category that run the tast executable to test Tast's behavior.
type Meta struct {
	// TastPath contains the absolute path to the tast executable.
	TastPath string
	// Target contains information about the DUT as "[<user>@]host[:<port>]".
	// DEPRECATED: Use ConnectionSpec instead.
	Target string
	// Flags contains flags that should be passed to the tast command's "list" and "run" subcommands.
	RunFlags []string
	// ConnectionSpec contains information about the DUT as "[<user>@]host[:<port>]".
	ConnectionSpec string
}

// clone returns a deep copy of m.
func (m *Meta) clone() *Meta {
	mc := *m
	mc.RunFlags = append([]string{}, m.RunFlags...)
	return &mc
}

// RPCHint contains information needed to establish gRPC connections.
type RPCHint struct {
	// localBundleDir is the directory on the DUT where local test bundle executables are located.
	// This path is used by remote entities to invoke gRPC services in local test bundles.
	localBundleDir string
	// testVars holds all test variables and will pass to local bundle services.
	testVars map[string]string
}

// NewRPCHint create a new RPCHint struct.
func NewRPCHint(localBundleDir string, testVars map[string]string) *RPCHint {
	return &RPCHint{
		localBundleDir: localBundleDir,
		testVars:       testVars,
	}
}

// clone returns a deep copy of h.
func (h *RPCHint) clone() *RPCHint {
	hc := *h
	return &hc
}

// ExtractLocalBundleDir extracts localBundleDir from RPCHint.
func ExtractLocalBundleDir(h *RPCHint) string {
	return h.localBundleDir
}

// ExtractTestVars extracts test vars from RPCHint.
func ExtractTestVars(h *RPCHint) map[string]string {
	return h.testVars
}
