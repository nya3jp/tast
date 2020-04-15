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
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
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
		out, _ := ioutil.ReadAll(res.Body)
		s := scrapeInternalError(out)
		return fmt.Errorf("check_health returned %d: %s", res.StatusCode, s)
	}
	return nil
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

// RealClient is an implementation of Client to communicate with real devservers.
type RealClient struct {
	servers         []server
	cl              *http.Client
	stageRetryWaits []time.Duration
	logFunc         func(string)
}

var _ Client = &RealClient{}

// RealClientOptions contains options used when connecting to devserver.
type RealClientOptions struct {
	// HTTPClient is HTTP client to use. If nil, defaultHTTPClient is used.
	HTTPClient *http.Client

	// StageRetryWaits instructs retry strategy for stage.
	// Its length is the number of retries and the i-th value is the interval before i-th retry.
	// If nil, default strategy is used. If zero-length slice, no retry is attempted.
	StageRetryWaits []time.Duration

	// LogFunc (if non-nil) is used to log non-fatal errors.
	LogFunc func(msg string)
}

var defaultOptions = &RealClientOptions{
	HTTPClient:      defaultHTTPClient,
	StageRetryWaits: []time.Duration{time.Duration(3 * time.Second)},
	LogFunc:         func(string) {},
}

// NewRealClient creates a RealClient.
// This function checks if devservers at dsURLs are up, and selects a subset of devservers to use.
// A devserver URL is usually in the form of "http://<hostname>:<port>", without trailing slashes.
// If we can not verify a devserver is up within ctx's timeout, it is considered down. Be sure to
// set ctx's timeout carefully since this function can block until it expires if any devserver is down.
// If o is nil, default options are used. If o is partially nil, defaults are used for them.
func NewRealClient(ctx context.Context, dsURLs []string, o *RealClientOptions) *RealClient {
	if o == nil {
		o = &RealClientOptions{}
	}
	cl := o.HTTPClient
	if cl == nil {
		cl = defaultOptions.HTTPClient
	}
	stageRetryWaits := o.StageRetryWaits
	if stageRetryWaits == nil {
		stageRetryWaits = defaultOptions.StageRetryWaits
	}
	logFunc := o.LogFunc
	if logFunc == nil {
		logFunc = defaultOptions.LogFunc
	}

	ch := make(chan server, len(dsURLs))

	for _, dsURL := range dsURLs {
		go func(dsURL string) {
			err := checkHealth(ctx, cl, dsURL)
			ch <- server{dsURL, err}
		}(dsURL)
	}

	var servers []server
	for range dsURLs {
		servers = append(servers, <-ch)
	}
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].url < servers[j].url
	})

	return &RealClient{servers, cl, stageRetryWaits, logFunc}
}

// upServerURLs returns URLs of operational devservers.
func (c *RealClient) upServerURLs() []string {
	var urls []string
	for _, s := range c.servers {
		if s.err == nil {
			urls = append(urls, s.url)
		}
	}
	return urls
}

// Status returns a message describing the status of devservers.
func (c *RealClient) Status() string {
	return fmt.Sprint(c.servers)
}

// Open downloads a file on GCS via devservers. It returns an error if no devserver is up.
func (c *RealClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	bucket, path, err := parseGSURL(gsURL)
	if err != nil {
		return nil, err
	}

	if len(c.upServerURLs()) == 0 {
		return nil, errors.New("no devserver is up")
	}

	sctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Use an already staged file if there is any.
	if dsURL, err := c.findStaged(sctx, bucket, path); err == nil {
		r, err := c.openStaged(ctx, dsURL, bucket, path)
		if err != nil {
			return nil, fmt.Errorf("failed to download from %s: %v", dsURL, err)
		}
		return r, nil
	} else if err != errNotStaged {
		return nil, fmt.Errorf("failed to find a staged file: %v", err)
	}

	// Choose a devserver and download the file via it.
	dsURL := c.chooseServer(gsURL)
	if err := c.stage(ctx, dsURL, bucket, path); err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to stage on %s: %v", dsURL, err)
	}

	// Do a sanity check that the file has been staged successfully.
	if err := c.checkStaged(ctx, dsURL, bucket, path); err != nil {
		return nil, fmt.Errorf("failed to stage on %s: %v", dsURL, err)
	}

	r, err := c.openStaged(ctx, dsURL, bucket, path)
	if err != nil {
		return nil, fmt.Errorf("failed to download from %s: %v", dsURL, err)
	}
	return r, nil
}

