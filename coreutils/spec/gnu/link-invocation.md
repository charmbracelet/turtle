# link invocation (GNU Coreutils 9.12)
### 12.1 `link`: Make a hard link via the link syscall

`link` creates a single hard link at a time.
It is a minimalist interface to the system-provided
`link` function. See [Hard Links](https://www.gnu.org/software/libc/manual/html_node/Hard-Links.html#Hard-Links) in The GNU C Library Reference Manual.
It avoids the bells and whistles of the more commonly-used
`ln` command (see [`ln`: Make links between files](ln-invocation.md)).
Synopsis:

```
link filename linkname
```

filename must specify an existing file, and linkname
must specify a nonexistent entry in an existing directory.
`link` simply calls `link (filename, linkname)`
to create the link.

On a GNU system, this command acts like ‘`ln --directory
--no-target-directory filename linkname`’. However, the
`--directory` and `--no-target-directory` options are
not specified by POSIX, and the `link` command is
more portable in practice.

If filename is a symbolic link, it is unspecified whether
linkname will be a hard link to the symbolic link or to the
target of the symbolic link. Use `ln -P` or `ln -L`
to specify which behavior is desired.

An exit status of zero indicates success,
and a nonzero value indicates failure.
