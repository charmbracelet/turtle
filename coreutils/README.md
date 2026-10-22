# charm.land/snail/coreutils

A Go implementation of the Unix core utilities.

This is not meant to be perfect ports of each coreutil. Instead, it's meant to
have "good enough" compatibility to be embbeded in Go applications, usually when
running a shell interpreter.

I'm aiming for basic implementation based on the POSIX specifications first.
We can follow with some GNU extensions, specially the most common additions.

## Reasoning

There are other Go projects out there with implementations of the Unix core
utilities. Yet, this project started because we need to fit a specific use case.

[`mvdan.cc/sh`](https://github.com/mvdan/sh) is a great shell toolset (parser,
formatter and interpreter). It's used by projects like
[Task](https://github.com/go-task/task) and
[Crush](https://github.com/charmbracelet/crush) to have a way to execute shell
commands with a common dialect on all operating systems. In particular, it's
useful to bring compatibility to Windows where something like Bash not natively
supported.

The limitation is that it does not include the core utilities (`cp`, `mkdir` and
many others), only some built-in functions. On Windows, these are usually not
available in the system.
This project was built specifically to allow including these core utilities on
the shell interpreter, having it all compiled in a single Go binary (your app).

These are same of our needs that other implementations don't fit:

* The coreutils should be importable packages. Standalone `main` packages are
  not useful because we can't import them as libraries.
* Each coreutils package should have the same API design / interface to allow
  reference them without too much glue code.
* The implementation should use an isolated context from the actual application
  (Go app) running it. The working directory, file system, environment
  variables, stdin/stdout/stderr, etc. should be isolated and not shared with
  the actual global state of the Go app or other shell interpreter instances
  running on other goroutines.

## Progress

> [!IMPORTANT]
> This is a working in progress. We'll be breaking compatibility while this is a
v0 and we do progress. We have some progress tracking below.

| Category           | Program      | POSIX | GNU |
| ------------------ | ------------ | ----- | --- |
| Foundational       | `true`       | ✅    | ❌  |
| Foundational       | `false`      | ✅    | ❌  |
| Foundational       | `echo`       | ✅    | ❌  |
| Foundational       | `pwd`        | ✅    | ❌  |
| Foundational       | `basename`   | ✅    | ❌  |
| Foundational       | `dirname`    | ✅    | ❌  |
| Foundational       | `wc`         | ✅    | ❌  |
| Foundational       | `printf`     | ❌    | ❌  |
| Foundational       | `test`       | ❌    | ❌  |
| Foundational       | `expr`       | ❌    | ❌  |
| Foundational       | `sleep`      | ❌    | ❌  |
| Filesystem         | `touch`      | ✅    | ❌  |
| Filesystem         | `chmod`      | ✅    | ❌  |
| Filesystem         | `cat`        | ✅    | ❌  |
| Filesystem         | `head`       | ✅    | ❌  |
| Filesystem         | `tail`       | ✅    | ❌  |
| Filesystem         | `mkdir`      | ✅    | ❌  |
| Filesystem         | `rmdir`      | ✅    | ❌  |
| Filesystem         | `cp`         | ✅    | ❌  |
| Filesystem         | `mv`         | ✅    | ❌  |
| Filesystem         | `rm`         | ✅    | ❌  |
| Filesystem         | `ln`         | ✅    | ❌  |
| Filesystem         | `ls`         | ✅    | ❌  |
| Filesystem         | `chgrp`      | ❌    | ❌  |
| Filesystem         | `chown`      | ❌    | ❌  |
| Filesystem         | `link`       | ❌    | ❌  |
| Filesystem         | `unlink`     | ❌    | ❌  |
| Filesystem         | `mkfifo`     | ❌    | ❌  |
| Filesystem         | `readlink`   | ❌    | ❌  |
| Filesystem         | `realpath`   | ❌    | ❌  |
| Filesystem         | `dd`         | ❌    | ❌  |
| Filesystem         | `df`         | ❌    | ❌  |
| Filesystem         | `du`         | ❌    | ❌  |
| Filesystem         | `pathchk`    | ❌    | ❌  |
| Filesystem         | `pax`        | ❌    | ❌  |
| Text               | `cut`        | ✅    | ❌  |
| Text               | `join`       | ❌    | ❌  |
| Text               | `sort`       | ✅    | ❌  |
| Text               | `paste`      | ✅    | ❌  |
| Text               | `tee`        | ✅    | ❌  |
| Text               | `tr`         | ❌    | ❌  |
| Text               | `uniq`       | ✅    | ❌  |
| Text               | `cmp`        | ✅    | ❌  |
| Text               | `comm`       | ✅    | ❌  |
| Text               | `csplit`     | ❌    | ❌  |
| Text               | `diff`       | ❌    | ❌  |
| Text               | `expand`     | ✅    | ❌  |
| Text               | `fold`       | ✅    | ❌  |
| Text               | `nl`         | ❌    | ❌  |
| Text               | `od`         | ✅    | ❌  |
| Text               | `pr`         | ❌    | ❌  |
| Text               | `sed`        | ❌    | ❌  |
| Text               | `split`      | ✅    | ❌  |
| Text               | `strings`    | ✅    | ❌  |
| Text               | `unexpand`   | ✅    | ❌  |
| Text               | `cksum`      | ❌    | ❌  |
| Text               | `patch`      | ❌    | ❌  |
| Search / Filtering | `find`       | ❌    | ❌  |
| Search / Filtering | `grep`       | ❌    | ❌  |
| Search / Filtering | `xargs`      | ❌    | ❌  |
| System / Process   | `date`       | ❌    | ❌  |
| System / Process   | `env`        | ❌    | ❌  |
| System / Process   | `id`         | ❌    | ❌  |
| System / Process   | `kill`       | ❌    | ❌  |
| System / Process   | `logname`    | ❌    | ❌  |
| System / Process   | `nice`       | ❌    | ❌  |
| System / Process   | `nohup`      | ❌    | ❌  |
| System / Process   | `ps`         | ❌    | ❌  |
| System / Process   | `time`       | ❌    | ❌  |
| System / Process   | `timeout`    | ❌    | ❌  |
| System / Process   | `uname`      | ❌    | ❌  |
| System / Process   | `who`        | ❌    | ❌  |
| Terminal           | `stty`       | ❌    | ❌  |
| Terminal           | `tput`       | ❌    | ❌  |
| Terminal           | `tabs`       | ❌    | ❌  |
| Terminal           | `tty`        | ❌    | ❌  |
| Terminal           | `more`       | ❌    | ❌  |
| Encoding / Archive | `iconv`      | ❌    | ❌  |
| Encoding / Archive | `compress`*  | ❌    | ❌  |
| Encoding / Archive | `uncompress` | ❌    | ❌  |
| Encoding / Archive | `zcat`       | ❌    | ❌  |
| Encoding / Archive | `uudecode`   | ✅    | ❌  |
| Encoding / Archive | `uuencode`   | ✅    | ❌  |
| GNU                | `base32`     | —     | ✅  |
| GNU                | `base64`     | —     | ✅  |
| GNU                | `gzip`       | —     | ✅  |
| GNU                | `gunzip`     | —     | ✅  |
| GNU                | `gzcat`      | —     | ✅  |
| GNU                | `zcat`       | —     | ✅  |
| GNU                | `install`    | —     | ❌  |
| GNU                | `md5sum`     | —     | ✅  |
| GNU                | `mktemp`     | —     | ❌  |
| GNU                | `nproc`      | —     | ❌  |
| GNU                | `printenv`   | —     | ❌  |
| GNU                | `seq`        | —     | ❌  |
| GNU                | `sha1sum`    | —     | ✅  |
| GNU                | `sha224sum`  | —     | ✅  |
| GNU                | `sha256sum`  | —     | ✅  |
| GNU                | `sha384sum`  | —     | ✅  |
| GNU                | `sha512sum`  | —     | ✅  |
| GNU                | `shuf`       | —     | ❌  |
| GNU                | `stat`       | —     | ❌  |
| GNU                | `sync`       | —     | ❌  |
| GNU                | `tac`        | —     | ❌  |
| GNU                | `tar`        | —     | ✅  |
| GNU                | `truncate`   | —     | ❌  |
| GNU                | `which`      | —     | ❌  |
| GNU                | `whoami`     | —     | ❌  |
| GNU                | `yes`        | —     | ❌  |

> [!NOTE]
> `*` The LZW algorithm (the POSIX default, with its `.Z` suffix) is not yet
> implemented for `compress`. Compression with `-g` or `-m gzip`/`-m deflate`
> works. `uncompress` and `zcat` can restore `.gz` files, but `.Z` input is
> unsupported until the LZW decoder is implemented. LZW compression returns the
> specification's unsupported-algorithm exit status (>2). This implementation
> also does not prompt before overwriting on a terminal or check for multiple
> hard links.
>
> `tar` supports the `-c`, `-r`, `-t`, `-u` and `-x` operations and the `-b`,
> `-e`, `-f`, `-h`, `-m`, `-o`, `-p`, `-v` and `-w` options, in both the dashed
> and the old option styles. Member operands of `-t` and `-x` are literal names
> or directory prefixes, not glob patterns; ownership is never changed, making
> `-o` a no-op; `-e` only ignores files which can't be read while an archive is
> created; extraction refuses to write through symbolic links; and the archive
> is the standard input or output (or the name in the `TAPE` environment
> variable) when `-f` is not given.
