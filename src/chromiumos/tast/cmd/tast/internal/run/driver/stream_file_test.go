// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package driver_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testingutil"
	"chromiumos/tast/testutil"
)

func TestDriver_StreamFile(t *testing.T) {
	done := make(chan bool, 1)
	defer func() { done <- true }()
	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	src := filepath.Join(dataDir, "src.log")
	dest := filepath.Join(dataDir, "dest.log")
	data := []string{
		`line1\nline`,
		`2\nline3\n`,
	}
	var want string
	for _, d := range data {
		want = want + d
	}
	env := runtest.SetUp(
		t,
		runtest.WithStreamFile(func(req *protocol.StreamFileRequest, srv protocol.TestService_StreamFileServer) error {
			if req.Name != src {
				t.Fatalf("Got file name %q; expect %q", req.Name, src)
			}
			go func() {
				var offset int64 = 0
				for _, d := range data {
					time.Sleep(time.Second)
					offset = offset + int64(len(d))
					rspn := &protocol.StreamFileResponse{
						Data:   []byte(d),
						Offset: offset,
					}
					if err := srv.Send(rspn); err != nil {
						t.Errorf("failed to send data %q: %v", d, err)
					}
				}
			}()
			<-done
			return nil
		}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.CheckTestDeps = true
	})

	t.Logf("TestDriver_StreamFile target %v", cfg.Target())
	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Errorf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	t.Logf("TestDriver_StreamFile driver info %+v", drv)

	var streamErr error
	defer func() {
		if streamErr != nil {
			t.Fatalf("Stream File failed")
		}
	}()

	dd, err := drv.Duplicate(ctx)
	if err != nil {
		t.Fatal("Failed to dupliate driver: ", err)
	}
	go func() {
		defer dd.Close(ctx)
		if err := dd.StreamFile(ctx, src, dest); err != nil {
			streamErr = errors.Wrap(err, "failed to stream file")
		}
	}()

	if err := testingutil.Poll(ctx, func(ctx context.Context) error {
		f, err := os.Stat(dest)
		if err != nil {
			return err
		}
		if int(f.Size()) < len(want) {
			return fmt.Errorf("expected file size for %q has not been reached", dest)
		}
		return nil
	}, &testingutil.PollOptions{Timeout: 3 * time.Minute}); err != nil {
		t.Log("Failed to reach expected file size")
	}

	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("Failed to read file %v", dest)
	}
	got := string(raw)
	if got != want {
		t.Fatalf("StreamFile mismatch: got %q want %q", got, want)
	}
}

func TestDriver_StreamFileWithInterruption(t *testing.T) {
	done := make(chan bool, 1)
	defer func() { done <- true }()
	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	src := filepath.Join(dataDir, "src.log")
	dest := filepath.Join(dataDir, "dest.log")

	var want string = `line1\nline2\nline3\n`
	endOffsets := []int64{int64(len(want) / 2), int64(len(want))}
	requestCount := 0
	env := runtest.SetUp(
		t,
		runtest.WithStreamFile(func(req *protocol.StreamFileRequest, srv protocol.TestService_StreamFileServer) error {
			if req.Name != src {
				t.Fatalf("Got file name %q; expect %q", req.Name, src)
			}
			if requestCount > len(endOffsets) {
				<-done
				return nil
			}
			endOffset := endOffsets[requestCount]
			startOffset := req.GetOffset()
			if startOffset < 0 {
				startOffset = 0
			}
			data := []byte(want[startOffset:endOffset])
			rspn := &protocol.StreamFileResponse{Data: data, Offset: endOffset}
			if err := srv.Send(rspn); err != nil {
				t.Errorf("failed to send data %q: %v", data, err)
			}
			requestCount++
			time.Sleep(1)
			if requestCount < len(endOffsets) {
				return errors.New("intentional error")
			}
			<-done
			return nil
		}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.CheckTestDeps = true
	})

	t.Logf("TestDriver_StreamFileWithInterruption target %v", cfg.Target())
	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Errorf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)
	t.Logf("TestDriver_StreamFile driver info %+v", drv)

	var streamErr error
	defer func() {
		if streamErr != nil {
			t.Fatalf("Stream File failed")
		}
	}()
	dd, err := drv.Duplicate(ctx)
	if err != nil {
		t.Fatal("Failed to dupliate driver: ", err)
	}
	go func() {
		defer dd.Close(ctx)
		if err := dd.StreamFile(ctx, src, dest); err != nil {
			streamErr = errors.Wrap(err, "failed to stream file")
		}
	}()

	if err := testingutil.Poll(ctx, func(ctx context.Context) error {
		f, err := os.Stat(dest)
		if err != nil {
			return err
		}
		if int(f.Size()) < len(want) {
			return fmt.Errorf("expected file size for %q has not been reached", dest)
		}
		return nil
	}, &testingutil.PollOptions{Timeout: 3 * time.Minute}); err != nil {
		t.Log("Failed to reach expected file size")
	}

	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("Failed to read file %v", dest)
	}
	got := string(raw)
	if got != want {
		t.Fatalf("StreamFile mismatch: got %q want %q", got, want)
	}
}
