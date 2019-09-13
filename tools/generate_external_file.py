"""
Copyright 2019 The Chromium OS Authors. All rights reserved.
Use of this source code is governed by a BSD-style license that can be
found in the LICENSE file.

Used to generate an external link format JSON file for the given tast data file.

In order to use provide the name of the data file that you want to upload as
well as the name of the test directory that the file is used for.

Example:

  ./generate_external_file.py test_data.mp3 audio

Will produce a file called 'test_data.mp3.external' in the external link format
in the current directory.

If the '--upload' option is provided then the given data file will be uploaded
to the following path in Google Cloud Storage:

  //chromiumos-test-assets-public/tast/cros/<test_dir>/<data_file>.external
"""

import argparse
import hashlib
import json
import os
import subprocess

gcp_prefix = "chromiumos-test-assets-public/tast/cros/"


def parse_args():
  parser = argparse.ArgumentParser()
  parser.add_argument("data_file", help="name of the data file")
  parser.add_argument(
      "test_dir",
      help="name of the associated test used to fill the 'url' field")
  parser.add_argument(
      "--upload",
      help="upload data file to Google Cloud Storage",
      action="store_true")
  return parser.parse_args()


def get_sha256_digest(path):
  sha256 = hashlib.sha256()
  with open(path, "rb") as infile:
    buf = infile.read(1024)
    while buf:
      sha256.update(buf)
      buf = infile.read(1024)
  return sha256.hexdigest()


def main():
  args = parse_args()

  url = os.path.join("gs://", gcp_prefix, args.test_dir, args.data_file)
  size = os.path.getsize(args.data_file)
  digest = get_sha256_digest(args.data_file)

  link = {"url": url, "size": size, "sha256sum": digest}

  # Write out the the JSON file in the external link format.
  external_file = args.data_file + ".external"
  with open(args.data_file + ".external", "w") as outfile:
    outfile.write(json.dumps(link, indent=True))
    outfile.write("\n")

  if args.upload:
    print("Uploading file...")
    ret = subprocess.run(["gsutil", "cp", args.data_file, url])
    if ret.returncode != 0:
      print("Failed to upload file: ", str(ret.stderr))


if __name__ == "__main__":
  main()
