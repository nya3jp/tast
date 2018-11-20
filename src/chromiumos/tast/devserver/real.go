// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"time"
)

var errNotStaged = errors.New("no staged file found")

// checkHealth makes a HTTP request to the devserver at dsURL to check if it is up.
func checkHealth(ctx context.Context, cl *http.Client, dsURL string) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/check_health", dsURL), nil)
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("check_health returned %d", res.StatusCode)
	}
	return nil
}

// RealClient is an implementation of Client to communicate with real devservers.
type RealClient struct {
	cl *http.Client

	all []server // all devservers
	ups []string // running devserver URLs
}

type server struct {
	url string // URL of a devserver in "http://host:port" format
	err error  // nil if the server is up; otherwise describes why it is considered down
}

func (s server) String() string {
	if s.err == nil {
		return fmt.Sprintf("[%s UP]", s.url)
	}
	return fmt.Sprintf("[%s DOWN (%v)]", s.url, s.err)
}

var _ Client = &RealClient{}

// NewRealClient creates a RealClient. If cl is nil, a default HTTP client is used.
func NewRealClient(cl *http.Client) *RealClient {
	if cl == nil {
		cl = defaultHTTPClient()
	}
	return &RealClient{cl: cl}
}

// SetServers checks if servers specified by dsURLs are up, and selects a subset of servers to use.
// If we can not verify a server is up within ctx's timeout, it is considered down. Be sure to
// set ctx's timeout carefully since this method can block until it expires if any server is down.
func (c *RealClient) SetServers(ctx context.Context, dsURLs []string) error {
	ch := make(chan server, len(dsURLs))

	for _, dsURL := range dsURLs {
		go func(dsURL string) {
			err := checkHealth(ctx, c.cl, dsURL)
			ch <- server{dsURL, err}
		}(dsURL)
	}

	var all []server
	var ups []string
	for range dsURLs {
		s := <-ch
		all = append(all, s)
		if s.err == nil {
			ups = append(ups, s.url)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].url < all[j].url
	})
	sort.Strings(ups)

	c.all = all
	c.ups = ups
	return nil
}

// Status returns a message describing the status of devservers.
func (c *RealClient) Status() string {
	return fmt.Sprint(c.all)
}

// DownloadGS downloads a file on GCS via devservers. It returns an error if no devserver is up.
func (c *RealClient) DownloadGS(ctx context.Context, w io.Writer, gsURL string) (size int64, err error) {
	bucket, path, err := parseGSURL(gsURL)
	if err != nil {
		return 0, err
	}

	if len(c.ups) == 0 {
		return 0, errors.New("no devserver is up")
	}

	sctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Use an already staged file if there is any.
	if dsURL, err := c.findStaged(sctx, bucket, path); err == nil {
		size, err := c.downloadFrom(ctx, w, dsURL, bucket, path)
		if err != nil {
			return 0, fmt.Errorf("failed to download: %v", err)
		}
		return size, nil
	} else if err != errNotStaged {
		return 0, fmt.Errorf("failed to find staged files: %v", err)
	}

	// Choose a devserver and download the file via it.
	dsURL := c.chooseServer(gsURL)
	if err := c.stage(ctx, dsURL, bucket, path); err != nil {
		return 0, fmt.Errorf("failed to stage: %v", err)
	}
	size, err = c.downloadFrom(ctx, w, dsURL, bucket, path)
	if err != nil {
		return 0, fmt.Errorf("failed to download: %v", err)
	}
	return size, nil
}

// findStages tries to find an already staged file from selected servers.
// It returns errNotStaged if no staged file is found.
func (c *RealClient) findStaged(ctx context.Context, bucket, path string) (dsURL string, err error) {
	ch := make(chan string, len(c.ups))

	for _, dsURL := range c.ups {
		go func(dsURL string) {
			if err := c.checkStaged(ctx, dsURL, bucket, path); err != nil {
				ch <- ""
			} else {
				ch <- dsURL
			}
		}(dsURL)
	}

	var found []string
	for range c.ups {
		dsURL := <-ch
		if dsURL != "" {
			found = append(found, dsURL)
		}
	}

	if len(found) == 0 {
		return "", errNotStaged
	}
	return found[rand.Intn(len(found))], nil
}

// checkStaged checks if a file is staged on the devserver at dsURL.
// It returned errNotStaged if a file is not yet staged.
func (c *RealClient) checkStaged(ctx context.Context, dsURL, bucket, path string) error {
	staticURL := fmt.Sprintf("%s/static/%s", dsURL, path)
	req, err := http.NewRequest("HEAD", staticURL, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	res, err := c.cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return errNotStaged
	case http.StatusInternalServerError:
		out, _ := ioutil.ReadAll(res.Body)
		err := scrapeInternalError(out)
		return fmt.Errorf("got status %d: %s", res.StatusCode, err)
	default:
		return fmt.Errorf("got status %d", res.StatusCode)
	}
}

// chooseServers chooses a devserver to use from c.selected. It tries to choose
// the same server for the same gsURL.
func (c *RealClient) chooseServer(gsURL string) string {
	dsURLs := make([]string, len(c.ups))
	copy(dsURLs, c.ups)

	score := func(i int) uint32 {
		return crc32.ChecksumIEEE([]byte(dsURLs[i] + "\x00" + gsURL))
	}
	sort.Slice(dsURLs, func(i, j int) bool {
		return score(i) < score(j)
	})
	return dsURLs[0]
}

// stages requests the devserver at dsURL to stage a file.
func (c *RealClient) stage(ctx context.Context, dsURL, bucket, gsPath string) error {
	gsDirURL := url.URL{
		Scheme: "gs",
		Host:   bucket,
		Path:   path.Dir(gsPath),
	}
	values := url.Values{
		"archive_url": {gsDirURL.String()},
		"files":       {path.Base(gsPath)},
	}
	stageURL := fmt.Sprintf("%s/stage?%s", dsURL, values.Encode())
	req, err := http.NewRequest("GET", stageURL, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	res, err := c.cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusInternalServerError:
		out, _ := ioutil.ReadAll(res.Body)
		s := scrapeInternalError(out)
		return fmt.Errorf("got status %d: %s", res.StatusCode, s)
	default:
		return fmt.Errorf("got status %d", res.StatusCode)
	}
}

// downloadFrom downloads a file from the devserver at dsURL.
func (c *RealClient) downloadFrom(ctx context.Context, w io.Writer, dsURL, bucket, path string) (size int64, err error) {
	staticURL := fmt.Sprintf("%s/static/%s", dsURL, path)
	req, err := http.NewRequest("GET", staticURL, nil)
	if err != nil {
		return 0, err
	}
	req = req.WithContext(ctx)

	res, err := c.cl.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return io.Copy(w, res.Body)
	case http.StatusInternalServerError:
		out, _ := ioutil.ReadAll(res.Body)
		s := scrapeInternalError(out)
		return 0, fmt.Errorf("got status %d: %s", res.StatusCode, s)
	default:
		return 0, fmt.Errorf("got status %d", res.StatusCode)
	}
}

var internalErrorRegexp = regexp.MustCompile(`(?m)^(.*)\n\s*</pre>`)

// scrapeInternalError scrapes an error message from an internal server response
// from devservers.
func scrapeInternalError(out []byte) string {
	m := internalErrorRegexp.FindStringSubmatch(string(out))
	if m == nil {
		return "unknown error"
	}
	return m[1]
}
