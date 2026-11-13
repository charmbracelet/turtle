# charm.land/snail/fsx

A superset of [`io/fs.FS`](https://pkg.go.dev/io/fs#FS) that also supports
writing.

The file system interface from the Go standard library is read-only. This
package embeds it and extends it with write-like operations, keeping the read
side fully compatible with the stdlib.

## Interfaces

The main interface, `fsx.FS`, embeds `io/fs.FS` plus:

- `WriteFS`: `OpenFile` opens files for writing and creates new ones, like
  `os.OpenFile`.
- `ChtimesFS`: `Chtimes` sets the access and modification times of a file.
- `ChmodFS`: `Chmod` changes the file mode bits of a file, like `os.Chmod`.
- `MkdirFS`: `Mkdir` creates a new directory, like `os.Mkdir`.
- `RemoveFS`: `Remove` removes a file or an empty directory, like
  `os.Remove`, returning `fsx.ErrNotEmpty` when the directory still holds
  entries.
- `RenameFS`: `Rename` renames a file, moving it between directories, like
  `os.Rename`.
- `LinkFS`: `Link` creates a new name for an existing file, like `os.Link`.
- `SymlinkFS`: `Symlink` creates a symbolic link, like `os.Symlink`. The
  interface also embeds `io/fs.ReadLinkFS`, which reads links with `ReadLink`
  and stats them without following with `Lstat`.

On top of that, `fsx.File` is an `fs.File` that can also be written to,
`fsx.Atime` reads the last access time of a file, which `fs.FileInfo` doesn't
expose, `fsx.Owner` reads the numeric owner and group IDs of a file for the same
reason, and `fsx.SeekFile` is a file whose offset can be changed, like
`os.File`. The latter isn't embedded into the other interfaces because not every
file is seekable, like pipes: callers should check for it and fall back to
reading sequentially when it isn't implemented.

`fsx.ChownFS` changes the numeric owner and group of a file, like `os.Chown`,
where a uid or gid of -1 means to not change that value. It isn't embedded into
`fsx.FS`: callers should check for it before relying on ownership changes. The
provided implementations support it.

`fsx.SyncFS` flushes any data a file system still holds with `Sync`, like
`sync(2)`. `OsFS` and `MemFS` support it, but it isn't embedded into `fsx.FS`
either, because not every file system can be flushed, like a network one:
callers should check for it and treat its absence as a successful no-op.

More operations will be added incrementally as we need them.

## Implementations

Three implementations are available:

- `fsx.OsFS(dir)` operates on the operating system's file system. Names are
  resolved inside `dir`, which works like a working directory, except that
  absolute paths are used as given. An empty `dir` means the current working
  directory of the process.
- `fsx.RootFS(root)` adapts an `os.Root`. Operations are confined to the root
  using `os.Root` semantics. The caller retains ownership and must close it.
- `fsx.MemFS` is an in-memory file system, meant mostly for writing tests of
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

This module was built to make [charm.land/snail/coreutils](/coreutils) possible.
Read-only file systems aren't enough to implement utilities like `touch`,
`mkdir` or `cp`, while keeping an `io/fs` compatible interface allows the
utilities to run isolated from the real file system, including on tests.

> [!IMPORTANT]
> This is a Work in Progress :tm: We'll likely break compatibility while this
> is a v0 and the interface grows.
