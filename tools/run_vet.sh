#!/bin/bash -e
# Copyright 2022 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Absolute path to the root directory of the repo checkout.
readonly repo_root="$(cd "$(dirname "$(readlink -e "$0")")/../../../.."; pwd)"
readonly tast_root="${repo_root}/src/platform/tast"

export GOBIN="${tast_root}/bin"

goPackages=()
for x in "$@"
do
    if [[ "${x}" == *.go ]]
    then
       goPackages+=($(dirname "${x}"))
    fi
done

if [ "${#goPackages[@]}" -eq 0 ]; then
    echo "No go packages impacted, skipping 'go vet'"
    exit 0
fi

uniquePackages=($(for x in "${goPackages[@]}"; do echo "${x}"; done | sort -u))

exec "${tast_root}/tools/go.sh" "vet" \
                                "-unusedresult.funcs=errors.New,errors.Wrap,errors.Wrapf,fmt.Errorf,fmt.Sprint,fmt.Sprintf,sort.Reverse" \
                                "-printf.funcs=Log,Logf,Error,Errorf,Fatal,Fatalf,Wrap,Wrapf" \
                                "${uniquePackages[@]}"
