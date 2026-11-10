// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast/core/autocaps"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/lsbrelease"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

// dutInfoCachePath caches a GetDUTInfo response with features. /run is a
// tmpfs, so the cache never outlives a boot.
const dutInfoCachePath = "/run/tast/dut_info_cache.pb"

// dutInfoCacheInputs lists files and directories whose stat metadata (and
// direct entries' stat metadata for directories) changes when GetDUTInfo inputs
// update without a reboot, in addition to the runner executable.
var dutInfoCacheInputs = []string{lsbrelease.Path, useFlagsFile, autocaps.DefaultCapabilityDir}

// GetDUTInfo implements the GetDUTInfo RPC method for ChromeOS.
func GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	return dutInfoCache{
		path:  dutInfoCachePath,
		keyFn: dutInfoCacheKey,
		getFn: getDUTInfo,
	}.get(ctx, req)
}

type dutInfoCache struct {
	path  string
	keyFn func(*protocol.GetDUTInfoRequest) (string, error)
	getFn func(context.Context, *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error)
}

// get returns DUT info for req, reusing the response cached in c.path when
// both features and caching are requested.
func (c dutInfoCache) get(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	if !req.GetFeatures() || !req.GetAllowCache() {
		return c.getFn(ctx, req)
	}
	// Detecting features takes several seconds on low-end DUTs, so reuse the
	// response for the same request earlier in this boot.
	key, err := c.keyFn(req)
	if err != nil {
		logging.Infof(ctx, "Not caching DUT info: %v", err)
		return c.getFn(ctx, req)
	}
	if res, err := loadDUTInfoCache(c.path, key); err == nil { // if NO error
		logging.Info(ctx, "Reusing DUT info detected earlier in this boot")
		return res, nil
	}
	res, err := c.getFn(ctx, req)
	if err != nil {
		return nil, err
	}
	// detectHardwareFeatures logs probe failures and returns a nil error, so a
	// canceled request can yield partial features; avoid persisting them.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := saveDUTInfoCache(c.path, key, res); err != nil {
		logging.Infof(ctx, "Failed to cache DUT info: %v", err)
	}
	return res, nil
}

// dutInfoCacheKey returns a cache key for req which changes on reboot, on an
// update of the runner, or on a change of dutInfoCacheInputs.
func dutInfoCacheKey(req *protocol.GetDUTInfoRequest) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return computeDUTInfoCacheKey(req, string(bootID), append([]string{exe}, dutInfoCacheInputs...))
}

// computeDUTInfoCacheKey returns a hash of req, bootID, and the stat metadata
// of paths (including direct entries of directories).
func computeDUTInfoCacheKey(req *protocol.GetDUTInfoRequest, bootID string, paths []string) (string, error) {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(req)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "boot_id=%s\nrequest=%x\n", strings.TrimSpace(bootID), b)
	for _, p := range paths {
		if err := hashPathStat(h, p); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashPathStat writes stat metadata for path to h so updates, creations,
// removals, and (for directories) changes to direct entries alter the hash.
func hashPathStat(h io.Writer, path string) error {
	// Size and mtime suffice to detect updates without reading large files such
	// as the runner binary itself.
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(h, "%q missing\n", path)
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		fmt.Fprintf(h, "%q %d %d\n", path, fi.Size(), fi.ModTime().UnixNano())
		return nil
	}
	// os.Stat on a directory misses in-place edits to existing files inside it,
	// so stat each direct entry (autocaps.Read only reads top-level files).
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(h, "%q dir %d\n", path, len(entries))
	for _, entry := range entries {
		p := filepath.Join(path, entry.Name())
		efi, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(h, "%q missing\n", p)
			continue
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%q %d %d\n", p, efi.Size(), efi.ModTime().UnixNano())
	}
	return nil
}

// loadDUTInfoCache returns the response cached in path under key.
func loadDUTInfoCache(path, key string) (*protocol.GetDUTInfoResponse, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cachedKey, data, found := bytes.Cut(b, []byte("\n"))
	if !found || string(cachedKey) != key {
		return nil, errors.New("cache key mismatch")
	}
	res := &protocol.GetDUTInfoResponse{}
	if err := proto.Unmarshal(data, res); err != nil {
		return nil, err
	}
	return res, nil
}

// saveDUTInfoCache caches res in path under key.
func saveDUTInfoCache(path, key string, res *protocol.GetDUTInfoResponse) error {
	data, err := proto.Marshal(res)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Write to a temporary file and rename it so that readers never see a
	// partially written cache.
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // fails harmlessly after the rename
	if _, err := f.Write(append([]byte(key+"\n"), data...)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// getDUTInfo detects DUT info for req.
func getDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
	var osVersion, defaultBuildArtifactsURL string

	if _, err := os.Stat(lsbrelease.Path); err == nil {
		kvs, err := lsbrelease.Load()
		if err != nil {
			return nil, err
		}

		if bp := kvs[lsbrelease.BuilderPath]; bp != "" {
			osVersion = bp
			defaultBuildArtifactsURL = "gs://chromeos-image-archive/" + bp + "/"
		} else {
			// Sometimes CHROMEOS_RELEASE_BUILDER_PATH is not in /etc/lsb-release.
			// Make up the string in this case.
			board := kvs[lsbrelease.Board]
			version := kvs[lsbrelease.Version]
			milestone := kvs[lsbrelease.Milestone]
			buildType := kvs[lsbrelease.BuildType]
			osVersion = fmt.Sprintf("%vR%v-%v (%v)", board, milestone, version, buildType)
		}
	} else {
		osVersion = "NotAvail"
	}

	var softwareFeatures *frameworkprotocol.SoftwareFeatures
	var hardwareFeatures *frameworkprotocol.HardwareFeatures

	if req.GetFeatures() {
		var err error
		softwareFeatures, err = detectSoftwareFeatures(ctx, req.GetExtraUseFlags())
		if err != nil {
			return nil, err
		}
		hardwareFeatures, err = detectHardwareFeatures(ctx)
		if err != nil {
			return nil, err
		}
	}

	return &protocol.GetDUTInfoResponse{
		DutInfo: &protocol.DUTInfo{
			Features: &frameworkprotocol.DUTFeatures{
				Software: softwareFeatures,
				Hardware: hardwareFeatures,
			},
			OsVersion:                osVersion,
			DefaultBuildArtifactsUrl: defaultBuildArtifactsURL,
		},
	}, nil
}
