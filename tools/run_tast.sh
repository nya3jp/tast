#!/bin/bash -e

# Copyright 2018 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Convenience script to make it easy for developers to directly run a tast
# executable from an autotest_server_package.tar.bz2 archive produced by a
# builder. This script is included in these archive files by cbuildbot.

if [[ $# -lt 2 || "$1" == '-h' || "$1" == '--help' ]]; then
  cat - << EOF
Usage: $(basename "$0") [flag]... [target] [pattern]...

Run one or more Tast tests against a Chrome OS test device using
binaries from an autotest_server_package.tar.bz2 archive.

Additional flags for "tast run" may be supplied, most notably:
  -keyfile=PATH     passwordless private SSH key to use
  -resultsdir=PATH  directory where results will be written

Positional arguments:
  target   DUT as "host", "host:port", or "user@host:port"
  pattern  test pattern as a test name (e.g. "login.Chrome"),
           wildcard (e.g. "power.*"), or parentheses-surrounded
           test attribute expression (e.g. "(bvt && chrome)")
EOF
  exit 1
fi

flags=()

# Try to find the testing SSH key. If -keyfile was passed as a positional
# argument, it will override the flag added here.
for dir in "${HOME}/chromeos" "${HOME}/chromiumos" "${HOME}/trunk"; do
  keyfile="${dir}/chromite/ssh_keys/testing_rsa"
  if [[ -e "${keyfile}" ]]; then
    flags+=("-keyfile=${keyfile}")
    break
  fi
done

# Files are expected to be located at the same paths relative to this script
# that are used within the archive file.
basedir=$(dirname "$0")
exec "${basedir}/tast" -verbose run -build=false \
  -remotebundledir="${basedir}/bundles/remote" \
  -remotedatadir="${basedir}/data" \
  -remoterunner="${basedir}/remote_test_runner" \
  -defaultvarsdir="${basedir}/vars" \
  "${flags[@]}" "$@"
