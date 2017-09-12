// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chrome implements a library used for communication with Chrome.
package chrome

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"chromiumos/tast/common/testing"
	"chromiumos/tast/local/dbusutil"
	"chromiumos/tast/local/upstart"

	"github.com/godbus/dbus"

	"github.com/mafredri/cdp/devtool"
)

const (
	chromeUser    = "chronos" // Chrome Unix username
	debuggingPort = 9222      // Chrome debugging port

	defaultUser   = "testuser@gmail.com"
	defaultPass   = "testpass"
	defaultGaiaID = "gaia-id"
)

// option is a self-referential function can be used to configure Chrome.
// See https://commandcenter.blogspot.com.au/2014/01/self-referential-functions-and-design.html
// for details about this pattern.
type option func(c *Chrome)

// Auth returns an option that can be passed to New to configure the login credentials used by Chrome.
func Auth(user, pass, gaiaID string) option {
	return func(c *Chrome) {
		c.user = user
		c.pass = pass
		c.gaiaID = gaiaID
	}
}

// PreserveProfile returns an option that can be passed to New to preserve the user's existing
// cryptohome (if any) instead of wiping it before logging in.
func KeepCryptohome() option {
	return func(c *Chrome) {
		c.keepCryptohome = true
	}
}

// Chrome interacts with the currently-running Chrome instance via the
// Chrome DevTools protocol (https://chromedevtools.github.io/devtools-protocol/).
type Chrome struct {
	devt               *devtool.DevTools
	user, pass, gaiaID string // login credentials
	keepCryptohome     bool

	extsDir         string // contains subdirs with unpacked extensions
	autotestExtId   string // ID for extension exposing autotestPrivate API
	autotestExtConn *Conn  // connection to extension exposing autotestPrivate API
}

// New restarts the ui job, tells Chrome to enable testing, and logs in.
func New(ctx context.Context, opts ...option) (*Chrome, error) {
	c := &Chrome{
		devt:           devtool.New(fmt.Sprintf("http://127.0.0.1:%d", debuggingPort)),
		user:           defaultUser,
		pass:           defaultPass,
		gaiaID:         defaultGaiaID,
		keepCryptohome: false,
	}
	for _, opt := range opts {
		opt(c)
	}

	// Clean up the partially-initialized object on error.
	toClose := c
	defer func() {
		if toClose != nil {
			toClose.Close(ctx)
		}
	}()

	if err := c.writeExtensions(); err != nil {
		return nil, err
	}
	if _, err := c.restartChromeForTesting(ctx); err != nil {
		return nil, err
	}
	if !c.keepCryptohome {
		if err := clearCryptohome(ctx, c.user); err != nil {
			return nil, err
		}
	}
	if err := c.logIn(ctx); err != nil {
		return nil, err
	}

	toClose = nil
	return c, nil
}

func (c *Chrome) Close(ctx context.Context) error {
	// TODO(derat): Decide if it's okay to skip restarting the ui job here.
	// We're leaving the system in a logged-in state, but at the same time,
	// restartChromeForTesting restarts the job too, and we can shave a few
	// seconds off each UI test by not doing it again here... ¯\_(ツ)_/¯
	if c.autotestExtConn != nil {
		c.autotestExtConn.Close()
	}
	if len(c.extsDir) > 0 {
		os.RemoveAll(c.extsDir)
	}
	return nil
}

// writeExtensions creates a temporary directory and writes standard extensions needed by
// tests to it.
func (c *Chrome) writeExtensions() error {
	var err error
	if c.extsDir, err = ioutil.TempDir("", "tast_chrome_extensions."); err != nil {
		return err
	}
	if c.autotestExtId, err = writeAutotestPrivateExtension(
		filepath.Join(c.extsDir, "autotest_private_ext")); err != nil {
		return err
	}
	// Chrome hangs with a nonsensical "Extension error: Failed to load extension
	// from: . Manifest file is missing or unreadable." error if an extension directory
	// is owned by another user.
	return chownContents(c.extsDir, chromeUser)
}

// restartChromeForTesting restarts the ui job, asks session_manager to enable Chrome testing,
// and waits for Chrome to listen on its debugging port.
func (c *Chrome) restartChromeForTesting(ctx context.Context) (testPath string, err error) {
	testing.ContextLog(ctx, "Restarting ui job")
	if err := upstart.RestartJob("ui"); err != nil {
		return "", err
	}

	bus, err := dbus.SystemBus()
	if err != nil {
		return "", fmt.Errorf("failed to connect to system bus: %v", err)
	}

	testing.ContextLogf(ctx, "Waiting for %s D-Bus service", dbusutil.SessionManagerName)
	if err = dbusutil.WaitForService(ctx, bus, dbusutil.SessionManagerName); err != nil {
		return "", fmt.Errorf("failed to wait for %s: %v", dbusutil.SessionManagerName, err)
	}

	extDirs, err := getExtensionDirs(c.extsDir)
	if err != nil {
		return "", err
	}

	testing.ContextLog(ctx, "Asking session_manager to enable Chrome testing")
	obj := bus.Object(dbusutil.SessionManagerName, dbusutil.SessionManagerPath)
	method := fmt.Sprintf("%s.%s", dbusutil.SessionManagerInterface, "EnableChromeTesting")
	args := []string{
		"--remote-debugging-port=" + strconv.Itoa(debuggingPort),
		"--disable-logging-redirect",  // Disable redirection of Chrome logging into cryptohome.
		"--ash-disable-system-sounds", // Disable system startup sound.
		"--oobe-skip-postlogin",       // Skip post-login screens.
		"--disable-gaia-services",     // TODO(derat): Reconsider this if/when supporting GAIA login.
	}
	if len(extDirs) > 0 {
		args = append(args, "--load-extension="+strings.Join(extDirs, ","))
	}
	if err = obj.Call(method, 0, true, args).Store(&testPath); err != nil {
		return "", err
	}

	testing.ContextLog(ctx, "Waiting for Chrome to listen on port ", debuggingPort)
	if err = waitForPort(ctx, fmt.Sprintf("127.0.0.1:%d", debuggingPort)); err != nil {
		return "", fmt.Errorf("failed to connect to Chrome debugging port %d: %v", debuggingPort, err)
	}

	return testPath, nil
}

