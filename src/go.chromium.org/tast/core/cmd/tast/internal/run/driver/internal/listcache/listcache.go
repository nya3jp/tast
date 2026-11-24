// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package listcache stores ListEntities responses of test runners on the host.
package listcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
)

// maxEntries is the number of responses a Cache keeps.
const maxEntries = 5

const entrySuffix = ".pb"

// Cache stores ListEntities responses of a test runner in files under a
// directory. An entry is keyed by the contents of the test runner and bundle
// executables, the runner initialization parameters, and the request, which
// together determine the response. A nil Cache stores nothing.
type Cache struct {
	dir    string
	paths  []string
	stamp  string                 // fileStamp(paths) in New
	digest func() ([]byte, error) // returns a hash of the executables
}

// New returns a Cache that stores responses under dir for the test runner and
// bundle executables at paths on the host. It must be called before the
// executables are pushed to the DUT: the Cache works only while they stay
// unchanged, which ensures that the hashed executables are the pushed ones.
func New(dir string, paths ...string) *Cache {
	// Take the stamp before hashing starts, so that an executable rewritten
	// at any time after that no longer matches it.
	stamp := fileStamp(paths)
	digest := sync.OnceValues(func() ([]byte, error) { return hashFiles(paths) })
	// Start hashing now, so that it overlaps with pushing the executables.
	go digest()
	return &Cache{dir: dir, paths: paths, stamp: stamp, digest: digest}
}

// fileStamp returns the sizes and modification times of the files at paths,
// which change when a file is rewritten, or "" if a file cannot be examined.
func fileStamp(paths []string) string {
	var sb strings.Builder
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return ""
		}
		fmt.Fprintln(&sb, fi.Size(), fi.ModTime().UnixNano())
	}
	return sb.String()
}

// hashFiles returns a hash of the hashes of the files at paths.
func hashFiles(paths []string) ([]byte, error) {
	h := sha256.New()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		fh := sha256.New()
		_, err = io.Copy(fh, f)
		f.Close()
		if err != nil {
			return nil, err
		}
		h.Write(fh.Sum(nil))
	}
	return h.Sum(nil), nil
}

// path returns the path of the entry for req sent to a test runner initialized
// with params.
func (c *Cache) path(params *protocol.RunnerInitParams, req *protocol.ListEntitiesRequest) (string, error) {
	digest, err := c.digest()
	if err != nil {
		return "", errors.Wrap(err, "failed to hash executables")
	}
	// The executables were hashed and pushed to the DUT after New. If another
	// process rewrote them meanwhile, the pushed ones may not be the hashed
	// ones.
	if s := fileStamp(c.paths); s == "" || s != c.stamp {
		return "", errors.New("executables were rewritten during the run")
	}
	h := sha256.New()
	h.Write(digest)
	for _, m := range []proto.Message{params, req} {
		b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
		if err != nil {
			return "", err
		}
		// Hash the length too, to separate the messages unambiguously.
		fmt.Fprintf(h, "%d:", len(b))
		h.Write(b)
	}
	return filepath.Join(c.dir, hex.EncodeToString(h.Sum(nil))+entrySuffix), nil
}

// Load returns the response to req sent to a test runner initialized with
// params, or nil if no valid entry is stored.
func (c *Cache) Load(ctx context.Context, params *protocol.RunnerInitParams, req *protocol.ListEntitiesRequest) *protocol.ListEntitiesResponse {
	if c == nil {
		return nil
	}
	path, err := c.path(params, req)
	if err != nil {
		logging.Infof(ctx, "Not using cached ListEntities responses: %v", err)
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// An entry is the SHA-256 of a serialized response followed by the
	// serialized response. Entries are written in place, so the checksum
	// rejects ones written partially by a concurrent or interrupted run.
	if len(b) >= sha256.Size {
		sum, data := [sha256.Size]byte(b[:sha256.Size]), b[sha256.Size:]
		res := &protocol.ListEntitiesResponse{}
		if sha256.Sum256(data) == sum && proto.Unmarshal(data, res) == nil {
			logging.Infof(ctx, "Using cached ListEntities response %s", path)
			return res
		}
	}
	logging.Infof(ctx, "Ignoring broken cached ListEntities response %s", path)
	return nil
}

// Store stores res as the response to req sent to a test runner initialized
// with params, and removes old entries. Errors are logged and otherwise
// ignored.
func (c *Cache) Store(ctx context.Context, params *protocol.RunnerInitParams, req *protocol.ListEntitiesRequest, res *protocol.ListEntitiesResponse) {
	if c == nil {
		return
	}
	if err := c.store(params, req, res); err != nil {
		logging.Infof(ctx, "Failed to cache ListEntities response: %v", err)
	}
}

func (c *Cache) store(params *protocol.RunnerInitParams, req *protocol.ListEntitiesRequest, res *protocol.ListEntitiesResponse) error {
	path, err := c.path(params, req)
	if err != nil {
		return err
	}
	data, err := proto.Marshal(res)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if err := os.WriteFile(path, append(sum[:], data...), 0644); err != nil {
		return err
	}

	// Remove entries other than the maxEntries newest ones.
	paths, err := filepath.Glob(filepath.Join(c.dir, "*"+entrySuffix))
	if err != nil {
		return err
	}
	mtimes := make(map[string]time.Time)
	for _, p := range paths {
		// An entry removed by a concurrent run counts as the oldest.
		if fi, err := os.Stat(p); err == nil {
			mtimes[p] = fi.ModTime()
		}
	}
	sort.Slice(paths, func(i, j int) bool { return mtimes[paths[i]].After(mtimes[paths[j]]) })
	for _, p := range paths[min(len(paths), maxEntries):] {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
