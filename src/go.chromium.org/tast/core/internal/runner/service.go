// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/devserver"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/rpc"
)

type testServer struct {
	protocol.UnimplementedTestServiceServer
	scfg         *StaticConfig
	runnerParams *protocol.RunnerInitParams
	bundleParams *protocol.BundleInitParams
}

// ErrFailedToReadFile is used for indicating a file failed to open at the beginning.
var ErrFailedToReadFile = errors.New("failed to read file at the beginning")

func newTestServer(scfg *StaticConfig, runnerParams *protocol.RunnerInitParams, bundleParams *protocol.BundleInitParams) *testServer {
	exec.Command("logger", "local_test_runner: New test server is up for serving requests").Run()
	return &testServer{
		scfg:         scfg,
		runnerParams: runnerParams,
		bundleParams: bundleParams,
	}
}

func (s *testServer) GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving GetDUTInfo Request")
	exec.Command("logger", "local_test_runner: Serving GetDUTInfo Request").Run()
	if s.scfg.GetDUTInfo == nil {
		return &protocol.GetDUTInfoResponse{}, nil
	}
	return s.scfg.GetDUTInfo(ctx, req)
}

func (s *testServer) GetSysInfoState(ctx context.Context, req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving GetSysInfoState Request")
	exec.Command("logger", "local_test_runner: Serving GetSysInfoState Request").Run()
	if s.scfg.GetSysInfoState == nil {
		return &protocol.GetSysInfoStateResponse{}, nil
	}
	return s.scfg.GetSysInfoState(ctx, req)
}

func (s *testServer) CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving CollectSysInfo Request")
	exec.Command("logger", "local_test_runner: Serving CollectSysInfo Request").Run()
	if s.scfg.CollectSysInfo == nil {
		return &protocol.CollectSysInfoResponse{}, nil
	}
	return s.scfg.CollectSysInfo(ctx, req)
}

func (s *testServer) DownloadPrivateBundles(ctx context.Context, req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
	// Logging added for b/213616631.
	logging.Debug(ctx, "Serving DownloadPrivateBundles Request")
	exec.Command("logger", "local_test_runner: Serving DownloadPrivateBundles Request").Run()

	if s.scfg.PrivateBundlesStampPath == "" {
		return nil, errors.New("this test runner is not configured for private bundles")
	}

	if req.GetBuildArtifactUrl() == "" {
		return nil, errors.New("failed to determine the build artifacts URL (non-official image?)")
	}

	if !s.needToDownload(ctx, req.GetBuildArtifactUrl()) {
		return &protocol.DownloadPrivateBundlesResponse{}, nil
	}

	// Download the archive via devserver.
	cl, err := devserver.NewClient(
		ctx, req.GetServiceConfig().GetDevservers(),
		req.GetServiceConfig().GetTlwServer(), req.GetServiceConfig().GetTlwSelfName(),
		req.GetServiceConfig().GetDutServer(),
		req.GetServiceConfig().GetSwarmingTaskID(),
		req.GetServiceConfig().GetBuildBucketID(),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new client [devservers=%v, TLWServer=%s]",
			req.GetServiceConfig().GetDevservers(), req.GetServiceConfig().GetTlwServer())
	}
	defer cl.TearDown()

	privateBundles := []string{
		"tast_bundles",
		"tast_intel_bundles",
	}

	for _, b := range privateBundles {
		if err := downloadPrivateBundle(ctx, cl, req.GetBuildArtifactUrl(), b, s.scfg.BundleType); err != nil {
			return nil, errors.Wrapf(err, "failed to download %s", b)
		}
	}

	if err := writeStampFile(s.scfg.PrivateBundlesStampPath, req.GetBuildArtifactUrl()); err != nil {
		return nil, errors.Wrapf(err, "failed to write stamp file %v", s.scfg.PrivateBundlesStampPath)
	}

	return &protocol.DownloadPrivateBundlesResponse{}, nil
}

