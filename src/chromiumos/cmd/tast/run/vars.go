// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// readAndUpdateVars reads static variables stored inside the directory as yaml files,
// and updates given vars. It doesn't override the existing key value pairs.
// vars isn't changed when error is returned.
// If dir doesn't exist, os.IsNotExist will return true for the returned error.
func readAndUpdateVars(dir string, vars map[string]string) error {
	vs, err := readStaticVars(dir)
	if err != nil {
		return err
	}
	for k, v := range vs {
		if _, ok := vars[k]; ok {
			continue
		}
		vars[k] = v
	}
	return nil
}

// readStaticVars reads static variables stored inside the directory as yaml files.
// If dir doesn't exist, os.IsNotExist will return true for the returned error.
func readStaticVars(dir string) (map[string]string, error) {
	res := make(map[string]string)
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("couldn't read vars dir: %v", err)
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		b, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("couldn't read vars file: %v", err)
		}
		m := make(map[string]string)
		err = yaml.Unmarshal(b, &m)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse %v: %v", f.Name(), err)
		}
		for k, v := range m {
			if _, ok := res[k]; ok {
				return nil, fmt.Errorf("key %q in %v is already defined in another file", k, f.Name())
			}
			res[k] = v
		}
	}
	return res, nil
}
