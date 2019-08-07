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

// deriveVars computes and assigns the vars for tast.
func deriveVars(c *Config) error {
	var dirs []string
	if c.build {
		for _, bi := range knownBundles {
			dirs = append(dirs, filepath.Join(c.trunkDir, bi.workspace, "vars"))
		}
	} else {
		// FIXME: read var package.
	}
	vs := make(map[string]string)
	for _, d := range dirs {
		files, err := ioutil.ReadDir(d)
		if err != nil {
			// FIXME: return error.
			continue
		}
		for _, f := range files {
			b, err := ioutil.ReadFile(filepath.Join(d, f.Name()))
			if err != nil {
				return err
			}
			t := make(map[string]string)
			err = yaml.Unmarshal(b, &t)
			if err != nil {
				return err
			}
			for k, v := range t {
				if v2, ok := vs[k]; ok {
					return fmt.Errorf("%s cannot override vs[%s]=%s", v, k, v2)
				}
				vs[k] = v
			}
		}
	}
	c.testVars = vs
	// FIXME: update with flagVars.
	fmt.Fprintf(os.Stderr, "hoge: %q", c.testVars)
	return nil
}
