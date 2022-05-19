// Copyright 2022 The ChromiumOS Authors.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestHardcodedUserDirs(t *testing.T) {
	const code = `package main

import (
	"file/filepath"

	"chromiumos/tast/local/chrome/uiauto/filesapp"
)

const homeDirMount = "/home/chronos/user"
const (
	downloadsPath = "/home/chronos/user/Downloads"
)

func main() {
	var x string
	const downloadsPath = "/home/chronos/user"
	x = "/home/chronos/user/Downloads"
	decl := "/home/chronos/user"
	stringArray := []string{"/home/chronos/user"}
	// Appending to string array.
	stringArray = append(stringArray, "/home/chronos/user/Downloads")
	for path := range []string{"/home/chronos/user", "/some/other/path"} {
		fmt.Println(path)
	}
	testFunc("/home/chronos/user")
	fileName := filepath.Join(filesapp.DownloadPath, "test.txt")
	fileName2 := x.(string).DownloadPath
	filePaths := []string{filesapp.MyFilesPath}

	comment := "/home/chronos/user in strings with whitespace should be ignored"
}

func testFunc(path string) {
	return path
}
`
	expects := []string{
		"testfile.go:9:22: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:11:18: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:16:24: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:17:6: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:18:10: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:19:26: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:21:36: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:22:29: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:25:11: A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
		"testfile.go:26:28: filesapp.DownloadPath references the /home/chronos/user bind mount which is being deprecated, please use the cryptohome package instead",
		"testfile.go:28:24: filesapp.MyFilesPath references the /home/chronos/user bind mount which is being deprecated, please use the cryptohome package instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := NoHardcodedUserDirs(fs, f)
	verifyIssues(t, issues, expects)
}
