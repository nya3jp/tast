// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
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

type testData struct {
	s        *Ephemeral
	cacheDir string
	url      string
	origPath string
}

func (td *testData) Close() error {
	os.Setenv("PATH", td.origPath)
	os.RemoveAll(td.cacheDir)
	return td.s.Close()
}

func (td *testData) Get(path string) (string, error) {
	res, err := http.Get(td.url + path)
	if err != nil {
		return "", fmt.Errorf("GET %s failed: %v", path, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s returned %d; want %d", path, res.StatusCode, http.StatusOK)
	}

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("GET %s returned malformed response: %v", path, err)
	}
	return string(b), nil
}

func newTestData(t *testing.T, gsutil string) *testData {
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
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal("Failed to listen on a TCP port: ", err)
	}

	url := fmt.Sprintf("http://%s", lis.Addr())

	s, err := NewEphemeral(lis, cacheDir, []string{"extra-allowed-bucket"})
	if err != nil {
		t.Fatal("Failed to start the ephemeral devserver: ", err)
	}

	success = true
	return &testData{s, cacheDir, url, origPath}
}

// TestCheckHealth checks /check_health returns 200 OK.
func TestCheckHealth(t *testing.T) {
	td := newTestData(t, "#!/bin/true")
	defer td.Close()

	if _, err := td.Get("/check_health"); err != nil {
		t.Error("Checking devserver health failed: ", err)
	}
}

// TestStage checks stage and download succeed.
func TestStage(t *testing.T) {
	td := newTestData(t, `#!/bin/bash
echo -n "$*" > ${!#}
`)
	defer td.Close()

	const (
		params     = "archive_url=gs://chromiumos-test-assets-public/path/to&files=file.bin"
		checkPath  = "/is_staged?" + params
		stagePath  = "/stage?" + params
		staticPath = "/static/path/to/file.bin"
	)

	if status, err := td.Get(checkPath); err != nil {
		t.Error("Checking staged status failed: ", err)
	} else if exp := "False"; status != exp {
		t.Errorf("Checking staged status failed: got %q, want %q", status, exp)
	}

	if _, err := td.Get(stagePath); err != nil {
		t.Fatal("Staging request failed: ", err)
	}

	if status, err := td.Get(checkPath); err != nil {
		t.Error("Checking staged status failed: ", err)
	} else if exp := "True"; status != exp {
		t.Errorf("Checking staged status failed: got %q, want %q", status, exp)
	}

	args, err := td.Get(staticPath)
	if err != nil {
		t.Fatal("Static file request failed: ", err)
	}

	const exp = "-m cp gs://chromiumos-test-assets-public/path/to/file.bin "
	if !strings.HasPrefix(args, exp) {
		t.Fatalf("Unexpected gsutil parameters: got %q, want prefix %q", args, exp)
	}
}

// TestNotFound checks an error is returned for missing files.
func TestNotFound(t *testing.T) {
	td := newTestData(t, `#!/bin/bash
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

func TestValidateGSURL(t *testing.T) {
	td := newTestData(t, "#!/bin/true")
	defer td.Close()

	for _, tc := range []struct {
		gsURL     string
		path      string
		errSubstr string
	}{
		{"gs://chromeos-image-archive/path/to/file.bin", "path/to/file.bin", ""},
		{"gs://chromeos-test-assets-private/path/to/file.bin", "path/to/file.bin", ""},
		{"gs://chromiumos-test-assets-public/path/to/file.bin", "path/to/file.bin", ""},
		{"gs://extra-allowed-bucket/path/to/file.bin", "path/to/file.bin", ""},
		{"http://chromeos-image-archive/path/to/file.bin", "", "is not a gs:// URL"},
		{"gs://secret-bucket/path/to/file.bin", "", "doesn't use an allowed bucket"},
		{"gs://chromeos-image-archive//path/to/file.bin", "", "isn't a clean URL"},
		{"gs://chromeos-image-archive/path/to/file.bin/", "", "isn't a clean URL"},
		{"gs://chromeos-image-archive/../path/to/file.bin", "", "isn't a clean URL"},
		{"gs://chromeos-image-archive/path/to/../file.bin", "", "isn't a clean URL"},
		{"#$%", "", "failed to parse URL"},
	} {
		path, err := td.s.validateGSURL(tc.gsURL)
		if tc.errSubstr == "" {
			if err != nil {
				t.Errorf("validateGSURL(%q) failed: %v", tc.gsURL, err)
			} else if path != tc.path {
				t.Errorf("validateGSURL(%q) = %q; want %q", tc.gsURL, path, tc.path)
			}
		} else {
			if err == nil {
				t.Errorf("validateGSURL(%q) = %q; want error with prefix %q", tc.gsURL, path, tc.errSubstr)
			} else if !strings.Contains(err.Error(), tc.errSubstr) {
				t.Errorf("validateGSURL(%q) returned error %q; should contain %q", tc.gsURL, err.Error(), tc.errSubstr)
			}
		}
	}
}
