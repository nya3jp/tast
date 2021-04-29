# Tast Test Validation

Reviewing test code can be time-consuming, so it's desirable to automate the
identification of common problems as much as possible. This document describes
and compares different approaches that are available.

[TOC]

## Considerations

Validation methods have different benefits and downsides, several of which are
listed here.

### Overridability

Some validation processes can be manually overridden by developers. This is
useful in the case of false positives, but can be problematic if the check is
trying to defend against severe issues that would also affect other tests.

### Using source code vs. compiled code

Validation processes can inspect either the original test source code or
behavior at runtime. Both techniques are important:

*   Source code validation is needed to detect coding style issues.
*   Runtime checks are needed to detect issues that can't be easily detected
    from source code, e.g. bad values in struct fields that are initialized by
    expressions or function calls.

### Ease of writing

Some validation code is easier to write than other code. In particular,
inspecting [ASTs] produced by parsing source code is a lot of work.

[ASTs]: https://en.wikipedia.org/wiki/Abstract_syntax_tree

### Early warning

It's best if a validation system notifies test authors about issues before
they've uploaded their code. If the checks are only performed when the test is
compiled and executed in the commit queue, developers may need to wait hours to
learn about issues.

## Validation methods

This section describes different validation methods that are in use in light of
the above considerations.

| Validation method    | Overridable | Uses source | Easy to write | Early warning |
|----------------------|:-----------:|:-----------:|:-------------:|:-------------:|
| `gofmt` / `goimport` |             |      ✓      |      n/a      |       ✓       |
| `go vet`             |             |      ✓      |      n/a      |               |
| `tast-lint`          |      ✓      |      ✓      |               |       ✓       |
| Test registration    |             |             |       ✓       |       ✓       |
| Unit tests           |             |             |       ✓       |               |

### gofmt and goimport

[gofmt] is used to format Go code and verify that it is syntactically valid.
It's typically configured to run automatically within developers' text editors
(sometimes via [goimport], which corrects and sorts `import` statements), so it
generally provides instant feedback. `tast-lint` (described below) also runs
`gofmt` when a change is uploaded for review.

[gofmt]: https://golang.org/cmd/gofmt/
[goimport]: https://godoc.org/golang.org/x/tools/cmd/goimports

### go vet

The [go vet] command is part of the official Go distribution:

> Vet examines Go source code and reports suspicious constructs, such as Printf
> calls whose arguments do not align with the format string. Vet uses heuristics
> that do not guarantee all reports are genuine problems, but it can find errors
> not caught by the compilers.

`go vet` is minimally configurable; we pass the `-printf.funcs` flag to enable
checking of additional `printf`-like functions in addition to the
`unusedresult.funcs` flag but otherwise use default settings.

`go vet` currently runs as part of the `src_test` stage when building test
bundle packages (e.g. `tast-local-tests-cros`). If developers don't manually
emerge the test bundle package with`FEATURES=test` or run `fast_build.sh -C`,
they may not learn about issues until their change is compiled in the Pre-Commit
Queue. Running `go vet` earlier (e.g. when uploading the change) seems
challenging since it expects to be able to resolve imports and therefore can't
be easily run outside the chroot. See [issue 888259] for further discussion, and
see also [Modifying Tast] for information about `fast_build.sh`.

[go vet]: https://golang.org/cmd/vet/
[issue 888259]: https://crbug.com/888259
[Modifying Tast]: modifying_tast.md

### tast-lint

[tast-lint] inspects modified source files. In addition to running `gofmt` and
`goimports`, it performs additional Tast-specific checks to identify problems
like disallowed cross-test dependencies and forbidden imports and function
calls. `tast-lint` is compiled and executed by [run_lint.sh], which is executed
by [PRESUBMIT.cfg] when a change is uploaded for review.

[tast-lint]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/src/chromiumos/tast/cmd/tast-lint/
[run_lint.sh]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/tools/run_lint.sh
[PRESUBMIT.cfg]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/PRESUBMIT.cfg

### Test registration

Some test metadata (as specified via the [testing.Test] struct) is validated at
runtime when tests are registered via [testing.AddTest]. This validation is
performed by the `instantiate` function in [test_instance.go]. Validated metadata
includes test names, data paths, and timeouts. If a test contains bad data, an
error is reported and no tests are executed. As such, test authors typically
notice problems while trying to run their tests locally.

Registration errors in local tests in the `cros` bundle are also caught by a
unit test in [main_test.go].

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[testing.AddTest]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#AddTest
[test_instance.go]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/src/chromiumos/tast/internal/testing/test_instance.go
[main_test.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/main_test.go

### Unit tests

Some validation is test-specific. For example, all [ARC] tests should depend on
the `android` and `chrome` [software features] and have a sufficiently-long
timeout. These sorts of checks can be performed by a unit test that inspects
test metadata after tests are registered. For the above ARC example, see
[registration_test.go] and the [testcheck] package.

You can use `fast_build.sh` to run unit tests in the `tast` and `tast-tests`
repositories. The script's usage is described in the [Modifying Tast] document.

[ARC]: https://developer.android.com/topic/arc/
[software features]: test_dependencies.md
[registration_test.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/arc/registration_test.go
[testcheck]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing/testcheck
