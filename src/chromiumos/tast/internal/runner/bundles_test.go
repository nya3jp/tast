// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/unix"
)

func TestKillSession(t *testing.T) {
	// Start a shell in a new session that runs sleep.
	// We can't tell the shell to run "sleep 60" directly since it execs sleep then.
	cmd := exec.Command("/bin/sh", "-c", "true && sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal("Failed to start command: ", err)
	}
	sid := cmd.Process.Pid // session ID matches PID

	// Waits up to maxTime for there to be num processes in session sid.
	waitForProcs := func(num int, maxTime time.Duration) error {
		start := time.Now()
		for {
			all, err := process.Processes()
			if err != nil {
				return err
			}

			matched := make(map[int]string) // keys are PIDs, values are command lines
			for _, p := range all {
				if s, err := unix.Getsid(int(p.Pid)); err == nil && s == sid {
					cl, _ := p.Cmdline()
					matched[int(p.Pid)] = cl
				}
			}

			if len(matched) == num {
				return nil
			} else if time.Now().Sub(start) > maxTime {
				return fmt.Errorf("got %v proc(s): %v", len(matched), matched)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Wait for the sh and sleep processes to show up.
	if err := waitForProcs(2, 10*time.Second); err != nil {
		t.Errorf("Didn't get 2 initial procs: %v", err)
	}

	// After killing the session and calling wait() on sh (to remove its process entry),
	// both processes should disappear.
	killSession(sid, syscall.SIGKILL)
	go cmd.Wait() // avoid blocking forever if killSession is broken
	if err := waitForProcs(0, 10*time.Second); err != nil {
		t.Errorf("Didn't get 0 procs after calling killSession: %v", err)
	}
}

// TestFindShardIndicesFirstEvenShard makes sure findShardIndices return correct indices for
// the first shard of an evenly distributed shards.
func TestFindShardIndicesFirstEvenShard(t *testing.T) {
	if err := testFindShardIndices(t, 9, 3, 0, 0, 3, false); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the first shard of an evenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesMiddleEvenShard makes sure findShardIndices return correct indices for
// the middle shard of an evenly distributed shards.
func TestFindShardIndicesMiddleEvenShard(t *testing.T) {
	if err := testFindShardIndices(t, 9, 3, 1, 3, 6, false); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the middle shard of an evenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesLastEvenShard makes sure findShardIndices return correct indices for
// the last shard of an evenly distributed shards.
func TestFindShardIndicesLastEvenShard(t *testing.T) {
	if err := testFindShardIndices(t, 9, 3, 2, 6, 9, false); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the last shard of an evenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesFirstUnevenShard makes sure findShardIndices return correct indices for
// the first shard of an unevenly distributed shards.
func TestFindShardIndicesFirstUnevenShard(t *testing.T) {
	if err := testFindShardIndices(t, 11, 3, 0, 0, 4, false); err != nil {
		fmt.Print("Error: ", err)
		t.Errorf("Failed to get correct indices from findShardIndices for the first shard of an unevenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesMiddleUnevenShard makes sure findShardIndices return correct indices for
// the middle shard of an unevenly distributed shards.
func TestFindShardIndicesMiddleUnevenShard(t *testing.T) {
	if err := testFindShardIndices(t, 11, 3, 1, 4, 8, false); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the middle shard of an unevenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesLastUnevenShard makes sure findShardIndices return correct indices for
// the last shard of an unevenly distributed shards.
func TestFindShardIndicesLastUnevenShard(t *testing.T) {
	if err := testFindShardIndices(t, 11, 3, 2, 8, 11, false); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the last shard of an unevenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesMoreShardsThanTests makes sure findShardIndices return correct indices when
// the number of shards is greater than number of tests.
func TestFindShardIndicesMoreShardsThanTests(t *testing.T) {
	if err := testFindShardIndices(t, 9, 10, 0, 0, 1, false); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices when the number of shards is greater than number of tests: %v", err)
	}
}

// TestFindShardIndicesInvalidIndex makes sure findShardIndices return error when
// the shard index is out of range.
func TestFindShardIndicesInvalidIndex(t *testing.T) {
	if err := testFindShardIndices(t, 9, 3, 4, 0, 3, true); err != nil {
		t.Errorf("Failed to get error from findShardIndices when the shard index is out of range: %v", err)
	}
	if err := testFindShardIndices(t, 9, 10, 11, 0, 3, true); err != nil {
		t.Errorf("Failed to get error from findShardIndices when the shard index is out of range: %v", err)
	}
}

// testFindShardIndices tests whether the function findShardIndices returning the correct indices.
func testFindShardIndices(t *testing.T,
	numTests, totalShards, shardIndex, wantedStartIndex, wantedEndIndex int,
	wantError bool) (err error) {
	startIndex, endIndex, commandErr := findShardIndices(numTests, totalShards, shardIndex)
	if commandErr != nil {
		if wantError {
			return nil
		}
		return fmt.Errorf("failed to find shard indices: %v", commandErr)
	}
	if wantError {
		return fmt.Errorf("test succeeded unexpectedly; getting startIndex %v endIndex %v", startIndex, endIndex)
	}
	if startIndex != wantedStartIndex {
		return fmt.Errorf("findShardIndices returned start index %d results; want %d", startIndex, wantedStartIndex)
	}
	if endIndex != wantedEndIndex {
		return fmt.Errorf("findShardIndices returned end index %d results; want %d", endIndex, wantedEndIndex)
	}
	return nil
}
