# Tast Codelab: Image-based UI Detection (go/tast-uidetection)

> This document assumes that you are familiar with writng Tast tests
> (go/tast-writing), and have already gone through [Codelab #1], [Codelab #2]
> and [Codelab #3]

This codelab follows the creation of a Tast test that uses the image-based UI
detection library. It goes over the general setups, how to use it, and some
common issues.

[Codelab #1]: codelab_1.md
[Codelab #2]: codelab_2.md
[Codelab #3]: codelab_3.md

[TOC]

## Background

The [uiauto] library uses the Chrome accessibility tree to view and control the
current state of the UI. The accessibility tree has access to:

*   The Chrome Browser
*   The ChromeOS Desktop UI
*   Chrome OS packaged apps
*   Web Apps/PWAs

That being said, it does not have access to UI elements in containers or VMs
(like VDI and Crostini). To close the automation gap for these apps, we
introduce another UI automation library [uidetection] that does not reply on the
accessibility tree. [uidetection] makes use of computer vision techniques and is
able to detect UI elements from the screen directly.

Note that [uidetection] also works on UI where the accessibility tree is
available, but it is **preferred** to use [uiauto] in this case due to the
efficiency and stability of using the accessibility tree.

Currently, we support the detections of three types of UI elements:

*   Custom icon detection. It allows the developer to provide a png icon image
    to find a match in the screen.
*   Word detection. It allows the detection of a specific word in the screen.
*   Text block detection. It allows the detection of a text block (i.e., a
    sentence or lines of sentences) that contains specific words.

In Tast, [uidetection] can be imported like so:

```go
import "chromiumos/tast/local/uidetection"
```

[uiauto]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto
[uidetection]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/uidetection

## Simple Starter Test

Here is some sample code using the UI detection library:

```go
func init() {
    testing.AddTest(&testing.Test{
        Func:         ExampleDetection,
        Desc:         "Example of using image-based UI detection API",
        Contacts:     []string{
            "my-group@chromium.org",
            "my-ldap@chromium.org",},
        Attr:         []string{"group:mainline", "informational"},
        SoftwareDeps: []string{"chrome"},
        Timeout:      3 * time.Minute,
        Data:         []string{"logo_chrome.png"}, // Icon file for detection.
        Fixture:      "chromeLoggedIn",
    })
}

func ExampleDetection(ctx context.Context, s *testing.State) {
    // Cleanup context setup.
    ...

    cr := s.FixtValue().(*chrome.Chrome)
    tconn, err := cr.TestAPIConn(ctx)
    if err != nil {
        s.Fatal("Failed to create Test API connection: ", err)
    }

    ud := uidetection.NewDefault(tconn)

    // Put UI interaction using ud here.
}
```

## UI interaction in the screen

In this sample test, we will perform the following operations that covers the
basic three types of detections from a newly logged-in device: 1. Click the
Chrome icon to open a Chrome browser (custom icon detection). 2. Click the
button that contains "Customize Chrome" (text block detection). 3. Click the
"Cancel" button (word detection).

### Click the Chrome icon

We first define a finder for the icon element as:

```go
icon := uidetection.CustomIcon(s.DataPath("logo_chrome.png"))
```

"logo_chrome.png" is the icon file declared in the test registration, see
[Data files in Tast] for details on using the data files in tast.

Then we can left-click the icon by:

```go
if err:= ud.LeftClick(icon) {
  s.Fatal("Failed to click Chrome icon", err)
}
```

### Click the "Customize Chrome" textblock

The button that contains multiple words is represented by the textblock finder:

```go
textblock := uidetection.TextBlock([]string{"Customize", "Chrome"})
```

The left-click operation is done by:

```go
if err:= ud.LeftClick(textblock) {
  s.Fatal("Failed to click Customize Chrome button", err)
}
```

Since the uidetection library takes stable screenshot by default, it waits for
the screen to be consistent between two intervals (300 ms). When the screen is
not expected to be static, i.e., in this scenario, the text cursor in the
browser address bar is blinking all the time, using stable screenshot strategy
can result in a test failure. The solution is to explicitly ask the API to take
the immediate screenshot using
`WithScreenshotStrategy(uidetection.ImmediateScreenshot)`:

```go
if err:= ud.WithScreenshotStrategy(uidetection.ImmediateScreenshot).LeftClick(textblock) {
  s.Fatal("Failed to click Customize Chrome button", err)
}
```

### Click the "Cancel" word

Similarly to the textblock detection, this operation can be defined as:

```go
word := uidetection.Word("Cancel")
if err:= ud.LeftClick(word) {
  s.Fatal("Failed to click Cancel button", err)
}
```

### Ensuring the customize panel is closed

Finally, after clicking the cancel button, we may also need to check whether the
test succeeded. In this case, we have to decide what demonstrates that the
successful close of the "customize chrome" layout. A simple solution is to check
if the cancel button disappeared from the screen:

```go
if err:= ud.WaitUntilGone(uidetection.Word("Cancel")) {
  s.Fatal("The cancel button still exists", err)
}
```

### Combine these actions

Ideally, you would use [uiauto.Combine] to deal with these actions as a group:

```go
if err := uiauto.Combine("verify detections",
        ud.LeftClick(uidetection.CustomIcon(s.DataPath("logo_chrome.png"))),
        ud.WithScreenshotStrategy(uidetection.ImmediateScreenshot).LeftClick(uidetection.TextBlock([]string{"Customize", "Chrome"})),
        ud.LeftClick(uidetection.Word("Cancel")),
        ud.WaitUntilGone(uidetection.Word("Cancel")),
    )(ctx); err != nil {
        s.Fatal("Failed to perform image-based UI interactions: ", err)
    }
```

[Data files in Tast]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Data-files
[uiauto.Combine]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/uiauto#Combine

## Full Code

```go
// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uidetection

import (
    "context"
    "time"

    "chromiumos/tast/ctxutil"
    "chromiumos/tast/local/chrome"
    "chromiumos/tast/local/chrome/uiauto"
    "chromiumos/tast/local/chrome/uiauto/faillog"
    "chromiumos/tast/local/uidetection"
    "chromiumos/tast/testing"
)

func init() {
    testing.AddTest(&testing.Test{
        Func: ExampleDetection,
        Desc: "Example of using image-based UI detection API",
        Contacts: []string{
            "my-group@chromium.org",
            "my-ldap@chromium.org"},
        Attr:         []string{"group:mainline", "informational"},
        SoftwareDeps: []string{"chrome"},
        Timeout:      3 * time.Minute,
        Data:         []string{"logo_chrome.png"}, // Icon file for detection.
        Fixture:      "chromeLoggedIn",
    })
}

func ExampleDetection(ctx context.Context, s *testing.State) {
    // Shorten deadline to leave time for cleanup.
    cleanupCtx := ctx
    ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
    defer cancel()

    cr := s.FixtValue().(*chrome.Chrome)
    tconn, err := cr.TestAPIConn(ctx)
    if err != nil {
        s.Fatal("Failed to create Test API connection: ", err)
    }
    defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "uidetection")

    ud := uidetection.NewDefault(tconn)
    if err := uiauto.Combine("verify detections",
        // Click Chrome logo icon to open a Chrome browser.
        ud.LeftClick(uidetection.CustomIcon(s.DataPath("logo_chrome.png"))),
        // Click the button that contains "Customize Chrome" textblock.
        ud.WithScreenshotStrategy(uidetection.ImmediateScreenshot).LeftClick(uidetection.TextBlock([]string{"Customize", "Chrome"})),
        // Click the "cancel" button.
        ud.LeftClick(uidetection.Word("Cancel")),
        // Verify the "cancel" button is gone.
        ud.WaitUntilGone(uidetection.Word("Cancel")),
    )(ctx); err != nil {
        s.Fatal("Failed to perform image-based UI interactions: ", err)
    }
}
```

## Advanced UI Detections

### Region of interest (ROI) detections

Using ROI detection may be desired to: 1. Improve the detection efficiency.
Smaller image results in faster detection. 2. Improve the detection accuracy.
The API may fail to detection small UI elements in the whole screenshot, and
using ROI instead of the whole screen can help to increase the detection
accuracy.

Currently, the UI detection library support ROI
[defined](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/chromiumos/tast/local/uidetection/finder.go)
by:

1.  Image-based UI elements.

    *   `Within(*uidetection.Finder)` finds a UI element within another UI
        element.
    *   `Above(*uidetection.Finder)` finds a UI element that is above another UI
        element.
    *   `Below(*uidetection.Finder)` finds a UI element that is below another UI
        element.
    *   `LeftOf(*uidetection.Finder)` finds a UI element that is in the left of
        another UI element.
    *   `RightOf(*uidetection.Finder)` finds a UI element that is in the right
        of another UI element.

    **Example**: find the word `next` that is above the textblock `some
    textblock`:

    ```go
    word := uidetection.Word("next").Above(uidetection.Textblock([]string{"some", "textblock"})))
    ```

2.  Assessbility-tree-based UI elements (reprensented by [nodewith.Finder]).

    *   `WithinA11yNode(*nodewith.Finder)` finds a UI element that is within a
        UI node in the accessibility tree.
    *   `AboveA11yNode(*nodewith.Finder)` finds a UI element that is above a UI
        node in the accessibility tree.
    *   `BelowA11yNode(*nodewith.Finder)` finds a UI element that is below a UI
        node in the accessibility tree.
    *   `LeftOfA11yNode(*nodewith.Finder)` finds a UI element that is in the
        left of a UI node in the accessibility tree.
    *   `RightOfA11yNode(*nodewith.Finder)` finds a UI element that is in the
        right of a UI node in the accessibility tree.

    **Example**: find an icon `icon.png` in the VS Code app:

    ```go
    vs_app_windown := nodewith.NameStartingWith("Get Started - Visual Studio Code").Role(role.Window).First() // Finder of the icon in the VS Code app.
    icon := uidetection.CustomIcon(s.DataPath("icon.png")).WithinA11yNode(vs_app_windown)
    ```

3.  ROIs in pixel (px), **USE WITH CAUTION**.

    *   `WithinPx(coords.Rect)` finds a UI element in the bounding box specified
        in pixels.
    *   `AbovePx(int)` finds a UI element above a pixel.
    *   `BelowPx(int)` finds a UI element below a pixel.
    *   `LeftOfPx(int)` finds a UI element in the left of a pixel.
    *   `RightOfPx(int)` finds a UI element in the right of a pixel.

4.  ROIs in density-independent pixels (dp) **USE WITH CAUTION**.
    `WithinDp(coords.Rect)`, `AboveDp(int)`, `BelowDp(int)`, `LeftOfDp(int)`,
    `RighttOfDp(int)` are defined analogously as the ROIs in pixels, except that
    they are in [density-independent pixels].

Note: usage of ROIs in pixels or in density-independent pixels is generally
discouraged, as they vary from devices to devices. Please first consider using
other two types of ROIs.

[nodewith.Finder]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/chromiumos/tast/local/chrome/uiauto/nodewith/nodewith.go?q=nodewith.go#:~:text=type%20Finder%20struct%20%7B
[density-independent pixels]: https://en.wikipedia.org/wiki/Device-independent_pixel

## FAQs

### How can I test if the library can find a UI element in a screenshot before coding? (Googlers only)

Try the playground at [go/acuiti-playground]. Upload a screenshot you want to
test and choose the detection type to text or custom icon. If you are able to
find the UI element there, there should be no problem that the [uidetection] can
find it too.

### I am getting errors in taking stable screenshot.

If you encounter error saying `screen has not stopped changing after XXXs,
perhaps increase timeout or use immediate-screenshot strategy`, this happens
because the screen is not static. You can check the two consecutive screenshots
`uidetection_screenshot.png` and `old_uidetection_screenshot.png` to see in
which location the screen keeps changing. If this is expected, try using the
immediate screenshot strategy
`WithScreenshotStrategy(uidetection.ImmediateScreenshot)`

**Example**: Left-click the "Customize Chrome" textblock using the immediate
screenshot strategy

```go
ud := uidetection.NewDefault(tconn)
ud.WithScreenshotStrategy(uidetection.ImmediateScreenshot).LeftClick(uidetection.TextBlock([]string{"Customize", "Chrome"})),
```

## Report Bugs

If you have any issues in using this [uidetection] library, please file a bug in
Buganizer to the component 1034649 and assign it to the hotlist 3788221.

[go/acuiti-playground]: http://go/acuiti-playground
