// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testutil provides support code for unit tests.
package testutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TempDir creates a temporary directory prefixed by "tast_unittest_[TestName]." and returns its path.
// If the directory cannot be created, a fatal error is reported to t.
func TempDir(t *testing.T) string {
	t.Helper()
	// Subtests have slashes in their name.
	// https://golang.org/pkg/testing/#hdr-Subtests_and_Sub_benchmarks
	name := strings.Replace(t.Name(), "/", "_", -1)
	td, err := ioutil.TempDir("", "tast_unittest_"+name+".")
	if err != nil {
		t.Fatal(err)
	}
	return td
}

// WriteFiles creates and writes files (keys are relative filenames,
// values are contents) within dir.
func WriteFiles(dir string, files map[string]string) error {
	for fn, c := range files {
		p := filepath.Join(dir, fn)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		if err := ioutil.WriteFile(p, []byte(c), 0644); err != nil {
			return err
		}
	}
	return nil
}

// ReadFiles reads all regular files under dir and returns their
// relative paths and contents.
func ReadFiles(dir string) (map[string]string, error) {
	files := make(map[string]string)
	wf := func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		b, err := ioutil.ReadFile(p)
		if err != nil {
			return err
		}
		// Remove base dir plus joining slash.
		files[p[len(dir)+1:]] = string(b)
		return nil
	}
	err := filepath.Walk(dir, wf)
	return files, err
}

// AppendToFile appends data to the file at path.
func AppendToFile(path, data string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write([]byte(data))
	return err
}
