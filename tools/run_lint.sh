#!/bin/bash -e
# Copyright 2018 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

tast_root="$(cd "$(dirname "$(readlink -e "$0")")/.."; pwd)"

GOPATH="${tast_root}" go install chromiumos/cmd/tast-lint

exec "${tast_root}/bin/tast-lint" "$@"
