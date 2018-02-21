#!/bin/bash -e

# Copyright 2017 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This script quickly builds the tast executable or its unit tests within a
# Chrome OS chroot.

# Personal Go workspace used to cache compiled packages.
readonly GOHOME="${HOME}/go"

# Directory where compiled packages are cached.
readonly PKGDIR="${GOHOME}/pkg"

# Go workspaces containing the Tast source.
readonly SRCDIRS=(
  "${HOME}/trunk/src/platform/tast"
  "${HOME}/trunk/src/platform/tast-tests"
)

# Package to build to produce tast executable.
readonly TAST_PKG="chromiumos/cmd/tast"

# Output filename for tast executable.
readonly TAST_OUT="${GOHOME}/bin/tast"

# Readonly Go workspaces containing source to build. Note that the packages
# installed to /usr/lib/gopath (dev-go/crypto, dev-go/subcommand, etc.) need to
# be emerged beforehand.
export GOPATH="$(IFS=:; echo "${SRCDIRS[*]}"):/usr/lib/gopath"

readonly CMD=$(basename "${0}")

# Prints usage information and exits.
usage() {
  cat - <<EOF >&2
Quickly builds the tast executable or its unit tests.

Usage: ${CMD}                     Builds the tast command to ${TAST_OUT}.
       ${CMD} -b <pkg> -o <path>  Builds <pkg> to <path>.
       ${CMD} [-v] -T             Tests all packages.
       ${CMD} [-v] -t <pkg>       Tests <pkg>.
       ${CMD} -C                  Checks all code using "go vet".
       ${CMD} -c <pkg>            Checks <pkg>'s code.

EOF
  exit 1
}

# Prints all checkable packages.
get_check_pkgs() {
  local dir
  for dir in "${SRCDIRS[@]}"; do
    (cd "${dir}/src"
     find -name '*.go' | xargs dirname | sort | uniq | cut -b 3-)
  done
}

# Prints all testable packages.
get_test_pkgs() {
  local dir
  for dir in "${SRCDIRS[@]}"; do
    (cd "${dir}/src"
     find -name '*_test.go' | xargs dirname | sort | uniq | cut -b 3-)
  done
}

# Builds an executable package to a destination path.
run_build() {
  local pkg="${1}"
  local dest="${2}"
  go build -i -pkgdir "${PKGDIR}" -o "${dest}" "${pkg}"
}

# Checks one or more packages.
run_vet() {
  go vet "${@}"
}

# Tests a package.
run_test() {
  go test ${verbose_flag} -pkgdir "${PKGDIR}" "${1}"
}

# Executable package to build.
build_pkg=

# Path to which executable package should be installed.
build_out=

# Package to check via "go vet".
check_pkg=

# Test package to build and run.
test_pkg=

# Verbose flag for testing.
verbose_flag=

while getopts "CTb:c:ho:t:v-" opt; do
  case "${opt}" in
    C)
      check_pkg=all
      ;;
    T)
      test_pkg=all
      ;;
    b)
      build_pkg="${OPTARG}"
      ;;
    c)
      check_pkg="${OPTARG}"
      ;;
    o)
      build_out="${OPTARG}"
      ;;
    t)
      test_pkg="${OPTARG}"
      ;;
    v)
      verbose_flag="-v"
      ;;
    *)
      usage
      ;;
  esac
done

if [ -n "${build_pkg}" ]; then
  if [ -z "${build_out}" ]; then
    echo "Required output file missing: -o <path>" >&2
    exit 1
  fi
  run_build "${build_pkg}" "${build_out}"
elif [ -n "${test_pkg}" ]; then
  if [ "${test_pkg}" = 'all' ]; then
    for p in $(get_test_pkgs); do run_test "${p}"; done
  else
    run_test "${test_pkg}"
  fi
elif [ -n "${check_pkg}" ]; then
  if [ "${check_pkg}" = 'all' ]; then
    run_vet $(get_check_pkgs)
  else
    run_vet "${check_pkg}"
  fi
else
  run_build "${TAST_PKG}" "${TAST_OUT}"
fi
