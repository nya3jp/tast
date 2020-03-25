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
  echo "Usage: $(basename "$0")" \
      "[COMMIT_HASH_FOR_INFRA] [COMMIT_HASH_FOR_CONFIG]"
  exit 1
fi

# Directories in chromiumos/infra/proto/src to be imported to Tast.
# TODO(crbug.com/950346): Add labs dir when lab peripherals are supported.
infra_import_dirs=("device" "test" "test_platform/common" "lab" "manufacturing")

# Directories in src/config/proto to be imported to Tast.
config_import_dirs=("api")

tast_dir="$(dirname $0)/.."
infra_proto_dir="${tast_dir}/../../../chromite/infra/proto"
config_proto_dir="${tast_dir}/../../../src/config/proto"

# Use HEAD commit symlink by default.
arg=$1
if [[ -z "${arg}" ]]; then
  arg="HEAD"
fi
infra_commit=$(git -C "${infra_proto_dir}" rev-parse --verify "${arg}")
arg=$2
if [[ -z "${arg}" ]]; then
  arg="HEAD"
fi
config_commit=$(git -C "${config_proto_dir}" rev-parse --verify "${arg}")

# Take the snapshot of proto files.
mkdir -p "${tast_dir}/proto"
rm -rf "${tast_dir}/proto/infra"
git -C "${infra_proto_dir}" archive --prefix=infra/ --format=tar \
  "${infra_commit}":src "${infra_import_dirs[@]}" | \
  tar x --exclude=OWNERS --directory="${tast_dir}/proto"

git -C "${config_proto_dir}" archive --prefix=config/ --format=tar \
  "${config_commit}": "${config_import_dirs[@]}" | \
  tar x --exclude=OWNERS --directory="${tast_dir}/proto" || exit 1

# Create README file containing the commit hash.
cat - > "${tast_dir}/proto/infra/README"  << EOF
# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

This directory contains the snapshot of infra protofiles.
The commit hash is ${commit}.

This directory contains the snapshot of config protofiles.
The commit hash is ${config_commit}.
EOF

# Generate go proto bindings.
rm -rf "${tast_dir}/src/go.chromium.org/chromiumos/infra/proto"
for dir in "${infra_import_dirs[@]}"; do
  # Compile each directory separately because they contain different go
  # package names.
  find "${tast_dir}/proto/infra/${dir}" -type d -print0 |
  while IFS= read -r -d '' subdir; do
    mapfile -t proto_files < <(find "${subdir}" -name '*.proto' -maxdepth 1)
    if [ "${#proto_files[@]}" == 0 ]; then
      continue
    fi
    protoc \
      --go_out="${tast_dir}/src" \
      --proto_path="${tast_dir}/proto/infra/" \
      --proto_path="${tast_dir}/proto/config/" \
      "${proto_files[@]}"
  done
done

rm -rf "${tast_dir}/src/go.chromium.org/chromiumos/config/go"
for dir in "${config_import_dirs[@]}"; do
  find "${tast_dir}/proto/config/${dir}" -name '*.proto' -print0 |
  while IFS= read -r -d '' proto_file; do
    protoc \
      --go_out="${tast_dir}/src" \
      --proto_path="${tast_dir}/proto/config/" \
      "${proto_file}"
  done
done
