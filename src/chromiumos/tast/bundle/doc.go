// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package bundle contains functionality shared by test bundles.

Test bundles are executables containing sets of local or remote tests.
They allow groups of tests to be built and deployed separately (because they
test different products based on the Chromium OS project, contain non-public
code, etc.).

Bundles are executed by test runners, which aggregate test results and
report them back to the tast command. Each test bundle call LocalDefault or
RemoteDefault (depending on the type of tests that the bundle contains) and pass
the returned status code to os.Exit.

Bundles write JSON-marshaled control messages to stdout. These messages are
relayed by the test runner back to the tast command. When a test bundle
encounters a (non-test) error, it writes a descriptive message to stderr and
exits with a nonzero status code. Otherwise, bundles exit with 0 (i.e. even
if one or more tests fail) -- runners learn about failed tests via EntityError
control messages.

The tast command contains a hardcoded assumption that the main function for
a local bundle named "foo" will exist at the Go import path
chromiumos/tast/local/bundles/foo, while the corresponding remote bundle's
code will be located at chromiumos/tast/remote/bundles/foo. Similarly, the
bundle must be installed by a package named
chromeos-base/tast-local-tests-foo or chromeos-base/tast-remote-tests-foo.

The bundle's code can be in an arbitrary repository, though.
*/
package bundle
