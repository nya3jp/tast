// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
	Package bundle contains functionality shared by test bundles.

	Test bundles are executables containing sets of local or remote tests.
	They allow groups of tests to be built and deployed separately (because they
	test different products based on the Chromium OS project, contain non-public
	code, etc.).

	Bundles are executed by test runners, which aggregate test results and report them
	back to the tast command.

	Each test bundle should pass os.Args[1:] to either Local or Remote (depending on the
	type of tests that the bundle contains) and pass the returned status code to os.Exit.
*/
package bundle
