# Secret variables (go/tast-secret-vars)

This feature is for internal developers, who has access to `tast-tests-private` package.

TODO(crbug.com/982546): this feature is under development.

## What

This feature allows you to store secret key/value pairs in a private repository, and use them from public tests.

## How

Let `foo.Bar` be the test which should access secret username and password.

If the variables are only used from the test, create the file `tast-tests-private/vars/foo.Bar.yaml` with the contents:

```Yaml
foo.Bar.user: someone@something.com
foo.Bar.password: whatever
```

If the values are shared among the `foo` category, create `foo.yaml` file instead.

```Yaml
foo.user: someone@something.com
foo.password: whatever
```

Then the test can access the variables just like normal variables assigned to the `tast` command with `-var`.

**Don't log secrets in tests** to avoid possible data leakage.

```Go
func init() {
	testing.AddTest(&testing.Test{
		Func:     Bar,
        ...
		Vars: []string{"foo.Bar.user", "foo.Bar.password"},
		// or foo.user, foo.password
	})
}

func Bar(ctx context.Context, s *testing.State) {
    user := s.RequiredVar("foo.Bar.user")
    ...
}
```

See example.PrivateVars for working example. TODO(oka): add the link to the test after it's submitted.

## Naming convention

Secret variables definition and usage should follow these rules:

* Variable name should have the form of `foo.Bar.something` or `foo.something`, where `something` matches `[a-zA-Z]\w*`
* The file defining `foo.Bar.something` should be `foo.Bar.yaml`
* The file defining `foo.something` should be `foo.yaml`
* Only the test `foo.Bar` can access `foo.Bar.something`
* Only tests in `foo` category can access `foo.something`

If one violates this convention, Tast linter will complain. Please honor the linter errors.

TODO(crbug.com/1014386): linter is not fully implemented.