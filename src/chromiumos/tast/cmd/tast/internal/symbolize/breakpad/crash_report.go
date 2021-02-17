// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package breakpad

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
)

const (
	crashReportMaxKeyLen       = 128  // max length of crash report keys
	crashReportMaxValueSizeLen = 10   // max length of decimal value size string
	crashReportMaxValueLen     = 1024 // max length of (non-dump) crash report values

	// crashReportDumpKey is used as the key for minidumps within crash reports.
	// Linux Chrome writes crash reports as multipart MIME data rather than crash_reporter's
	// colon-separated format, and oddly, the Chrome OS report writer uses this MIME-related
	// snippet as a key instead of defining something else. crash_reporter also contains
	// code to look for this string.
	crashReportDumpKey = "upload_file_minidump\"; filename=\"dump\""
)

// readField reads to the first occurrence of delim in r's input and returns the resulting data,
// not including delim. At most max bytes will be read. r is advanced just beyond the delimiter.
// An error is returned if delim is not found within the first max bytes.
func readField(r *bufio.Reader, delim byte, max int) (string, error) {
	b, err := r.Peek(max)
	if err != nil && err != io.EOF {
		return "", err
	}

	index := bytes.IndexByte(b, delim)
	if index == -1 {
		return "", fmt.Errorf("byte %v not found", delim)
	}

	b = b[:index]
	if _, err = r.Discard(index + 1); err != nil {
		return "", err
	}
	return string(b), nil
}

// ReadCrashReport reads a Chrome OS crash report file written by Chrome.
// These files are typically parsed by crash-reporter and use a custom format consisting of
// repeated colon-separated (key, decimal-value-length, value-data) sequences:
//
//	<key>:<decimal-value-length>:<value-data><key>:...
//
// Breakpad minidump data is typically included, and the data's offset and length within the
// reader is returned separately without being loaded into memory. All other key/value pairs
// are returned via a map.
//
// See https://chromium.googlesource.com/chromium/src/+/HEAD/components/crash/content/app/breakpad_linux.cc
// for more details.
func ReadCrashReport(r io.Reader) (pairs map[string]string, dumpOffset, dumpLen int, err error) {
	br := bufio.NewReader(r)
	nr := 0
	pairs = make(map[string]string)

	for {
		// Stop at the end of the input.
		if _, err := br.Peek(1); err == io.EOF {
			break
		}

		// We should see a key name followed by a colon.
		key, err := readField(br, ':', crashReportMaxKeyLen+1)
		if err != nil {
			return nil, 0, 0, err
		}
		if len(key) == 0 {
			return nil, 0, 0, fmt.Errorf("empty key at %v", nr)
		}
		nr += len(key) + 1

		// Next, we should get the decimal value length followed by a colon.
		valLenStr, err := readField(br, ':', crashReportMaxValueSizeLen)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to read value length for key %q: %v", key, err)
		}
		nr += len(valLenStr) + 1

		valLen, err := strconv.Atoi(valLenStr)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("unparsable length %q for key %q: %v", valLenStr, key, err)
		} else if valLen < 0 {
			return nil, 0, 0, fmt.Errorf("bad length %v for key %q", valLen, key)
		}

		if key == crashReportDumpKey {
			// Skip over minidump data.
			dumpOffset = nr
			dumpLen = valLen
			if _, err = br.Discard(dumpLen); err != nil {
				return nil, 0, 0, fmt.Errorf("failed to skip past %d-byte dump at %v: %v", dumpLen, dumpOffset, err)
			}
			nr += dumpLen
		} else {
			// Read the value itself.
			if valLen > crashReportMaxValueLen {
				return nil, 0, 0, fmt.Errorf("bad length %v for key %q", valLen, key)
			}
			val := make([]byte, valLen)
			if _, err = io.ReadFull(br, val); err != nil {
				return nil, 0, 0, fmt.Errorf("failed to read %d-byte value for key %q: %v", valLen, key, err)
			}
			nr += len(val)

			pairs[key] = string(val)
		}
	}

	return pairs, dumpOffset, dumpLen, nil
}
