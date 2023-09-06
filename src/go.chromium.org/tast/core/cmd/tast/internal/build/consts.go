// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

const (
	// LocalRunnerPkg is the Go package for local_test_runner.
	LocalRunnerPkg = "go.chromium.org/tast/core/cmd/local_test_runner"

	// RemoteRunnerPkg is the Go package for remote_test_runner.
	RemoteRunnerPkg = "go.chromium.org/tast/core/cmd/remote_test_runner"

	// LocalBundlePkgPathPrefix is the Go package path prefix for local test bundles.
	LocalBundlePkgPathPrefix = "go.chromium.org/tast-tests/cros/local/bundles"

	// RemoteBundlePkgPathPrefix is the Go package path prefix for remote test bundles.
	RemoteBundlePkgPathPrefix = "go.chromium.org/tast-tests/cros/remote/bundles"

	// LocalBundleBuildSubdir is a subdirectory used for compiled local test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	LocalBundleBuildSubdir = "local_bundles"

	// RemoteBundleBuildSubdir is a subdirectory used for compiled remote test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	RemoteBundleBuildSubdir = "remote_bundles"
)
