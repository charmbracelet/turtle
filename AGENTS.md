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
* Run `task lint` to format and check the code before committing.
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
  * `coreutils/spec/posix/*.md`
  * `coreutils/spec/gnu/*.md`
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
  the GNU layout and describing only what the utility implements. The two
  standard entries close the text, set apart from the aligned option table
  as current GNU does:

  ```
        --help
           display this help and exit
        --version
           output version information and exit
  ```

  The exceptions are `echo`, `printf` and `test`, whose specifications forbid
  options. `false` still exits with status 1 after handling them.
* The option table of a usage text is always in the hanging style: each
  option alone on its line, indented 2 spaces (6 when it has no short
  form), with its description indented 9 spaces below, wrapped greedily so
  no line exceeds 80 columns. This keeps the table uniform with the
  closing `--help` and `--version` pair, even when current GNU keeps a
  two-column table for a few utilities.
* Usage errors, like invalid options and missing or extra operands, are
  followed by the GNU hint on the standard error, written with
  `stdopts.TryHelp`.
* Avoid `panic`s. We should return errors instead.
* Make sure to implement code in smaller functions rather than having a big
  `Run` implementation.

Before writing a new shared helper, check `coreutils/internal/` — these
packages already exist:

* `backup`: the backup scheme GNU cp, install, ln and mv share, before a
  file is overwritten or removed. This is the home of the GNU backup-option
  logic once cp, mv and ln implement it.
* `basex`: the behavior shared by base32 and base64, on top of the RFC 4648
  encodings of the standard library.
* `bracket`: the POSIX bracket expressions shared by the basic and extended
  regular expression translators.
* `bre`: compiles POSIX basic regular expressions into Go ones.
* `charset`: the character set syntax shared by the utilities that translate
  characters, like tr. sed's y command looks similar but follows its own
  escape, range and delimiter rules, so it does not use this package.
* `cnum`: the C integer constant scanning (hex and octal prefixes) shared by
  the byte-offset operands of od and cmp.
* `cmdutil`: the `Main` helper the `cmd/{prog}` main packages delegate to.
* `ctxio`: I/O helpers that stop operations once a context is done.
* `digest`: the behavior shared by the checksum utilities (cksum, md5sum,
  the sha*sum family), which differ only in the algorithm.
* `diff`: the line diffing and the diff formats shared by diff and patch.
* `durationparse`: the duration operands with the s/m/h/d suffixes shared by
  sleep and timeout, which differ only on how they treat infinity.
* `ere`: compiles POSIX extended regular expressions into Go ones.
* `fileinfo`: file information beyond `io/fs` (birth time, ownership, link
  counts), read from the `Sys` value of a `FileInfo`, used by find, ls, stat
  and du.
* `filehdr`: the "==> name <==" headers which precede the output of each
  input file, as head and tail print them, and the modes which decide when
  they are written.
* `filemode`: parsing of the mode operands (octal or symbolic) shared by the
  utilities which match or change permission bits, like chmod, ls, stat and
  find. `install`, `mkdir` and `uudecode` depend on this package for mode
  parsing, not on the `chmod` program package.
* `fsop`: the file operand plumbing shared by cp, install, ln and mv: the
  source/destination pairs, the `-t`/`-T` operand split, the symbolic link
  chase and the interactive confirmation of rm and tar.
* `getopt`: the POSIX option syntax, described above.
* `glob`: compiles shell filename expansion patterns into regular
  expressions, as find's pattern primaries and grep's file selection options
  take them.
* `gnudate`: parses GNU-style date strings, like the `-d` options of date
  and touch.
* `input`: mapping the input operands to files or the standard input, and
  opening them.
* `numparse`: the ASCII digit predicates and the plain decimal integer
  parsing shared by the utilities taking counts, widths, positions and field
  numbers as operands.
* `output`: creating or appending the output operands the utilities write.
* `patherr`: unwraps `*fs.PathError` so diagnostics don't repeat the file
  name.
* `printfmt`: the pieces of printf-style format strings shared by printf and
  the `-printf`/`-fprintf` primaries of find.
* `rex`: helpers shared by the basic and extended regular expression
  translators.
* `sizeparse`: parses GNU-style size operands with multiplicative suffixes,
  like the bytes of head, tail, split and od.
* `sizefmt`: formats sizes the GNU way: `--block-size` specs and the
  human-readable notation of `-h`, used by du, ls and find.
* `stdopts`: the `--help` and `--version` common options, described above,
  plus the diagnostics every utility shares: the closing entries of the
  usage text (`Tail`), the version text (`VersionText`), the usage-error
  report (`UsageError`) and the program-prefixed diagnostic (`Errorf`).
* `strftime`: strftime-style time formats, as the `+format` operand of date
  takes.
* `tabutil`: the tab stop handling shared by expand and unexpand.
* `term`: reports whether file descriptors are terminals.
* `testutil`: the test scaffolding shared by the `{prog}` package tests: the
  table-driven case runner with the common Runtime setup and assertions, and
  the reader, writer and file system fakes.
* `textutil`: the line handling shared by the text processing utilities,
  like skipping the fields or characters of a line.
* `uux`: the formats shared by uuencode and uudecode.
* `usernames`: the user and group identifiers resolved to names, remembering
  each lookup, as ls, stat and find show them, with the fallback each one
  reports.
* `verbose`: the `-c`/`-v` reporting levels of the utilities which change
  file attributes, and the operand names they show the files they walk by.
* `walk`: traverses the file hierarchies below starting points, following
  symbolic links as `-H`/`-L`/`-P` ask, used by find, du and grep. It also
  holds the shared link-chain resolution, and the directory identifiers and
  name joins the recursive utilities and ls use.
* `zcore`: the behavior shared by the compression utilities, gzip and
  compress, which differ only in options and defaults.

`main` packages will look similar to this:

```go
func main() {
	cmdutil.Main({prog}.New)
}
```
