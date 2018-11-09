// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// proximity indicates proximity of a devserver from the local host.
type proximity string

const (
	// near indicates a devserver is on the same network as the local host.
	near proximity = "NEAR"

	// far indicates a devserver is on a different network as the local host.
	far = "FAR"
)

var errNotStaged = errors.New("no staged file found")

// resolveIPv4 resolves addr into a IPv4 address. addr is formatted as "host:port" and
// the port portion is ignored.
func resolveIPv4(ctx context.Context, addr string) (net.IP, error) {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid address %s", addr)
	}

	if _, err := strconv.ParseUint(parts[1], 10, 16); err != nil {
		return nil, fmt.Errorf("invalid address %s: %v", addr, err)
	}

	name := parts[0]
	if ip := net.ParseIP(name).To4(); len(ip) == 4 {
		return ip, nil
	}

	ipaddrs, err := net.DefaultResolver.LookupIPAddr(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %v", name, err)
	}

	var ip net.IP
	for _, ipaddr := range ipaddrs {
		i := ipaddr.IP.To4()
		if len(i) == 4 {
			ip = i
			break
		}
	}
	if ip != nil {
		return nil, fmt.Errorf("failed to resolve %s: no IPv4 address", name)
	}
	return ip, nil
}

// checkHealth makes a HTTP request to the devserver at addr to check if it is up.
func checkHealth(ctx context.Context, cl *http.Client, addr string) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/check_health", addr), nil)
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

// computeProximity computes proximity of the devserver at addr. If the server is down, non-nil error
// describing why it is considered down is returned.
func computeProximity(ctx context.Context, cl *http.Client, localNets networks, addr string) (proximity, error) {
	ip, err := resolveIPv4(ctx, addr)
	if err != nil {
		return far, err
	}
	var prox proximity
	if localNets.Contains(ip) {
		prox = near
	} else {
		prox = far
	}
	return prox, checkHealth(ctx, cl, addr)
}

// DevserverClient is an implementation of Client to download files from GCS via devservers.
type DevserverClient struct {
	cl *http.Client

	selected []string    // selected devserver addresses
	all      []devserver // all devservers
}

type devserver struct {
	addr string // address of a devserver in "host:port" format
	prox proximity
	err  error // nil if the server is up; otherwise describes why it is considered down
}

func (ds devserver) String() string {
	if ds.err == nil {
		return fmt.Sprintf("[%s %s UP]", ds.addr, ds.prox)
	}
	return fmt.Sprintf("[%s %s DOWN (%v)]", ds.addr, ds.prox, ds.err)
}

var _ Client = &DevserverClient{}

// NewDevserverClient creates a DevserverClient. If cl is nil, a default HTTP client is used.
func NewDevserverClient(cl *http.Client) *DevserverClient {
	if cl == nil {
		cl = defaultHTTPClient()
	}
	return &DevserverClient{cl: cl}
}

// SetServers checks if servers specified by addrs are up, and selects a subset of servers to use.
// If we can not verify a server is up within ctx's timeout, it is considered down. Be sure to
// set ctx's timeout carefully since this method can block until it expires if any server is down.
func (c *DevserverClient) SetServers(ctx context.Context, addrs []string) error {
	localNets, err := localNetworks()
	if err != nil {
		return err
	}

	ch := make(chan devserver, len(addrs))

	for _, addr := range addrs {
		go func(addr string) {
			prox, err := computeProximity(ctx, c.cl, localNets, addr)
			ch <- devserver{addr, prox, err}
		}(addr)
	}

	var all []devserver
	ups := make(map[proximity][]string)
	for range addrs {
		ds := <-ch
		all = append(all, ds)
		if ds.err == nil {
			ups[ds.prox] = append(ups[ds.prox], ds.addr)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].addr < all[j].addr
	})

	c.all = all
	if len(ups[near]) > 0 {
		c.selected = ups[near]
	} else {
		c.selected = ups[far]
	}
	return nil
}