// NewConn creates a new Chrome renderer and returns a connection to it.
func (c *Chrome) NewConn(ctx context.Context, url string) (*Conn, error) {
	var t *devtool.Target
	var err error
	if url == "" {
		testing.ContextLog(ctx, "Creating new blank page")
		t, err = c.devt.Create(ctx)
	} else {
		testing.ContextLogf(ctx, "Creating new page with URL ", url)
		t, err = c.devt.CreateURL(ctx, url)
	}
	if err != nil {
		return nil, err
	}
	return newConn(ctx, t.WebSocketDebuggerURL)
}

// AutotestConn returns a shared connection to the autotestPrivate extension's
// background page. The connection is lazily created, and this function will
// block until the extension is loaded or ctx's deadline is reached.
func (c *Chrome) AutotestConn(ctx context.Context) (*Conn, error) {
	if c.autotestExtConn != nil {
		return c.autotestExtConn, nil
	}

	extUrl := "chrome-extension://" + c.autotestExtId + "/_generated_background_page.html"
	var target *devtool.Target
	f := func() bool {
		ts, err := c.getDevtoolTargets(ctx, func(t *devtool.Target) bool {
			return t.URL == extUrl
		})
		if err == nil && len(ts) > 0 {
			target = ts[0]
		}
		return target != nil
	}
	if err := poll(ctx, f); err != nil {
		return nil, fmt.Errorf("didn't get target: %v", err)
	}

	var err error
	c.autotestExtConn, err = newConn(ctx, target.WebSocketDebuggerURL)
	return c.autotestExtConn, err
}

// getDevtoolTargets returns all DevTools targets matched by f.
func (c *Chrome) getDevtoolTargets(ctx context.Context, f func(*devtool.Target) bool) ([]*devtool.Target, error) {
	all, err := c.devt.List(ctx)
	if err != nil {
		return nil, err
	}

	matches := make([]*devtool.Target, 0)
	for _, t := range all {
		if f(t) {
			matches = append(matches, t)
		}
	}
	return matches, nil
}

// getFirstOOBETarget returns the first OOBE-related DevTools target that it finds.
// nil is returned if no target is found.
func (c *Chrome) getFirstOOBETarget(ctx context.Context) (*devtool.Target, error) {
	targets, err := c.getDevtoolTargets(ctx, func(t *devtool.Target) bool {
		return strings.HasPrefix(t.URL, "chrome://oobe")
	})
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets[0], nil
}

// logIn logs in to a freshly-restarted Chrome instance.
// It waits for the login process to complete before returning.
func (c *Chrome) logIn(ctx context.Context) error {
	testing.ContextLog(ctx, "Finding OOBE DevTools target")
	var target *devtool.Target
	if err := poll(ctx, func() bool {
		var e error
		target, e = c.getFirstOOBETarget(ctx)
		return e == nil && target != nil
	}); err != nil {
		return fmt.Errorf("OOBE target not found: %v", err)
	}

	conn, err := newConn(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Cribbed from telemetry/internal/backends/chrome/cros_browser_backend.py in Catapult.
	testing.ContextLog(ctx, "Waiting for OOBE")
	if err = conn.WaitForExpr(ctx, "typeof Oobe == 'function' && Oobe.readyForTesting"); err != nil {
		return err
	}
	missing := true
	if err = conn.Eval(ctx, "typeof Oobe.loginForTesting == 'undefined'", &missing); err != nil {
		return err
	}
	if missing {
		return errors.New("Oobe.loginForTesting API is missing")
	}

	testing.ContextLogf(ctx, "Logging in as user %q", c.user)
	if err = conn.Exec(ctx, fmt.Sprintf("Oobe.loginForTesting('%s', '%s', '%s', false)", c.user, c.pass, c.gaiaID)); err != nil {
		return err
	}

	if err = waitForCryptohome(ctx, c.user); err != nil {
		return err
	}

	// TODO(derat): Probably need to reconnect here if Chrome restarts due to logging in as guest (or flag changes?).

	testing.ContextLogf(ctx, "Waiting for OOBE to be dismissed")
	if err = poll(ctx, func() bool {
		t, err := c.getFirstOOBETarget(ctx)
		return err == nil && t == nil
	}); err != nil {
		return fmt.Errorf("OOBE not dismissed: %v", err)
	}
	return nil
}
