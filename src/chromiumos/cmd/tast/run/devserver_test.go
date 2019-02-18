// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chromiumos/tast/testutil"
)

type ephemeralDevserverTestData struct {
	s        *ephemeralDevserver
	cacheDir string
	url      string
	origPath string
}

func (td *ephemeralDevserverTestData) Close() error {
	os.Setenv("PATH", td.origPath)
	os.RemoveAll(td.cacheDir)
	return td.s.Close(context.Background())
}

func newEphemeralDevserverTestData(t *testing.T, gsutil string) *ephemeralDevserverTestData {
	success := false

	cacheDir := testutil.TempDir(t)
	defer func() {
		if !success {
			os.RemoveAll(cacheDir)
		}
	}()

	// Create a fake "gsutil" command in cacheDir and update $PATH to include cacheDir.
	if err := ioutil.WriteFile(filepath.Join(cacheDir, "gsutil"), []byte(gsutil), 0700); err != nil {
		t.Fatal("Failed to save a fake gsutil: ", err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", cacheDir+":"+origPath)
	defer func() {
		if !success {
			os.Setenv("PATH", origPath)
		}
	}()

	// Start the ephemeral devserver.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen on a TCP port: ", err)
	}

	url := fmt.Sprintf("http://%s", lis.Addr())

	s, err := newEphemeralDevserver(lis, cacheDir)
	if err != nil {
		t.Fatal("Failed to start the ephemeral devserver: ", err)
	}

	success = true
	return &ephemeralDevserverTestData{s, cacheDir, url, origPath}
}

// TestEphemeralDevserverCheckHealth checks /check_health returns 200 OK.
func TestEphemeralDevserverCheckHealth(t *testing.T) {
	td := newEphemeralDevserverTestData(t, "#!/bin/true")
	defer td.Close()

	url := td.url + "/check_health"
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d; want %d", url, res.StatusCode, http.StatusOK)
	}
}

// TestEphemeralDevserverStage checks stage and download succeed.
func TestEphemeralDevserverStage(t *testing.T) {
	td := newEphemeralDevserverTestData(t, `#!/bin/bash
echo -n "$*" > ${!#}
`)
	defer td.Close()

	url := td.url + "/stage?archive_url=gs://chromiumos-test-assets-public/path/to&files=file.bin"
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d; want %d", url, res.StatusCode, http.StatusOK)
	}

	url = td.url + "/static/path/to/file.bin"
	res, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	out, _ := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d; want %d", url, res.StatusCode, http.StatusOK)
	}

	const exp = "-m cp gs://chromiumos-test-assets-public/path/to/file.bin "
	args := string(out)
	if !strings.HasPrefix(args, exp) {
		t.Fatalf("Unexpected gsutil parameters: got %q, want prefix %q", args, exp)
	}
}

// TestEphemeralDevserverNotFound checks an error is returned for missing files.
func TestEphemeralDevserverNotFound(t *testing.T) {
	td := newEphemeralDevserverTestData(t, `#!/bin/bash
echo "No URLs matched" >&2
exit 1
`)
	defer td.Close()

	url := td.url + "/stage?archive_url=gs://chromiumos-test-assets-public/path/to&files=file.bin"
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	out, _ := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GET %s returned %d; want %d", url, res.StatusCode, http.StatusInternalServerError)
	} else if msg := "file not found"; !strings.Contains(string(out), msg) {
		t.Fatalf("GET %s returned %q; should contain %q", url, out, msg)
	}

	url = td.url + "/static/path/to/file.bin"
	res, err = http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET %s returned %d; want %d", url, res.StatusCode, http.StatusNotFound)
	}
}

func TestParseGSURL(t *testing.T) {
	for _, tc := range []struct {
		gsURL     string
		path      string
		errSubstr string
	}{
		{"gs://chromeos-image-archive/path/to/file.bin", "path/to/file.bin", ""},
		{"gs://chromeos-test-assets-private/path/to/file.bin", "path/to/file.bin", ""},
		{"gs://chromiumos-test-assets-public/path/to/file.bin", "path/to/file.bin", ""},
		{"http://chromeos-image-archive/path/to/file.bin", "", "is not a gs:// URL"},
		{"gs://secret-bucket/path/to/file.bin", "", "doesn't use a whitelisted bucket"},
		{"gs://chromeos-image-archive//path/to/file.bin", "", "isn't a clean URL"},
		{"gs://chromeos-image-archive/path/to/file.bin/", "", "isn't a clean URL"},
		{"gs://chromeos-image-archive/../path/to/file.bin", "", "isn't a clean URL"},
		{"gs://chromeos-image-archive/path/to/../file.bin", "", "isn't a clean URL"},
		{"#$%", "", "failed to parse URL"},
	} {
		path, err := parseGSURL(tc.gsURL)
		if tc.errSubstr == "" {
			if err != nil {
				t.Errorf("parseGSURL(%q) failed: %v", tc.gsURL, err)
			} else if path != tc.path {
				t.Errorf("parseGSURL(%q) = %q; want %q", tc.gsURL, path, tc.path)
			}
		} else {
			if err == nil {
				t.Errorf("parseGSURL(%q) = %q; want error with prefix %q", tc.gsURL, path, tc.errSubstr)
			} else if !strings.Contains(err.Error(), tc.errSubstr) {
				t.Errorf("parseGSURL(%q) returned error %q; should contain %q", tc.gsURL, err.Error(), tc.errSubstr)
			}
		}
	}
}
