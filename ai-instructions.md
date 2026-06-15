## Handling Structs and Pointers

When working with structs in Go, always treat structs as pointer receivers, pointer arguments, and pointer return values by default.
Additionally, use pointer types for struct fields whenever possible.

The main reason is to maintain consistency so that client code always handles structs through pointers.
Efficiency gained from pointer usage is the second reason. If working with pointers makes the code harder to use or read, feel free to use value types instead.

In all other cases, use value types only when the drawbacks of using pointers outweigh the benefits.

Please treat primitive values as value types by default.

## Use constants whenever possible

If a function repeatedly uses fixed strings, numbers, time.Duration, or similar values, define them as constants whenever possible and use those constants within the function.
The same applies to defining default values.

## About Backward Compatibility

This project has not been released yet.
Therefore, you do not need to write code that considers backward compatibility, such as providing function aliases.

## About Tests

Please write tests using Ginkgo.
If the project does not already include `github.com/onsi/gomega` and `github.com/onsi/ginkgo`, install them when creating tests.

Please create test files on a per-file basis, rather than having a single test file per package.

For each package, create exactly one Ginkgo *_suite_test.go file.
Always keep the *_suite_test.go file separate from the test files that contain the actual test cases.

When writing tests, be aware that the implementation may be incorrect. In such cases, do not write tests just to make them pass. Instead, implement the test as the correct and intended test, even if it would fail, and mark the test as skipped.
Additionally, add a comment to that test saying: FIXME: consider fixing the implementation. After that, ask me whether the implementation should be fixed so that the test can pass.
