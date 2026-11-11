# rmdir invocation (GNU Coreutils 9.12)
### 12.7 `rmdir`: Remove empty directories

`rmdir` removes empty directories. Synopsis:

```
rmdir [option]... directory...
```

If any directory argument does not refer to an existing empty
directory, it is an error.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`--ignore-fail-on-non-empty`’
:   Ignore each failure to remove a directory that is non-empty.

‘`-p`’

‘`--parents`’
:   Remove directory, then try to remove each component of directory.
    So, for example, ‘`rmdir -p a/b/c`’ is similar to ‘`rmdir a/b/c a/b a`’.
    As such, it fails if any of those directories turns out not to be empty.
    Use the `--ignore-fail-on-non-empty` option to make it so such
    a failure does not evoke a diagnostic and does not cause `rmdir` to
    exit unsuccessfully.

‘`-v`’

‘`--verbose`’
:   Give a diagnostic for each successful removal.
    directory is removed.

See [`rm`: Remove files or directories](rm-invocation.md), for how to remove non-empty directories recursively.

To remove all empty directories under dirname, including
directories that become empty because other directories are removed,
you can use either of the following commands:

```
# This uses GNU extensions.
find dirname -type d -empty -delete

# This runs on any POSIX platform.
find dirname -depth -type d -exec rmdir {} +
```

An exit status of zero indicates success,
and a nonzero value indicates failure.
