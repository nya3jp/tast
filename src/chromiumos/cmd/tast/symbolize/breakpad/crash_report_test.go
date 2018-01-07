// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package breakpad

import (
	"bytes"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestReadCrashReport(t *testing.T) {
	for _, tc := range []struct {
		in   string
		kv   map[string]string
		do   int
		dl   int
		fail bool
	}{
		// Valid input:
		{"key:3:val", map[string]string{"key": "val"}, 0, 0, false},
		{"key1:0:key2:3:val", map[string]string{"key1": "", "key2": "val"}, 0, 0, false},
		{"key1:4:val1key2:6:value2", map[string]string{"key1": "val1", "key2": "value2"}, 0, 0, false},
		{crashReportDumpKey + ":10:0123456789", map[string]string{}, len(crashReportDumpKey) + 4, 10, false},
		{"key:3:val" + crashReportDumpKey + ":10:0123456789",
			map[string]string{"key": "val"}, 9 + len(crashReportDumpKey) + 4, 10, false},
		{"key1:4:val1" + crashReportDumpKey + ":10:0123456789key2:4:val2",
			map[string]string{"key1": "val1", "key2": "val2"}, 11 + len(crashReportDumpKey) + 4, 10, false},

		// Bad input:
		{":3:val", nil, 0, 0, true},                     // empty key
		{"key", nil, 0, 0, true},                        // unterminated key
		{"key:", nil, 0, 0, true},                       // missing value length
		{"key::", nil, 0, 0, true},                      // empty value length
		{"key:0", nil, 0, 0, true},                      // unterminated value length
		{"key:-1:", nil, 0, 0, true},                    // negative value length
		{"key:3:va", nil, 0, 0, true},                   // truncated value
		{"key:3:vara", nil, 0, 0, true},                 // extra data after value
		{crashReportDumpKey + ":2:0", nil, 0, 0, true},  // truncated dump data
		{crashReportDumpKey + ":1:01", nil, 0, 0, true}, // extra byte after dump data

		// Excessive key and value lengths:
		{strings.Repeat("a", crashReportMaxKeyLen) + "a:3:val",
			nil, 0, 0, true},
		{"key:" + strconv.Itoa(crashReportMaxValueLen+1) + ":" + strings.Repeat("a", crashReportMaxValueLen+1),
			nil, 0, 0, true},
	} {
		kv, do, dl, err := ReadCrashReport(bytes.NewBufferString(tc.in))
		if tc.fail {
			if err == nil {
				t.Errorf("ReadCrashReport(%q) succeeded; want error", tc.in)
			}
		} else {
			if err != nil {
				t.Errorf("ReadCrashReport(%q) failed: %v", tc.in, err)
				continue
			}
			if !reflect.DeepEqual(kv, tc.kv) {
				t.Errorf("ReadCrashReport(%q) returned key/vals %v; want %v", tc.in, kv, tc.kv)
			}
			if do != tc.do || dl != tc.dl {
				t.Errorf("ReadCrashReport(%q) returned dump offset/length [%v, %v]; want [%v, %v]",
					tc.in, do, dl, tc.do, tc.dl)
			}
		}
	}
}

func TestReadCrashReportFull(t *testing.T) {
	const (
		reportPath = "testdata/chrome_crash_report.dmp"
		versionKey = "ver"
		versionVal = "65.0.3299.0"
		dumpOffset = 0x523
		dumpLen    = 0xadd
	)

	f, err := os.Open(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	pairs, do, dl, err := ReadCrashReport(f)
	if err != nil {
		t.Fatal("ReadCrashReport failed:", err)
	}
	if pairs[versionKey] != versionVal {
		t.Errorf("ReadCrashReport returned %q value %q; want %q", versionKey, pairs[versionKey], versionVal)
	}
	if do != dumpOffset || dl != dumpLen {
		t.Errorf("ReadCrashReport returned dump offset/length [%v %v]; want [%v %v]", do, dl, dumpOffset, dumpLen)
	}
}
