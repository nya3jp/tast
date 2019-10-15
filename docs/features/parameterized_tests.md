# Parameterized tests (go/tast-parameterized-tests)

A test may specify `Params` to generate variations of the test. A test with
`Params` is called a *parameterized test*. Registration of a parameterized test
looks like the following:

```
type animal struct {
    numLegs int
    crying  string
}

func init() {
    testing.AddTest(&testing.Test{
        Func:     Param,
        Desc:     "Parameterized test example",
        Contacts: []string{"tast-owners@google.com"},
        Params: []testing.Param{{
            Name: "dog",
            Val: animal{
                numLegs: 4,
                crying:  "bow-wow",
            },
        }, {
            Name: "duck",
            Val: animal{
                numLegs: 2,
                crying:  "quack",
            },
        }},
    })
}

func Param(ctx context.Context, s *testing.State) {
    // This will print {4 bow-wow} for "dog", and {2 quack} for "duck".
    s.Logf("Value: ", s.Param().(animal))
}
```

## Name

Each `Param` can have a `Name`. It will be appended to the test name with
a leading `.`. For example, `category.TestFuncName.param1`. The `Name` should
be in `lower_case_snake_case` style. All `Name`s in a `Params` array should be
unique. `Name` can be empty (or can be omitted). In this case, no suffix
(including a leading `.`) will be appended. Because of the uniqueness
requirement, a `Params` array can have at most one unnamed param case.

## Val

Each `Param` can have a `Val`. The specified value can be accessed via the
`testing.State.Param()` method in the test body. Because it just returns an
`interface{}`, the returned value should be type-asserted to some concrete
type immediately. All `Val`s in a `Params` array should have the same type.

## ExtraAttr, ExtraData, ExtraSoftwareDeps, Pre, Timeout

Each `Param` can declare `ExtraAttr`, `ExtraData`, `ExtraSoftwareDeps`, `Pre`,
and `Timeout` properties. For example, in the following code:

```
testing.AddTest(&testing.Test{
    Func: DoSomething,
    ...
    SoftwareDeps: []string{"chrome"},
    Params: []testing.Param{{
        Name: "play",
        ExtraSoftwareDeps: []string{"audio_play"},
        Pre: arc.Booted(),
        Timeout: 3 * time.Minute,
    }, {
        Name: "record",
        ExtraSoftwareDeps: []string{"audio_record"},
        Pre: arc.VMBooted(),
        Timeout: 10 * time.Minute,
    }}
})
```

In each generated test, the `Extra*` values are appended to `Attr`, `Data` and
`SoftwareDeps` in the enclosing `testing.Test` respectively. For example,
`DoSomething.play` will run on DUTs with `"chrome"` and `"audio_play"`
available, while `DoSomething.record` will run on DUTs with `"chrome"` and
`"audio_record"` available. Note that both will run on a DUT with all
`"chrome"`, `"audio_play"` and `"audio_record"` available.

For `Pre` and `Timeout`, each parameterized test can define its own `Pre` and/or
`Timeout` as long as it is not defined in the enclosing `testing.Test`.
If `Pre` and/or `Timeout` is only defined in the enclosing `testing.Test`,
it will be inherited by the parameterized tests.

If more than one parameterized test define `Pre`, these `Pre` must return the
same value type. E.g: It is Ok if a parameterized test uses `arc.Booted()` and
another one uses the hypothetical `arc.VMBooted()` precondition, since both
preconditions return the same value type. But it will fail if one uses `arc.Booted()`
and another one uses `chrome.LoggedIn()` since they return different value types.

Please see also [attributes], [data] and [software dependencies] for details.

[attributes]: ../test_attributes.md
[data]: ../writing_tests.md#Data-files
[software dependencies]: ../test_dependencies.md

## Parameterized test registration

Because test registration should be declarative as written in
[test registration], `Params` should be an array literal containing `Param`
struct literals. In each `Param` struct, `Name` should be a string literal with
`snake_case` name if present. `ExtraAttr`, `ExtraData`, `ExtraSoftwareDeps` and
`Pre`, should follow the rule of the corresponding `Attr`, `Data` ,`SoftwareDeps`
and `Pre` in [test registration].

[test registration]: ../writing_tests.md#Test-registration
