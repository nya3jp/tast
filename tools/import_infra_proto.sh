#!/bin/bash -e

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Convenience script to import infra proto snapshot into tast repository.
#
# This script takes the commit hash of chromiumos/infra/proto repository
# as an argument, 1) takes its snapshot under tast/proto/infra, and
# 2) geneerates go protobuf bindings.
# This script needs to run inside the chroot for protoc command and
# its go-plugin compatibility.

if [[ "$1" == "-h" || "$1" == "--help" ]]; then
  echo "Usage: $(basename "$0") [COMMIT_HASH]"
  exit 1
fi

# Directories in chromiumos/infra/proto/src to be imported to Tast.
# TODO(crbug.com/950346): Add labs dir when lab peripherals are supported.
import_dirs=("device")

tast_dir="$(dirname $0)/.."
infra_proto_dir="${tast_dir}/../../../chromite/infra/proto"

# Use HEAD commit symlink by default.
arg=$1
if [[ -z "$arg" ]]; then
  arg="HEAD"
fi
commit=`git -C "${infra_proto_dir}" rev-parse --verify $arg`

# Take the snapshot of proto files.
mkdir -p "${tast_dir}/proto"
rm -rf "${tast_dir}/proto/infra"
git -C "${infra_proto_dir}" archive --prefix=infra/ --format=tar \
  "${commit}":src "${import_dirs[@]}" | \
  tar x --exclude=OWNERS --directory="${tast_dir}/proto"

# Create README file containing the commit hash.
cat - > "${tast_dir}/proto/infra/README"  << EOF
# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

This directory contains the snapshot of infra protofiles.
The commit hash is ${commit}.
EOF

# Generate go proto bindings.
rm -rf "${tast_dir}/src/go.chromium.org/chromiumos/infra/proto"
proto_files=($(find "${tast_dir}"/proto/infra/ -name '*.proto'))
protoc \
  --go_out="${tast_dir}/src" \
  --proto_path="${tast_dir}/proto/infra/" \
  "${proto_files[@]}"
