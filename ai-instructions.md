## Handling Structs and Pointers

When working with structs in Go, always treat structs as pointer receivers, pointer arguments, and pointer return values by default.
Additionally, use pointer types for struct fields whenever possible.

The main reason is to maintain consistency so that client code always handles structs through pointers.
Efficiency gained from pointer usage is the second reason. If working with pointers makes the code harder to use or read, feel free to use value types instead.

Basically, structs should be handled using pointers, but there are exceptions.
If there are established conventions or good practices in Go that specify whether to use values or pointers, prioritize following those conventions/good practices.
For example, `sync.RWMutex` is almost never stored as a pointer in a struct field.

In all other cases, use value types only when the drawbacks of using pointers outweigh the benefits.

Please treat primitive values as value types by default.

## Use constants whenever possible.

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

## What to Include After Code Fixes or Implementation

After making changes, please always include the key points to review. Also mention anything else that caught your attention while modifying the code, as well as any areas where the correctness of the implementation should be verified.

If the changes are extremely minor, you do not need to include review points. Instead, simply provide a brief summary of what changed.

Please also provide a concise, one-line Git commit message.

## About Language

Although the instructions and conversation will be in Japanese, please write all code comments in English. The implementation output may use Japanese or other languages, but all code comments should be in English.

If there is a special need to use another language, I will explicitly instruct you to do so.

## When I Need to Make a Decision

When I need to choose among several options, please explain the factors I should consider, along with the advantages and disadvantages of each option.