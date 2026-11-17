// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	gotesting "testing"

	"go.chromium.org/tast/core/internal/devserver/devservertest"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/logging/loggingtest"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/testutil"
)

type countingDevserver struct {
	URL          string
	healthChecks atomic.Int32
}

func newCountingDevserver(t *gotesting.T, files []*devservertest.File) *countingDevserver {
	t.Helper()
	ds, err := devservertest.NewServer(devservertest.Files(files))
	if err != nil {
		t.Fatal("devservertest.NewServer failed: ", err)
	}
	t.Cleanup(ds.Close)

	cd := &countingDevserver{}
	// devservertest.Server has no /check_health hook, so front its
	// handler with a wrapper server to count health checks.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/check_health" {
			cd.healthChecks.Add(1)
		}
		ds.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	cd.URL = srv.URL
	return cd
}

func entityWithData(name string, dataFiles ...string) *protocol.Entity {
	return &protocol.Entity{
		Name:    name,
		Package: "pkg",
		Dependencies: &protocol.EntityDependencies{
			DataFiles: dataFiles,
		},
	}
}

func TestDownloaderNoDownloads(t *gotesting.T) {
	const (
		fileURL  = "gs://bucket/cached.txt"
		filePath = "pkg/data/cached.txt"
		fileData = "cached-data"
	)

	for _, tc := range []struct {
		name     string
		files    map[string]string
		entities []*protocol.Entity
		mode     protocol.DownloadMode
	}{
		{
			name:  "no_external_links_batch",
			files: map[string]string{"pkg/data/local.txt": "local"},
			entities: []*protocol.Entity{
				entityWithData("pkg.Test1", "local.txt"),
				entityWithData("pkg.Test2"),
			},
			mode: protocol.DownloadMode_BATCH,
		},
		{
			name:  "no_external_links_lazy",
			files: map[string]string{"pkg/data/local.txt": "local"},
			entities: []*protocol.Entity{
				entityWithData("pkg.Test1", "local.txt"),
				entityWithData("pkg.Test2"),
			},
			mode: protocol.DownloadMode_LAZY,
		},
		{
			name: "already_present_batch",
			files: map[string]string{
				filePath + testing.ExternalLinkSuffix: buildLink(t, fileURL, fileData),
				filePath:                              fileData,
			},
			entities: []*protocol.Entity{entityWithData("pkg.Test1", "cached.txt")},
			mode:     protocol.DownloadMode_BATCH,
		},
		{
			name: "already_present_lazy",
			files: map[string]string{
				filePath + testing.ExternalLinkSuffix: buildLink(t, fileURL, fileData),
				filePath:                              fileData,
			},
			entities: []*protocol.Entity{entityWithData("pkg.Test1", "cached.txt")},
			mode:     protocol.DownloadMode_LAZY,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			ctx := t.Context()
			cd := newCountingDevserver(t, []*devservertest.File{
				{URL: fileURL, Data: []byte(fileData)},
			})

			dataDir := filepath.Join(t.TempDir(), "data")
			if err := testutil.WriteFiles(dataDir, tc.files); err != nil {
				t.Fatal("WriteFiles failed: ", err)
			}

			beforeDownloadCalls := 0
			dl, err := newDownloader(ctx, &Config{
				Dirs:     &protocol.RunDirectories{DataDir: dataDir},
				Service:  &protocol.ServiceConfig{Devservers: []string{cd.URL}},
				DataFile: &protocol.DataFileConfig{DownloadMode: tc.mode},
				BeforeDownload: func(context.Context) {
					beforeDownloadCalls++
				},
			})
			if err != nil {
				t.Fatal("newDownloader failed: ", err)
			}

			dl.BeforeRun(ctx, tc.entities)
			for _, e := range tc.entities {
				release := dl.BeforeEntity(ctx, e)
				release()
			}
			if err := dl.TearDown(); err != nil {
				t.Error("TearDown failed: ", err)
			}

			if got := cd.healthChecks.Load(); got != 0 {
				t.Errorf("/check_health called %d times; want 0", got)
			}
			if beforeDownloadCalls != 0 {
				t.Errorf("BeforeDownload called %d times; want 0", beforeDownloadCalls)
			}
		})
	}
}

