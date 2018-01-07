// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package breakpad

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	//crashServer          = "chromeos-gt-devserver5.hot.corp.google.com:8082"
	//crashServer          = "localhost:8123"
	crashServerFormField = "minidump"
	baseImageArchiveURL  = "gs://chromeos-image-archive/"
	crashServerTimeout   = 2 * time.Minute

	crashServerINISection = "CROS"
	crashServerININame    = "crash_server"
)

// GetCrashServerURLs parses the Autotest global_config.ini file at configPath and
// returns the list of crash servers listed in it.
func GetCrashServerURLs(configPath string) ([]string, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := getINIField(f, crashServerINISection, crashServerININame)
	if err != nil {
		return nil, err
	} else if data == "" {
		return nil, nil
	}

	urls := strings.Split(data, ",")
	for i := range urls {
		urls[i] = strings.TrimSpace(urls[i])
	}
	return urls, nil
}

// getINIField reads INI data from r and returns name's value in section.
// section may be empty. If the key isn't found, an empty string is returned.
// If the data is malformed, an error is returned.
// See https://en.wikipedia.org/wiki/INI_file for details.
func getINIField(r io.Reader, section, name string) (string, error) {
	curSection := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || line[0] == ';' || line[0] == '#':
			continue
		case line[0] == '[' && line[len(line)-1] == ']':
			curSection = line[1 : len(line)-1]
		default:
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				return "", fmt.Errorf("bad line %q", line)
			}
			n, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if len(n) == 0 {
				return "", fmt.Errorf("no name in line %q", line)
			}
			if curSection == section && n == name {
				return v, nil
			}
		}
	}
	return "", nil
}

// PostMinidumpToCrashServer posts the minidump file at dumpPath to server (of form "scheme://host:port"),
// writing the resulting symbolized stack trace to w. symbolsURL points at an archive containing Breakpad
// symbols for the crash's system image; see GetSymbolsURL.
func PostMinidumpToCrashServer(ctx context.Context, server, dumpPath, symbolsURL string, w io.Writer) error {
	f, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer f.Close()

	pr, pw := io.Pipe()

	cl := http.Client{Timeout: crashServerTimeout}
	if cd, ok := ctx.Deadline(); ok && cd.Before(time.Now().Add(cl.Timeout)) {
		cl.Timeout = cd.Sub(time.Now())
	}

	crashURL := fmt.Sprintf("%s/symbolicate_dump?archive_url=%s", server, symbolsURL)
	req, err := http.NewRequest(http.MethodPost, crashURL, pr)
	if err != nil {
		return err
	}
	mw := multipart.NewWriter(pw)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	ch := make(chan error)
	go func() {
		ch <- writeEncodedMinidump(mw, f, filepath.Base(f.Name()))
		pw.Close()
	}()

	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err = <-ch; err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %q", crashURL, resp.Status)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// writeEncodedMinidump copies a minidump file from r to w as a multipart MIME message
// containing a form file with filename fn. w is closed on completion.
func writeEncodedMinidump(w *multipart.Writer, r io.Reader, fn string) error {
	fw, err := w.CreateFormFile(crashServerFormField, fn)
	if err != nil {
		return err
	}
	if _, err = io.Copy(fw, r); err != nil {
		return err
	}
	return w.Close()
}
