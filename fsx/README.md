# charm.land/turtle/fsx

[![Go Reference](https://pkg.go.dev/badge/charm.land/turtle/fsx.svg)](https://pkg.go.dev/charm.land/turtle/fsx)

A superset of [`io/fs.FS`](https://pkg.go.dev/io/fs#FS) that also supports
write-like operations.

The file system interface from the Go standard library is read-only. This
package embeds it and extends it with write-like operations, keeping the read
side fully compatible with the stdlib.

## Installation

```bash
go install charm.land/turtle/fsx@latest
```

## Example

The same file system value supports writes and the standard library's read
helpers. `OsFS("")` resolves relative names in the current working directory
of the process:

```go
package main

import (
    "fmt"
    "io"
    "io/fs"
    "log"
    "os"

    "charm.land/turtle/fsx"
)

func main() {
    const workingDir = "" // empty means current

    fsys := fsx.OsFS(workingDir)
    flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
    f, err := fsys.OpenFile("hello.txt", flags, 0o644)
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()

    if _, err := io.WriteString(f, "hello, world\n"); err != nil {
        log.Fatal(err)
    }
    contents, err := fs.ReadFile(fsys, "hello.txt")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Print(string(contents))
}
```

To execute the same operations in memory, replace `OsFS` with an empty
`fsx.MemFS{}`. Files can also be seeded when constructing the map, as shown
below.

## Interfaces

### `fsx.FS`

The main interface, `fsx.FS`, embeds `io/fs.FS` plus:

- `WriteFS`
- `ChtimesFS`
- `ChmodFS`
- `MkdirFS`
- `RemoveFS`
- `RenameFS`
- `LinkFS`
- `SymlinkFS`

### `fsx.File`

`fsx.File` embeds `io/fs.FS` + `io.Writer`.

Additionally, we expose additional functionality like:

* `fsx.Atime` reads the last access time of a file, which `fs.FileInfo` doesn't
expose
* `fsx.Owner` reads the numeric owner and group IDs of a file for the same
  reason
* `fsx.SeekFile` is a file whose offset can be changed, like `os.File`
* `fsx.ChownFS` changes the numeric owner and group of a file, like `os.Chown`,
  where a uid or gid of -1 means to not change that value
* `fsx.SyncFS` flushes any data a file system still holds with `Sync`, like
`sync(2)`

`fsx.SeekFile` isn't embedded into the other interfaces because not every
file is seekable, like pipes: callers should check for it and fall back to
reading sequentially when it isn't implemented.

`fsx.ChownFS` isn't embedded into `fsx.FS`: callers should check for it before
relying on ownership changes. The provided implementations support it.

`fsx.SyncFS` is supported by `OsFS` and `MemFS`, but it isn't embedded into
`fsx.FS` either, because not every file system can be flushed, like a network
one: callers should check for it and treat its absence as a successful no-op.

## Implementations

Three implementations are available:

* `fsx.OsFS(dir)` operates on the operating system's file system. Names are
  resolved inside `dir`, which works like a working directory, except that
  absolute paths are used as given. An empty `dir` means the current working
  directory of the process.
* `fsx.RootFS(root)` adapts an `os.Root`. Operations are confined to the root
  using `os.Root` semantics. The caller retains ownership and must close it.
* `fsx.MemFS` is an in-memory file system, meant mostly for writing tests of
  code that uses this package. Files are seeded with a map literal, like in
  `testing/fstest.MapFS`:

```go
fsys := fsx.MemFS{
    "file.txt": {Data: []byte("contents")},
}
```

Besides `Data`, a `MemFile` sets the `Mode`, `ModTime`, `Uid` and `Gid` of
the entry. Its access time starts as the modification time, and is otherwise
only changed by `Chtimes`.

Directories are implicit: a name of the form "dir/file" makes "dir" statable
as a directory.

Symbolic links are entries with the `fs.ModeSymlink` mode bit and the link
target as their data, and are followed by the read operations, like `Open` and
`Stat`. `Link` makes two names share the same entry, so writes through either
are seen by the other.

## Why?

This module was built to make [charm.land/turtle/coreutils](/coreutils/README.md)
possible. Read-only file systems aren't enough to implement utilities like
`touch`, `mkdir` or `cp`, while keeping an `io/fs` compatible interface allows
the utilities to run isolated from the real file system, including on tests.
