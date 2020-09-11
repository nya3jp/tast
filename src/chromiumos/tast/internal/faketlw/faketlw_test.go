// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package faketlw

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"go.chromium.org/chromiumos/config/go/api/test/tls/dependencies/longrunning"
	"google.golang.org/grpc"
)

func TestWiringServer_OpenDutPort(t *testing.T) {
	ctx := context.Background()

	src := NamePort{Name: "foo", Port: 1234}
	dst := NamePort{Name: "bar", Port: 2345}
	stopFunc, addr, err := StartWiringServer(t, WithDUTPortMap(map[NamePort]NamePort{src: dst}))
	if err != nil {
		t.Fatal("Failed to start wiring server: ", err)
	}
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
	fileMap := map[string]string{
		"gs://foo/bar/baz": "foo/bar/baz",
	}
	stopFunc, addr, err := StartWiringServer(t, WithCacheFileMap(fileMap))
	if err != nil {
		t.Fatal("Failed to start wiring server: ", err)
	}
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
	for {
		if op.GetDone() {
			break
		}
		time.Sleep(1 * time.Second)
		op, err = opcli.GetOperation(ctx, &longrunning.GetOperationRequest{
			Name: op.GetName(),
		})
		if err != nil {
			t.Fatal("Failed to get Operation: ", err)
		}
	}
	resp := &tls.CacheForDutResponse{}
	if err := ptypes.UnmarshalAny(op.GetResponse(), resp); err != nil {
		t.Fatalf("Failed to unmarshal response [%v]: %s", resp, err)
	}

	hreq, err := http.NewRequest("GET", resp.Url, nil)
	if err != nil {
		t.Fatalf("Failed to create new HTTP request (URL=%v): %s", resp.Url, err)
	}
	hreq = hreq.WithContext(ctx)

	defaultHTTPClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			Proxy:               http.ProxyFromEnvironment,
		},
	}
	res, err := defaultHTTPClient.Do(hreq)
	if err != nil {
		t.Fatalf("Failed to get from download URL (%s): %s", resp.Url, err)
	}

	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("Got status %d %v", res.StatusCode, hreq)
	}
}
