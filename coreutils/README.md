# charm.land/turtle/coreutils

A Go implementation of the Unix core utilities.

Utility implementations live in `programs/`, with their standalone command
wrappers in `cmd/`. For example, `cat` is imported as
`charm.land/turtle/coreutils/programs/cat`.

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

The status icons link to the respective POSIX and GNU documentation, from
the copies committed at `spec/posix` and `spec/gnu`. Tools whose manuals aren't part of
these copies (like `sed` or `tar`) link to their online documentation instead.

Every utility honors the GNU common options `--help` and `--version`, and
its usage errors are followed by the `Try 'prog --help' for more information.`
hint. The exceptions are `echo`, `printf` and `test`, whose specifications
forbid options, and `false`, which always exits with status 1, even when
invoked with them.

| Category | Program | Description | POSIX | GNU |
| --- | --- | --- | --- | --- |
| Foundational | `true` | Return a zero exit status | [✅](spec/posix/true.md) | [✅](spec/gnu/true-invocation.md) |
| Foundational | `false` | Return a non-zero exit status | [✅](spec/posix/false.md) | [✅](spec/gnu/false-invocation.md) |
| Foundational | `echo` | Write arguments to standard output | [✅](spec/posix/echo.md) | [✅](spec/gnu/echo-invocation.md) |
| Foundational | `pwd` | Print the current working directory | [✅](spec/posix/pwd.md) | [✅](spec/gnu/pwd-invocation.md) |
| Foundational | `basename` | Print a path without its directory | [✅](spec/posix/basename.md) | [✅](spec/gnu/basename-invocation.md) |
| Foundational | `dirname` | Print the directory part of a path | [✅](spec/posix/dirname.md) | [✅](spec/gnu/dirname-invocation.md) |
| Foundational | `wc` | Count lines, words, and bytes | [✅](spec/posix/wc.md) | [✅](spec/gnu/wc-invocation.md) |
| Foundational | `printf` | Format and print arguments | [✅](spec/posix/printf.md) | [✅](spec/gnu/printf-invocation.md) |
| Foundational | `test` | Evaluate a conditional expression | [✅](spec/posix/test.md) | [✅](spec/gnu/test-invocation.md) |
| Foundational | `expr`* | Evaluate an expression | [✅](spec/posix/expr.md) | [✅](spec/gnu/expr-invocation.md) |
| Foundational | `sleep` | Suspend execution for an interval | [✅](spec/posix/sleep.md) | [✅](spec/gnu/sleep-invocation.md) |
| Filesystem | `touch` | Change file access and modification times | [✅](spec/posix/touch.md) | [✅](spec/gnu/touch-invocation.md) |
| Filesystem | `chmod` | Change file permission modes | [✅](spec/posix/chmod.md) | [✅](spec/gnu/chmod-invocation.md) |
| Filesystem | `cat` | Concatenate and print files | [✅](spec/posix/cat.md) | [✅](spec/gnu/cat-invocation.md) |
| Filesystem | `head` | Print the first part of files | [✅](spec/posix/head.md) | [✅](spec/gnu/head-invocation.md) |
| Filesystem | `tail` | Print the last part of files | [✅](spec/posix/tail.md) | [✅](spec/gnu/tail-invocation.md) |
| Filesystem | `mkdir` | Create directories | [✅](spec/posix/mkdir.md) | [✅](spec/gnu/mkdir-invocation.md) |
| Filesystem | `rmdir` | Remove empty directories | [✅](spec/posix/rmdir.md) | [✅](spec/gnu/rmdir-invocation.md) |
| Filesystem | `cp`* | Copy files and directories | [✅](spec/posix/cp.md) | [✅](spec/gnu/cp-invocation.md) |
| Filesystem | `mv` | Move or rename files | [✅](spec/posix/mv.md) | [✅](spec/gnu/mv-invocation.md) |
| Filesystem | `rm` | Remove files and directories | [✅](spec/posix/rm.md) | [✅](spec/gnu/rm-invocation.md) |
| Filesystem | `ln` | Create links between files | [✅](spec/posix/ln.md) | [✅](spec/gnu/ln-invocation.md) |
| Filesystem | `ls`* | List directory contents | [✅](spec/posix/ls.md) | [✅](spec/gnu/ls-invocation.md) |
| Filesystem | `chgrp`* | Change file group ownership | [✅](spec/posix/chgrp.md) | [✅](spec/gnu/chgrp-invocation.md) |
| Filesystem | `chown`* | Change file ownership | [✅](spec/posix/chown.md) | [✅](spec/gnu/chown-invocation.md) |
| Filesystem | `link` | Create a hard link to a file | [✅](spec/posix/link.md) | [✅](spec/gnu/link-invocation.md) |
| Filesystem | `unlink` | Remove a directory entry | [✅](spec/posix/unlink.md) | [✅](spec/gnu/unlink-invocation.md) |
| Filesystem | `mkfifo` | Create FIFO special files | [❌](spec/posix/mkfifo.md) | [❌](spec/gnu/mkfifo-invocation.md) |
| Filesystem | `readlink`* | Print the target of a symbolic link | [✅](spec/posix/readlink.md) | [✅](spec/gnu/readlink-invocation.md) |
| Filesystem | `realpath`* | Print the resolved absolute path | [✅](spec/posix/realpath.md) | [✅](spec/gnu/realpath-invocation.md) |
| Filesystem | `dd`* | Convert and copy a file | [✅](spec/posix/dd.md) | [✅](spec/gnu/dd-invocation.md) |
| Filesystem | `df` | Report file system space usage | [❌](spec/posix/df.md) | [❌](spec/gnu/df-invocation.md) |
| Filesystem | `du`* | Estimate file space usage | [✅](spec/posix/du.md) | [✅](spec/gnu/du-invocation.md) |
| Filesystem | `pathchk` | Check pathnames for validity | [❌](spec/posix/pathchk.md) | [❌](spec/gnu/pathchk-invocation.md) |
| Filesystem | `pax` | Read and write file archives | [❌](spec/posix/pax.md) | ❌ |
| Text | `cut` | Cut out fields from input lines | [✅](spec/posix/cut.md) | [✅](spec/gnu/cut-invocation.md) |
| Text | `join`* | Join lines of two files on a common field | [✅](spec/posix/join.md) | [✅](spec/gnu/join-invocation.md) |
| Text | `sort` | Sort lines of text | [✅](spec/posix/sort.md) | [✅](spec/gnu/sort-invocation.md) |
| Text | `paste` | Merge lines of files | [✅](spec/posix/paste.md) | [✅](spec/gnu/paste-invocation.md) |
| Text | `tee`* | Duplicate standard input | [✅](spec/posix/tee.md) | [✅](spec/gnu/tee-invocation.md) |
| Text | `tr` | Translate or delete characters | [✅](spec/posix/tr.md) | [✅](spec/gnu/tr-invocation.md) |
| Text | `uniq` | Filter adjacent repeated lines | [✅](spec/posix/uniq.md) | [✅](spec/gnu/uniq-invocation.md) |
| Text | `cmp`* | Compare two files byte by byte | [✅](spec/posix/cmp.md) | [✅](https://web.archive.org/web/https://www.gnu.org/software/diffutils/manual/html_node/Invoking-cmp.html) |
| Text | `comm` | Compare sorted files line by line | [✅](spec/posix/comm.md) | [✅](spec/gnu/comm-invocation.md) |
| Text | `csplit` | Split a file by context | [✅](spec/posix/csplit.md) | [✅](spec/gnu/csplit-invocation.md) |
| Text | `diff`* | Compare files line by line | [✅](spec/posix/diff.md) | [✅](https://web.archive.org/web/https://www.gnu.org/software/diffutils/manual/html_node/Invoking-diff.html) |
| Text | `expand` | Convert tabs to spaces | [✅](spec/posix/expand.md) | [✅](spec/gnu/expand-invocation.md) |
| Text | `fold` | Wrap long input lines | [✅](spec/posix/fold.md) | [✅](spec/gnu/fold-invocation.md) |
| Text | `nl` | Number lines of files | [✅](spec/posix/nl.md) | [✅](spec/gnu/nl-invocation.md) |
| Text | `od`* | Dump files in octal and other formats | [✅](spec/posix/od.md) | [✅](spec/gnu/od-invocation.md) |
| Text | `pr`* | Paginate files for printing | [✅](spec/posix/pr.md) | [✅](spec/gnu/pr-invocation.md) |
| Text | `sed`* | Filter and transform text | [✅](spec/posix/sed.md) | [✅](https://web.archive.org/web/https://www.gnu.org/software/sed/manual/sed.html) |
| Text | `split` | Split a file into pieces | [✅](spec/posix/split.md) | [✅](spec/gnu/split-invocation.md) |
| Text | `strings` | Print printable strings in files | [✅](spec/posix/strings.md) | [✅](https://web.archive.org/web/https://sourceware.org/binutils/docs/binutils/strings.html) |
| Text | `unexpand` | Convert spaces to tabs | [✅](spec/posix/unexpand.md) | [✅](spec/gnu/unexpand-invocation.md) |
| Text | `cksum`* | Compute CRC checksums for files | [✅](spec/posix/cksum.md) | [✅](spec/gnu/cksum-invocation.md) |
| Text | `patch`* | Apply a diff file to an original | [✅](spec/posix/patch.md) | ✅ |
| Search / Filtering | `find`* | Find files matching given criteria | [✅](spec/posix/find.md) | [✅](https://web.archive.org/web/https://www.gnu.org/software/findutils/manual/html_node/find_html/Invoking-find.html) |
| Search / Filtering | `grep`* | Print lines matching a pattern | [✅](spec/posix/grep.md) | [✅](https://www.gnu.org/software/grep/manual/grep.html) |
| Search / Filtering | `xargs` | Build and execute command lines | [✅](spec/posix/xargs.md) | [❌](https://web.archive.org/web/https://www.gnu.org/software/findutils/manual/html_node/find_html/Invoking-xargs.html) |
| System / Process | `date`* | Print or set the date and time | [✅](spec/posix/date.md) | [✅](spec/gnu/date-invocation.md) |
| System / Process | `env` | Run a command in a modified environment | [✅](spec/posix/env.md) | [✅](spec/gnu/env-invocation.md) |
| System / Process | `id`* | Print user and group IDs | [✅](spec/posix/id.md) | [✅](spec/gnu/id-invocation.md) |
| System / Process | `kill` | Send a signal to processes | [❌](spec/posix/kill.md) | ❌ |
| System / Process | `logname`* | Print the user's login name | [✅](spec/posix/logname.md) | [✅](spec/gnu/logname-invocation.md) |
| System / Process | `nice`* | Run a command with modified priority | [✅](spec/posix/nice.md) | [✅](spec/gnu/nice-invocation.md) |
| System / Process | `nohup`* | Run a command immune to hangups | [✅](spec/posix/nohup.md) | [✅](spec/gnu/nohup-invocation.md) |
| System / Process | `ps` | Report process status | [❌](spec/posix/ps.md) | ❌ |
| System / Process | `time`* | Time the execution of a command | [✅](spec/posix/time.md) | [✅](https://www.gnu.org/software/time/manual/time.html) |
| System / Process | `timeout`* | Run a command with a time limit | [✅](spec/posix/timeout.md) | [✅](spec/gnu/timeout-invocation.md) |
| System / Process | `uname`* | Print system information | [✅](spec/posix/uname.md) | [✅](spec/gnu/uname-invocation.md) |
| System / Process | `who` | Print who is logged in | [❌](spec/posix/who.md) | [❌](spec/gnu/who-invocation.md) |
| Terminal | `stty` | Set the options for a terminal | [❌](spec/posix/stty.md) | [❌](spec/gnu/stty-invocation.md) |
| Terminal | `tput` | Query terminal capabilities | [❌](spec/posix/tput.md) | ❌ |
| Terminal | `tabs` | Set terminal tab stops | [❌](spec/posix/tabs.md) | ❌ |
| Terminal | `tty` | Print the terminal's file name | [❌](spec/posix/tty.md) | [❌](spec/gnu/tty-invocation.md) |
| Terminal | `more` | Page through text files | [❌](spec/posix/more.md) | ❌ |
| Encoding / Archive | `iconv` | Convert text between character encodings | [❌](spec/posix/iconv.md) | ❌ |
| Encoding / Archive | `compress`* | Compress files | [✅](spec/posix/compress.md) | ❌ |
| Encoding / Archive | `uncompress` | Decompress files | [✅](spec/posix/uncompress.md) | ❌ |
| Encoding / Archive | `zcat` | Decompress files to standard output | [✅](spec/posix/zcat.md) | ❌ |
| Encoding / Archive | `uudecode` | Decode a uuencoded file | [✅](spec/posix/uudecode.md) | ❌ |
| Encoding / Archive | `uuencode` | Encode a binary file as text | [✅](spec/posix/uuencode.md) | ❌ |
| GNU | `base32` | Encode and decode base32 data | — | [✅](spec/gnu/base32-invocation.md) |
| GNU | `base64` | Encode and decode base64 data | — | [✅](spec/gnu/base64-invocation.md) |
| GNU | `gzip` | Compress files with gzip | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `gunzip` | Decompress gzip files | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `gzcat` | Decompress gzip files to standard output | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `install` | Copy files and set attributes | — | [✅](spec/gnu/install-invocation.md) |
| GNU | `md5sum` | Compute MD5 checksums | — | [✅](spec/gnu/md5sum-invocation.md) |
| GNU | `mktemp` | Create a temporary file or directory | — | [✅](spec/gnu/mktemp-invocation.md) |
| GNU | `nproc` | Print the number of processing units | — | [✅](spec/gnu/nproc-invocation.md) |
| GNU | `printenv` | Print environment variables | — | [✅](spec/gnu/printenv-invocation.md) |
| GNU | `seq` | Print a sequence of numbers | — | [✅](spec/gnu/seq-invocation.md) |
| GNU | `sha1sum` | Compute SHA-1 checksums | — | [✅](spec/gnu/sha1sum-invocation.md) |
| GNU | `sha224sum` | Compute SHA-224 checksums | — | [✅](spec/gnu/sha2-utilities.md) |
| GNU | `sha256sum` | Compute SHA-256 checksums | — | [✅](spec/gnu/sha2-utilities.md) |
| GNU | `sha384sum` | Compute SHA-384 checksums | — | [✅](spec/gnu/sha2-utilities.md) |
| GNU | `sha512sum` | Compute SHA-512 checksums | — | [✅](spec/gnu/sha2-utilities.md) |
| GNU | `shasum`* | Print or check SHA checksums | — | [✅](https://perldoc.perl.org/shasum) |
| GNU | `shuf`* | Generate random permutations | — | [✅](spec/gnu/shuf-invocation.md) |
| GNU | `stat`* | Print file and file system status | — | [✅](spec/gnu/stat-invocation.md) |
| GNU | `sync`* | Flush file system buffers | — | [✅](spec/gnu/sync-invocation.md) |
| GNU | `tac` | Concatenate and print files in reverse | — | [✅](spec/gnu/tac-invocation.md) |
| GNU | `tar`* | Create and extract file archives | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/tar/manual/tar.html) |
| GNU | `truncate` | Shrink or extend the size of files | — | [✅](spec/gnu/truncate-invocation.md) |
| GNU | `which`* | Locate a command in the search path | — | [✅](https://www.gnu.org/software/which/manual/) |
| GNU | `whoami`* | Print the effective user name | — | [✅](spec/gnu/whoami-invocation.md) |
| GNU | `yes` | Print a string repeatedly | — | [✅](spec/gnu/yes-invocation.md) |

A `*` on a program marks limitations and deviations from the specifications, described in the [Deviations](DEVIATIONS.md) file.
