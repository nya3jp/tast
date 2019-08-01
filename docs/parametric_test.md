# Parametric tests

A test may specify `Params` to generate variations of the test.
A test with `Params` is called a *parametric test*.
Registration of a parametric test looks as follows:

```
testing.AddTest(&testing.Test{
  Func: DoSomething,
  Desc: "Description of the test",
  Contacts: []string{"owner@chromium.org"},
  Params: []testing.Param{{
    Name: "param1",
    Val: 10,
  }, {
    Name: "param2",
    Val: 20,
  }},
})

func DoSomething(ctx context.Context, s *testing.State) {
  // This will print 10 for DoSomething.param1 and 20 for DoSomething.param2.
  s.Logf("%d", s.Param().(int))
}
```

## Name

Each `Param` can have `Name`.
It will be appended to the test name with a leading `.`.
For example, `category.TestFuncName.param1`.
The `Name` should be in `lower_case_snake_case` style.
All `Name` in a `Params` array should be unique.
`Name` can be empty (or can be ommitted). In the case, no suffix, including a leading `.`,
won't be appended. Because of the uniqueness requirement, a `Params` array
can have at most one unnamed param case.

## Val

Each `Param` can have `Val`. The specified value can be accessed via
`testing.State.Param()` method in the test body. Because it just returns an
`interface{}`, practically the returned value should be type-asserted to some
concrete type immediately.
All `Val`s in a `Params` array should have the same type.

## ExtraAttr, ExtraData, ExtraSoftwareDeps

Each `Param` can declare `ExtraAttr`, `ExtraData` and `ExtraSoftwareDeps`.
For example, in the following code:

```
testing.AddTest(&testing.Test{
  Func: DoSomething,
  ...
  SoftwareDeps: []string{"chrome"},
  Params: []testing.Param{{
    Name: "play",
    ExtraSoftwareDeps: []string{"audio_play"},
  }, {
    Name: "record",
    ExtraSoftwareDeps: []string{"audio_record"},
  }}
})
```

In each generated test, those values are appended to `Attr`, `Data` and `SoftwareDeps`
in the enclosing `testing.Test` respectively.
For example, `DoSomething.play` will run on DUTs with `"chrome"` and `"audio_play"` available,
while `DoSomething.record` will run on DUTs with `"chrome"` and `"audio_record"` available.
Note that both will run on a DUT with all `"chrome"`, `"audio_play"` and `"audio_record"`
available.
Please see also [attributes], [data] and [software dependencies] for details.

[attributes]: test_attributes.md
[data]: writing_tests.md#Data-files
[software dependencies]: test_dependencies.md

## Test registration

Because test registration should be declarative as written in [test registration],
`Params` should be an array literal containing `Param` struct literal.
In each `Param` struct, `Name` should be a string literal with `snake_case` name if present.
`ExtraAttr`, `ExtraData` and `ExtraSoftwareDeps` should follow
the rule of the corresponding `Attr`, `Data` and `SoftwareDeps` in [test registration].

[test registration]: writing_tests.md#Test-registration
