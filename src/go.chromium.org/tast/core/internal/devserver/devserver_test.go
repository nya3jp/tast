// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/logging/loggingtest"
)

func TestSelectDevserversForClient(t *testing.T) {
	const (
		lab1 = "http://10.128.176.210:8082"
		lab2 = "http://10.128.176.211:8082"
	)

	for _, tc := range []struct {
		name      string
		cloudbots string
		in        []string
		want      []string
	}{
		{
			name: "non_cloudbots",
			in:   []string{"http://127.0.0.1:1234", lab1, lab2},
			want: []string{"http://127.0.0.1:1234", lab1, lab2},
		},
		{
			name:      "cloudbots_ipv4_loopback",
			cloudbots: "example.com",
			in:        []string{"http://127.0.0.1:1234", lab1, lab2},
			want:      []string{"http://127.0.0.1:1234"},
		},
		{
			name:      "cloudbots_ipv6_loopback",
			cloudbots: "example.com",
			in:        []string{"http://[::1]:1234", lab1},
			want:      []string{"http://[::1]:1234"},
		},
		{
			name:      "cloudbots_localhost",
			cloudbots: "example.com",
			in:        []string{"http://localhost:1234", lab1},
			want:      []string{"http://localhost:1234"},
		},
		{
			name:      "cloudbots_https_loopback",
			cloudbots: "example.com",
			in:        []string{"https://127.0.0.1:1234", lab1},
			want:      []string{"https://127.0.0.1:1234"},
		},
		{
			name:      "cloudbots_multiple_loopbacks",
			cloudbots: "example.com",
			in:        []string{"http://127.0.0.1:1234", lab1, "http://[::1]:5678"},
			want:      []string{"http://127.0.0.1:1234", "http://[::1]:5678"},
		},
		{
			name:      "cloudbots_no_loopback",
			cloudbots: "example.com",
			in:        []string{lab1, lab2},
			want:      []string{lab1, lab2},
		},
		{
			name:      "cloudbots_only_loopback",
			cloudbots: "example.com",
			in:        []string{"http://127.0.0.1:1234"},
			want:      []string{"http://127.0.0.1:1234"},
		},
		{
			name:      "cloudbots_malformed_and_loopback",
			cloudbots: "example.com",
			in:        []string{"://bad", "http://127.0.0.1:1234", lab1},
			want:      []string{"http://127.0.0.1:1234"},
		},
		{
			name:      "cloudbots_schemeless_localhost",
			cloudbots: "example.com",
			in:        []string{"localhost:1234", lab1},
			want:      []string{"localhost:1234", lab1},
		},
		{
			name:      "cloudbots_dns_hostname",
			cloudbots: "example.com",
			in:        []string{"http://127.0.0.1:1234", "http://localhost.example.com:8082"},
			want:      []string{"http://127.0.0.1:1234"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLOUDBOTS_LAB_DOMAIN", tc.cloudbots)
			in := append([]string(nil), tc.in...)
			got := selectDevserversForClient(in)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("selectDevserversForClient(%q) (CLOUDBOTS_LAB_DOMAIN=%q) mismatch (-got +want):\n%s",
					tc.in, tc.cloudbots, diff)
			}
			if diff := cmp.Diff(in, tc.in); diff != "" {
				t.Errorf("selectDevserversForClient(%q) mutated input (-got +want):\n%s", tc.in, diff)
			}
		})
	}
}

func TestNewClientCloudBots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/check_health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	const labURL = "http://192.0.2.1:8082"

	t.Run("cloudbots_skips_non_loopback", func(t *testing.T) {
		t.Setenv("CLOUDBOTS_LAB_DOMAIN", "example.com")
		logger := loggingtest.NewLogger(t, logging.LevelInfo)
		ctx := logging.AttachLogger(t.Context(), logger)
		devservers := []string{srv.URL, labURL}
		cl, err := NewClient(ctx, devservers, "", "", "", "", "")
		if err != nil {
			t.Fatalf("NewClient(%q) failed: %v", devservers, err)
		}
		rc, ok := cl.(*RealClient)
		if !ok {
			t.Fatalf("NewClient(%q) returned %T; want *RealClient", devservers, cl)
		}
		if diff := cmp.Diff(rc.UpServerURLs(), []string{srv.URL}); diff != "" {
			t.Errorf("NewClient(%q) UpServerURLs() mismatch (-got +want):\n%s", devservers, diff)
		}
		wantStatus := fmt.Sprintf("[[%s UP]]", srv.URL)
		if got := rc.Status(); got != wantStatus {
			t.Errorf("NewClient(%q) Status() = %q; want %q", devservers, got, wantStatus)
		}
		wantLog := fmt.Sprintf("CLOUDBOTS_LAB_DOMAIN is set; probing only loopback devservers %v of %v", []string{srv.URL}, devservers)
		if !strings.Contains(logger.String(), wantLog) {
			t.Errorf("NewClient(%q) logs = %q; want it to contain %q", devservers, logger.String(), wantLog)
		}
	})

	t.Run("cloudbots_no_loopback_keeps_lab", func(t *testing.T) {
		t.Setenv("CLOUDBOTS_LAB_DOMAIN", "example.com")
		logger := loggingtest.NewLogger(t, logging.LevelInfo)
		ctx, cancel := context.WithTimeout(logging.AttachLogger(t.Context(), logger), 100*time.Millisecond)
		defer cancel()
		devservers := []string{labURL}
		cl, err := NewClient(ctx, devservers, "", "", "", "", "")
		if err != nil {
			t.Fatalf("NewClient(%q) failed: %v", devservers, err)
		}
		rc, ok := cl.(*RealClient)
		if !ok {
			t.Fatalf("NewClient(%q) returned %T; want *RealClient", devservers, cl)
		}
		if got := rc.Status(); !strings.Contains(got, labURL) {
			t.Errorf("NewClient(%q) Status() = %q; want it to contain %q", devservers, got, labURL)
		}
		if strings.Contains(logger.String(), "CLOUDBOTS_LAB_DOMAIN is set") {
			t.Errorf("NewClient(%q) logs = %q; want no CloudBots filter log", devservers, logger.String())
		}
	})

	t.Run("non_cloudbots_probes_all", func(t *testing.T) {
		t.Setenv("CLOUDBOTS_LAB_DOMAIN", "")
		logger := loggingtest.NewLogger(t, logging.LevelInfo)
		ctx, cancel := context.WithTimeout(logging.AttachLogger(t.Context(), logger), 100*time.Millisecond)
		defer cancel()
		devservers := []string{srv.URL, labURL}
		cl, err := NewClient(ctx, devservers, "", "", "", "", "")
		if err != nil {
			t.Fatalf("NewClient(%q) failed: %v", devservers, err)
		}
		rc, ok := cl.(*RealClient)
		if !ok {
			t.Fatalf("NewClient(%q) returned %T; want *RealClient", devservers, cl)
		}
		got := rc.Status()
		if !strings.Contains(got, srv.URL) || !strings.Contains(got, labURL) {
			t.Errorf("NewClient(%q) Status() = %q; want it to contain both %q and %q",
				devservers, got, srv.URL, labURL)
		}
		if strings.Contains(logger.String(), "CLOUDBOTS_LAB_DOMAIN is set") {
			t.Errorf("NewClient(%q) logs = %q; want no CloudBots filter log", devservers, logger.String())
		}
	})
}
