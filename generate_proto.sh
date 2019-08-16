#!/bin/bash
# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

if [[ ! -f  /etc/cros_chroot_version ]]; then
    echo "This script must be run in Chrome OS chroot" >&2
    exit 1
fi

cd "$(dirname "$0")"

set -ex

protoc -I src:../../../chromite/infra/proto/src --go_out=src \
    ../../../chromite/infra/proto/src/device/*.proto
protoc -I src:../../../chromite/infra/proto/src --go_out=src \
    src/chromiumos/tast/testing/*.proto
