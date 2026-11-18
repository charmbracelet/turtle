# charm.land/turtle

🐢 Turtle is a collection of shell tools and libraries in Go,
brought to you by [Charm][charm].

## [charm.land/turtle/coreutils](/coreutils/README.md)

A Go implementation of the Unix core utilities.

* Mostly compatible with POSIX and GNU
* Importable Go packages
* Isolated state (including filesystem)
* Zero dependencies
* Pluggable into Go [shell interpreters][mvdan]
* Used by [Crush][crush], [Task][task] and soon others!

## [charm.land/turtle/fsx](/fsx/README.md)

A filesystem abstraction for Go. Built to power the core utilities module,
but generic enough to be useful to many other projects.

* A superset interface of the standard library's
  [`io/fs.FS`](https://pkg.go.dev/io/fs#FS)
* Extends the stdlib's interface to support write operations like: write,
  rename, remove, chmod, mkdir and others
* Default implementations:
  * System (delegates to real FS)
  * Memory FS
  * [`os.Root`][osroot] wrapper

[charm]: https://charm.land
[mvdan]: https://github.com/mvdan/sh
[crush]: https://github.com/charmbracelet/crush
[task]: https://taskfile.dev
[osroot]: https://pkg.go.dev/os#Root
