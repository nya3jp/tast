# Tast: Testing Isolated Web Applications (IWAs) on ChromeOS (go/tast-iwa)

> This document assumes that you are familiar with Tast [test writing], [execution], and [debugging], and have already gone through [Codelab #1] and [Codelab #2]. You should also know what [Isolated Web Application] is.

[Codelab #1]: codelab_1.md
[Codelab #2]: codelab_2.md
[test writing]: http://go/tast-writing
[execution]: http://go/tast-running
[debugging]: http://go/debug-tast-tests
[Isolated Web Application]: https://chromeos.dev/en/tutorials/getting-started-with-isolated-web-apps

[TOC]

# Testing Isolated Web Apps in Tast
This document provides guidance on how to test Isolated Web Apps (IWAs) within the Tast testing framework.

## Testing Pyramid and Lower-Level Testing
*   Adopt a **testing pyramid** approach, which emphasizes a balance of different test types. Read more about it at [Google Testing Blog](https://testing.googleblog.com/2024/10/smurf-beyond-test-pyramid.html).
*   There should be a significant number of tests checking the application's code, focusing on individual components and their interactions, including components' integrations. These tests should be prioritized to ensure a robust and reliable application.
*   Testing the integration of the IWA with the Chrome browser is currently under development (crbug.com/337872319).

## Setting up the Test Environment

1. **Test Device:**
    *   **Recommended Approach: VM Testing**
        *   It is highly recommended to start testing with a ChromeOS VM. This allows for faster iteration, easier debugging, and a more controlled environment.
        *   **VM Setup:**
            1.  **Linux Chromium Checkout:** Ensure you have a [Linux Chromium checkout](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/linux/build_instructions.md) with `depot_tools` installed.
            2.  **Virtualization Enabled:** Your system firmware (BIOS) must have virtualization features enabled, and KVM must be enabled in your kernel.
            3.  **Simple Chrome Setup:** You should have [Simple Chrome](https://www.chromium.org/chromium-os/developer-library/guides/development/simple-chrome-workflow/) set up.
            4.  **Choose a Board:** Select a suitable board. `betty` is recommended for Googlers, while `amd64-generic-vm` is suitable for open-source contributors. Set the board using `export BOARD=betty`
            5.  **Launch the VM:** Use `cros vm start` or `cros_vm --board ${BOARD}` to launch the VM.\
            Refer to the [official ChromeOS VM documentation](https://www.chromium.org/chromium-os/developer-library/guides/containers/cros-vm/) for detailed instructions.
        *   **Why VM First?** VMs offer a faster, more controlled environment for initial testing and development. They allow for quick modifications and easier debugging before moving to physical hardware.

    *   **Physical Device Testing:**
        *   While VMs are ideal for initial testing, it's crucial to test on physical devices.
        *   **Recommended Devices:**
            *   **Primary Device:** Target the most used device within your customer base. This ensures the IWA functions well on the most common hardware.
            *   **Low-End Device:** Include a low-end device in your testing to ensure the IWA remains performant even under resource constraints.
        *   **Why Physical Devices?** Physical devices provide a real-world view of performance, hardware interactions, and user experience.

1. **Update manifest:** The application should be deployed with its update manifest accessible.

## Steps to Writing Tast Tests for IWAs

Here's a general outline for writing Tast tests for IWAs:

### Define application details

```go
const (
	kitchenSinkIWAUpdateManifestURL = "https://github.com/chromeos/iwa-sink/releases/latest/download/update.json"
	kitchenSinkIWAWebBundleID       = "aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
)
```

### Prepare the policies

```go
pb := policy.NewBlob()
policies := []policy.Policy{
	&policy.IsolatedWebAppInstallForceList{
		Val: []*policy.IsolatedWebAppInstallForceListValue{
			{
				UpdateManifestUrl: kitchenSinkIWAUpdateManifestURL,
				WebBundleId:       kitchenSinkIWAWebBundleID,
				PinnedVersion:     "0.17.0",
			},
		},
	},
}
```
**NOTE:**
It is recommended to use `PinnedVersion` and the latest ChromeOS to ensure that ChromeOS changes do not impact the IWA's functionality. When testing an unpinned IWA version, use a stable, unchanging ChromeOS version. This will help isolate whether failures are due to changes in the application or in ChromeOS.

### Add policies and load them

```go
if err := pb.AddPolicies(policies); err != nil {
	s.Fatal("Failed to add policies for public account setup: ", err)
}

if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
	s.Fatal("Failed to update policies: ", err)
}
```

### Create a test connection

```go
tconn, err := cr.TestAPIConn(ctx)
if err != nil {
	s.Fatal("Failed to create test API connection: ", err)
}
```

### Create uiauto object

```go
ui := uiauto.New(tconn)
createSocketConnButton := nodewith.Name("Create new socket connection").Role(role.Button)
sendMessageTextField := nodewith.Name("Send a message").Role(role.TextField)
engageMessage := nodewith.Name("Foo").Role(role.InlineTextBox)
responseMessage := nodewith.Name("Bar").Role(role.InlineTextBox)
```

### Create virtual keyboard object

```go
kb, err := input.VirtualKeyboard(ctx)
if err != nil {
	s.Fatal("Failed to get keyboard: ", err)
}
defer kb.Close(ctx)
```

### Start the application
```go
if err := uiauto.Combine("Launch Kitchen Sink IWA",
		// Launch Kitchen Sink IWA.
		launcher.SearchAndLaunch(tconn, kb, "Kitchen Sink IWA"),
		// Wait till the IWA is launched and the Create Socket button appears.
	ui.WithTimeout(30*time.Second).WaitUntilExists(createSocketConnButton),
	)(ctx); err != nil {
		s.Fatal("Failed to launch Kitchen Sink IWA: ", err)
	}
```

### Interact with the application
```go
if err := uiauto.Combine("Interact with Kitchen Sink IWA UI",
		// Create a new socket connection.
		ui.LeftClick(createSocketConnButton),
		ui.WaitUntilExists(sendMessageTextField.Nth(1)),
		// Send messages to the TCP Server.
		ui.LeftClickUntil(sendMessageTextField.First(), ui.Exists(sendMessageTextField.Focused())),
		kb.TypeAction("Foo\n"),
		ui.WaitUntilExists(engageMessage),
		// Send a message from the TCP Server.
		ui.LeftClickUntil(sendMessageTextField.Nth(1), ui.Exists(sendMessageTextField.Focused())),
		kb.TypeAction("Bar\n"),
		ui.WaitUntilExists(responseMessage),
	)(ctx); err != nil {
		s.Fatal("Failed to interact with the Kitchen Sink IWA: ", err)
	}
```

The full code of the example is available in the [LaunchIWA](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/iwa/launch_iwa.go) test.

## Important Considerations

*   **Test Stability:** Write robust tests that can handle potential network issues, UI changes, and other factors that might impact the IWA's behavior.
*   **Error Handling:** Include proper error handling to identify and address issues quickly.
*   **Version Control:** Test against different versions of the IWA to ensure compatibility.
*   **Clear Test Descriptions:** Provide descriptive test names and comments to make the tests easier to understand.
*   **Follow [design principles]** to make your test more robust.
*   Prioritize **VM testing** for initial development and debugging.
*   Include testing on the most used and a low-end physical device for optimal coverage.

### Commercial setup considerations
*   If a Chrome restart is required (e.g., for auto-starting the IWA), use `fixture.FakeDMS`. Chrome restart needs depend on the type of policies you are trying to apply. For example, the `MultiScreenCaptureAllowedForUrls` policy requires a restart (`Dynamic Policy Refresh: No`).
*   To launch and manually interact with the IWA, use `fixture.ChromePolicyLoggedIn`.

[design principles]: http://go/tast-design
[MultiScreenCaptureAllowedForUrls]: https://chromeenterprise.google/policies/#MultiScreenCaptureAllowedForUrls

### Examples

All IWA tests are held within the [iwa package](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/iwa/). Examples include:

*   [Screen capture test](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/iwa/screen_capture.go).
*   [Starting IWA app from launcher test](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/iwa/launch_iwa.go).
*   [Autostart and prevent closing test](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/iwa/autostart_iwa.go).

## Passing Variables to the Test

This is useful when you need to pass sensitive information, such as server credentials, without hardcoding them into the test itself.

### Using Run Command

During runtime you can pass variables to a Tast test using the `-var` flag with the `tast run` command. This may apply to you if you want to connect to a server and login, and not store those secrets in the test code.
To store and access private data such as credentials:

**Syntax**:

```bash
tast run -var=name=value <dut> <tests>
```

*   `name`: The name of the variable.
*   `value`: The value to assign to the variable.
*   `<dut>`: The target device.
*   `<tests>`: The test(s) to run.

Multiple variables can be passed by repeating the `-var` flag.

**Example**:
```bash
tast run -var=serverUrl=https://validTestEndpoint -var=userName=foo -var=userPassword=bar <dut> <tests>
```

All details are available on the following [runtime variables](https://chromium.googlesource.com/chromiumos/platform/tast/+/b5d9cbe7de67/docs/writing_tests.md#runtime-variables) documentation.


In your Tast test, you can access the variable using the `s.Var` method. Make sure to declare the variable in the `Vars` field of the test's struct.

**Example**:

```go
package mytestpackage


var exampleStrVar = testing.RegisterVarString(
        "mytestpackage.ServerUrl",
        "Default value",
        "An example variable of string type",
)

func init() {
testing.AddTest(&testing.Test{
	Func: MyTest,
	Desc: "Test that will read the variable from the command line argument",
// ...
// ...
})
}

func MyTest(ctx context.Context, s *testing.State) {
strVal := exampleStrVar.Value()
// ...
}
```

A full code example of this is in [this](https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/runtime_vars.go) test.

### Using Secret Variable
This way can be used by an internal developer, who has access to the `tast-tests-private` package. Learn more about it from [this](https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#secret-variables) article.

# Testing IWAs in Kiosk Mode
The main difference between a regular IWA test and a Kiosk IWA test lies in the setup. Instead of using policies to force-install the IWA, you will configure the device to launch directly into the IWA in a Kiosk session.

## Steps to Writing Tast Tests for IWAs in Kiosk Mode

Here’s a general outline for writing Tast tests for IWAs in Kiosk mode:

1.  **Use the `fixture.FakeDMSEnrolled` fixture** in your test definition to simulate a managed device.

    ```go
    func init() {
        testing.AddTest(&testing.Test{
            // ...
            Fixture: fixture.FakeDMSEnrolled,
        })
    }
    ```

2.  **Define the application details**, including the update manifest URL and the web bundle ID for your IWA.

    ```go
    var updateManifestURL string = "https://github.com/chromeos/iwa-sink/releases/latest/download/update.json"
    var webBundleID string = "aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
    ```

3.  **Start Chrome in Kiosk mode** using `kioskmode.New()`. You will need to define a `DeviceLocalAccount` for the IWA and configure it to auto-launch.

    ```go
    iwaKioskAccountType := policy.AccountTypeKioskIWA

    kiosk, cr, err := kioskmode.New(
        ctx,
        fdms,
        s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
        kioskmode.CustomLocalAccounts(
            &policy.DeviceLocalAccounts{
                Val: []policy.DeviceLocalAccountInfo{
                    {
                        AccountID:   &kioskmode.KioskAppAccountID,
                        AccountType: &iwaKioskAccountType,
                        IsolatedWebAppKioskInfo: &policy.IsolatedWebAppKioskInfo{
                            WebBundleId: &webBundleID,
                            ManifestUrl: &updateManifestURL,
                        },
                    },
                },
            },
        ),
        kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
    )
    if err != nil {
        s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
    }
    defer kiosk.Close(cleanupContext)
    ```

4.  **Wait for the Kiosk app to launch** using `kiosk.WaitLaunchLogs()`.

    ```go
    if err := kiosk.WaitLaunchLogs(ctx); err != nil {
        s.Fatal("Failed to launch Kiosk: ", err)
    }
    ```

5.  **Interact with the application** using the `uiauto` library, just as you would in a regular IWA test. You can create a test API connection, define UI nodes, and perform actions like clicking buttons and typing text.

    ```go
    tconn, err := cr.TestAPIConn(ctx)
    if err != nil {
        s.Fatal("Failed to create test API connection: ", err)
    }

    ui := uiauto.New(tconn)
    createSocketConnButton := nodewith.Name("Create new socket connection").Role(role.Button)
    // ...

    if err := uiauto.Combine("Interact with Kitchen Sink IWA UI",
        ui.WithTimeout(30*time.Second).WaitUntilExists(createSocketConnButton),
        // ...
    )(ctx); err != nil {
        s.Fatal("Failed to interact with the Kitchen Sink IWA: ", err)
    }
    ```

## Example

For a complete example of an IWA test running in Kiosk mode, see the [launch_iwa.go](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/kiosk/launch_iwa.go) test.
