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

# Directories under /proto/ directory in chromiumos/config repository, to be
# imported to Tast.
config_import_dirs=("api")

tast_dir="$(dirname $0)/.."
infra_proto_dir="${tast_dir}/../../../chromite/infra/proto"
config_dir="${tast_dir}/../../../src/config"

# Use HEAD commit symlink by default.
if [[ -z "${infra_arg}" ]]; then
  infra_arg="HEAD"
fi
infra_commit=$(git -C "${infra_proto_dir}" rev-parse --verify "${infra_arg}")
if [[ -z "${config_arg}" ]]; then
  config_arg="HEAD"
fi
config_commit=$(git -C "${config_dir}" rev-parse --verify "${config_arg}")

# Take the snapshot of proto files.
mkdir -p "${tast_dir}/proto"
rm -rf "${tast_dir}/proto/{infra,config}"
git -C "${infra_proto_dir}" archive --prefix=infra/ --format=tar \
  "${infra_commit}":src "${infra_import_dirs[@]}" | \
  tar x --exclude=OWNERS --directory="${tast_dir}/proto" || exit 1

git -C "${config_dir}/proto" archive --prefix=config/ --format=tar \
  "${config_commit}": "${config_import_dirs[@]}" | \
  tar x --exclude=OWNERS --directory="${tast_dir}/proto" || exit 1

function create_readme() {
  name="$1"
  repo_name="$2"
  commit_hash="$3"
  cat - > "${tast_dir}/proto/${name}/README"  << EOF
# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

This directory contains the snapshot of ${repo_name} protofiles.
The commit hash is ${commit_hash}.
EOF
}

function generate_proto_bindings() {
  name="$1"
  extra_protoc_options="$2"
  shift 2
  import_dirs=("$@")
  rm -rf "${tast_dir}/src/go.chromium.org/chromiumos/${name}/proto"
  for dir in "${import_dirs[@]}"; do
    # Compile each directory separately because they contain different go
    # package names.
    find "${tast_dir}/proto/${name}/${dir}" -type d -print0 |
    while IFS= read -r -d '' subdir; do
      mapfile -t proto_files < <(find "${subdir}" -name '*.proto' -maxdepth 1)
      if [ "${#proto_files[@]}" == 0 ]; then
        continue
      fi
      # ${extra_protoc_options} is intentionally left not wrapped by quotes
      # because it can be empty.
      protoc \
        --go_out="${tast_dir}/src" \
        --proto_path="${tast_dir}/proto/${name}/" \
        ${extra_protoc_options} \
        "${proto_files[@]}"
    done
  done
}

create_readme infra chromiumos/infra/proto "${infra_commit}"
generate_proto_bindings infra --proto_path="${tast_dir}/proto/config/" \
    "${infra_import_dirs[@]}"
create_readme config src/config "${config_commit}"
generate_proto_bindings config "" "${config_import_dirs[@]}"
