// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakedutserver_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/longrunning"
	"go.chromium.org/chromiumos/config/go/test/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chromiumos/tast/internal/fakedutserver"
	"chromiumos/tast/testutil"
)

func TestDutServiceServer_Cache(t *testing.T) {
	ctx := context.Background()
	expectedContent := "content of foo/bar/baz"
	fileMap := map[string][]byte{
		"gs://foo/bar/baz": []byte(expectedContent),
	}
	stopFunc, addr := fakedutserver.Start(t, fakedutserver.WithCacheFileMap(fileMap))
	defer stopFunc()

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to Dial: ", err)
	}
	defer conn.Close()

	cl := api.NewDutServiceClient(conn)

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	destFile := filepath.Join(dir, "foo/bar/baz")
	req := &api.CacheRequest{
		Destination: &api.CacheRequest_File{
			File: &api.CacheRequest_LocalFile{
				Path: destFile,
			},
		},
		Source: &api.CacheRequest_GsFile{
			GsFile: &api.CacheRequest_GSFile{
				SourcePath: "gs://foo/bar/baz",
			},
		},
	}
	op, err := cl.Cache(ctx, req)
	if err != nil {
		t.Fatalf("CacheForDUT(%q): %v", req.GetGsFile().SourcePath, err)
	}
	opcli := longrunning.NewOperationsClient(conn)
	op, err = opcli.WaitOperation(ctx, &longrunning.WaitOperationRequest{
		Name: op.GetName(),
	})
	if err != nil {
		t.Fatal("Failed to wait Operation: ", err)
	}
	if !op.GetDone() {
		t.Fatalf("operation did not finish")
	}
	resp := &api.CacheResponse{}
	if err := ptypes.UnmarshalAny(op.GetResponse(), resp); err != nil {
		t.Fatalf("Failed to unmarshal response [%v]: %s", resp, err)
	}
	content, err := ioutil.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", destFile, err)
	}

	if string(content) != expectedContent {
		t.Errorf("Wrong content: got %q, want %q", string(content), expectedContent)
	}
}

func TestDutServiceServer_Cache_Errors(t *testing.T) {
	ctx := context.Background()
	expectedContent := "content of foo/bar/baz"

	fileMap := map[string][]byte{
		"gs://foo/bar/baz": []byte(expectedContent),
	}
	stopFunc, addr := fakedutserver.Start(t, fakedutserver.WithCacheFileMap(fileMap))
	defer stopFunc()

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to Dial: ", err)
	}
	defer conn.Close()

	cl := api.NewDutServiceClient(conn)

	var errCases = []struct {
		req          *api.CacheRequest
		expectedCode codes.Code
	}{
		{
			&api.CacheRequest{
				Destination: &api.CacheRequest_File{
					File: &api.CacheRequest_LocalFile{
						Path: "/tmp/dut001",
					},
				},
				Source: &api.CacheRequest_GsFile{
					GsFile: &api.CacheRequest_GSFile{
						SourcePath: "gs://non-existent-resource",
					},
				},
			},
			codes.NotFound,
		},
	}
	for _, c := range errCases {
		req := c.req
		_, err = cl.Cache(ctx, req)
		if err == nil {
			t.Fatalf("CacheForDUT(%q) unexpectedly succeded", req.GetGsFile().SourcePath)
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("Failed to get error status: %v", err)
		}
		if st.Code() != c.expectedCode {
			t.Fatalf("CacheForDUT(%q) returned unexpected status code: got %s, want %s", req.GetGsFile().SourcePath, err, codes.NotFound)
		}
	}
}
