// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
	Package runner provides functionality shared by test runners.

	Test runners are executables that run one or more test bundles and
	aggregate the results. Runners are executed by the tast command.

	There is a local test runner that executes local bundles on-device and a
	remote test runner that executes remote bundles on the system where the
	tast command is running (e.g. a developer's workstation).
*/
package runner
