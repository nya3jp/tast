# Tast Codelab: Chrome UI Automation (go/tast-codelab-3)

> This document assumes that you've already gone through [Codelab #1].

This codelab follows the creation of a Tast test that uses the the
chrome.Automation library to change the wallpaper. It goes over the background
of chrome.Automation, how to use it, and some common issues.

[Codelab #1]: codelab_1.md


## Background

The [chrome.automation] library uses the Chrome Accessibility Tree to view and
control the current state of the UI. The Accessibility Tree has access to:
* The Chrome Browser
* The ChromeOS Desktop UI
* Chrome OS packaged apps
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

In Tast, [chrome.automation] is wrapped in [chrome/uiauto] and can be imported like so:
```go
import "chromiumos/tast/local/chrome/uiauto"
```
[Accessibility Node]: https://developer.chrome.com/docs/extensions/reference/automation/#type-AutomationNode
[chrome.automation]: https://developer.chrome.com/docs/extensions/reference/automation/
[chrome/uiauto]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto
[Role]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto/role#Role
[Location]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/coords#Rect
[States List]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto/state#State


## Simple Starter Test

A good starting point for most chrome.Automation tests is to use the
"chromeLoggedIn" fixture and then force the test to fail and give you a
dump of the Accessibility tree. That way you can look at the tree and decide what
node you want to interact with. Here is some sample code:
```go
func init() {
	testing.AddTest(&testing.Test{
		Func: Change,
		Desc: "Follows the user flow to change the wallpaper",
		Contacts: []string{
			"my-group@chromium.org",
			"my-ldap@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func Change(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
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
`${CHROMEOS_SRC}/chroot/tmp/tast/results/latest/tests/${TEST_NAME}/faillog/ui_tree.txt`

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
[Codelab]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/autotest/files/docs/chrome-automation-codelab.md?q=chrome-automation-codelab.md



In this case, we want to start by right clicking on the wallpaper. Looking at
the tree, it looks like we will want to right click
`node id=37 role=unknown state={} parentID=36 childIds=[] className=WallpaperView`.
It looks like its class name is a unique identifier we
can use to find it, so let's find and right click that node:
```go
ui := uiauto.New(tconn)
if err := ui.RightClick(nodewith.ClassName("WallpaperView"))(ctx); err != nil {
  s.Fatal("Failed to right click the wallpaper view: ", err)
}
```
Now those few lines are pretty simple, but introduce a lot of library specific information.
Lets break that down some.

Firstly, there is the [nodewith] package that is used to describe a way to find a node.
With it, you can specify things like the [Name("")], [Role(role.Button)], or [Focused()].
A chain of nodes can be defined by using [Ancestor(ancestorNode)].

[nodewith]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto/nodewith
[Name("")]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto/nodewith#Name
[Role(role.Button)]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto/nodewith#Role
[Focused()]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto/nodewith#Focused
[Ancestor(ancestorNode)]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto/nodewith#Ancestor

The a11y tree can sometimes be hard to interact with directly.
From nodes moving around to parts of the tree temporarily disappearing,
this instability can often lead to flakes in tests.
[uiauto.Context] is focused on creating a flake resistant way to interact with a11y tree.
By default, it uses polling to wait for stability before performing actions.
These actions include things like [LeftClick], [WaitUntilExists], and [FocusAndWait].
If for some reason the default polling options do not work for your test case,
you can modify them with [WithTimeout], [WithInterval], and [WithPollOpts].
For example, if we needed a longer timeout to ensure the location was stable before
right clicking, we could write:
```go
ui.WithTimeout(time.Minute).RightClick(nodewith.ClassName("WallpaperView"))
```

[uiauto.Context]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Context
[LeftClick]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Context.LeftClick
[WaitUntilExists]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Context.WaitUntilExists
[FocusAndWait]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Context.FocusAndWait
[WithTimeout]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Context.WithTimeout
[WithInterval]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Context.WithInterval
[WithPollOpts]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Context.WithPollOpts

Finally, you may have noticed the slightly strange syntax `(ctx)` after
`ui.RightClick(nodewith.ClassName("WallpaperView"))`.
This is because `ui.RightClick` returns a `uiauto.Action`.
A [uiauto.Action] is just a `func(context.Context) error`.
It is used to enable easy chaining of multiple actions.
For example, if you wanted to right click a node, left click a different node,
and then wait for a third node to exist, you could write:
```go
if err := ui.RightClick(node1)(ctx); err != nil {
  s.Fatal("Failed to right click node1: ", err)
}
if err := ui.LeftClick(node2)(ctx); err != nil {
  s.Fatal("Failed to left click node2: ", err)
}
if err := ui.WaitUntilExists(node3)(ctx); err != nil {
  s.Fatal("Failed to wait for node3: ", err)
}
```
Or, you could use [uiauto.Combine] to deal with these actions as a group:
```go
if err := uiauto.Combine("do some bigger action",
  ui.RightClick(node1),
  ui.LeftClick(node2),
  ui.WaitUntilExists(node3),
)(ctx); err != nil {
  s.Fatal("Failed to do some bigger action: ", err)
}
```

> Note: I generally advise using [uiauto.Combine] if you are doing more
than one action in a row.

[uiauto.Action]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Action
[uiauto.Combine]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Combine

## Dealing With a Race Condition

Now if we look at `ui_tree.txt`, we can see the right click menu:
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
if err := ui.LeftClick(nodewith.Name("Set wallpaper").Role(role.MenuItem))(ctx); err != nil {
  s.Fatal(...)
}
```

When you run the test, depending on the speed of your device and your luck, the
"Set wallpaper" menu item may or may not have been clicked. We have just hit a
race condition where the menu may not be fully ready to be clicked by
the time that we try to click it. To fix this, we will simply keep clicking the
menu item until it no longer exists:
```go
setWallpaperMenu := nodewith.Name("Set wallpaper").Role(role.MenuItem)
if err := ui.LeftClickUntil(setWallpaperMenu, ui.Gone(setWallpaperMenu))(ctx); err != nil {
  s.Fatal(...)
}
```

> Note: Most nodes will not have race conditions and do not require this extra
work. The issue is that we do not have a indicator for when the menu
button is ready to be clicked.

## More Basic Interactions

Now that the wallpaper picker is open, let's set the background to a solid color.
We left click for the node corresponding to the 'Solid colors' tab in `ui_tree.txt`:
```
node id=245 role=genericContainer state={} parentID=243 childIds=[250,251]
  node id=250 role=paragraph state={} parentID=245 childIds=[252] name=Solid colors
    node id=252 role=staticText state={} parentID=250 childIds=[362] name=Solid colors
      node id=362 role=inlineTextBox state={} parentID=252 childIds=[] name=Solid colors
```
```go
if err := ui.LeftClick(nodewith.Name("Solid colors").Role(role.StaticText))(ctx); err != nil {
  s.Fatal(...)
}
```

Personally, I am a fan of the 'Deep Purple' background, so that is what I am going
to pick:
```
node id=410 role=listBoxOption state={"focusable":true} parentID=409 childIds=[477] name=Deep Purple
```
```go
if err := ui.LeftClick(nodewith.Name("Deep Purple").Role(role.ListBoxOption))(ctx); err != nil {
  s.Fatal(...)
}
```

## Scrolling to Target

We found the above code fails to find the "Deep Purple" node on some device
models. We examined and found that the "Solid color" list item was not visible
without scrolling. This could be verified either by seeing the DUT screen or
by seeing the node having "offscreen" state true:
```
node id=252 role=staticText state={"offscreen":true} parentID=250 childIds=[362] name=Solid colors
  node id=362 role=inlineTextBox state={"offscreen":true} parentID=252 childIds=[] name=Solid colors
```

This happened due to different screen sizes of devices, which affects the
window size. In order to make this test more robust, we need to make the item
visible before clicking:

```go
if err := ui.MakeVisible(nodewith.Name("Solid colors").Role(role.StaticText))(ctx); err != nil {
  s.Fatal(...)
}
// same as the previsous section
if err := ui.LeftClick(nodewith.Name("Solid colors").Role(role.StaticText))(ctx); err != nil {
  s.Fatal(...)
}
```

However, there is still a race with this. The list items are loaded
asynchronously. (You may be able to see only the first item is shown in the
list and then the others are loaded few seconds later.)
So the item may not exist in the accessibility tree yet, right after previous
step. Therefore we will wait until the item appears:

```go
solidColorsMenu := nodewith.Name("Solid colors").Role(role.StaticText)
if err := ui.WaitUntilExists(solidColorsMenu)(ctx); err != nil {
  s.Fatal(...)
}
if err := ui.MakeVisible(solidColorsMenu)(ctx); err != nil {
  s.Fatal(...)
}
if err := ui.LeftClick(solidColorsMenu)(ctx); err != nil {
  s.Fatal(...)
}
```

Note that `ui.LeftClick` has integrated logic to wait until the target is
stable (i.e. exists and its position kept unchanged) but `MakeVisible`
doesn't.

## Ensuring the Background Changed

Checking that a test succeeded can often be harder than expected. In this case,
we have to decide what demonstrates a successful wallpaper change. A good solution
would probably be to check a pixel in the background and make sure it is the
same color as deep purple. Sadly, that is not currently easy to do in Tast. A
simpler solution for now is to check for the text 'Deep Purple' in the heading
because the wallpaper picker displays the name of the currently selected wallpaper:
```
node id=109 role=heading state={} parentID=34 childIds=[] name=Currently set Deep Purple
```
```go
if err := ui.WaitUntilExists(nodewith.NameContaining("Deep Purple").Role(role.Heading))(ctx); err != nil {
  s.Fatal(...)
}
```

## Full Code

> Note: The code below is using [uiauto.Combine] to simplify all of the steps above into
one chain of operations.

[uiauto.Combine]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Combine

```go
// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Change,
		Desc: "Follows the user flow to change the wallpaper",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func Change(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)
	setWallpaperMenu := nodewith.Name("Set wallpaper").Role(role.MenuItem)
	solidColorsMenu := nodewith.Name("Solid colors").Role(role.StaticText)
	if err := uiauto.Combine("change the wallpaper",
		ui.RightClick(nodewith.ClassName("WallpaperView")),
		// This button takes a bit before it is clickable.
		// Keep clicking it until the click is received and the menu closes.
		ui.WithInterval(500*time.Millisecond).LeftClickUntil(setWallpaperMenu, ui.Gone(setWallpaperMenu)),
		ui.WaitUntilExists(solidColorsMenu),
		ui.MakeVisible(solidColorsMenu),
		ui.LeftClick(solidColorsMenu),
		ui.LeftClick(nodewith.Name("Deep Purple").Role(role.ListBoxOption)),
		// Ensure that "Deep Purple" text is displayed.
		// The UI displays the name of the currently set wallpaper.
		ui.WaitUntilExists(nodewith.NameContaining("Deep Purple").Role(role.Heading)),
	)(ctx); err != nil {
		s.Fatal("Failed to change the wallpaper: ", err)
	}
}
```