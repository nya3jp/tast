// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package network contains local Tast tests that exercise the Chrome OS network stack.
package network

/*
This file serves as a wrapper to allow tast tests to query 'iw' for network device characteristics.
iw_runner.go is the analog of {@link iw_runner.py} in the autotest suite.
*/

import (
	"chromiumos/tast/testing"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

/*
Parses link or station dump output for link key value pairs.

Link or station dump information is in the format below:

Connected to 74:e5:43:10:4f:c0 (on wlan0)
      SSID: PMKSACaching_4m9p5_ch1
      freq: 5220
      RX: 5370 bytes (37 packets)
      TX: 3604 bytes (15 packets)
      signal: -59 dBm
      tx bitrate: 13.0 MBit/s MCS 1

      bss flags:      short-slot-time
      dtim period:    5
      beacon int:     100

@param link_information: string containing the raw link or station dump
    information as reported by iw. Note that this parsing assumes a single
    entry, in the case of multiple entries (e.g. listing stations from an
    AP, or listing mesh peers), the entries must be split on a per
    peer/client basis before this parsing operation.
@return a map containing all the link key/value pairs.

*/
func getAllLinkKeys(link_information string) map[string]string {
	link_key_value_pairs := make(map[string]string)
	r := regexp.MustCompile("^[[:space:]]+(.*):[[:space:]]+(.*)$")
	for _, link_key := range strings.Split(link_information, "\n") {
		if r.MatchString(link_key) {
			match_group := r.FindStringSubmatch(link_key)
			link_key_value_pairs[match_group[1]] = match_group[2]
		}
	}
	return link_key_value_pairs
}

/*
Get the BSSID that |interface_name| is associated with.

See doc for _get_all_link_keys() for expected format of the station or link
information entry.

@param link_information: string containing the raw link or station dump
    information as reported by iw. Note that this parsing assumes a single
    entry, in the case of multiple entries (e.g. listing stations from an AP
    or listing mesh peers), the entries must be split on a per peer/client
    basis before this parsing operation.
@param interface_name: string name of interface (e.g. 'wlan0').
@param station_dump: boolean indicator of whether the link information is
    from a 'station dump' query. If False, it is assumed the string is from
    a 'link' query.
@return string bssid of the current association, or None if no matching
    association information is found.

*/
func extractBssid(link_information string, interface_name string, station_dump bool) string {

	// We're looking for a line like this when parsing the output of a 'link'
	// query:
	// Connected to 04:f0:21:03:7d:bb (on wlan0)
	// We're looking for a line like this when parsing the output of a
	// 'station dump' query:
	// Station 04:f0:21:03:7d:bb (on mesh-5000mhz)
	identifier := func() string {
		if station_dump {
			return "Station"
		} else {
			return "Connected to"
		}
	}()
	search_re := regexp.MustCompile(fmt.Sprintf(`%s ([0-9a-fA-F:]{17}) \(on %s\)`, identifier, interface_name))
	match_group := search_re.FindStringSubmatch(link_information)
	if len(match_group) == 0 {
		return ""
	}
	return match_group[1]
}

/*
IwRunner stores metadata to allow its methods to invoke commands on `iw` in a concise manner.

Test code should only have to interface with `iw` through the methods exposed by IwRunner
*/
type IwRunner struct {
	Run func(string) // Function alias that will determine how commands are executed whether the test
	// 	is a client test or a remote test (TODO).
	Host_addr  string         // Host address for remote tests (TODO).
	Iw_command string         // Path to invoke `iw`. By default, we expect iw to be in $PATH, so this value should be `iw`.
	Log_id     int            // Id for logging.
	s          *testing.State // Test State
}

/*
IwRunner factory.
*/
func NewIwRunner(state *testing.State) *IwRunner {
	return &IwRunner{
		Run:        clientCommandExec,
		Host_addr:  "",
		Iw_command: "iw",
		s:          state,
	}
}

/*
Runs a shell command over ssh and reports the binary output.

clientCommandExec runs in a blocking fashion and will not return until the shell command
	itself terminates.
@param shellCommand: string containing the shell command to be sent to the DUT. An example of
	a valid string is "ls -lat"
@return bytestream output of stdout from the DUT.
*/
func (iwr IwRunner) clientCommandExec(shellCommand string) []byte {
	out, err := exec.Command(shellCommand).Output()
	if err != nil {
		iwr.s.Fatal(fmt.Sprintf("Command \" %s \" failed with non-zero error code", shellCommand), err)
	}
	return out
}
