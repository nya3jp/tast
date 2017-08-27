// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dbusutil

import (
	"testing"

	"github.com/godbus/dbus"
)

func TestMatchesSignal(t *testing.T) {
	const (
		sender = "org.example.MyService"
		path   = "/org/example/MyService"
		iface  = "org.example.MyServiceInterface"
		member = "MySignal"
		arg0   = "MyArg"
	)

	sig := &dbus.Signal{
		Sender: sender,
		Path:   dbus.ObjectPath(path),
		Name:   iface + "." + member,
		Body:   []interface{}{arg0},
	}

	for _, tc := range []struct {
		spec MatchSpec
		sig  *dbus.Signal
		exp  bool
	}{
		{MatchSpec{}, sig, true},
		{MatchSpec{Type: "signal"}, sig, true},
		{MatchSpec{Type: "method"}, sig, false},
		{MatchSpec{Path: dbus.ObjectPath(path)}, sig, true},
		{MatchSpec{Path: dbus.ObjectPath("/foo")}, sig, false},
		{MatchSpec{Sender: sender}, sig, true},
		{MatchSpec{Sender: "bloop"}, sig, false},
		{MatchSpec{Interface: iface}, sig, true},
		{MatchSpec{Interface: "org.blah"}, sig, false},
		{MatchSpec{Member: member}, sig, true},
		{MatchSpec{Member: "blippo"}, sig, false},
		{MatchSpec{Arg0: arg0}, sig, true},
		{MatchSpec{Arg0: "splat"}, sig, false},
		{MatchSpec{
			Type:      "signal",
			Path:      dbus.ObjectPath(path),
			Sender:    sender,
			Interface: iface,
			Member:    member,
			Arg0:      arg0,
		}, sig, true},
		{MatchSpec{Arg0: arg0}, &dbus.Signal{}, false},
	} {
		if act := tc.spec.MatchesSignal(tc.sig); act != tc.exp {
			t.Errorf("%v.MatchesSignal(%v) = %v; want %v", tc.spec, tc.sig, act, tc.exp)
		}
	}
}
