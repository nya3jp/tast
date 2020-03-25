#!/bin/bash -e
# Copyright 2018 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Absolute path to the root directory of the repo checkout.
declare -r repo_root="$(cd "$(dirname "$(readlink -e "$0")")/../../../.."; pwd)"

declare -r tast_root="${repo_root}/src/platform/tast"
declare -r chroot_gopath="${repo_root}/chroot/usr/lib/gopath"

export GOBIN="${tast_root}/bin"
export GOPATH="${tast_root}:${chroot_gopath}"

if ! go install chromiumos/tast/cmd/tast-lint; then
  echo "*** Failed to build tast-lint. Please run update_chroot."
  exit 1
fi

exec "${tast_root}/bin/tast-lint" "$@"
