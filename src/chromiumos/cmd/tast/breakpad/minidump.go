// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package breakpad processes minidump crash reports created by Breakpad.
//
// See https://chromium.googlesource.com/breakpad/breakpad/ for more details about Breakpad.
package breakpad

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	debugSuffix            = ".debug"   // suffix that Chrome OS build process adds to files with debugging symbols
	mdMagic                = "MDMP"     // magic bytes occurring at the beginning of minidump files
	mdMaxStreams           = 32         // max streams to read from minidump file
	mdReleaseStreamType    = 0x47670005 // minidump stream type used for /etc/lsb-release data
	mdReleaseStreamMaxSize = 4096       // max bytes to read from mdReleaseStreamType streams
)

// missingRegexp extracts module paths and IDs from messages logged by minidump_stackwalk.
var missingRegexp *regexp.Regexp

func init() {
	// minidump_stackwalk writes a message like the following to stderr for each missing symbol file:
	//   2017-12-07 11:05:36: stackwalker.cc:103: INFO: Couldn't load symbols for: /lib64/libc-2.23.so|7219A63C9901FA247C197BCFED143B110
	//   2017-12-07 11:05:36: stackwalker.cc:103: INFO: Couldn't load symbols for: /usr/lib64/libevent_core-2.1.so.6.0.2|3F4224A41349B3B9600315956E3D6CA70
	// The hexadecimal string at the end corresponds to ModuleInfo.ID.
	missingRegexp = regexp.MustCompile("Couldn't load symbols for:\\s+([^|]+)\\|(\\S+)")
}

// ModuleInfo contains data from a Breakpad symbol file's MODULE record.
// See https://chromium.googlesource.com/breakpad/breakpad/+/master/docs/symbol_files.md#records-1 for details.
type ModuleInfo struct {
	// OS identifies the operating system on which the executable or shared library was intended to run.
	OS string
	// Arch indicates the processor architecture the executable or shared library contains machine code for.
	Arch string
	// ID is an opaque sequence of hexadecimal digits that identifies the exact executable or library whose
	// contents the symbol file describes.
	ID string
	// Name contains the base name (the final component of the directory path) of the executable or library.
	Name string
}

// GetDebugBinaryPath returns the location within a Chrome OS chroot where board's
// debug binary should be located for path (e.g. "/bin/grep").
func GetDebugBinaryPath(path, board string) string {
	return filepath.Join("/build", board, "/usr/lib/debug", path+debugSuffix)
}

// parseSymbolFileModuleRecord parses the MODULE record from a Breakpad symbol file.
func parseSymbolFileModuleRecord(line string) (*ModuleInfo, error) {
	parts := strings.Fields(line)
	if len(parts) != 5 {
		return nil, fmt.Errorf("got %v part(s) instead of 5", len(parts))
	}
	if parts[0] != "MODULE" {
		return nil, errors.New("didn't start with MODULE")
	}
	return &ModuleInfo{parts[1], parts[2], parts[3], parts[4]}, nil
}

// WriteSymbolFile writes a Breakpad symbols file for binPath, a binary file with debugging symbols,
// to the Breakpad-expected location within outDir.
// Breakpad's dump_syms program must be present.
func WriteSymbolFile(binPath, outDir string) (*ModuleInfo, error) {
	cmd := exec.Command("dump_syms", binPath)
	cl := strings.Join(cmd.Args, " ")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe for %v: %v", cl, err)
	}
	defer stdout.Close()

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %v: %v", cl, err)
	}

	// Read the first line of output to determine what we're generating.
	br := bufio.NewReader(stdout)
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read MODULE record from %v: %v", cl, err)
	}
	mi, err := parseSymbolFileModuleRecord(line)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MODULE record %q: %v", line, err)
	}

	// Now copy the line and the rest of the output to the file.
	// Omit the ".debug" suffix since it probably won't be present in the minidump.
	name := mi.Name
	if strings.HasSuffix(name, debugSuffix) {
		name = name[0 : len(name)-len(debugSuffix)]
	}
	p := filepath.Join(outDir, name, mi.ID, fmt.Sprintf("%s.sym", name))
	if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return mi, err
	}
	f, err := os.Create(p)
	if err != nil {
		return mi, err
	}
	defer f.Close()

	if _, err = io.WriteString(f, line); err != nil {
		return mi, fmt.Errorf("failed to write MODULE record to %v: %v", p, err)
	}
	if _, err = io.Copy(f, br); err != nil {
		return mi, fmt.Errorf("failed to copy symbols to %v: %v", p, err)
	}
	return mi, cmd.Wait()
}

