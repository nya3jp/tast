// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"encoding/json"
	"fmt"

	"chromiumos/tast/common/testing"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/rpcc"
)

// Conn represents a connection to a web content view, e.g. a tab.
type Conn struct {
	co *rpcc.Conn
	cl *cdp.Client
}

func newConn(ctx context.Context, url string) (*Conn, error) {
	testing.ContextLog(ctx, "Connecting to Chrome at ", url)
	co, err := rpcc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}

	c := &Conn{
		co: co,
		cl: cdp.NewClient(co),
	}
	if err = c.cl.Page.Enable(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

// Close frees the connection's resources.
func (c *Conn) Close() {
	c.co.Close()
}

// Exec executes the JavaScript expression expr and discards its result.
// An error is returned if an exception is generated.
func (c *Conn) Exec(ctx context.Context, expr string) error {
	repl, err := c.cl.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(expr))
	if err != nil {
		return err
	}
	if repl.ExceptionDetails != nil {
		return fmt.Errorf("got exception: %s", repl.ExceptionDetails.Exception.String())
	}
	return err
}

// Eval evaluates the JavaScript expression expr and stores its result in out.
// An error is returned if the result can't be unmarshalled into out.
func (c *Conn) Eval(ctx context.Context, expr string, out interface{}) error {
	args := runtime.NewEvaluateArgs(expr).SetReturnByValue(true)
	repl, err := c.cl.Runtime.Evaluate(ctx, args)
	if err != nil {
		return err
	}
	return json.Unmarshal(repl.Result.Value, out)
}

// WaitForExpr repeatedly evaluates the JavaScript expression expr until it returns true.
func (c *Conn) WaitForExpr(ctx context.Context, expr string) error {
	return poll(ctx, func() bool {
		v := false
		if err := c.Eval(ctx, expr, &v); err != nil {
			return false
		}
		return v
	})
}

// PageContent returns the current top-level page content.
func (c *Conn) PageContent(ctx context.Context) (string, error) {
	_, err := c.cl.DOM.GetDocument(ctx, nil)
	if err != nil {
		return "", err
	}
	return "", nil
}

// Navigate navigates to url.
func (c *Conn) Navigate(ctx context.Context, url string) error {
	testing.ContextLog(ctx, "Navigating to ", url)
	fired, err := c.cl.Page.DOMContentEventFired(ctx)
	if err != nil {
		return err
	}
	defer fired.Close()

	if _, err := c.cl.Page.Navigate(ctx, page.NewNavigateArgs(url)); err != nil {
		return err
	}
	if _, err = fired.Recv(); err != nil {
		return err
	}
	return nil
}
