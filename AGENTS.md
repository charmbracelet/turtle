# charm.land/snail

This repository is a collection of tools and libraries for Go.

* Each sub-project is a separate Go module, in the form of
  `charm.land/snail/{proj}` where `proj` is a sub-directory of this repository.

## Guidelines

* We'll be using [Scoped Commits](https://scopedcommits.com/) for commit style,
  so usually commit titles should either start with something like `{proj}:`,
  `all:`, `ci:`, `docs:`, `website:`, etc.
* Commit titles should be all lowercase most of the time.
* Make sure commit body is short and straightforward. We should actually omit
  it completely most of the time, unless we have contextual info really worth
  mentioning.
* We should write godoc comments for exported stuff, but they should be as short
  as possible. Unexported things don't need a godoc comment at all.
* In general, you should only write comments when really worth it, and if so,
  make them as clean and short as possible.
* Run `task fmt modernize` to format the code before committing.
* When writing tests, use sub-tests to group tests of the same topic when it
  makes sense.
* Also on testing, use table-driven tests to make them legible and easy to read,
  change and add more.
* Use `t.Context()` for contexts in tests, instead of `context.Background()`.

## charm.land/snail/fsx

* This is meant to be a Go package which is a superset of the existing `io/fs`
  interface from the Go standard library.
* The original one only support read operations, like `Read` and `Stat`.
* We want to be able to embed that interface, but add extra methods to allow
  some write operations as well (like `OpenFile`, `Chmod`, etc).
* We'll likely implement functionality incrementally as we need while working on
  coreutils rather than implementing it all at once.
* We'll also want to have at least two implementations here:
  1. An implementation that literally do that real operations on the file
     system.
  2. A memory implementation that mostly target the ability to write tests for
     code that will use this package.
* The first implementation should allow to set a specific base directory. We'll
  need to use functions like `filepath.Join` to make all functions respect the
  given base directory (like a "working directory").
* The main use case for this project will be empowering the next project
  described below: `charm.land/snail/coreutils`.

The main `fsx.FS` interface embeds `fs.FS` from the standard library plus the
extra functionality, split into multiple smaller interfaces:

```go
type FS interface {
    fs.FS

    WriteFS
    ChtimesFS
    ChmodFS
    ChownFS
    MkdirFS
    RemoveFS
    RenameFS
    LinkFS
    SymlinkFS
}
```

Each of these interfaces covers one group of write-like operations, so
callers and implementations can depend on only what they need.

## charm.land/snail/coreutils

This Go module will hold the implementation of many coreutils in Go.

* We'll have a common Go interface in the root (`charm.land/snail/coreutils`)
  that every coreutil will need to implement.
* We'll also have in root a type called `NewFunc` with a function signature all
  coreutils should respect for their `{package}.New` functions.
* The package that will hold every implementation will be in
  `charm.land/snail/coreutils/{prog}`.
* For every coreutils we'll also have a respective `main` package in
  `charm.land/snail/coreutils/cmd/{prog}`. This `main` package will just be a
  thin wrapper around the respective coreutil implementation.
* We should have extensive tests for every behavior, based on the coreutils
  specification.

The process to implement a coreutils should be as follow:

* We'll be targeting POSIX compatibility first. (GNU extensions will follow
  later).
* The POSIX and GNU specifications / manuals were downloaded and made available
  locally at:
  * `coreutils/spec/markdown/posix/*.md`
  * `coreutils/spec/markdown/gnu/*.md`
* Tests should be written according to the POSIX specification.
  * Every POSIX option must have at least one dedicated test. Options should
    also have tests covering invalid usage and important interactions with other
    options where the specification defines them.
  * When relevant, we should probably have some tests for a combination of
    multiple tests.
* Windows compatibility is important.
  * Given these programs will be mostly considered "Bash commands" we should
    probably accept forward slashes and make them work as they would on an
    Unix-like system.
  * For utilities that take pathnames, normalize the operands with
    `filepath.ToSlash` before processing, so Windows-style backslashes are
    treated as separators there. On other platforms this is a no-op, keeping
    the POSIX behavior untouched. (GNU coreutils does the same with its
    platform-dependent `ISSLASH` macro.)
  * When a given behavior is impossible on Windows, it probably makes sense to
    implement a no-op behavior rather than let is break or return an error.
  * It might be worth to have Windows-specific tests as well (when it makes
    sense). Gate them with `runtime.GOOS` and skip on the other platforms, so
    they actually run on Windows. We'll setup CI to also run on Windows to make
    sure these tests pass.
* Implement the minimum behavior necessary to satisfy the specification. Don't
  write more code than needed, and keep it clean.
* Once a core utility is implemented, the table on `coreutils/README.md` should
  be updated.
* IMPORTANT: Be careful with Go functions that depend on system ENV, system
  working directory or other program state. You should use the memory state
  passed given from `{package}.New`.

Code style guidelines:

* Be sure to read `coreutils/coreutils.go` to understand how the shared
  interface work. We might need to add some small bits of functionality there
  as we progress.
* All the `main` files in `charm.land/snail/coreutils/cmd/{proj}/` should be
  called `main.go` and be a thin wrapper around the implementation.
* `main` packages should delegate to `cmdutil.Main` from
  `charm.land/snail/coreutils/internal/cmdutil`, which builds the `Runtime`,
  runs the utility and maps errors to exit statuses.
* The implementation of a coreutil should be a `Prog` struct with at least
  `Runtime` as one of the fields. It should have a `{package}.New` function
  respecting the `charm.land/snail/coreutils.NewFunc` signature to return `Prog`.
* Respect the `context.Context` given to `Run`. Long operations (like reading
  input) should check it periodically and return the context error as-is when
  it's done, so callers can interrupt them and use `errors.Is`.
* We should use as much as possible of the standard library, and avoid external
  dependencies, even for tests.
* Option parsing belongs to `charm.land/snail/coreutils/internal/getopt`, which
  implements the POSIX syntax and its diagnostics: grouped single-character
  options, option-arguments attached or in the next argument, `--` ending the
  options, parsing stopping at the first operand, and long options accepted
  only when a utility documents them. Utilities keep their own option table,
  defaults and conflict checks in their `parseFlags` function. (This doesn't
  apply to `echo`, which the POSIX specification forbids from supporting
  options at all.)
* The GNU common options `--help` and `--version` are shared: every utility
  which takes options calls `stdopts.Check` from
  `charm.land/snail/coreutils/internal/stdopts` at the top of its `Run`, with
  its `getopt.Set` and its usage text, a `usage` string constant following
  the GNU layout and describing only what the utility implements. The
  exceptions are `echo`, `printf` and `test`, whose specifications forbid
  options. `false` still exits with status 1 after handling them.
* Usage errors, like invalid options and missing or extra operands, are
  followed by the GNU hint on the standard error, written with
  `stdopts.TryHelp`.
* Avoid `panic`s. We should return errors instead.
* Make sure to implement code in smaller functions rather than having a big
  `Run` implementation.

`main` packages will look similar to this:

```go
func main() {
	cmdutil.Main({prog}.New)
}
```
