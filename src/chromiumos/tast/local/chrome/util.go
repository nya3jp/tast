// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"
)

const (
	pollInterval = 10 * time.Millisecond
)

// poll runs f repeatedly until it returns true and then returns nil.
// If ctx returns an error before then, the error is returned.
// TODO(derat): Probably ought to give f a way to return an error too.
func poll(ctx context.Context, f func() bool) error {
	for {
		if f() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(pollInterval)
	}
}

// waitForPort waits until a connection can be established to the port at addr (of the form "host:port").
func waitForPort(ctx context.Context, addr string) error {
	return poll(ctx, func() bool {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return true
		}
		return false
	})
}

// chownContents recursively chowns dir's contents to username's uid and gid.
func chownContents(dir string, username string) error {
	var u *user.User
	var err error
	if u, err = user.Lookup(username); err != nil {
		return err
	}

	var uid, gid int64
	if uid, err = strconv.ParseInt(u.Uid, 10, 32); err != nil {
		return fmt.Errorf("failed to parse uid %q: %v", u.Uid, err)
	}
	if gid, err = strconv.ParseInt(u.Gid, 10, 32); err != nil {
		return fmt.Errorf("failed to parse gid %q: %v", u.Gid, err)
	}

	return filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(p, int(uid), int(gid))
	})
}
