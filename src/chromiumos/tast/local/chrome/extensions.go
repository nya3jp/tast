// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
)

// getExtensionDirs returns a slice of directories containing unpacked Chrome
// extensions within baseDir. Absolute paths are returned.
func getExtensionDirs(baseDir string) ([]string, error) {
	fis, err := ioutil.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(fis))
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}
		extDir := filepath.Join(baseDir, fi.Name())
		if _, err = os.Stat(filepath.Join(extDir, "manifest.json")); err == nil {
			dirs = append(dirs, extDir)
		}
	}
	return dirs, nil
}

// readKeyFromExtensionManifest returns the decoded public key from an
// extension manifest located at path. An error is returned if the manifest
// is missing or malformed. A nil key is returned if the manifest is
// parsable but doesn't contain a key.
func readKeyFromExtensionManifest(path string) ([]byte, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	j := make(map[string]interface{})
	if err = json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	if enc, ok := j["key"].(string); ok {
		return base64.StdEncoding.DecodeString(enc)
	}
	return nil, nil
}

// computeExtensionId computes the 32-character ID that Chrome will use for an unpacked
// extension in dir. If the extension's manifest file contains a public key, it is hashed
// into the ID; otherwise the directory name is hashed.
func computeExtensionId(dir string) (string, error) {
	key := []byte(dir)
	mp := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(mp); !os.IsNotExist(err) {
		if k, err := readKeyFromExtensionManifest(mp); err != nil {
			return "", err
		} else if k != nil {
			key = k
		}
	}

	// Chrome computes an extension's ID by creating a SHA-256 digest of the extension's public key
	// and converting its first 16 bytes to 32 hex characters, with the added twist that the
	// characters 'a'-'p' are used rather than '0'-'f'.
	sum := sha256.Sum256(key)
	id := make([]byte, 32)
	for i, b := range sum[:len(id)/2] {
		id[i*2] = b/16 + 'a'
		id[i*2+1] = b%16 + 'a'
	}
	return string(id), nil
}

// writeAutotestPrivateExtension writes an empty extension with access to the
// autotestPrivate Chrome API, needed for performing various tasks without
// interacting with the UI (e.g. enabling the ARC Play Store). The extension's
// ID is returned.
func writeAutotestPrivateExtension(dir string) (id string, err error) {
	if err = os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Based on Autotest's client/common_lib/cros/autotest_private_ext/manifest.json.
	// It appears to be the case that this key must be present in the manifest in order
	// for the extension's autotestPrivate permission request to be granted.
	manifest := `{
  "key": "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDuUZGKCDbff6IRaxa4Pue7PPkxwPaNhGT3JEqppEsNWFjM80imEdqMbf3lrWqEfaHgaNku7nlpwPO1mu3/4Hr+XdNa5MhfnOnuPee4hyTLwOs3Vzz81wpbdzUxZSi2OmqMyI5oTaBYICfNHLwcuc65N5dbt6WKGeKgTpp4v7j7zwIDAQAB",
  "description": "autotestPrivate API extension (used by tests)",
  "name": "autotestPrivate API extension",
  "background": { "scripts": ["background.js"] },
  "manifest_version": 2,
  "version": "0.1",
  "permissions": [ "autotestPrivate" ]
}`

	for _, f := range []struct{ name, data string }{
		{"manifest.json", manifest},
		{"background.js", ""},
	} {
		if err = ioutil.WriteFile(filepath.Join(dir, f.name), []byte(f.data), 0644); err != nil {
			return "", err
		}
	}
	return computeExtensionId(dir)
}
