#!/bin/bash -e
# Copyright 2018 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Absolute path to the root directory of the repo checkout.
readonly repo_root="$(cd "$(dirname "$(readlink -e "$0")")/../../../.."; pwd)"
readonly tast_root="${repo_root}/src/platform/tast"
readonly tast_lint="go.chromium.org/tast/core/cmd/tast-lint"


export GOBIN="${tast_root}/bin"

if ! "${tast_root}/tools/go.sh" install "${tast_lint}"; then
  echo "*** Failed to build and install tast-lint."
  exit 1
fi

exec "${tast_root}/bin/tast-lint" "$@"
