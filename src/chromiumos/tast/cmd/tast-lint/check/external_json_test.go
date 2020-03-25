// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestExternalJSONBad(t *testing.T) {
	const code = `{
  "url": "gs://chromiumos-test-assets-public/tast/cros/arcapp/ArcWMTestApp_24.apk",
  "size": 313077,
  "sha256sum": "285e7d8d6df63516f26a6e01394ddf5575bf6be9f008371f5e7aba155b4d4fac"
}
`
	const path = "src/chromiumos/tast/local/bundles/cros/arc/data/ArcWMTestApp_24.apk.external"
	issues := ExternalJSON(path, []byte(code))
	expects := []string{
		path + ": include the date as a suffix in the filename like \".../ArcWMTestApp_24_YYYYMMDD.apk\"",
	}
	verifyIssues(t, issues, expects)
}

func TestExternalJSONInvalidDate(t *testing.T) {
	const code = `{
	"_comment": "20191131 is invalid date.",
  "url": "gs://chromiumos-test-assets-public/tast/cros/arcapp/ArcWMTestApp_24_20191131.invalid.apk",
  "size": 313077,
  "sha256sum": "285e7d8d6df63516f26a6e01394ddf5575bf6be9f008371f5e7aba155b4d4fac"
}
`
	const path = "src/chromiumos/tast/local/bundles/cros/arc/data/ArcWMTestApp_24.apk.external"
	issues := ExternalJSON(path, []byte(code))
	expects := []string{
		path + ": include the date as a suffix in the filename like \".../ArcWMTestApp_24_20191131.invalid_YYYYMMDD.apk\"",
	}
	verifyIssues(t, issues, expects)
}

func TestExternalJSONNoURL(t *testing.T) {
	const code = `{
	"_comment": "no url is OK.",
  "size": 313077,
  "sha256sum": "285e7d8d6df63516f26a6e01394ddf5575bf6be9f008371f5e7aba155b4d4fac"
}
`
	const path = "src/chromiumos/tast/local/bundles/cros/arc/data/ArcWMTestApp_24.apk.external"
	issues := ExternalJSON(path, []byte(code))
	verifyIssues(t, issues, nil)
}

func TestExternalJSONOK1(t *testing.T) {
	const code = `{
  "url": "gs://chromiumos-test-assets-public/tast/cros/arcapp/ArcWMTestApp_24_20191010.apk",
  "size": 313077,
  "sha256sum": "285e7d8d6df63516f26a6e01394ddf5575bf6be9f008371f5e7aba155b4d4fac"
}
`
	const path = "src/chromiumos/tast/local/bundles/cros/arc/data/ArcWMTestApp_24.apk.external"
	issues := ExternalJSON(path, []byte(code))
	verifyIssues(t, issues, nil)
}

func TestExternalJSONOK2(t *testing.T) {
	const code = `{
  "sha256sum": "6cc39af8e8f23278e6cb12413c01761e97173e0ac072aa40ceedc20bff6c69f3",
  "size": 70946,
  "url": "gs://chromiumos-test-assets-public/tast/cros/printer/gstopdf_golden.pdf_20191009-135923"
}
`
	const path = "src/chromiumos/tast/local/bundles/cros/printer/data/gstopdf_golden.pdf.external"
	issues := ExternalJSON(path, []byte(code))
	verifyIssues(t, issues, nil)
}

func TestExternalJSONOK3(t *testing.T) {
	const code = `{
  "url": "gs://chromeos-test-assets-private/tast/crosint/arcapp/linpack_1.1-20100228.apk",
  "size": 100000,
  "sha256sum": "somehash"
}
`
	const path = "src/chromiumos/tast/local/bundles/crosint/arc/data/linpack.apk.external"
	issues := ExternalJSON(path, []byte(code))
	verifyIssues(t, issues, nil)
}

func TestExternalJSONCrostini(t *testing.T) {
	const code = `{
	"_comment": "crostini should be ignored."
	"url": "gs://chromeos-test-assets-private/tast/crosint/graphics/traces/10000/hl1_linux.trace.xz",
	"sha256sum": "somehash",
	"size": 100000
}
`
	const path = "src/chromiumos/tast/local/bundles/crosint/graphics/data/crostini_trace_l0001.trace.external"
	issues := ExternalJSON(path, []byte(code))
	verifyIssues(t, issues, nil)
}

func TestExternalJSONCameraApk(t *testing.T) {
	const code = `{
	"_comment": "Camera APK name pattern"
	"url": "gs://chromeos-test-assets-private/tast/cros/camera/GoogleCameraArc_20191017_RC02.apk",
	"sha256sum": "somehash",
	"size": 100000
}
`
	const path = "src/chromiumos/tast/local/bundles/cros/camera/data/GoogleCameraArc.apk.external"
	issues := ExternalJSON(path, []byte(code))
	verifyIssues(t, issues, nil)
}
