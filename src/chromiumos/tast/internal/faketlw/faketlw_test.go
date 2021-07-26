// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package faketlw

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"go.chromium.org/chromiumos/config/go/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWiringServer_OpenDutPort(t *testing.T) {
	ctx := context.Background()

	src := NamePort{Name: "foo", Port: 1234}
	dst := NamePort{Name: "bar", Port: 2345}
	stopFunc, addr := StartWiringServer(t, WithDUTPortMap(map[NamePort]NamePort{src: dst}))
	defer stopFunc()

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to Dial: ", err)
	}
	defer conn.Close()

	cl := tls.NewWiringClient(conn)

	req := &tls.OpenDutPortRequest{Name: src.Name, Port: src.Port}
	res, err := cl.OpenDutPort(ctx, req)
	if err != nil {
		t.Errorf("OpenDutPort(%q, %d): %v", req.Name, req.Port, err)
	} else if res.GetAddress() != dst.Name || res.GetPort() != dst.Port {
		t.Errorf("OpenDutPort(%q, %d) = (%q, %d); want (%q, %d)", req.Name, req.Port, res.GetAddress(), res.GetPort(), dst.Name, dst.Port)
	}

	req = &tls.OpenDutPortRequest{Name: src.Name, Port: 9999}
	res, err = cl.OpenDutPort(ctx, req)
	if err == nil {
		t.Errorf("OpenDutPort(%q, %d): unexpectedly succeeded", req.Name, req.Port)
	}
}

func TestWiringServer_CacheForDut(t *testing.T) {
	ctx := context.Background()
	fileMap := map[string][]byte{
		"gs://foo/bar/baz": []byte("content of foo/bar/baz"),
	}
	stopFunc, addr := StartWiringServer(t, WithCacheFileMap(fileMap), WithDUTName("dut001"))
	defer stopFunc()

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to Dial: ", err)
	}
	defer conn.Close()

	cl := tls.NewWiringClient(conn)

	req := &tls.CacheForDutRequest{Url: "gs://foo/bar/baz", DutName: "dut001"}
	op, err := cl.CacheForDut(ctx, req)
	if err != nil {
		t.Fatalf("CacheForDUT(%q, %q): %v", req.DutName, req.Url, err)
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
	resp := &tls.CacheForDutResponse{}
	if err := ptypes.UnmarshalAny(op.GetResponse(), resp); err != nil {
		t.Fatalf("Failed to unmarshal response [%v]: %s", resp, err)
	}

	httpReq, err := http.NewRequest("GET", resp.Url, nil)
	if err != nil {
		t.Fatalf("Failed to create new HTTP request (URL=%v): %s", resp.Url, err)
	}
	httpReq = httpReq.WithContext(ctx)
	res, err := (http.DefaultClient).Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to get from download URL (%s): %s", resp.Url, err)
	}
	content, err := ioutil.ReadAll(res.Body)
	expectedContent := "content of foo/bar/baz"
	if string(content) != expectedContent {
		t.Errorf("Wrong content: got %q, want %q", content, expectedContent)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Got status %d %v", res.StatusCode, httpReq)
	}
}

func TestWiringServer_CacheForDut_Errors(t *testing.T) {
	ctx := context.Background()
	fileMap := map[string][]byte{
		"gs://foo/bar/baz": []byte("content of foo/bar/baz"),
	}
	stopFunc, addr := StartWiringServer(t, WithCacheFileMap(fileMap), WithDUTName("dut001"))
	defer stopFunc()

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to Dial: ", err)
	}
	defer conn.Close()

	cl := tls.NewWiringClient(conn)

	var errCases = []struct {
		req          *tls.CacheForDutRequest
		expectedCode codes.Code
	}{
		{&tls.CacheForDutRequest{Url: "gs://non-existent-resource", DutName: "dut001"}, codes.NotFound},
		{&tls.CacheForDutRequest{Url: "gs://foo/bar/baz", DutName: "dut002"}, codes.InvalidArgument},
	}
	for _, c := range errCases {
		req := c.req
		_, err = cl.CacheForDut(ctx, req)
		if err == nil {
			t.Fatalf("CacheForDUT(%q, %q) unexpectedly succeded", req.DutName, req.Url)
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("Failed to get error status: %v", err)
		}
		if st.Code() != c.expectedCode {
			t.Fatalf("CacheForDUT(%q, %q) returned unexpected status code: got %s, want %s", req.DutName, req.Url, st.Code(), codes.NotFound)
		}
	}
}