func TestDownloaderLazyReusesClient(t *gotesting.T) {
	const (
		cachedURL  = "gs://bucket/cached.txt"
		cachedPath = "pkg/data/cached.txt"
		cachedData = "cached"
		file1URL   = "gs://bucket/file1.txt"
		file1Path  = "pkg/data/file1.txt"
		file1Data  = "data1"
		file2URL   = "gs://bucket/file2.txt"
		file2Path  = "pkg/data/file2.txt"
		file2Data  = "data2"
	)

	ctx := t.Context()
	cd := newCountingDevserver(t, []*devservertest.File{
		{URL: cachedURL, Data: []byte(cachedData)},
		{URL: file1URL, Data: []byte(file1Data)},
		{URL: file2URL, Data: []byte(file2Data)},
	})

	dataDir := filepath.Join(t.TempDir(), "data")
	if err := testutil.WriteFiles(dataDir, map[string]string{
		cachedPath + testing.ExternalLinkSuffix: buildLink(t, cachedURL, cachedData),
		cachedPath:                              cachedData,
		file1Path + testing.ExternalLinkSuffix:  buildLink(t, file1URL, file1Data),
		file2Path + testing.ExternalLinkSuffix:  buildLink(t, file2URL, file2Data),
	}); err != nil {
		t.Fatal("WriteFiles failed: ", err)
	}

	e0 := entityWithData("pkg.Test0", "cached.txt")
	e1 := entityWithData("pkg.Test1", "file1.txt")
	e2 := entityWithData("pkg.Test2", "file2.txt")

	beforeDownloadCalls := 0
	dl, err := newDownloader(ctx, &Config{
		Dirs:     &protocol.RunDirectories{DataDir: dataDir},
		Service:  &protocol.ServiceConfig{Devservers: []string{cd.URL}},
		DataFile: &protocol.DataFileConfig{DownloadMode: protocol.DownloadMode_LAZY},
		BeforeDownload: func(context.Context) {
			beforeDownloadCalls++
			if beforeDownloadCalls == 1 {
				if got := cd.healthChecks.Load(); got != 0 {
					t.Errorf("/check_health called %d times before first BeforeDownload; want 0", got)
				}
			}
		},
	})
	if err != nil {
		t.Fatal("newDownloader failed: ", err)
	}

	dl.BeforeRun(ctx, []*protocol.Entity{e0, e1, e2})
	if got := cd.healthChecks.Load(); got != 0 {
		t.Fatalf("/check_health called %d times after BeforeRun in LAZY mode; want 0", got)
	}

	r0 := dl.BeforeEntity(ctx, e0)
	r0()
	if got := cd.healthChecks.Load(); got != 0 {
		t.Fatalf("/check_health called %d times when cached file is up to date; want 0", got)
	}
	if beforeDownloadCalls != 0 {
		t.Fatalf("BeforeDownload called %d times when cached file is up to date; want 0", beforeDownloadCalls)
	}

	r1 := dl.BeforeEntity(ctx, e1)
	r1()
	if got := cd.healthChecks.Load(); got != 1 {
		t.Errorf("/check_health called %d times after first download; want 1", got)
	}
	if beforeDownloadCalls != 1 {
		t.Errorf("BeforeDownload called %d times after first download; want 1", beforeDownloadCalls)
	}
	if b, err := os.ReadFile(filepath.Join(dataDir, file1Path)); err != nil {
		t.Errorf("Failed to read %s: %v", file1Path, err)
	} else if string(b) != file1Data {
		t.Errorf("Read %s = %q; want %q", file1Path, string(b), file1Data)
	}

	r2 := dl.BeforeEntity(ctx, e2)
	r2()
	if got := cd.healthChecks.Load(); got != 1 {
		t.Errorf("/check_health called %d times after second download; want 1", got)
	}
	if beforeDownloadCalls != 2 {
		t.Errorf("BeforeDownload called %d times after second download; want 2", beforeDownloadCalls)
	}
	if b, err := os.ReadFile(filepath.Join(dataDir, file2Path)); err != nil {
		t.Errorf("Failed to read %s: %v", file2Path, err)
	} else if string(b) != file2Data {
		t.Errorf("Read %s = %q; want %q", file2Path, string(b), file2Data)
	}

	if err := dl.TearDown(); err != nil {
		t.Error("TearDown failed: ", err)
	}
}

func TestDownloaderBatch(t *gotesting.T) {
	const (
		fileURL  = "gs://bucket/file1.txt"
		filePath = "pkg/data/file1.txt"
		fileData = "batch-data"
	)

	ctx := t.Context()
	cd := newCountingDevserver(t, []*devservertest.File{
		{URL: fileURL, Data: []byte(fileData)},
	})

	dataDir := filepath.Join(t.TempDir(), "data")
	if err := testutil.WriteFiles(dataDir, map[string]string{
		filePath + testing.ExternalLinkSuffix: buildLink(t, fileURL, fileData),
	}); err != nil {
		t.Fatal("WriteFiles failed: ", err)
	}

	beforeDownloadCalls := 0
	dl, err := newDownloader(ctx, &Config{
		Dirs:     &protocol.RunDirectories{DataDir: dataDir},
		Service:  &protocol.ServiceConfig{Devservers: []string{cd.URL}},
		DataFile: &protocol.DataFileConfig{DownloadMode: protocol.DownloadMode_BATCH},
		BeforeDownload: func(context.Context) {
			beforeDownloadCalls++
		},
	})
	if err != nil {
		t.Fatal("newDownloader failed: ", err)
	}
	if got := cd.healthChecks.Load(); got != 0 {
		t.Fatalf("/check_health called %d times in newDownloader; want 0", got)
	}

	e := entityWithData("pkg.Test1", "file1.txt")
	dl.BeforeRun(ctx, []*protocol.Entity{e})
	if got := cd.healthChecks.Load(); got != 1 {
		t.Errorf("/check_health called %d times after BeforeRun; want 1", got)
	}
	if beforeDownloadCalls != 1 {
		t.Errorf("BeforeDownload called %d times; want 1", beforeDownloadCalls)
	}
	if b, err := os.ReadFile(filepath.Join(dataDir, filePath)); err != nil {
		t.Errorf("Failed to read %s: %v", filePath, err)
	} else if string(b) != fileData {
		t.Errorf("Read %s = %q; want %q", filePath, string(b), fileData)
	}

	release := dl.BeforeEntity(ctx, e)
	release()
	if got := cd.healthChecks.Load(); got != 1 {
		t.Errorf("/check_health called %d times after BeforeEntity in BATCH mode; want 1", got)
	}

	if err := dl.TearDown(); err != nil {
		t.Error("TearDown failed: ", err)
	}
}

