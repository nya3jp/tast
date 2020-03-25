#!/bin/bash -e

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Convenience script to import infra proto snapshot into tast repository.
#
# This script takes the commit hash of chromiumos/infra/proto and
# src/config/proto repositories as an argument,
# 1) takes its snapshot under tast/proto/infra, and
# 2) geneerates go protobuf bindings.
# This script needs to run inside the chroot for protoc command and
# its go-plugin compatibility.

args=$(getopt c:i:h "$@")
set -- ${args}
for opt in "$@"
do
  echo $opt
  case ${opt} in
    -c)
      config_arg="$2"
      shift 2
      ;;
    -i)
      infra_arg="$2"
      shift 2
      ;;
    -h)
      echo "Usage: $(basename "$0")" \
          "[-i COMMIT_HASH_FOR_INFRA]" \
          "[-c COMMIT_HASH_FOR_CONFIG]"
      exit 1
  esac
done

# Directories in chromiumos/infra/proto/src to be imported to Tast.
# TODO(crbug.com/950346): Add labs dir when lab peripherals are supported.
infra_import_dirs=("device" "test" "test_platform/common" "lab" "manufacturing")

# Directories in chromiumos/config repository to be imported to Tast.
config_import_dirs=("api")

tast_dir="$(dirname $0)/.."
infra_proto_dir="${tast_dir}/../../../chromite/infra/proto"
config_proto_dir="${tast_dir}/../../../src/config/proto"

# Use HEAD commit symlink by default.
if [[ -z "${infra_arg}" ]]; then
  infra_arg="HEAD"
fi
infra_commit=$(git -C "${infra_proto_dir}" rev-parse --verify "${infra_arg}")
if [[ -z "${config_arg}" ]]; then
  config_arg="HEAD"
fi
config_commit=$(git -C "${config_proto_dir}" rev-parse --verify "${config_arg}")

# Take the snapshot of proto files.
mkdir -p "${tast_dir}/proto"
rm -rf "${tast_dir}/proto/{infra,config}"
git -C "${infra_proto_dir}" archive --prefix=infra/ --format=tar \
  "${infra_commit}":src "${infra_import_dirs[@]}" | \
  tar x --exclude=OWNERS --directory="${tast_dir}/proto" || exit 1

git -C "${config_proto_dir}" archive --prefix=config/ --format=tar \
  "${config_commit}": "${config_import_dirs[@]}" | \
  tar x --exclude=OWNERS --directory="${tast_dir}/proto" || exit 1

# Create README file containing the commit hash.
cat - > "${tast_dir}/proto/infra/README"  << EOF
# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

This directory contains the snapshot of chromiumos/infra/proto protofiles.
The commit hash is ${infra_commit}.

This directory contains the snapshot of chromiumos/config protofiles.
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