// Status returns a message describing the status of devservers.
func (c *DevserverClient) Status() string {
	return fmt.Sprint(c.all)
}

// Download downloads a file on GCS via devservers. It returns an error if no devserver is up.
func (c *DevserverClient) Download(ctx context.Context, w io.Writer, gsURL string) (size int64, err error) {
	bucket, path, err := parseGSURL(gsURL)
	if err != nil {
		return 0, err
	}

	if len(c.selected) == 0 {
		return 0, errors.New("no devserver is up")
	}

	sctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Use an already staged file if there is any.
	if addr, err := c.findStaged(sctx, bucket, path); err == nil {
		size, err := c.downloadFrom(ctx, w, addr, bucket, path)
		if err != nil {
			return 0, fmt.Errorf("failed to download: %v", err)
		}
		return size, nil
	} else if err != errNotStaged {
		return 0, fmt.Errorf("failed to find staged files: %v", err)
	}

	// Choose a devserver and download the file via it.
	addr := c.chooseServer(gsURL)
	if err := c.stage(ctx, addr, bucket, path); err != nil {
		return 0, fmt.Errorf("failed to stage: %v", err)
	}
	size, err = c.downloadFrom(ctx, w, addr, bucket, path)
	if err != nil {
		return 0, fmt.Errorf("failed to download: %v", err)
	}
	return size, nil
}

// findStages tries to find an already staged file from selected servers.
// It returns errNotStaged if no staged file is found.
func (c *DevserverClient) findStaged(ctx context.Context, bucket, path string) (addr string, err error) {
	ch := make(chan string, len(c.selected))
	for _, addr := range c.selected {
		go func(addr string) {
			if err := c.checkStaged(ctx, addr, bucket, path); err != nil {
				ch <- ""
			} else {
				ch <- addr
			}
		}(addr)
	}

	var found []string
	for range c.selected {
		addr := <-ch
		if addr != "" {
			found = append(found, addr)
		}
	}

	if len(found) == 0 {
		return "", errNotStaged
	}
	return found[rand.Intn(len(found))], nil
}

// checkStaged checks if a file is staged on the devserver at addr.
// It returned errNotStaged if a file is not yet staged.
func (c *DevserverClient) checkStaged(ctx context.Context, addr, bucket, path string) error {
	staticURL := url.URL{
		Scheme: "http",
		Host:   addr,
		Path:   "/static/" + path,
	}
	req, err := http.NewRequest("HEAD", staticURL.String(), nil)
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
func (c *DevserverClient) chooseServer(gsURL string) string {
	servers := make([]string, len(c.selected))
	copy(servers, c.selected)

	score := func(i int) uint32 {
		return crc32.ChecksumIEEE([]byte(servers[i] + "\x00" + gsURL))
	}
	sort.Slice(servers, func(i, j int) bool {
		return score(i) < score(j)
	})
	return servers[0]
}

// stages requests the devserver at addr to stage a file.
func (c *DevserverClient) stage(ctx context.Context, addr, bucket, gsPath string) error {
	gsDirURL := url.URL{
		Scheme: "gs",
		Host:   bucket,
		Path:   path.Dir(gsPath),
	}
	values := url.Values{
		"archive_url": {gsDirURL.String()},
		"files":       {path.Base(gsPath)},
	}
	stageURL := url.URL{
		Scheme:   "http",
		Host:     addr,
		Path:     "/stage",
		RawQuery: values.Encode(),
	}
	req, err := http.NewRequest("GET", stageURL.String(), nil)
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

// downloadFrom downloads a file from the devserver at addr.
func (c *DevserverClient) downloadFrom(ctx context.Context, w io.Writer, addr, bucket, path string) (size int64, err error) {
	staticURL := url.URL{
		Scheme: "http",
		Host:   addr,
		Path:   "/static/" + path,
	}
	req, err := http.NewRequest("GET", staticURL.String(), nil)
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