func TestDownloaderClientError(t *gotesting.T) {
	const (
		cachedURL  = "gs://bucket/cached.txt"
		cachedPath = "pkg/data/cached.txt"
		cachedData = "cached"
		need1URL   = "gs://bucket/need1.txt"
		need1Path  = "pkg/data/need1.txt"
		need1Data  = "need1"
		need2URL   = "gs://bucket/need2.txt"
		need2Path  = "pkg/data/need2.txt"
		need2Data  = "need2"
	)

	logger := loggingtest.NewLogger(t, logging.LevelInfo)
	ctx := logging.AttachLogger(t.Context(), logger)
	countClientErrLogs := func() int {
		n := 0
		for _, msg := range logger.Logs() {
			if strings.HasPrefix(msg, "Failed to create devserver client:") {
				n++
			}
		}
		return n
	}

	dataDir := filepath.Join(t.TempDir(), "data")
	if err := testutil.WriteFiles(dataDir, map[string]string{
		cachedPath + testing.ExternalLinkSuffix: buildLink(t, cachedURL, cachedData),
		cachedPath:                              cachedData,
		need1Path + testing.ExternalLinkSuffix:  buildLink(t, need1URL, need1Data),
		need2Path + testing.ExternalLinkSuffix:  buildLink(t, need2URL, need2Data),
	}); err != nil {
		t.Fatal("WriteFiles failed: ", err)
	}

	// Setting both Devservers and DutServer causes devserver.NewClient to fail.
	dl, err := newDownloader(ctx, &Config{
		Dirs: &protocol.RunDirectories{DataDir: dataDir},
		Service: &protocol.ServiceConfig{
			Devservers: []string{"http://127.0.0.1:1"},
			DutServer:  "127.0.0.1:2",
		},
		DataFile: &protocol.DataFileConfig{DownloadMode: protocol.DownloadMode_LAZY},
	})
	if err != nil {
		t.Fatal("newDownloader failed: ", err)
	}

	// Zero-job entity succeeds without creating .external-error.
	r0 := dl.BeforeEntity(ctx, entityWithData("pkg.TestCached", "cached.txt"))
	r0()
	cachedErrFile := filepath.Join(dataDir, cachedPath+testing.ExternalErrorSuffix)
	if _, err := os.Stat(cachedErrFile); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) error = %v; want %v", cachedErrFile, err, os.ErrNotExist)
	}
	if got := countClientErrLogs(); got != 0 {
		t.Errorf("client error logs = %d; want 0", got)
	}

	// Entities needing downloads record .external-error files.
	for _, tc := range []struct {
		entity *protocol.Entity
		path   string
		url    string
	}{
		{entity: entityWithData("pkg.TestNeed1", "need1.txt"), path: need1Path, url: need1URL},
		{entity: entityWithData("pkg.TestNeed2", "need2.txt"), path: need2Path, url: need2URL},
	} {
		r := dl.BeforeEntity(ctx, tc.entity)
		r()
		errFile := filepath.Join(dataDir, tc.path+testing.ExternalErrorSuffix)
		b, err := os.ReadFile(errFile)
		if err != nil {
			t.Errorf("Failed to read %s: %v", errFile, err)
			continue
		}
		got := string(b)
		for _, wantSub := range []string{
			"failed to download " + tc.url,
			"failed to create new client",
			"both dutServer",
		} {
			if !strings.Contains(got, wantSub) {
				t.Errorf("%s = %q; want substring %q", errFile, got, wantSub)
			}
		}
	}
	if got := countClientErrLogs(); got != 1 {
		t.Errorf("client error logs = %d; want 1", got)
	}

	if err := dl.TearDown(); err != nil {
		t.Error("TearDown failed: ", err)
	}
}