// SymbolFileMap maps from the full paths of binaries (as absolute on-device paths, e.g.
// "/usr/lib64/libbase-core-395517.so") to the corresponding symbol file IDs wanted by
// minidump_stackwalk (i.e. breakpad.ModuleInfo.ID).
type SymbolFileMap map[string]string

// WalkMinidump writes a human-readable representation of the stack trace contained within
// the minidump file at path to w. symDir is a directory containing symbol files that will
// be used to make a best-effort attempt symbolize the trace. Binaries with missing symbol
// files are returned.
//
// Minidump's minidump_stackwalk program must be present, and symDir should follow the
// <module>/<ID>/<module>.sym layout directory expected by it, e.g.
// libc-2.23.so/7219A63C9901FA247C197BCFED143B110/libc-2.23.so.sym.
func WalkMinidump(path, symDir string, w io.Writer) (missing SymbolFileMap, err error) {
	cmd := exec.Command("minidump_stackwalk", path, symDir)
	cl := strings.Join(cmd.Args, " ")

	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	cmd.Stdout = w

	if err = cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run %v: %v", cl, err)
	}

	missing = make(SymbolFileMap)
	sc := bufio.NewScanner(strings.NewReader(stderr.String()))
	for sc.Scan() {
		m := missingRegexp.FindStringSubmatch(sc.Text())
		if m != nil {
			missing[m[1]] = m[2]
		}
	}
	return missing, nil
}

// IsMinidump returns true if r (which should be at the beginning of a file) is a minidump file.
func IsMinidump(r io.Reader) (bool, error) {
	b := make([]byte, len(mdMagic))
	if _, err := io.ReadFull(r, b); err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	return bytes.Equal(b, []byte(mdMagic)), nil
}

// mdStreamInfo contains information about a stream located in a minidump file.
type mdStreamInfo struct {
	streamType uint32 // identifies type of data in stream
	offset     uint32 // from beginning of file
	size       uint32
}

// readMinidumpStreamInfo returns stream information from f, a minidump file.
//
// Here are the relevant parts near the start of a minidump file:
//	...
//	0x08  stream count (4 bytes)
//	...
//	0x20  stream 0 type (4 bytes)
//	0x24  stream 0 size (4 bytes)
//	0x28  stream 0 offset (4 bytes)
//	0x2c  stream 1 type (4 bytes)
//	etc.
//
// See https://chromium.googlesource.com/breakpad/breakpad/+/master/src/google_breakpad/common/minidump_format.h
// for more details.
func readMinidumpStreamInfo(f *os.File) ([]mdStreamInfo, error) {
	// First read the stream count.
	if _, err := f.Seek(0x8, 0); err != nil {
		return nil, err
	}
	var numStreams uint32
	if err := binary.Read(f, binary.LittleEndian, &numStreams); err != nil {
		return nil, err
	}
	if numStreams > mdMaxStreams {
		return nil, fmt.Errorf("too many streams (%v)", numStreams)
	}

	// Now iterate over all of the stream directory listings to get their
	// types and bounds.
	if _, err := f.Seek(0x20, 0); err != nil {
		return nil, err
	}
	infos := make([]mdStreamInfo, numStreams)
	for i := uint32(0); i < numStreams; i++ {
		b := make([]uint32, 3)
		if err := binary.Read(f, binary.LittleEndian, &b); err != nil {
			return nil, err
		}
		infos[i].streamType = b[0]
		infos[i].size = b[1]
		infos[i].offset = b[2]
	}
	return infos, nil
}

// GetMinidumpReleaseInfo returns the contents of /etc/lsb-release if it
// is present in f, a minidump file. The Linux version of Breakpad includes
// this information in crashes.
func GetMinidumpReleaseInfo(f *os.File) (string, error) {
	infos, err := readMinidumpStreamInfo(f)
	if err != nil {
		return "", err
	}
	var releaseInfo *mdStreamInfo
	for _, info := range infos {
		if info.streamType == mdReleaseStreamType {
			releaseInfo = &info
			break
		}
	}
	if releaseInfo == nil {
		return "", errors.New("no lsb-release stream")
	}

	if _, err = f.Seek(int64(releaseInfo.offset), 0); err != nil {
		return "", err
	}
	b := make([]byte, releaseInfo.size)
	if _, err = io.ReadFull(f, b); err != nil {
		return "", err
	}
	return string(b), nil
}
