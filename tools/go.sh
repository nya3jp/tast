#!/bin/bash
# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Runs the go command with environment suitable for dealing with Tast code.

readonly trunk_dir="$(realpath -e "$(dirname "$0")/../../../..")"

# Go workspaces containing the Tast source code.
readonly src_dirs=(
  "${trunk_dir}/src/platform/tast"
  "${trunk_dir}/src/platform/tast-tests"
  "${trunk_dir}/src/platform/tast-tests-pita"
  "${trunk_dir}/src/platform/tast-tests-private"
)

readonly gopath_dir="${trunk_dir}/chroot/usr/lib/gopath"

export GOPATH="$(IFS=:; echo "${src_dirs[*]}"):${gopath_dir}"

# Disable cgo and PIE on building Tast binaries. See:
# https://crbug.com/976196
# https://github.com/golang/go/issues/30986#issuecomment-475626018
export CGO_ENABLED=0
export GOPIE=0

# Disable Go modules. Go 1.16+ enables Go modules by default.
export GO111MODULE=auto

exec go "$@"
