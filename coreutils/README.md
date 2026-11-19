# charm.land/turtle/coreutils

A Go implementation of the Unix core utilities.

The intention is not to aim for 100% compatibility, but have something still
powerful and flexible enough to be imported by other Go programs, in particular
when using a [shell interpreter][mvdan].

## Example

`OsRuntime` wires utilities to the operating system, including the standard
streams, environment and file system. An empty directory argument uses the
current working directory of the process:

```go
package main

import (
    "context"
    "log"

    "charm.land/turtle/coreutils"
    "charm.land/turtle/coreutils/programs/cp"
    "charm.land/turtle/coreutils/programs/mv"
)

func main() {
    const workingDir = "" // empty means current

    rt := coreutils.OsRuntime(workingDir)
    ctx := context.Background()

    if err := cp.New(rt).Run(ctx, "input.txt", "copy.txt"); err != nil {
        log.Fatal(err)
    }
    if err := mv.New(rt).Run(ctx, "copy.txt", "archive.txt"); err != nil {
        log.Fatal(err)
    }
}
```

For custom or in-memory execution, instantiate a `Runtime` directly, setting
`FS` to an [`fsx.MemFS`](/fsx/README.md) and providing any desired standard
streams, working directory or environment. Utilities use the state in their
runtime, so no process-wide changes are needed.

## Structure

* `charm.land/turtle/coreutils`: base program interface and other types
* `charm.land/turtle/coreutils/cmd/*`: `main` packages; compile to to get the
  binary (`task bin` compiles them all on `./bin`)
* `charm.land/turtle/coreutils/programs/*`: importable packages for each utility
* `charm.land/turtle/coreutils/internal/*`: where shared code lives

## Why?

The [shell interpreter][mvdan] is a pretty popular project, namely used by
coding agents like [Crush][crush], developer tools like [Task][task] and others.
The interpreter doesn't include the core utilities though, with the exception
of a few built-ins. Having them would be particularly useful on Windows.

Historically, there has been other attempts to implement the core utilities in
Go, but none properly filled our needs completely, so we built ours!

Things that this implementation does that it's important for us:

* Importable packages! (some other implementation had non-importable `main`
  packages only)
* A common interface for the max flexibility
* Vast coverage of the POSIX and GNU specs (even if not 100% perfect)
* Compatible with all Go-supported operating systems
  (including JavaScript/WebAssembly!)
* Great support for Windows with sane compatibility (`/` works as a path
  separator, etc.)
* Complete state isolation:
  * stdin / stdout / stderr
  * File system (via [`charm.land/turtle/fsx`](/fsx/README.md))
  * Working directory
  * Environment variables
  * Hooks for the utilities that runs other programs (`timeout`, `xargs`, etc.)
  * Others

For example, our test suite runs nearly 100% in-memory! We want to make that
isolation possible when using from other Go programs as well.

Want to check our compatibility with POSIX and GNU? Check out our
[compatibility table](COMPATIBILITY.md). We also document
[deviations](DEVIATIONS.md) from the POSIX and GNU specs.

[mvdan]: https://github.com/mvdan/sh
[crush]: https://github.com/charmbracelet/crush
[task]: https://taskfile.dev
