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

The status icons link to the respective POSIX and GNU documentation pages.

| Category | Program | Description | POSIX | GNU |
| --- | --- | --- | --- | --- |
| Foundational | `true` | Return a zero exit status | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/true.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/true-invocation.html) |
| Foundational | `false` | Return a non-zero exit status | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/false.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/false-invocation.html) |
| Foundational | `echo` | Write arguments to standard output | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/echo.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/echo-invocation.html) |
| Foundational | `pwd` | Print the current working directory | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/pwd.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/pwd-invocation.html) |
| Foundational | `basename` | Print a path without its directory | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/basename.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/basename-invocation.html) |
| Foundational | `dirname` | Print the directory part of a path | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/dirname.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/dirname-invocation.html) |
| Foundational | `wc` | Count lines, words, and bytes | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/wc.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/wc-invocation.html) |
| Foundational | `printf` | Format and print arguments | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/printf.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/printf-invocation.html) |
| Foundational | `test` | Evaluate a conditional expression | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/test.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/test-invocation.html) |
| Foundational | `expr` | Evaluate an expression | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/expr.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/expr-invocation.html) |
| Foundational | `sleep` | Suspend execution for an interval | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/sleep.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sleep-invocation.html) |
| Filesystem | `touch` | Change file access and modification times | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/touch.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/touch-invocation.html) |
| Filesystem | `chmod` | Change file permission modes | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/chmod.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/chmod-invocation.html) |
| Filesystem | `cat` | Concatenate and print files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/cat.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/cat-invocation.html) |
| Filesystem | `head` | Print the first part of files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/head.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/head-invocation.html) |
| Filesystem | `tail` | Print the last part of files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/tail.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/tail-invocation.html) |
| Filesystem | `mkdir` | Create directories | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/mkdir.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/mkdir-invocation.html) |
| Filesystem | `rmdir` | Remove empty directories | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/rmdir.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/rmdir-invocation.html) |
| Filesystem | `cp` | Copy files and directories | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/cp.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/cp-invocation.html) |
| Filesystem | `mv` | Move or rename files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/mv.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/mv-invocation.html) |
| Filesystem | `rm` | Remove files and directories | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/rm.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/rm-invocation.html) |
| Filesystem | `ln` | Create links between files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/ln.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/ln-invocation.html) |
| Filesystem | `ls` | List directory contents | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/ls.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/ls-invocation.html) |
| Filesystem | `chgrp` | Change file group ownership | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/chgrp.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/chgrp-invocation.html) |
| Filesystem | `chown` | Change file ownership | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/chown.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/chown-invocation.html) |
| Filesystem | `link` | Create a hard link to a file | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/link.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/link-invocation.html) |
| Filesystem | `unlink` | Remove a directory entry | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/unlink.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/unlink-invocation.html) |
| Filesystem | `mkfifo` | Create FIFO special files | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/mkfifo.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/mkfifo-invocation.html) |
| Filesystem | `readlink` | Print the target of a symbolic link | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/readlink.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/readlink-invocation.html) |
| Filesystem | `realpath` | Print the resolved absolute path | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/realpath.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/realpath-invocation.html) |
| Filesystem | `dd` | Convert and copy a file | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/dd.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/dd-invocation.html) |
| Filesystem | `df` | Report file system space usage | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/df.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/df-invocation.html) |
| Filesystem | `du` | Estimate file space usage | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/du.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/du-invocation.html) |
| Filesystem | `pathchk` | Check pathnames for validity | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/pathchk.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/pathchk-invocation.html) |
| Filesystem | `pax` | Read and write file archives | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/pax.html) | ❌ |
| Text | `cut` | Cut out fields from input lines | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/cut.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/cut-invocation.html) |
| Text | `join` | Join lines of two files on a common field | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/join.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/join-invocation.html) |
| Text | `sort` | Sort lines of text | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/sort.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sort-invocation.html) |
| Text | `paste` | Merge lines of files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/paste.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/paste-invocation.html) |
| Text | `tee` | Duplicate standard input | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/tee.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/tee-invocation.html) |
| Text | `tr` | Translate or delete characters | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/tr.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/tr-invocation.html) |
| Text | `uniq` | Filter adjacent repeated lines | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/uniq.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/uniq-invocation.html) |
| Text | `cmp` | Compare two files byte by byte | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/cmp.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/diffutils/manual/html_node/Invoking-cmp.html) |
| Text | `comm` | Compare sorted files line by line | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/comm.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/comm-invocation.html) |
| Text | `csplit` | Split a file by context | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/csplit.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/csplit-invocation.html) |
| Text | `diff` | Compare files line by line | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/diff.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/diffutils/manual/html_node/Invoking-diff.html) |
| Text | `expand` | Convert tabs to spaces | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/expand.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/expand-invocation.html) |
| Text | `fold` | Wrap long input lines | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/fold.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/fold-invocation.html) |
| Text | `nl` | Number lines of files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/nl.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/nl-invocation.html) |
| Text | `od` | Dump files in octal and other formats | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/od.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/od-invocation.html) |
| Text | `pr` | Paginate files for printing | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/pr.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/pr-invocation.html) |
| Text | `sed` | Filter and transform text | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/sed.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/sed/manual/sed.html) |
| Text | `split` | Split a file into pieces | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/split.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/split-invocation.html) |
| Text | `strings` | Print printable strings in files | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/strings.html) | [❌](https://web.archive.org/web/https://sourceware.org/binutils/docs/binutils/strings.html) |
| Text | `unexpand` | Convert spaces to tabs | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/unexpand.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/unexpand-invocation.html) |
| Text | `cksum` | Compute CRC checksums for files | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/cksum.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/cksum-invocation.html) |
| Text | `patch`* | Apply a diff file to an original | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/patch.html) | ❌ |
| Search / Filtering | `find` | Find files matching given criteria | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/find.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/findutils/manual/html_node/find_html/Invoking-find.html) |
| Search / Filtering | `grep` | Print lines matching a pattern | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/grep.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/grep/manual/grep.html) |
| Search / Filtering | `xargs` | Build and execute command lines | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/xargs.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/findutils/manual/html_node/find_html/Invoking-xargs.html) |
| System / Process | `date` | Print or set the date and time | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/date.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/date-invocation.html) |
| System / Process | `env` | Run a command in a modified environment | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/env.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/env-invocation.html) |
| System / Process | `id` | Print user and group IDs | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/id.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/id-invocation.html) |
| System / Process | `kill` | Send a signal to processes | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/kill.html) | ❌ |
| System / Process | `logname` | Print the user's login name | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/logname.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/logname-invocation.html) |
| System / Process | `nice` | Run a command with modified priority | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/nice.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/nice-invocation.html) |
| System / Process | `nohup` | Run a command immune to hangups | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/nohup.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/nohup-invocation.html) |
| System / Process | `ps` | Report process status | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/ps.html) | ❌ |
| System / Process | `time` | Time the execution of a command | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/time.html) | ❌ |
| System / Process | `timeout` | Run a command with a time limit | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/timeout.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/timeout-invocation.html) |
| System / Process | `uname` | Print system information | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/uname.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/uname-invocation.html) |
| System / Process | `who` | Print who is logged in | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/who.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/who-invocation.html) |
| Terminal | `stty` | Set the options for a terminal | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/stty.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/stty-invocation.html) |
| Terminal | `tput` | Query terminal capabilities | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/tput.html) | ❌ |
| Terminal | `tabs` | Set terminal tab stops | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/tabs.html) | ❌ |
| Terminal | `tty` | Print the terminal's file name | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/tty.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/tty-invocation.html) |
| Terminal | `more` | Page through text files | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/more.html) | ❌ |
| Encoding / Archive | `iconv` | Convert text between character encodings | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/iconv.html) | ❌ |
| Encoding / Archive | `compress`* | Compress files | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/compress.html) | ❌ |
| Encoding / Archive | `uncompress` | Decompress files | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/uncompress.html) | ❌ |
| Encoding / Archive | `zcat` | Decompress files to standard output | [❌](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/zcat.html) | [❌](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| Encoding / Archive | `uudecode` | Decode a uuencoded file | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/uudecode.html) | ❌ |
| Encoding / Archive | `uuencode` | Encode a binary file as text | [✅](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/uuencode.html) | ❌ |
| GNU | `base32` | Encode and decode base32 data | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/base32-invocation.html) |
| GNU | `base64` | Encode and decode base64 data | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/base64-invocation.html) |
| GNU | `gzip` | Compress files with gzip | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `gunzip` | Decompress gzip files | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `gzcat` | Decompress gzip files to standard output | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `zcat` | Decompress gzip files to standard output | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `install` | Copy files and set attributes | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/install-invocation.html) |
| GNU | `md5sum` | Compute MD5 checksums | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/md5sum-invocation.html) |
| GNU | `mktemp` | Create a temporary file or directory | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/mktemp-invocation.html) |
| GNU | `nproc` | Print the number of processing units | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/nproc-invocation.html) |
| GNU | `printenv` | Print environment variables | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/printenv-invocation.html) |
| GNU | `seq` | Print a sequence of numbers | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/seq-invocation.html) |
| GNU | `sha1sum` | Compute SHA-1 checksums | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sha1sum-invocation.html) |
| GNU | `sha224sum` | Compute SHA-224 checksums | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sha2-utilities.html) |
| GNU | `sha256sum` | Compute SHA-256 checksums | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sha2-utilities.html) |
| GNU | `sha384sum` | Compute SHA-384 checksums | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sha2-utilities.html) |
| GNU | `sha512sum` | Compute SHA-512 checksums | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sha2-utilities.html) |
| GNU | `shuf` | Generate random permutations | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/shuf-invocation.html) |
| GNU | `stat` | Print file and file system status | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/stat-invocation.html) |
| GNU | `sync` | Flush file system buffers | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/sync-invocation.html) |
| GNU | `tac` | Concatenate and print files in reverse | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/tac-invocation.html) |
| GNU | `tar` | Create and extract file archives | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/tar/manual/tar.html) |
| GNU | `truncate` | Shrink or extend the size of files | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/truncate-invocation.html) |
| GNU | `which` | Locate a command in the search path | — | ❌ |
| GNU | `whoami` | Print the effective user name | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/whoami-invocation.html) |
| GNU | `yes` | Print a string repeatedly | — | [❌](https://web.archive.org/web/https://www.gnu.org/software/coreutils/manual/html_node/yes-invocation.html) |

> [!NOTE]
> `*` The LZW algorithm (the POSIX default, with its `.Z` suffix) is not yet
> implemented for `compress`. Compression with `-g` or `-m gzip`/`-m deflate`
> works. `uncompress` and `zcat` can restore `.gz` files, but `.Z` input is
> unsupported until the LZW decoder is implemented. LZW compression returns the
> specification's unsupported-algorithm exit status (>2). This implementation
> also does not prompt before overwriting on a terminal or check for multiple
> hard links.
>
> `patch` applies unified and context diffs with all the POSIX options
> except `-D`, `-e` and `-n`, which wait for `#ifdef` marks and the ed and
> normal diff formats. It also deviates from GNU and BSD in two ways POSIX
> allows: informative messages go to standard error instead of standard
> output, and already applied hunks are ignored instead of saved to the
> reject file.
>
> `tar` supports the `-c`, `-r`, `-t`, `-u` and `-x` operations and the `-b`,
> `-e`, `-f`, `-h`, `-m`, `-o`, `-p`, `-v` and `-w` options, in both the dashed
> and the old option styles. Member operands of `-t` and `-x` are literal names
> or directory prefixes, not glob patterns; ownership is never changed, making
> `-o` a no-op; `-e` only ignores files which can't be read while an archive is
> created; extraction refuses to write through symbolic links; and the archive
> is the standard input or output (or the name in the `TAPE` environment
> variable) when `-f` is not given.