// findStaged tries to find an already staged file from selected servers.
// It returns errNotStaged if no staged file is found.
func (c *RealClient) findStaged(ctx context.Context, bucket, path string) (dsURL string, err error) {
	dsURLs := c.upServerURLs()
	ch := make(chan string, len(dsURLs))

	for _, dsURL := range dsURLs {
		go func(dsURL string) {
			if err := c.checkStaged(ctx, dsURL, bucket, path); err != nil {
				ch <- ""
			} else {
				ch <- dsURL
			}
		}(dsURL)
	}

	var found []string
	for range dsURLs {
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
// It returns errNotStaged if a file is not yet staged.
func (c *RealClient) checkStaged(ctx context.Context, dsURL, bucket, gsPath string) error {
	gsDirURL := url.URL{
		Scheme: "gs",
		Host:   bucket,
		Path:   path.Dir(gsPath),
	}
	values := url.Values{
		"archive_url": {gsDirURL.String()},
		"files":       {path.Base(gsPath)},
	}
	checkURL := fmt.Sprintf("%s/is_staged?%s", dsURL, values.Encode())
	req, err := http.NewRequest("GET", checkURL, nil)
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
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}
		switch val := strings.TrimSpace(string(b)); val {
		case "True":
			return nil
		case "False":
			return errNotStaged
		case "This is an ephemeral devserver provided by Tast.":
			// TODO(nya): Remove this check after 20190710.
			return fmt.Errorf("tast command is old; please run ./update_chroot")
		default:
			return fmt.Errorf("got response %q", val)
		}
	case http.StatusInternalServerError:
		out, _ := ioutil.ReadAll(res.Body)
		err := scrapeInternalError(out)
		return fmt.Errorf("got status %d: %s", res.StatusCode, err)
	default:
		return fmt.Errorf("got status %d", res.StatusCode)
	}
}

// chooseServer chooses a devserver to use from c.selected. It tries to choose
// the same server for the same gsURL.
func (c *RealClient) chooseServer(gsURL string) string {
	dsURLs := c.upServerURLs()

	// score returns a random number from a devserver URL and a file URL as seeds.
	// By using this function, the same devserver is usually selected for a file
	// provided that the same set of devservers are up.
	score := func(i int) uint32 {
		return crc32.ChecksumIEEE([]byte(dsURLs[i] + "\x00" + gsURL))
	}
	sort.Slice(dsURLs, func(i, j int) bool {
		return score(i) < score(j)
	})
	return dsURLs[0]
}

// stage requests the devserver at dsURL to stage a file.
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

	for i := 0; ; i++ {
		start := time.Now()

		retryable, err := c.sendStageRequest(ctx, req)
		if err == nil || !retryable || i >= len(c.stageRetryWaits) {
			return err
		}

		elapsed := time.Now().Sub(start)
		if remaining := c.stageRetryWaits[i] - elapsed; remaining > 0 {
			c.logFunc(fmt.Sprintf("Retry stage in %v: %v", remaining.Round(time.Millisecond), err))
			select {
			case <-time.After(remaining):
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			c.logFunc(fmt.Sprintf("Retrying stage: %v", err))
		}
	}
}

// sendStageRequest sends the stage request to devserver.
// It analyzes error (if any) and determines if it is retryable.
func (c *RealClient) sendStageRequest(ctx context.Context, req *http.Request) (retryable bool, err error) {
	res, err := c.cl.Do(req)
	if err != nil {
		return true, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return false, nil
	case http.StatusInternalServerError:
		out, _ := ioutil.ReadAll(res.Body)
		s := scrapeInternalError(out)
		if strings.Contains(s, "Could not find") || strings.Contains(s, "file not found") {
			return false, os.ErrNotExist
		}
		return true, fmt.Errorf("got status %d: %s", res.StatusCode, s)
	default:
		return true, fmt.Errorf("got status %d", res.StatusCode)
	}
}

// openStaged opens a staged file from the devserver at dsURL.
func (c *RealClient) openStaged(ctx context.Context, dsURL, bucket, path string) (io.ReadCloser, error) {
	staticURL, err := url.Parse(dsURL)
	if err != nil {
		return nil, err
	}
	staticURL.Path += "/static/" + path
	query := make(url.Values)
	query.Set("gs_bucket", bucket)
	staticURL.RawQuery = query.Encode()
	req, err := http.NewRequest("GET", staticURL.String(), nil)
	if err != nil {
		return nil, err
	}
	// Negotiate header disables automatic content negotiation. See:
	// https://crbug.com/967305
	// https://tools.ietf.org/html/rfc2295#section-8.4
	req.Header.Set("Negotiate", "vlist")
	req = req.WithContext(ctx)

	res, err := c.cl.Do(req)
	if err != nil {
		return nil, err
	}

	switch res.StatusCode {
	case http.StatusOK:
		return res.Body, nil
	case http.StatusInternalServerError:
		defer res.Body.Close()
		out, _ := ioutil.ReadAll(res.Body)
		s := scrapeInternalError(out)
		return nil, fmt.Errorf("got status %d: %s", res.StatusCode, s)
	default:
		res.Body.Close()
		return nil, fmt.Errorf("got status %d", res.StatusCode)
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
