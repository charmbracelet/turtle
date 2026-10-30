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

The status icons link to the respective POSIX and GNU documentation, from
the copies committed at `spec/markdown`. Tools whose manuals aren't part of
these copies (like `sed` or `tar`) link to their online documentation instead.

| Category | Program | Description | POSIX | GNU |
| --- | --- | --- | --- | --- |
| Foundational | `true` | Return a zero exit status | [✅](spec/markdown/posix/true.md) | [✅](spec/markdown/gnu/true-invocation.md) |
| Foundational | `false` | Return a non-zero exit status | [✅](spec/markdown/posix/false.md) | [✅](spec/markdown/gnu/false-invocation.md) |
| Foundational | `echo` | Write arguments to standard output | [✅](spec/markdown/posix/echo.md) | [✅](spec/markdown/gnu/echo-invocation.md) |
| Foundational | `pwd` | Print the current working directory | [✅](spec/markdown/posix/pwd.md) | [✅](spec/markdown/gnu/pwd-invocation.md) |
| Foundational | `basename` | Print a path without its directory | [✅](spec/markdown/posix/basename.md) | [✅](spec/markdown/gnu/basename-invocation.md) |
| Foundational | `dirname` | Print the directory part of a path | [✅](spec/markdown/posix/dirname.md) | [✅](spec/markdown/gnu/dirname-invocation.md) |
| Foundational | `wc` | Count lines, words, and bytes | [✅](spec/markdown/posix/wc.md) | [✅](spec/markdown/gnu/wc-invocation.md) |
| Foundational | `printf` | Format and print arguments | [✅](spec/markdown/posix/printf.md) | [✅](spec/markdown/gnu/printf-invocation.md) |
| Foundational | `test` | Evaluate a conditional expression | [✅](spec/markdown/posix/test.md) | [✅](spec/markdown/gnu/test-invocation.md) |
| Foundational | `expr` | Evaluate an expression | [❌](spec/markdown/posix/expr.md) | [❌](spec/markdown/gnu/expr-invocation.md) |
| Foundational | `sleep` | Suspend execution for an interval | [✅](spec/markdown/posix/sleep.md) | [✅](spec/markdown/gnu/sleep-invocation.md) |
| Filesystem | `touch` | Change file access and modification times | [✅](spec/markdown/posix/touch.md) | [✅](spec/markdown/gnu/touch-invocation.md) |
| Filesystem | `chmod` | Change file permission modes | [✅](spec/markdown/posix/chmod.md) | [✅](spec/markdown/gnu/chmod-invocation.md) |
| Filesystem | `cat` | Concatenate and print files | [✅](spec/markdown/posix/cat.md) | [✅](spec/markdown/gnu/cat-invocation.md) |
| Filesystem | `head` | Print the first part of files | [✅](spec/markdown/posix/head.md) | [✅](spec/markdown/gnu/head-invocation.md) |
| Filesystem | `tail` | Print the last part of files | [✅](spec/markdown/posix/tail.md) | [❌](spec/markdown/gnu/tail-invocation.md) |
| Filesystem | `mkdir` | Create directories | [✅](spec/markdown/posix/mkdir.md) | [✅](spec/markdown/gnu/mkdir-invocation.md) |
| Filesystem | `rmdir` | Remove empty directories | [✅](spec/markdown/posix/rmdir.md) | [✅](spec/markdown/gnu/rmdir-invocation.md) |
| Filesystem | `cp`*********** | Copy files and directories | [✅](spec/markdown/posix/cp.md) | [✅](spec/markdown/gnu/cp-invocation.md) |
| Filesystem | `mv` | Move or rename files | [✅](spec/markdown/posix/mv.md) | [✅](spec/markdown/gnu/mv-invocation.md) |
| Filesystem | `rm` | Remove files and directories | [✅](spec/markdown/posix/rm.md) | [✅](spec/markdown/gnu/rm-invocation.md) |
| Filesystem | `ln` | Create links between files | [✅](spec/markdown/posix/ln.md) | [✅](spec/markdown/gnu/ln-invocation.md) |
| Filesystem | `ls` | List directory contents | [✅](spec/markdown/posix/ls.md) | [❌](spec/markdown/gnu/ls-invocation.md) |
| Filesystem | `chgrp` | Change file group ownership | [❌](spec/markdown/posix/chgrp.md) | [❌](spec/markdown/gnu/chgrp-invocation.md) |
| Filesystem | `chown` | Change file ownership | [❌](spec/markdown/posix/chown.md) | [❌](spec/markdown/gnu/chown-invocation.md) |
| Filesystem | `link` | Create a hard link to a file | [✅](spec/markdown/posix/link.md) | [✅](spec/markdown/gnu/link-invocation.md) |
| Filesystem | `unlink` | Remove a directory entry | [✅](spec/markdown/posix/unlink.md) | [✅](spec/markdown/gnu/unlink-invocation.md) |
| Filesystem | `mkfifo` | Create FIFO special files | [❌](spec/markdown/posix/mkfifo.md) | [❌](spec/markdown/gnu/mkfifo-invocation.md) |
| Filesystem | `readlink` | Print the target of a symbolic link | [❌](spec/markdown/posix/readlink.md) | [❌](spec/markdown/gnu/readlink-invocation.md) |
| Filesystem | `realpath` | Print the resolved absolute path | [❌](spec/markdown/posix/realpath.md) | [❌](spec/markdown/gnu/realpath-invocation.md) |
| Filesystem | `dd` | Convert and copy a file | [❌](spec/markdown/posix/dd.md) | [❌](spec/markdown/gnu/dd-invocation.md) |
| Filesystem | `df` | Report file system space usage | [❌](spec/markdown/posix/df.md) | [❌](spec/markdown/gnu/df-invocation.md) |
| Filesystem | `du` | Estimate file space usage | [❌](spec/markdown/posix/du.md) | [❌](spec/markdown/gnu/du-invocation.md) |
| Filesystem | `pathchk` | Check pathnames for validity | [❌](spec/markdown/posix/pathchk.md) | [❌](spec/markdown/gnu/pathchk-invocation.md) |
| Filesystem | `pax` | Read and write file archives | [❌](spec/markdown/posix/pax.md) | ❌ |
| Text | `cut` | Cut out fields from input lines | [✅](spec/markdown/posix/cut.md) | [✅](spec/markdown/gnu/cut-invocation.md) |
| Text | `join`**** | Join lines of two files on a common field | [✅](spec/markdown/posix/join.md) | [✅](spec/markdown/gnu/join-invocation.md) |
| Text | `sort` | Sort lines of text | [✅](spec/markdown/posix/sort.md) | [✅](spec/markdown/gnu/sort-invocation.md) |
| Text | `paste` | Merge lines of files | [✅](spec/markdown/posix/paste.md) | [✅](spec/markdown/gnu/paste-invocation.md) |
| Text | `tee` | Duplicate standard input | [✅](spec/markdown/posix/tee.md) | [❌](spec/markdown/gnu/tee-invocation.md) |
| Text | `tr` | Translate or delete characters | [✅](spec/markdown/posix/tr.md) | [✅](spec/markdown/gnu/tr-invocation.md) |
| Text | `uniq` | Filter adjacent repeated lines | [✅](spec/markdown/posix/uniq.md) | [✅](spec/markdown/gnu/uniq-invocation.md) |
| Text | `cmp` | Compare two files byte by byte | [✅](spec/markdown/posix/cmp.md) | [✅](https://web.archive.org/web/https://www.gnu.org/software/diffutils/manual/html_node/Invoking-cmp.html) |
| Text | `comm` | Compare sorted files line by line | [✅](spec/markdown/posix/comm.md) | [✅](spec/markdown/gnu/comm-invocation.md) |
| Text | `csplit` | Split a file by context | [✅](spec/markdown/posix/csplit.md) | [✅](spec/markdown/gnu/csplit-invocation.md) |
| Text | `diff` | Compare files line by line | [✅](spec/markdown/posix/diff.md) | [❌](https://web.archive.org/web/https://www.gnu.org/software/diffutils/manual/html_node/Invoking-diff.html) |
| Text | `expand` | Convert tabs to spaces | [✅](spec/markdown/posix/expand.md) | [✅](spec/markdown/gnu/expand-invocation.md) |
| Text | `fold` | Wrap long input lines | [✅](spec/markdown/posix/fold.md) | [✅](spec/markdown/gnu/fold-invocation.md) |
| Text | `nl` | Number lines of files | [✅](spec/markdown/posix/nl.md) | [✅](spec/markdown/gnu/nl-invocation.md) |
| Text | `od` | Dump files in octal and other formats | [✅](spec/markdown/posix/od.md) | [❌](spec/markdown/gnu/od-invocation.md) |
| Text | `pr` | Paginate files for printing | [✅](spec/markdown/posix/pr.md) | [❌](spec/markdown/gnu/pr-invocation.md) |
| Text | `sed`****** | Filter and transform text | [✅](spec/markdown/posix/sed.md) | [❌](https://web.archive.org/web/https://www.gnu.org/software/sed/manual/sed.html) |
| Text | `split` | Split a file into pieces | [✅](spec/markdown/posix/split.md) | [✅](spec/markdown/gnu/split-invocation.md) |
| Text | `strings` | Print printable strings in files | [✅](spec/markdown/posix/strings.md) | [✅](https://web.archive.org/web/https://sourceware.org/binutils/docs/binutils/strings.html) |
| Text | `unexpand` | Convert spaces to tabs | [✅](spec/markdown/posix/unexpand.md) | [✅](spec/markdown/gnu/unexpand-invocation.md) |
| Text | `cksum`********** | Compute CRC checksums for files | [✅](spec/markdown/posix/cksum.md) | [✅](spec/markdown/gnu/cksum-invocation.md) |
| Text | `patch`** | Apply a diff file to an original | [✅](spec/markdown/posix/patch.md) | ✅ |
| Search / Filtering | `find` | Find files matching given criteria | [❌](spec/markdown/posix/find.md) | [❌](https://web.archive.org/web/https://www.gnu.org/software/findutils/manual/html_node/find_html/Invoking-find.html) |
| Search / Filtering | `grep` | Print lines matching a pattern | [❌](spec/markdown/posix/grep.md) | [❌](https://web.archive.org/web/https://www.gnu.org/software/grep/manual/grep.html) |
| Search / Filtering | `xargs` | Build and execute command lines | [✅](spec/markdown/posix/xargs.md) | [❌](https://web.archive.org/web/https://www.gnu.org/software/findutils/manual/html_node/find_html/Invoking-xargs.html) |
| System / Process | `date`******* | Print or set the date and time | [✅](spec/markdown/posix/date.md) | [✅](spec/markdown/gnu/date-invocation.md) |
| System / Process | `env` | Run a command in a modified environment | [✅](spec/markdown/posix/env.md) | [✅](spec/markdown/gnu/env-invocation.md) |
| System / Process | `id` | Print user and group IDs | [❌](spec/markdown/posix/id.md) | [❌](spec/markdown/gnu/id-invocation.md) |
| System / Process | `kill` | Send a signal to processes | [❌](spec/markdown/posix/kill.md) | ❌ |
| System / Process | `logname` | Print the user's login name | [❌](spec/markdown/posix/logname.md) | [❌](spec/markdown/gnu/logname-invocation.md) |
| System / Process | `nice` | Run a command with modified priority | [❌](spec/markdown/posix/nice.md) | [❌](spec/markdown/gnu/nice-invocation.md) |
| System / Process | `nohup` | Run a command immune to hangups | [❌](spec/markdown/posix/nohup.md) | [❌](spec/markdown/gnu/nohup-invocation.md) |
| System / Process | `ps` | Report process status | [❌](spec/markdown/posix/ps.md) | ❌ |
| System / Process | `time` | Time the execution of a command | [❌](spec/markdown/posix/time.md) | ❌ |
| System / Process | `timeout` | Run a command with a time limit | [❌](spec/markdown/posix/timeout.md) | [❌](spec/markdown/gnu/timeout-invocation.md) |
| System / Process | `uname` | Print system information | [❌](spec/markdown/posix/uname.md) | [❌](spec/markdown/gnu/uname-invocation.md) |
| System / Process | `who` | Print who is logged in | [❌](spec/markdown/posix/who.md) | [❌](spec/markdown/gnu/who-invocation.md) |
| Terminal | `stty` | Set the options for a terminal | [❌](spec/markdown/posix/stty.md) | [❌](spec/markdown/gnu/stty-invocation.md) |
| Terminal | `tput` | Query terminal capabilities | [❌](spec/markdown/posix/tput.md) | ❌ |
| Terminal | `tabs` | Set terminal tab stops | [❌](spec/markdown/posix/tabs.md) | ❌ |
| Terminal | `tty` | Print the terminal's file name | [❌](spec/markdown/posix/tty.md) | [❌](spec/markdown/gnu/tty-invocation.md) |
| Terminal | `more` | Page through text files | [❌](spec/markdown/posix/more.md) | ❌ |
| Encoding / Archive | `iconv` | Convert text between character encodings | [❌](spec/markdown/posix/iconv.md) | ❌ |
| Encoding / Archive | `compress`* | Compress files | [✅](spec/markdown/posix/compress.md) | ❌ |
| Encoding / Archive | `uncompress` | Decompress files | [✅](spec/markdown/posix/uncompress.md) | ❌ |
| Encoding / Archive | `zcat` | Decompress files to standard output | [✅](spec/markdown/posix/zcat.md) | ❌ |
| Encoding / Archive | `uudecode` | Decode a uuencoded file | [✅](spec/markdown/posix/uudecode.md) | ❌ |
| Encoding / Archive | `uuencode` | Encode a binary file as text | [✅](spec/markdown/posix/uuencode.md) | ❌ |
| GNU | `base32` | Encode and decode base32 data | — | [✅](spec/markdown/gnu/base32-invocation.md) |
| GNU | `base64` | Encode and decode base64 data | — | [✅](spec/markdown/gnu/base64-invocation.md) |
| GNU | `gzip` | Compress files with gzip | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `gunzip` | Decompress gzip files | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `gzcat` | Decompress gzip files to standard output | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `zcat` | Decompress gzip files to standard output | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/gzip/manual/gzip.html) |
| GNU | `install` | Copy files and set attributes | — | [❌](spec/markdown/gnu/install-invocation.md) |
| GNU | `md5sum` | Compute MD5 checksums | — | [✅](spec/markdown/gnu/md5sum-invocation.md) |
| GNU | `mktemp` | Create a temporary file or directory | — | [✅](spec/markdown/gnu/mktemp-invocation.md) |
| GNU | `nproc` | Print the number of processing units | — | [✅](spec/markdown/gnu/nproc-invocation.md) |
| GNU | `printenv` | Print environment variables | — | [✅](spec/markdown/gnu/printenv-invocation.md) |
| GNU | `seq` | Print a sequence of numbers | — | [✅](spec/markdown/gnu/seq-invocation.md) |
| GNU | `sha1sum` | Compute SHA-1 checksums | — | [✅](spec/markdown/gnu/sha1sum-invocation.md) |
| GNU | `sha224sum` | Compute SHA-224 checksums | — | [✅](spec/markdown/gnu/sha2-utilities.md) |
| GNU | `sha256sum` | Compute SHA-256 checksums | — | [✅](spec/markdown/gnu/sha2-utilities.md) |
| GNU | `sha384sum` | Compute SHA-384 checksums | — | [✅](spec/markdown/gnu/sha2-utilities.md) |
| GNU | `sha512sum` | Compute SHA-512 checksums | — | [✅](spec/markdown/gnu/sha2-utilities.md) |
| GNU | `shasum`*** | Print or check SHA checksums | — | [✅](https://perldoc.perl.org/shasum) |
| GNU | `shuf` | Generate random permutations | — | [❌](spec/markdown/gnu/shuf-invocation.md) |
| GNU | `stat` | Print file and file system status | — | [❌](spec/markdown/gnu/stat-invocation.md) |
| GNU | `sync`******** | Flush file system buffers | — | [✅](spec/markdown/gnu/sync-invocation.md) |
| GNU | `tac` | Concatenate and print files in reverse | — | [✅](spec/markdown/gnu/tac-invocation.md) |
| GNU | `tar`***** | Create and extract file archives | — | [✅](https://web.archive.org/web/https://www.gnu.org/software/tar/manual/tar.html) |
| GNU | `truncate` | Shrink or extend the size of files | — | [✅](spec/markdown/gnu/truncate-invocation.md) |
| GNU | `which` | Locate a command in the search path | — | ❌ |
| GNU | `whoami`********* | Print the effective user name | — | [✅](spec/markdown/gnu/whoami-invocation.md) |
| GNU | `yes` | Print a string repeatedly | — | [✅](spec/markdown/gnu/yes-invocation.md) |

> [!NOTE]
> `*` The LZW algorithm (the POSIX default, with its `.Z` suffix) is not yet
> implemented for `compress`. Compression with `-g` or `-m gzip`/`-m deflate`
> works. `uncompress` and `zcat` can restore `.gz` files, but `.Z` input is
> unsupported until the LZW decoder is implemented. LZW compression returns the
> specification's unsupported-algorithm exit status (>2). This implementation
> also does not prompt before overwriting on a terminal or check for multiple
> hard links.
>
> `**` `patch` applies unified and context diffs with all the POSIX options
> except `-D`, `-e` and `-n`, which wait for `#ifdef` marks and the ed and
> normal diff formats. It also deviates from GNU and BSD in two ways POSIX
> allows: informative messages go to standard error instead of standard
> output, and already applied hunks are ignored instead of saved to the
> reject file.
>
> `***` `shasum` follows the Perl script of the `Digest::SHA` distribution, but it
> doesn't take its `-U` (Universal Newlines) and `-0` (BITS) modes. When
> verifying, it hashes with the algorithm `-a` selects, instead of the one
> each checksum line implies.
>
> `****` `join` compares its join fields in the bytewise collating sequence of
> the C locale, the one the `sort` and `comm` of this project use: no
> utility here implements the collating sequences of other locales, so
> keys which only another `LC_COLLATE` makes equal stay different.
>
> `*****` `tar` supports the `-c`, `-r`, `-t`, `-u` and `-x` operations and the `-b`,
> `-e`, `-f`, `-h`, `-m`, `-o`, `-p`, `-v` and `-w` options, in both the dashed
> and the old option styles. Member operands of `-t` and `-x` are literal names
> or directory prefixes, not glob patterns; ownership is never changed, making
> `-o` a no-op; `-e` only ignores files which can't be read while an archive is
> created; extraction refuses to write through symbolic links; and the archive
> is the standard input or output (or the name in the `TAPE` environment
> variable) when `-f` is not given.
>
> `******` `sed` supports BRE and ERE (`-E`) patterns, but not backreferences
> (`\1` to `\9`) in patterns, nor the collating symbols and equivalence
> classes of bracket expressions. Backreferences in the replacement of `s`
> work as specified.
>
> `*******` `date` can't set the system date: its settime operand
> (`mmddhhmm[[cc]yy]`) is parsed to diagnose invalid dates, but setting the
> clock fails, since the runtime exposes a clock to read but none to set.
> The GNU `-r` (`--reference`) option, which displays the modification time
> of a file, is also supported.
>
> `********` `sync` flushes through the runtime file system, so the whole
> system is only flushed when the FS maps the real one, and the `-d` and
> `-f` flags degrade to the closest operation the platform offers.
>
> `*********` `whoami` takes the name from the `USER` or `LOGNAME`
> environment variables, since the isolated runtime exposes no user
> database.
>
> `**********` `cksum` takes its algorithms from the standard library, so
> `blake2b` and `sm3` are missing, and it doesn't verify checksums with
> `-c`.
>
> `***********` `cp` doesn't implement the backup options (`-b`, `-S`),
> `-l`, `-s`, `-x` or `--parents`, and `--update=WHEN` takes only the plain
> `-u` form.