func writeStampFile(path, content string) error {
	stampfileDir := filepath.Dir(path)
	if err := os.MkdirAll(stampfileDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory %v", stampfileDir)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return nil
}

// needToDownload check stamp file exists and buildArtifactURL to decided whether bundle need to download
func (s *testServer) needToDownload(ctx context.Context, buildArtifactURL string) bool {
	if _, err := os.Stat(s.scfg.PrivateBundlesStampPath); err != nil {
		return true
	}
	content, err := os.ReadFile(s.scfg.PrivateBundlesStampPath)
	if err != nil {
		logging.Infof(ctx, "read file %s error: %v", s.scfg.PrivateBundlesStampPath, err)
		return true
	}
	if string(content) == buildArtifactURL {
		return false
	}
	return true
}

// downloadPrivateBundle downloads a single private bundle.
func downloadPrivateBundle(ctx context.Context, cl devserver.Client, archiveURBase, bundle string, bundleType BundleType) error {
	// Download the archive via devserver.
	archiveURL := archiveURBase + bundle + ".tar.bz2"
	logging.Infof(ctx, "Downloading private bundle %s", archiveURL)

	r, err := cl.Open(ctx, archiveURL)
	if err != nil {
		if errors.As(err, &os.ErrNotExist) {
			// Not all private bundles are available for all users.
			// It is fine to not finding certain bundles for certain
			// users.
			logging.Infof(ctx, "Private bundle %s not found", archiveURL)
			return nil
		}
		return err
	}
	defer r.Close()

	tf, err := os.CreateTemp("", bundle+".")
	if err != nil {
		return err
	}
	defer os.Remove(tf.Name())

	_, err = io.Copy(tf, r)

	if cerr := tf.Close(); err == nil {
		err = cerr
	}

	if err == nil {
		switch bundleType {
		case Local:
			return localBundleDownload(ctx, tf)
		case Remote:
			return remoteBundleDownload(ctx, tf)
		}
	} else if os.IsNotExist(err) {
		logging.Info(ctx, "Private bundles not found")
	} else {
		return errors.Errorf("failed to copy downloaded archive %s: %v", archiveURL, err)
	}

	return nil
}

// localBundleDownload extract the archive when local bundle type
func localBundleDownload(ctx context.Context, tf *os.File) error {
	// Extract the archive, and touch the stamp file.
	cmd := exec.Command("tar", "xf", tf.Name(), "--wildcards",
		"libexec/tast/bundles/local*",
		"share/tast/data/go.chromium.org*")
	cmd.Dir = "/usr/local"
	if err := cmd.Run(); err != nil {
		return errors.Errorf("failed to extract %s: %v", strings.Join(cmd.Args, " "), err)
	}
	logging.Info(ctx, "Local bundle download finished successfully")
	return nil
}

// remoteBundleDownload extract the archive when remote bundle type
func remoteBundleDownload(ctx context.Context, tf *os.File) error {
	// Initialize a directory for the remote bundle.
	if err := os.MkdirAll("/usr/libexec/tast/bundles/remote", 0755); err != nil {
		return errors.Errorf("failed to create directory: %v", err)
	}
	tarCmd := exec.Command("sudo", "tar", "xf", tf.Name(),
		"broot/usr/libexec/tast/bundles/remote",
		"--transform", "s,^broot/usr/,,")
	tarCmd.Dir = "/usr"
	if err := tarCmd.Run(); err != nil {
		return errors.Errorf("failed to extract %s: %v", strings.Join(tarCmd.Args, " "), err)
	}
	logging.Info(ctx, "Remote bundle download finished successfully")
	return nil
}

func (s *testServer) ListEntities(ctx context.Context, req *protocol.ListEntitiesRequest) (*protocol.ListEntitiesResponse, error) {
	var entities []*protocol.ResolvedEntity
	// Logging added for b/213616631 to see ListEntities progress on the DUT.
	logging.Debug(ctx, "Serving ListEntities Request")
	exec.Command("logger", "local_test_runner: Serving ListEntities Request").Run()
	// ListEntities should not set runtime global information during handshake.
	// TODO(b/187793617): Always pass s.bundleParams to bundles once we fully migrate to gRPC-based protocol.
	// This workaround is currently needed because BundleInitParams is unavailable when this method is called internally for handling JSON-based protocol methods.
	if err := s.forEachBundle(ctx, nil, func(ctx context.Context, ts protocol.TestServiceClient) error {
		res, err := ts.ListEntities(ctx, req) // pass through req
		if err != nil {
			return err
		}
		entities = append(entities, res.GetEntities()...)
		return nil
	}); err != nil {
		return nil, err
	}
	// Logging added for b/213616631 to see ListEntities progress on the DUT.
	logging.Debug(ctx, "Finish serving ListEntities Request")
	exec.Command("logger", "local_test_runner: Finish serving ListEntities Request").Run()
	return &protocol.ListEntitiesResponse{Entities: entities}, nil
}

func (s *testServer) GlobalRuntimeVars(ctx context.Context, req *protocol.GlobalRuntimeVarsRequest) (*protocol.GlobalRuntimeVarsResponse, error) {
	var vars []*protocol.GlobalRuntimeVar
	logging.Debug(ctx, "Serving GlobalRuntimeVars Request")
	exec.Command("logger", "local_test_runner: Serving GlobalRuntimeVars Request").Run()
	// GlobalRuntimeVars should not set runtime global information during handshake.

	if err := s.forEachBundle(ctx, nil, func(ctx context.Context, ts protocol.TestServiceClient) error {
		res, err := ts.GlobalRuntimeVars(ctx, req) // pass through req
		if err != nil {
			return err
		}
		vars = append(vars, res.GetVars()...)
		return nil
	}); err != nil {
		return nil, err
	}

	logging.Debug(ctx, "Finish serving GlobalRuntimeVars Request")
	exec.Command("logger", "local_test_runner: Finish serving GlobalRuntimeVars Request").Run()
	return &protocol.GlobalRuntimeVarsResponse{Vars: vars}, nil
}

func (s *testServer) RunTests(srv protocol.TestService_RunTestsServer) error {
	// Logging added for b/213616631.
	exec.Command("logger", "local_test_runner: Serving RunTests Request").Run()
	ctx := srv.Context()
	logger := logging.NewFuncLogger(func(level logging.Level, ts time.Time, msg string) {
		srv.Send(&protocol.RunTestsResponse{
			Type: &protocol.RunTestsResponse_RunLog{
				RunLog: &protocol.RunLogEvent{
					Time:  timestamppb.New(ts),
					Text:  msg,
					Level: protocol.LevelToProto(level),
				},
			},
		})
	})
	// Logs from RunTests should not be routed to protocol.Logging service.
	ctx = logging.AttachLoggerNoPropagation(ctx, logger)

	initReq, err := srv.Recv()
	if err != nil {
		return err
	}
	if _, ok := initReq.GetType().(*protocol.RunTestsRequest_RunTestsInit); !ok {
		return errors.Errorf("RunTests: unexpected initial request message: got %T, want %T", initReq.GetType(), &protocol.RunTestsRequest_RunTestsInit{})
	}

	if s.scfg.KillStaleRunners {
		killStaleRunners(ctx, unix.SIGTERM)
	}

	return s.forEachBundle(ctx, s.bundleParams, func(ctx context.Context, ts protocol.TestServiceClient) error {
		st, err := ts.RunTests(ctx)
		if err != nil {
			return err
		}
		defer st.CloseSend()

		// Duplicate the initial request.
		if err := st.Send(initReq); err != nil {
			return err
		}

		// Relay responses.
		for {
			res, err := st.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if err := srv.Send(res); err != nil {
				return err
			}
		}
	})
}

func (s *testServer) forEachBundle(ctx context.Context, bundleParams *protocol.BundleInitParams, f func(ctx context.Context, ts protocol.TestServiceClient) error) error {
	bundlePaths, err := filepath.Glob(s.runnerParams.GetBundleGlob())
	if err != nil {
		return err
	}
	// Sort bundles for determinism.
	sort.Strings(bundlePaths)

	for _, bundlePath := range bundlePaths {
		if err := func() error {
			// Logging added for b/213616631 to see ListEntities progress on the DUT.
			logging.Debugf(ctx, "Sending request to bundle %s", bundlePath)
			cl, err := rpc.DialExec(ctx, bundlePath, true,
				&protocol.HandshakeRequest{BundleInitParams: bundleParams})
			if err != nil {
				return err
			}
			defer cl.Close()

			return f(ctx, protocol.NewTestServiceClient(cl.Conn()))
		}(); err != nil {
			return errors.Wrap(err, filepath.Base(bundlePath))
		}
	}
	return nil
}

func (s *testServer) StreamFile(req *protocol.StreamFileRequest, srv protocol.TestService_StreamFileServer) error {
	// Logging added for b/213616631.
	exec.Command("logger", "local_test_runner: Serving StreamFile Request").Run()
	path := req.Name
	ctx := srv.Context()

	fs, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(ErrFailedToReadFile, "file %v does not exist on the DUT: %v", path, err)
	}
	offset := req.GetOffset()
	// If offset is less than 0, start streaming from the bottom of the file.
	if req.Offset < 0 {
		offset = fs.Size()
	}

	const maxRetries = 10
	const interval = time.Second
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
		fs, err := os.Stat(path)
		if err != nil {
			if failures < maxRetries {
				failures = failures + 1
				continue
			}
			return errors.Wrapf(err, "failed to get size of file %v", path)
		}
		failures = 0
		if fs.Size() == offset {
			// Nothing new was added to the file.
			continue
		}
		if fs.Size() < offset {
			// The file is smaller now which may be due to file rotation.
			// Read the entire file instead.
			offset = 0
		}
		data, n, err := readFileWithOffset(path, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return errors.Wrapf(err, "failed to read file %v", path)
		}
		if n == 0 {
			continue
		}
		nextOffset := offset + n
		rspn := &protocol.StreamFileResponse{Data: data, Offset: nextOffset}
		if err := srv.Send(rspn); err != nil {
			return err
		}
		offset = nextOffset
	}
}

func readFileWithOffset(path string, offset int64) ([]byte, int64, error) {
	const megabyte = 1 << 20
	buf := make([]byte, megabyte*2)
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}
	n, err := f.Read(buf)
	if err != nil {
		return nil, 0, err
	}
	return buf[0:n], int64(n), nil
}
