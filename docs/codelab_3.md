# Tast Codelab: Chrome UI Automation (go/tast-codelab-3)

> This document assumes that you've already gone through [Codelab #1].

This codelab follows the creation of a Tast test that uses the the
chrome.Automation library to change the wallpaper. It goes over the background
of chrome.Automation, how to use it, and some common issues.

[Codelab #1]: codelab_1.md


## Background

The chrome.Automation library uses the Chrome Accessibility Tree to view and
control the current state of the UI. The Accessibility Tree has access to:
* The Chrome Browser
* The ChromeOS Desktop UI
<!-- TODO(crbug/1135046) Replace "Native Apps" by more appropriate term. -->
* Native Apps
* Web Apps/PWAs

That being said, it does not have access to UI elements in containers or VMs
(like ARC and Crostini).

The Accessibility Tree is a collection of nodes that map out the entire desktop.
Accessibility Tree nodes are similar to HTML nodes, but definitely do not map to
HTML nodes. An [Accessibility Node] has many attributes, including but not limited
to:
* ID -> This changes between tests runs and cannot be used in tests.
* [Role]
* Class
* Name -> This is language dependent but often the only unique identifier.
* [Location]
* Parent Node
* Children Nodes
* [States List]

In Tast, chrome.Automation is wrapped in [chrome/ui] and can be imported like so:
```go
import "chromiumos/tast/local/chrome/ui"
```
[Accessibility Node]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/ui#Node
[chrome/ui]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/ui
[Role]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/ui#RoleType
[Location]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/coords#Rect
[States List]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/ui#StateType


## Simple Starter Test

A good starting point for most chrome.Automation tests is to use the
chrome.LoggedIn() precondition and then force the test to fail and give you a
dump of the Accessibility tree. That way you can look at the tree and decide what
node you want to interact with. Here is some sample code:
```go
func init() {
	testing.AddTest(&testing.Test{
		Func: ChangeWallpaper,
		Desc: "Follows the user flow to change the wallpaper",
		Contacts: []string{
			"my-group@chromium.org",
			"my-ldap@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
	})
}

func ChangeWallpaper(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Put test code here.

	s.Fatal("I would like a UI dump")
}
```

# Interacting with the Accessibility Tree

After running the test on a device, you should be able to find the UI dump at:
`{$CHROMEOS_SRC}/chroot/tmp/tast/results/latest/tests/{$TEST_NAME}/faillog/ui_tree.txt`

The tree can be a little complex and unintuitive at times, but it should have
nodes for anything we are looking for.

> Note: You can inspect the standard UI by enabling
chrome://flags/#enable-ui-devtools on your device, going to
chrome://inspect/#other, and clicking inspect under UiDevToolsClient. More
details available [here].

> Note: You can interact directly with chrome.Automation on your device by:
Opening chrome, clicking Test Api Extension(T in top right) > Manage extensions,
Enabling Developer mode toggle, Clicking background page > Console. It has a
[Codelab].

[here]: https://sites.google.com/a/chromium.org/dev/developers/how-tos/inspecting-ash
[Codelab]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/third_party/autotest/files/docs/chrome-automation-codelab.md?q=chrome-automation-codelab.md



In this case, we want to start by right clicking on the wallpaper. Looking at
the tree, it looks like we will want to right click
`node id=37 role=unknown state={} parentID=36 childIds=[] className=WallpaperView`.
It looks like its class name is a unique identifier we
can use to [find it], so let's find and right click that class:
```go
params := ui.FindParams{ClassName: "WallpaperView"}
wallpaperView, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
if err != nil {
	s.Fatal("Failed to find the wallpaper view: ", err)
}
defer wallpaperView.Release(ctx)

if err := wallpaperView.RightClick(ctx); err != nil {
	s.Fatal("Failed to right click the wallpaper view: ", err)
}
```

> **Warning:** Always remember to defer the release of UI nodes.

> Note: Generally it is preferred to use FindWithTimeout over Find because the
Accessibility Tree is asynchronous and may not immediately contain a node. If
you are certain that a node exists, you can use Find.

[find it]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/ui#FindParams

## Dealing With a Race Condition

Now if we look at `ui_tree.txt`, we can see the see the right click menu:
```
node id=118 role=menuListPopup state={"vertical":true} parentID=117 childIds=[119,121,124] className=SubmenuView
  node id=119 role=menuItem state={} parentID=118 childIds=[] name=Autohide shelf className=MenuItemView
  node id=121 role=menuItem state={} parentID=118 childIds=[] name=Shelf position className=MenuItemView
  node id=124 role=menuItem state={} parentID=118 childIds=[] name=Set wallpaper className=MenuItemView
```

> Note: If you don't see an update to `ui_tree.txt`, you may need to add
`testing.Sleep(time.Second)` before causing the test to fail. Events are
asynchronous and might not immediately update the UI tree.

Next, we want to click on the "Set wallpaper" menu item:
```go
params = ui.FindParams{Role: ui.RoleTypeMenuItem, Name: "Set wallpaper"}
setWallpaper, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
if err != nil {
	s.Fatal("Failed to find the set wallpaper menu item: ", err)
}
defer setWallpaper.Release(ctx)

if err := setWallpaper.LeftClick(ctx); err != nil {
	s.Fatal("Failed to click set wallpaper: ", err)
}
```

When you run the test, depending on the speed of your device and your luck, the
"Set wallpaper" menu item may or may not have been clicked. We have just hit a
race condition where the menu may not be fully rendered and ready to click by
the time that we try to click it. To fix this, we will simply keep clicking the
menu item until it no longer exists:
```go
if err := testing.Poll(ctx, func(ctx context.Context) error {
	if exists, err := ui.Exists(ctx, tconn, params); err != nil {
		return testing.PollBreak(err)
	} else if exists {
		if err := setWallpaper.LeftClick(ctx); err != nil {
			return errors.Wrap(err, "failed to click set wallpaper")
		}
		return errors.New("click may not have been received yet")
	}
	return nil
}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
	s.Fatal("Failed to open wallpaper picker: ", err)
}
```

> Note: Most nodes will not have race conditions and do not require this extra
work. UI nodes that are animating(like this menu) are an exception.

## More Basic Interactions

Now that the wallpaper picker is open, let's set the background to a solid color.
This is basically the same as above. We look for the node corresponding to the 'Solid colors' tab in `ui_tree.txt` and
then add code to click it:
```
node id=301 role=listItem state={} parentID=245 childIds=[341]
  node id=341 role=genericContainer state={} parentID=301 childIds=[342]
    node id=342 role=staticText state={} parentID=341 childIds=[343] name=Solid colors
      node id=343 role=inlineTextBox state={} parentID=342 childIds=[] name=Solid colors
```
```go
params = ui.FindParams{Role: ui.RoleTypeStaticText, Name: "Solid colors"}
solidColors, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
if err != nil {
	s.Fatal("Failed to find the solid colors button: ", err)
}
defer solidColors.Release(ctx)

if err := solidColors.LeftClick(ctx); err != nil {
	s.Fatal("Failed to click the solid colors button: ", err)
}
```

Personally, I am a fan of the 'Deep Purple' background, so that is what I am going
to pick:
```
node id=355 role=listItem state={"focusable":true} parentID=264 childIds=[] name=Deep Purple
```
```go
params = ui.FindParams{Role: ui.RoleTypeListItem, Name: "Deep Purple"}
deepPurple, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
if err != nil {
	s.Fatal("Failed to find the deep purple button: ", err)
}
defer deepPurple.Release(ctx)

if err := deepPurple.LeftClick(ctx); err != nil {
	s.Fatal("Failed to click the deep purple button: ", err)
}
```

## Ensuring the Background Changed

Checking that a test succeeded can often be harder than expected. In this case,
we have to decide what demonstrates a successful wallpaper change. A good solution
would probably be to check a pixel in the background and make sure it is the
same color as deep purple. Sadly, that is not currently easy to do in Tast. A
simpler solution for now is to check for the text 'Deep Purple' because the
wallpaper picker displays the name of the currently selected wallpaper:
```
node id=412 role=staticText state={} parentID=206 childIds=[413] name=Deep Purple
```
```go
params = ui.FindParams{Role: ui.RoleTypeStaticText, Name: "Deep Purple"}
deepPurpleText, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
if err != nil {
	s.Fatal("Failed to set wallpaper, wallpaper name not changed: ", err)
}
defer deepPurpleText.Release(ctx)
```

## Full Code

```go
// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ui"
	"chromiumos/tast/local/chrome/ui/faillog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChangeWallpaper,
		Desc: "Follows the user flow to change the wallpaper",
		Contacts: []string{
			"bhansknecht@chromium.org",
			"kyleshima@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
	})
}

func ChangeWallpaper(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s, tconn)

	// Right click the wallpaper.
	params := ui.FindParams{ClassName: "WallpaperView"}
	wallpaperView, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to find the wallpaper view: ", err)
	}
	defer wallpaperView.Release(ctx)

	if err := wallpaperView.RightClick(ctx); err != nil {
		s.Fatal("Failed to right click the wallpaper view: ", err)
	}

	// Open wallpaper picker by clicking set wallpaper.
	params = ui.FindParams{Role: ui.RoleTypeMenuItem, Name: "Set wallpaper"}
	setWallpaper, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to find the set wallpaper menu item: ", err)
	}
	defer setWallpaper.Release(ctx)

	// This button takes a bit before it is clickable.
	// Keep clicking it until the click is received and the menu closes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if exists, err := ui.Exists(ctx, tconn, params); err != nil {
			return testing.PollBreak(err)
		} else if exists {
			if err := setWallpaper.LeftClick(ctx); err != nil {
				return errors.Wrap(err, "failed to click set wallpaper")
			}
			return errors.New("click may not have been received yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to open wallpaper picker: ", err)
	}

	params = ui.FindParams{Role: ui.RoleTypeStaticText, Name: "Solid colors"}
	solidColors, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to find the solid colors button: ", err)
	}
	defer solidColors.Release(ctx)

	if err := solidColors.LeftClick(ctx); err != nil {
		s.Fatal("Failed to click the solid colors button: ", err)
	}

	params = ui.FindParams{Role: ui.RoleTypeListItem, Name: "Deep Purple"}
	deepPurple, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to find the deep purple button: ", err)
	}
	defer deepPurple.Release(ctx)

	if err := deepPurple.LeftClick(ctx); err != nil {
		s.Fatal("Failed to click the deep purple button: ", err)
	}

	// Ensure that "Deep Purple" text is displayed.
	// The UI displays the name of the currently set wallpaper.
	params = ui.FindParams{Role: ui.RoleTypeStaticText, Name: "Deep Purple"}
	deepPurpleText, err := ui.FindWithTimeout(ctx, tconn, params, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to set wallpaper, wallpaper name not changed: ", err)
	}
	defer deepPurpleText.Release(ctx)
}

```
