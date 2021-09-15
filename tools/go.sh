#!/bin/bash
# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Runs the go command with environment suitable for dealing with Tast code.

if ! which go > /dev/null; then
  echo "*** Golang go command was not available. Please do either:
  run update_chroot if you are running the command inside CrOS chroot, or
  install 'go' command (Go language) to a location listed in \$PATH otherwise."
  exit 1
fi

readonly trunk_dir="$(realpath -e "$(dirname "$0")/../../../..")"

# Go workspaces containing the Tast source code.
readonly src_dirs=(
  "${trunk_dir}/src/platform/tast"
  "${trunk_dir}/src/platform/tast-tests"
  "${trunk_dir}/src/platform/tast-tests-private"
)

if [[ -f "/etc/cros_chroot_version" ]]; then
  readonly chroot_dir=""
else
  readonly chroot_dir="${trunk_dir}/chroot"
fi

readonly gopath_dir="${chroot_dir}/usr/lib/gopath"

export GOPATH="$(IFS=:; echo "${src_dirs[*]}"):${gopath_dir}"

# Disable cgo and PIE on building Tast binaries. See:
# https://crbug.com/976196
# https://github.com/golang/go/issues/30986#issuecomment-475626018
export CGO_ENABLED=0
export GOPIE=0

# Disable Go modules. Go 1.16+ enables Go modules by default.
export GO111MODULE=off

exec go "$@"
