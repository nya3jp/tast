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

	The tast command passes the -report flag when executing test runners,
	which instructs runners to report progress by writing JSON-marshaled
	control messages to stdout. In this mode, the runner exits with status
	code 0 in almost all cases (the one exception being malformed arguments),
	since the result of the run is already communicated via control messages.

	When a test runner is executed manually without the -report flag, the
	runner instead logs human-readable progress to stdout. The runner exits
	with a nonzero status code if an error is encountered.
*/
package runner
