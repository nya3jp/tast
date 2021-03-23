# Tast How-To: (go/tast-howto)

> This document assumes that you've already gone through [Codelab #1].

This document is intended to give an overview of some of the possibilities you have for the set up and evaluation of your Tast test. This is not an exhaustive list but contains some of the most used techniques that are available to create tests.

[Codelab #1]: codelab_1.md

## Evaluation
### Checking wrapped errors
Go offers the functionality to wrap errors in other errors to allow returning all occurred error messages from a function call. To check if any of these wrapped errors is of a specific type that should be handled differently [errors.Is] can be used.
```
var ErrWindowNotFound = errors.New("window not found")
// FindMinimizedWindow returns a minimized window, if any. If there is no minimized
// window, ErrWindowNotFound is returned.
func FindMinimizedWindow() (*Window, error) {
	ws, err := findAllWindows()
	if err != nil {
		return nil, err
	}
	for _, w := range ws {
		if w.Minimized {
			return w, nil
		}
	}
	return nil, ErrWindowNotFound
}

func someFunction(...) error {
	w, err := FindMinimizedWindow()
	if err != nil {
		if errors.Is(err, ErrWindowNotFound) {
			return nil
		}
		return err
	}
	...
}
```

[errors.Is]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors#Is

### Command line
As Chrome OS is based on Linux we can execute Linux commands on the command line that can give us the needed information of the state of the system. This is done with the [testexec.CommandContext] function.
The `CommandContext()` function wraps the standard go exec package to honor the timeout of the context in which the test is running.
```
out, err := testexec.CommandContext(ctx, "lshw", "-C", "multimedia").Output(testexec.DumpLogOnError)
if err != nil {
	// Do error handling here.
}
```
In the example the command `lshw -C multimedia` is executed on the command line. The output of the execution is written into the out variable by calling the `Output` function and should then contain a list of all connected multimedia devices.
By passing `testexec.DumpLogOnError` we also get the stderr output in case the execution fails.

[testexec.CommandContext]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/testexec#CommandContext

### Checking windows
In some cases checking if certain windows have been opened, or a certain number of windows have been opened can be enough to check if a test was successful or not. To do that [ash.GetAllWindows] can be used. See the [ash package documentation] for more information.
It requires a context and a test connection, which is obtained by a call to [chrome.TestAPIConn], and will return an array of all open windows. This array can then be checked for example for the title of the windows to see if a desired window is open.
```
tconn, err := cr.TestAPIConn(ctx)
if err != nil {
	// Do error handling here.
}

ws, err := ash.GetAllWindows(ctx, tconn)
if err != nil {
	// Do error handling here.
}

// Find the desired window with expectedTitle.
for _, w := range ws {
	if strings.Contains(w.Title, expectedTitle) {
		// The test was successful.
	}
}
```

[ash.GetAllWindows]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/ash#GetAllWindows
[chrome.TestAPIConn]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome#Chrome.TestAPIConn
[ash package documentation]: https://chromium.googlesource.com/chromium/src/+/HEAD/ash/README.md

### Checking files
Similar to the windows the existence or non exsitence of a file might be the needed information to determine if the test was successful.
Go offers the [os.Stat] function for checking the existence or attributes of files.
```
fileInfo, err := os.Stat("/path/to/file/myfile")
if os.IsNotExist(err) {
	return ...  // File was not found
}
if err != nil {
	return ...  // Unknown error occurred
}
// File exists, fileInfo is valid
```
If [os.Stat] doesn't return an error, the file exists and additional information about the file is written to `fileInfo`.

[os.Stat]: https://golang.org/pkg/os/#Stat

### JavaScript evaluation
With [chrome.Conn.Eval] arbitrary JavaScript expressions can be evaluated. The function takes a context, a JavaScript expression and an interface as arguments. If the JavaScript expression returns a value, it will be unmarshaled into the given interface parameter. If umarshalling fails an error will be returned.

```
conn, err := cr.NewConn(ctx, URL)
if err != nil {
	// Do error handling here.
}
defer conn.Close()

var message string
if err := conn.Eval(ctx, `document.getElementById('element_id').innerText`, &message); err != nil {
	// Do error handling here.
}
if strings.Contains(message, 'this is the element_id') {
	// The test was successful.
}
```
In this example we open a new Chrome window with some URL and then we evaluate the JavaScript expression `document.getElementById('element_id').innerText` in this Chrome window. The result is written into the string message and is then checked if it contains a desired text.
This can also be used for expressions returning a promise, in which case the function will wait until the promise is settled.
```
const code = `return new Promise((resolve, reject) => {
	const element = document.getElementById('element_id');
	if (element === null) {
		resolve(false);
		return;
	}
	if (element.innerText !== 'some text') {
		reject(new Error('Unexpected inner text: want some text; got ' + element.innerText));
		return;
	}
	resolve(true);
})`

var found bool
if err := conn.Eval(ctx, code, &found); err != nil {
	// Do error handling here.
}
```
The [chrome.Conn.Eval] can also be used on the connection to Tast's test extension which gives access to other APIs. A connection can be created with [chrome.TestAPIConn]. The connection to Tast's test extension should not be closed as it is shared.

[chrome.Conn.Eval]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome#Conn.Eval

### Find the JavaScript path for an element
In the previous paragraph we took a look at how to evaluate JavaScript expressions in a Tast test, however finding the JavaScript expression you need for a certain test can be difficult.
The Developer Tools of Chrome can be very helpful for that. Open them by pressing CTRL + SHIFT +  I (or by opening the menu -> more Tools -> Developer Tools) in a Chrome window. In the Elements tab you can browse through the elements of a page and expand them. The selected element will be highlighted. Once you got to the element you want to check right click it in the Elements tab and select Copy -> Copy JS path. This gives you the expression to get the desired element. In the Console Tab you can try beforehand if the JavaScript expression you want to use delivers the desired output.

### Interacting with the UI
It is also possible to directly interact with the elements of the UI (like clicking on them, or just hovering over the mouse), or just to get information about their status. This can be done through the Test API with the help of the automation library. The basics of the usage of this library can be found in [Codelab #3].

[Codelab #3]: codelab_3.md

### Waiting in tests
To check some condition it is sometimes necessary to wait until certain changes have been processed in Chrome OS. For such cases the [testing.Poll] function should be used instead of sleeping in tests, as it does not introduce unnecessary delays and race conditions in integration tests. See also [Context and timeouts].
```
// Wait until the condition is true.
if err := testing.Poll(ctx, func(ctx context.Context) error {

	if err := doSomething(); err != nil {

		// In case something went wrong we can stop waiting and return an error with testing.PollBreak().
		return testing.PollBreak(errors.Wrap(err, "failed to do something critical"))
	}

	// Get the current state of our condition.
	condition, err := checkCondition()
	if err != nil {
		// Do error handling here.
	}
	if condition != expectedCondition {
		return errors.Errorf("unexpected condition: got %q; want %q", condition, expectedCondition)
	}

	return nil

}, &testing.PollOptions{
	Timeout: 30 * time.Second,
	Interval: 5 * time.Second,
}); err != nil {
	s.Fatal("Did not reach expected state: ", err)
}
```

[testing.Poll]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Poll
[Context and timeouts]: https://chromium.googlesource.com/chromiumos/platform/tast/+/refs/heads/main/docs/writing_tests.md#contexts-and-timeouts

## Setup
### Using the Launcher
We can use the Chrome Launcher in our tests to search and start applications or websites.
```
tconn, err := cr.TestAPIConn(ctx)
if err != nil {
	// Do error handling here.
}

if err := launcher.SearchAndLaunch(ctx, tconn, appName); err != nil {
	// Do error handling here.
}
```
The launcher requires a context, a test connection and a string that will be typed into the launcher. This example will start the application defined in `appName`, or if it isn't found a Google search for `appName` will be opened in a new Chrome window.

### HTTP Server
In many tests having a local http server can be helpful to avoid being dependent on a network connection. The [httptest.NewServer] function can be used to start your own http server from within the test.
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
In this example we created a small website with two files: `my_test.html` and `my_test.js`, and added the files to the test in the definition of the metadata for the test.
In the test we start a HTTP server as a `http.FileServer` which serves requests for the files located in the folder given as argument. The used folder, `s.DataFileSystem()`, is the folder where additional files for the test are copied to on the test device, which is where our files for the website end up. Then we open the website in a new Chrome window.

[httptest.NewServer]: https://golang.org/pkg/os/
