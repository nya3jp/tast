# Tast HowTo #4: kernel.LogMount (go/tast-howto)

> This document assumes that you've already gone through [Codelab #1].

This document is intended to give an overview of some of the possibilities you have for the set up and evaluation of your tast-test. This is not an exhaustive list but contains some of the most used functions that are available to create tests.

## Evaluation
### Checking wrapped errors
Go offers the functionality to wrap errors in other errors to allow returning all occurred error messages from a function call. To check if any of these wrapped errors is of a specific type that should be handeled differently [errors.Is()](https://golang.org/pkg/errors/) can be used.
In tast an error that comes up often is context.DeadlineExceeded. Any function running within a context can throw it to indicate that the timeout of the context was reached before the execution of the function had finished.
```
	if err := someFunction(ctx); errors.Is(err, context.DeadlineExceeded) {
		// Do handling of context.DeadlineExceeded here.
	} else if err != nil {
		// Do general Error handling here.
	}
```

### Command line
As ChromeOS is based on linux we can execute linux commands on the command line that can give us the needed information of the state of the system. This is done with the [testexec.CommandContext()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/testexec/testexec.go;l=84?q=testing%20CommandContext) function.
The CommandContext() function wraps the standard go exec package to honor the timeout of the context in which the test is running.
```
out, err := testexec.CommandContext(ctx, "ls", "-l", “/home/chronos/user”).Output()
if err != nil {
	// Do error handling here.
}
```
In the example the command "ls -l /home/chronos/user" is executed on the command line. The output of the execution is written into the out variable by calling the Output() function and should then contain the list of files with details that are in the /home/chronos/user directory. This could be used to check i.e. if a specific file has the correct size.

### Checking windows
In some cases checking if certain windows have been opened, or a certain number of windows have been opened can be enough to check if a test was successful or not. To do that [ash.GetAllWindows()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/chrome/ash/wm.go;l=371?q=ash%20GetAllWindows) can be used.
It requires a context and a chrome.TestConn, which is obtained by a call to [chrome.TestAPIConn()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/chrome/chrome.go;l=953?q=TestAPIConn), and will return an array of all open windows. This array can then be checked for example for the title of the windows to see if a desired window is open.
[ash package documentation](https://chromium.googlesource.com/chromium/src/+/refs/heads/master/ash/README.md)
```
tconn, err := cr.TestAPIConn(ctx)
if err != nil {
	// Do error handling here.
}

windows, err := ash.GetAllWindows(ctx, tconn)
if err != nil {
	// Do error handling here.
}

// Find desired window with expectedTitle
for _, window := range windows {
	if strings.Contains(window.Title, expectedTitle) {
		// The test was successful.
	}
}
```
In the example we first create a TestAPIConn from our chrome instance (cr), then we call GetAllWindows() to get an array of all windows which we check for the desired window.

### Checking files
Similar to the windows the existence or non exsitence of a file might be the needed information to determine if the test was successful.
Go offers the [os.Stat()](https://golang.org/pkg/os/) function for checking the existence or attributes of files.
```
if fileinfo, err := os.Stat(/path/to/file/myfile); os.IsNotExist(err) {
	// The file does not exits.
} else if err != nil {
	// Do error handling here.
}
```
If [os.Stat()](https://golang.org/pkg/os/) doesn't return an error the file exists and additional information about the file is written to fileinfo.

### Javascript evaluation
With [chrome.Eval()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/chrome/conn.go;l=111?q=chrome%20Eval) arbitrary javascript expressions can be evaluated. The function takes a context, a javascript expression and an interface as arguments. If the javascript expression returns a JSObject a reference to it will be written into the interface parameter. If the return value cannot be unmarshaled into the given interface parameter an error will be returned.

```
conn, err := cr.NewConn(ctx, url)
if err != nil {
	// Do error handling here.
}
defer conn.Close()

var message string
if err := conn.Eval(ctx, `document.getElementById("element_id").innerText`, &message); err != nil {
	// Do error handling here.
}
if strings.Contains(message, "this is the element_id")
{
	// The test was successful.
}
```
In this example we open a new chrome window with some url and then we evaluate the javascript expression ```document.getElementById("element_id").innerText``` in this chrome window. The result is written into the string message and is then checked if it contains a desired text.
This can also be used for expressions returning a promise, in which case the function will wait until the promise is settled.
```
var foundIt bool
expectedText := "some text"
if err := conn.Eval(ctx, `return new Promise((resolve, reject) => {
		element = document.getElementById("element_id");
		if element === null {
			return resolve(false);
		}
		if element.innerText !== expectedText {
			return reject(new Errorf('Unexpected inner text: want %s; got %s', expectedText, element.innerText));
		}
		return resolve(true)
	})`, &foundIt); err != nil {
	// Do error handling here.
}
```
The [chrome.Eval()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/chrome/conn.go;l=111?q=chrome%20Eval) can also be used on tast's background page which gives access to other APIs. A connection to the background page can be created with [chrome.TestAPIConn()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/chrome/chrome.go;l=953?q=TestAPIConn).

### Find the Javascript path for an element
In the previous paragraph we took a look at how to evaluate javascript expressions in a tast-test, however finding the javascript expression you need for a certain test can be difficult.
The Developer Tools of Chrome can be very helpful for that. Open them by pressing CTRL + SHIFT +  I (or by opening the menu -> more Tools -> Developer Tools) in a chrome window. In the Elements tab you can browse through the elements of a page and expand them. The selected element will be highlighted. Once you got to the element you want to check right click it in the Elements tab and select Copy -> Copy JS path. This gives you the expression to get the desired element. In the Console Tab you can try beforehand if the javascript expression you want to use delivers the desired output.

### Interacting with the UI
It is also possible to directly interact with the elements of the UI (like clicking on them, or just hovering over the mouse), or just to get information about their status. This can be done through the Test API with the help of the automation library. The basics of the usage of this library can be found in [Codelab #3].

### Waiting in tests
To check some condition it is sometimes necessary to wait until certain changes have been processed in ChromeOS. For such cases the [testing.Poll()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast/src/chromiumos/tast/testing/util.go;l=42?q=testing%20Poll) function should be used instead of sleeping in tests, as it honors the deadline of the context the test is running in. See also [Context and timeouts](https://chromium.googlesource.com/chromiumos/platform/tast/+/refs/heads/master/docs/writing_tests.md#contexts-and-timeouts).
```
// Wait until the condition is true.
if err := testing.Poll(ctx, func(ctx context.Context) error {

	if err := doSomething(); err != nil {

		// In case something went wrong we can stop waiting and return an error with testing.PollBreak().
		return testing.PollBreak(errors.Wrap(err, "failed to do something critical"))
	}

	// Get the current state of our condition.
	if condition, err := checkCondition(); err != nil {
		Do error handling here.
	}

	if condition != expectedCondition {
		return errors.Errorf("unexpected condition: got %q; want %q", condition, expectedCondition)
	}

	return nil

}, &testing.PollOptions{
	Timeout: 30 * time.Second,
	Interval: 5 * time.Second,
})); errors.Is(err, context.DeadlineExceeded)
{
	s.Error("Failed to reach desired state within the timeout: ", err)
} else if err != nil {
	s.Fatal("Failed to check the state: ", err)
}
```
The last argument of [testing.Poll()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast/src/chromiumos/tast/testing/util.go;l=42?q=testing%20Poll) is a [testing.PollOptions](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast/src/chromiumos/tast/testing/util.go;l=15?q=testing%20Poll) struct in which we can define a custom timeout and interval for the polling.
In case something goes wrong during polling from which the test cannot recover there is the option to break the polling early with [testing.PollBreak()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/platform/tast/src/chromiumos/tast/testing/util.go;l=26?q=testing%20PollBreak).

## Setup
### Using the Launcher
We can use the chrome Launcher in our tests to search and start applications or websites. 
```
tconn, err := cr.TestAPIConn(ctx)
if err != nil {
	// Do error handling here.
}

if err := launcher.SearchAndLaunch(ctx, tconn, appName); err != nil {
	// Do error handling here.
}
```
The launcher requires a context (ctx), a TestConn (tconn) and a string that will be typed into the launcher. This example will start the application defined in appName, or if it isn't found a Google search for appName will be opened in a new chrome window.

### HTTP Server
In many tests having a local http server can be helpful to avoid being dependent on a network connection. The [httptest.NewServer()](https://source.chromium.org/chromiumos/chromiumos/codesearch/+/master:src/aosp/toolchain/gcc/gcc-4.9/libgo/go/net/http/httptest/server.go;l=82?q=httptest%20NewServer) function can be used to start your own http server from within the test.
```
func init() {
	testing.AddTest(&testing.Test{
		Func: MyTest,
		Data:         []string{"my_test.html", "my_test.js"},
	})
}

func MyTest(ctx context.Context, s *testing.State) {

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL+"/my_test.html")
	if err != nil {
		// Do error handling here.
	}
	defer conn.Close()
	...
}
```
In this example we created a small website with two files: my_test.html and my_test.js, and added the files to the test in the definition of the metadata for the test.
In the test we start an httpserver as a http.FileServer which serves requests for the files located in the folder given as argument. The used folder, s.DataFileSystem(), is the folder where additional files for the test are copied to on the DuT, which is where our files for the website end up. Then we open the website in a new chrome window.
