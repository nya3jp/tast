// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

func TestGetDUTInfo(t *testing.T) {
	extraUseFlags := []string{"foo", "bar", "baz"}
	scfg := StaticConfig{
		Type: LocalRunner,
		GetDUTInfo: func(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			logging.Infof(ctx, "Some logs")
			if diff := cmp.Diff(req.GetExtraUseFlags(), extraUseFlags); diff != "" {
				t.Errorf("Unexpected extra USE flags (-got +want):\n%s", diff)
			}
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							Available:   []string{"a", "b", "c"},
							Unavailable: []string{"x", "y", "z"},
						},
					},
					OsVersion:                "octopus-release/R86-13312.0.2020_07_02_1108",
					DefaultBuildArtifactsUrl: "gs://foo/bar",
				},
			}, nil
		},
	}

	status, stdout, _, sig := callRun(
		t, nil,
		&jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerGetDUTInfoMode,
			GetDUTInfo: &jsonprotocol.RunnerGetDUTInfoArgs{
				ExtraUSEFlags: extraUseFlags,
			},
		},
		nil, &scfg)
	if status != statusSuccess {
		t.Fatalf("%v = %v; want %v", sig, status, statusSuccess)
	}

	var got *jsonprotocol.RunnerGetDUTInfoResult
	if err := json.NewDecoder(stdout).Decode(&got); err != nil {
		t.Fatalf("%v gave bad output: %v", sig, err)
	}

	want := &jsonprotocol.RunnerGetDUTInfoResult{
		SoftwareFeatures: &protocol.SoftwareFeatures{
			Available:   []string{"a", "b", "c"},
			Unavailable: []string{"x", "y", "z"},
		},
		OSVersion:                "octopus-release/R86-13312.0.2020_07_02_1108",
		DefaultBuildArtifactsURL: "gs://foo/bar",
		Warnings:                 []string{"Some logs"},
	}
	if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
		t.Errorf("%v wrote unexpected result (-got +want):\n%s", sig, diff)
	}
}
