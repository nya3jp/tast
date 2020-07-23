# Tast Codelab #4: kernel.LogMount (go/tast-codelab-4)

> This document assumes that you've already gone through [Codelab #1].

This codelab is intended to give an overview of some of the possibilities you have for the set up and evaluation of your tast-test. This is not an exhaustive list but contains some of the most used functions that are available to create tests.

## Setup
### HTTP Server
In many tests having a local http server can be helpful to avoid being dependent on an internet connection for interactions with websites. The httptest package (https://osscs.corp.google.com/chromiumos/chromiumos/codesearch/+/master:src/aosp/toolchain/gcc/gcc-4.9/libgo/go/net/http/httptest/) offers the functions to start your own http server from within the test.
```
func init() {
	testing.AddTest(&testing.Test{
		Func: MyTest,
		.
		.
		.
		Data:         []string{"my_test.html", "my_test.js"},
	})
}

func MyTest(ctx context.Context, s *testing.State) {
	.
	.
	.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL+"/my_test.html")
	if err != nil {
		// Do error handling here.
	}
	defer conn.Close()
	.
	.
	.
	}
```
In this example we created a small website with two file, my_test.html and my_test.js, and added the files to the test in the definition of the metadata for the test.
In the test we start an httpserver as a http.FileServer which serves requests for the files located in the folder given as argument. The used folder, s.DataFileSystem(), is the folder where additional files for the test are copied to on the DuT, which is where our files for the website end up. Then we open the website in a new chrome window from our chrome instance (cr).

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
The launcher requires a context (ctx), a TestAPIConn (tconn) and a string that will be typed into the launcher. This example will start the application defined in appName, or if it isn't found a Google search for appName will be opened in a new chrome window.

## Evaluation
### Command line
As chromeOS is based on linux we can execute linux commands on the command line that can give us the needed information of the state of the system. This is done with the CommandContext() function from the testexec package (https://osscs.corp.google.com/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/testexec/testexec.go).
The CommandContext() function gets a context as argument, either your test context or a derived shortened context, as well as the command that should be executed on the command line. The command is separated into several function arguments, one for each space separated part of the command.
With the Output() function the output of the command can be captured which can then be used for further evaluation.
```
out, err := testexec.CommandContext(ctx, "ls", "-l", “/home/chronos/user”).Output()
if err != nil {
	// Do error handling here.
}
```
In the example the command "ls -l /home/chronos/user" is executed on the command line. The output of the execution is written into the out variable by calling the Output() function and should then contain the list of files with details that are in the /home/chronos/user directory. This could be used to check i.e. if a specific file has the correct size.

### Checking windows
In some cases checking if certain windows have been opened, or a certain number of windows have been opened can be enough to check if a test was successful or not. To do that the GetAllWindows() function of the ash package can be used (https://osscs.corp.google.com/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/chrome/ash/wm.go).
The GetAllWindows() function requires a context and a TestAPIConn and will return an array of all open windows. This array can then be checked for example for the title of the windows to see if a desired window is open.
```
tconn, err := cr.TestAPIConn(ctx)
if err != nil {
	// Do error handling here.
}

windows, err := ash.GetAllWindows(ctx, tconn)
if err != nil {
	// Do error handling here.
}

for _, window := range windows {
	if strings.Contains(window.Title, expectedTitle) {
		// The test was successful.
	}
}
```
In the example we first create a TestAPIConn from our chrome instance (cr), then we call GetAllWindows() and so we get an array of all windows which we then check for a specific window by looping over all windows and comparing their title to the expected window title.

### Checking files
Similar to the windows the existence or non exsitence of a file might be the needed information to determine if the test was successful.
Of course files can be checked over the command line too, but if the files are in a chrome specific directory it is better to use the FilesApp (https://osscs.corp.google.com/chromiumos/chromiumos/codesearch/+/master:src/platform/tast-tests/src/chromiumos/tast/local/chrome/ui/filesapp/filesapp.go).
In addition to check if a file is there the FilesApp can be used to select and open files, open the quickview menu for a file or to get a ui node of a file.
```
tconn, err := cr.TestAPIConn(ctx)
if err != nil {
	// Do error handling here.
}

files, err := filesapp.Launch(ctx, tconn)
if err != nil {
	// Do error handling here.
}
defer files.Close(ctx)

if err := files.OpenDownloads(ctx); err != nil {
	// Do error handling here.
}
if err := files.WaitForFile(ctx, "file", 5*time.Second); err != nil {
	if errors.Is(err, context.DeadlineExceeded) {
		// Do error handling here.
	} else {
		// Do error handling here.
	}
} 
```
In the example we create a TestAPIConn from our instance of chrome (cr), then we launch the FilesApp and open the downloads folder with the FilesApp. Finally we poll for a specific file for 5 seconds with the WaitForFile() function.

### Javascript evaluation
With the Eval() function from the chrome package arbitrary javascript expressions can be evaluated. The Eval() function takes a context a javascript expression and a reference to a string expression as arguments. The output of the javascript expression will be written into the string.
To get the javascript expression the Developer Tools of chrome can be used. Open it by pressing CTRL + SHIFT +  I (or by opening the menu -> more Tools -> Developer Tools) in a chrome window. In the Elements tab you can browse through the elements of the page and expand them. The selected element will be highlighted. Once you got to the element you want to check right click it in the Elements tab and select Copy -> Copy JS path. This gives you the expression to get the desired element. In the Console Tab you can try beforehand if the javascript expression you want to use delivers the desired output.

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
In this example we open a new chrome window with some url and then we evaluate the javascript expression ```document.getElementById("element_id").innerText``` in this chrome window. The results are written in the string message and is then checked if it contains a desired text.

### Interacting with the UI
It is also possible to directly interact with the elements of the UI (like clicking on them, or just hovering over the mouse), or just to get information about their status. This can be done through the Test API with the help of the automation library. The basics of the usage of this library can be found in the 3rd Tast Codelab.
It is important to note that not all of the UI elements exists immediately, in some cases it is better to use the FindWithTimeout() function instead of the Find() function.
In some cases we need to check or interact with lesser used attributes of the UI elements that have no built in support in the automation library, for example:
```
// Check the checked state of the toggle button.
if checked, err := tbNode.Attribute(ctx, "checked"); err != nil {
	s.Fatal("Failed to get the checked attribute of the toggle button: ", err)
} else if checkedStr, ok := checked.(string); !ok {
	s.Fatal("The checked attribute of the toggle button is not a string: ", checked)
} else {
	// We can use the value of this attribute,
}
```
In these cases we first have to check whether the given attribute exists, than check if it has the type we expect it to have, and only after these checks can we proceed with our checks.

### Waiting for results
To check some condition it is sometimes necessary to wait until certain changes have processed in chromeOS. For such cases the Poll() function from the testing package can be used (https://osscs.corp.google.com/chromiumos/chromiumos/codesearch/+/master:src/platform/tast/src/chromiumos/tast/testing/util.go).
```
// Wait until the condition is true.
if err := testing.Poll(ctx, func(ctx context.Context) error {

	// Get the current state of our condition.
	
	// In case something went wrong we cannot recover from we can break the Poll() function here.
	//return testing.PollBreak(errors.Wrap(err, "failed to do something critical"))

	if condition != expectedCondition {
		return errors.Errorf("unexpected condition: got %q; want %q", condition, expectedCondition)
	}

	return nil

}, nil}); err != nil {
	// error handling
}
```
The Poll function executes periodically a given function that returns an error until the error is nil or the context it is running in reaches its deadline. When the context reaches its deadline the last error will be returned. In the function given as argument for the Poll() function we can now check for a certain condition to be met in which case we return nil to finish the Poll() function.
The last argument of the Poll() function is a PollOptions struct in which we can define a custom timeout and interval for the polling.
In case something goes wrong during polling from which the test cannot recover there is the option to break the polling early with the PollBreak() function.

