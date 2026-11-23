# Deviations

### chgrp

* Reports the changed and retained groups by their numeric identifiers in
  the `-v` and `-c` messages, where GNU uses the names.

### chgrp and chown

* Leave the symbolic links their `-h` option and the `-P` recursion act on
  alone instead of changing their ownership: the file system interface has
  no link-aware chown, and the one it exposes follows links, so changing
  the link would change its referent.
* On the systems without an ownership model, like Windows, a change the
  system cannot represent succeeds without effect, as with `install`. The
  `setuid` and `setgid` bits are cleared by the file system when it does
  so, not by the utilities.

### chown

* Writes only the errno line when a change fails under `-v` and `-c`, where
  GNU adds a line naming the previous and wanted owner; its group-only
  messages omit the colon GNU puts before the group; a numeric owner the
  user database doesn't know is applied without diagnosing, where GNU
  errors; and usage errors exit 2, like `chmod` exits them here, where GNU
  exits 1.

### cksum

* Takes its algorithms from the standard library, so `blake2b` and `sm3`
  are missing, and it doesn't verify checksums with `-c`.

### cmp

* The first-difference message says `byte`, as GNU does outside the POSIX
  locale, instead of the POSIX `char`, and with `-b` the byte values follow
  GNU's `%3o` and `%-4s` fields while the byte numbers stay unpadded, as
  POSIX formats them.
* Count values take the multiplicative suffixes of GNU cmp in its exact
  case, so the `b`, `c`, `w`, `R` and `Q` multipliers other utilities
  accept are refused.
* Options after the first operand are operands, as the shared option parser
  stops there, and not permuted as GNU's does; the obsolescent `-c` alias
  of `-b` is not implemented.
* Comparing the standard input with itself is refused with status 2, as BSD
  cmp does, where GNU reports the two inputs as identical without reading
  them; POSIX leaves this case undefined.

### compress

* Doesn't implement the LZW algorithm (the POSIX default, with its `.Z`
  suffix) yet. Compression with `-g` or `-m gzip`/`-m deflate` works.
* `uncompress` and `zcat` can restore `.gz` files, but `.Z` input is
  unsupported until the LZW decoder is implemented. LZW compression returns
  the specification's unsupported-algorithm exit status (>2).
* Doesn't prompt before overwriting on a terminal or check for multiple
  hard links.

### cp

* Doesn't implement the backup options (`-b`, `-S`), `-l`, `-s`, `-x` or
  `--parents`, and `--update=WHEN` takes only the plain `-u` form.

### date

* Can't set the system date: its settime operand (`mmddhhmm[[cc]yy]`) is
  parsed to diagnose invalid dates, but setting the clock fails, since the
  runtime exposes a clock to read but none to set.
* The GNU `-r` (`--reference`) option, which displays the modification time
  of a file, is also supported.

### dd

* Implements the conversions and operands of the POSIX specification, plus
  the GNU `status=` levels, the `fullblock` and `append` flags and the
  `excl`, `nocreat` and `fsync` conversions.
* Doesn't implement `conv=sparse` or `fdatasync`, and rejects the other
  `iflag`/`oflag` flags as invalid, like GNU does where it doesn't support
  them.
* Prints no transfer-rate line: `status=noxfer` behaves like the default,
  and `status=progress` reports the record counts once a second.
* Since the file system interface has no truncate, a `seek` past the
  output's end without `conv=notrunc` extends the file by reading,
  truncating and rewriting its null-filled prefix, and a non-seekable
  output with `seek` is null-filled.
* Block sizes are capped at 2^31-1.

### diff

* Compares files line by line with the POSIX options and the GNU output
  formats: normal, context, unified, ed, forward-ed, RCS, merged `#ifdef`,
  side-by-side and brief.
* Left out are `-l` pagination, `--color`/`--palette`, `-d`/`--minimal`,
  `-H`, `-S`, `-p`/`-F` function headings, the
  `--old-line-format`/`--old-group-format` family,
  `--ignore-file-name-case`, `--no-dereference`, `--suppress-blank-empty`
  and `--horizon-lines`.
* Repeated `-c`, `-C`, `-U`, `--context` and `--unified` keep the last
  count given where GNU keeps the largest; the `-I` patterns are Go regular
  expressions rather than POSIX ones; `-x` patterns use the shell matching
  of `path.Match`; and with `-N`, a missing operand against a directory is
  an error where GNU compares the directory against an empty one.
* The side-by-side columns count bytes as columns, so double-width runes
  can push the right column out, and the width is 130 without a terminal
  to ask.
* File contents are read whole, as GNU also does for text, and file times
  print with second precision in the local zone, where GNU can add
  nanoseconds.

### du

* Counts a file by its size rounded up to 512-byte blocks when the file
  system reports none, as `ls` does, and deduplicates hard links only when
  the file system reports a file identity: without one, as on a memory file
  system or Windows, every occurrence is counted.
* The GNU options outside its manual's documented set (`-l`, `-0`,
  `--inodes`, `--threshold`, `--exclude`, `--time`, `--files0-from` and
  `DU_BLOCK_SIZE`) are not implemented.

### expr

* The arithmetic is 64-bit, so operands or results beyond int64 fail with
  exit status 3 ('Numerical result out of range'), where GNU computes
  exactly with multiple precision.
* Patterns support no backreferences, and GNU's `\+`, `\?` and `\|`
  extensions match literally, like the patterns of the `sed` and `grep` of
  this project.
* String comparisons order bytewise, in the C locale, like the other
  utilities here.

### find

* Doesn't implement `-fstype` and `-context`, which need file system
  knowledge no runtime reports, and no birth time, which makes `-newerXY`
  with a `B` fail like GNU does when the reference has none.
* The `-readable`, `-writable` and `-executable` primaries approximate the
  access checks from the permission bits, with no access control list.
* The `-follow`, `-daystart` and `-regextype` options apply to the whole
  traversal, not only to the primaries after them, and the regular
  expressions of `-regex` are POSIX extended ones by default, with the
  types of `-regextype` reduced to basic and extended.
* The `-O` and `-D` options are accepted and change nothing.
* The `-ls` action follows the GNU layout, with the block counts of a file
  system which reports none taken from the file size, and the fractional
  seconds of the `-printf` time formats are always nine digits.
* The `-mount` and `-xdev` primaries compare device IDs, which a runtime
  file system may not report, like a memory one, where they never prune
  anything; the loop detection needs the device and inode numbers as well.
* The sets of the aggregating `-exec` form are flushed at 128 KB of
  pathnames instead of the system's `ARG_MAX` limit, and `-nouser` and
  `-nogroup` consult the user and group databases of the host system.

### grep

* Doesn't implement backreferences in BREs, nor `-P`, which would need an
  engine beyond the one of the standard library.
* The `\<` and `\>` anchors stand for the word boundary of either edge.
* Like `sed`, its bracket expressions take a collating symbol or equivalence
  class of a single character for that character, and no other locale than
  the C one orders them.
* The case-insensitive matching uses Go's simple case folding.
* A binary input is detected one line at a time, so the lines before the
  one holding a null byte are still output, which the GNU manual allows;
  the message about a matching binary file goes to the standard output, as
  the GNU implementation does, although its manual says error.
* The colors of `--color` follow the `GREP_COLORS` capabilities of the
  manual, closing every span with a reset and a clear to the end of line,
  which the `ne` capability omits.

### id

* The identity is the host process's, through the standard library's
  `os/user` and `os` packages, so the real and effective IDs are the same
  and `-r` is accepted and without effect.
* Names resolve through the host user database, printing the numeric IDs
  when they don't.
* The `user` operand resolves through the same database, as a name or a
  numeric user ID, but the group memberships of the selected user cannot be
  enumerated with the standard library, so they are omitted: the default
  format prints only the uid and gid, and `-G` prints only the primary
  group ID.
* Only one operand is taken, where GNU takes several.
* On Windows there are no numeric IDs, so the SID strings the platform
  reports are printed in their place, and the supplementary groups are
  unavailable and omitted.

### join

* Compares its join fields in the bytewise collating sequence of the C
  locale, the one the `sort` and `comm` of this project use: no utility
  here implements the collating sequences of other locales, so keys which
  only another `LC_COLLATE` makes equal stay different.

### logname

* Takes the name from the `LOGNAME` environment variable, against the POSIX
  note that it be ignored, since the isolated runtime exposes no login
  session to query.

### ls

* Doesn't implement the color output (`--color`, with the `LS_COLORS` setup
  of `dircolors`), the hyperlinks (`--hyperlink`), the Emacs dired mode
  (`-D`), nor the security contexts (`-Z`).
* Works the way GNU does in a UTF-8 locale, since it doesn't implement the
  other locales: names are sorted in the bytewise collating sequence of the
  C locale, quoted and timestamped as GNU quotes and timestamps them in a
  UTF-8 one, and `LC_TIME` only picks between the POSIX locale and the rest.
* The terminal width isn't detected, so the columns fill the width of
  `COLUMNS` or 80.
* The runtime file system carries no owners or access control lists, so a
  file whose system information isn't available, like in a memory file
  system, is written with the numeric identifiers zero and no alternate
  access method character; character and block special files are written
  with their size instead of their device numbers; and a birth time only
  the system reports is available on the usual systems.
* Following the GNU defaults rather than the POSIX ones, the block counts
  are written in units of 1024 bytes, with `POSIXLY_CORRECT` turning back
  to 512.

### nice

* Parses and validates the niceness adjustment, but it is never applied,
  since the runtime cannot change the scheduling priority.
* The utility operand still runs, as GNU does when it lacks the privileges
  to lower the niceness, and the adjustments beyond the range of a 64-bit
  integer are reported like GNU reports them.
* With no utility operand, nice reports a usage error, where GNU would
  print the current niceness, which the runtime exposes no way to read.

### nohup

* The hangup immunity is a no-op, since the runtime cannot block or ignore
  the signals of the utility it runs.
* The redirect behavior follows POSIX and GNU: a terminal standard output
  is appended to `nohup.out`, falling back to `$HOME/nohup.out`, with the
  file created readable and writable only by its owner, and a terminal
  standard error goes to the same open file.
* A terminal standard input keeps the null device the execution gives the
  utility, since the runtime cannot provide the unreadable substitute
  descriptor GNU opens.
* The failures of nohup itself, like the ones of a usage error, exit with
  125, the status GNU uses without `POSIXLY_CORRECT`, where POSIX requires
  127.

### od

* Takes the type sizes of an x86-64 GNU system: 16-byte integers are
  rejected and a 16-byte float is the x87 80-bit long double, read from the
  first ten bytes and printed exactly through the math of `math/big`. The
  byte order of `--endian` is that of a little-endian system, whatever the
  host is.
* Options stop at the first operand, the way the shared option parser
  works, where GNU od would also permute the arguments after it.
* An empty attached value, as in `--width=` or `--strings=`, is read as the
  default instead of the error GNU gives, because the option parser reports
  it like the bare long form.
* The undocumented shorthand aliases of GNU od (`-B`, `-D`, `-e`, `-F`,
  `-H`, `-h`, `-I`, `-L`, `-O` and `-X`) are not taken.
* The input is read whole before it is formatted, rather than streamed.

### patch

* Applies unified and context diffs with all the POSIX options except `-D`,
  `-e` and `-n`, which wait for `#ifdef` marks and the ed and normal diff
  formats.
* Deviates from GNU and BSD in two ways POSIX allows: informative messages
  go to standard error instead of standard output, and already applied
  hunks are ignored instead of saved to the reject file.

### pr

* `-p` is accepted but ignored: the output is never a terminal for a
  library caller, so there is nothing to pause for.
* `-s` takes exactly one character, as POSIX specifies; use `-S` for
  multi-character separators.
* `-J` and merged output may contract blank runs to tabs at slightly
  different places than GNU pr when a page is cut by a form feed or ends
  with empty columns, the rendered column widths being identical, and input
  TAB characters that survive to the output with `-J` count as one column
  for truncation instead of being re-expanded at the tab stops.
* `pr -W width -w width` is equivalent to `pr -W width -J`, matching the
  option translation of GNU pr, not its documentation.

### readlink

* Canonicalizes through the same resolver of `realpath`, so the file system
  caveats of that section apply.
* With `-n` given with multiple operands it warns on the standard error and
  prints no delimiters at all.

### realpath

* Resolves through the runtime file system, so with a memory one the
  canonical names stay relative to the file system's root, and a leading
  `/` cannot resolve.
* `-L` binds the `..` of the given operands lexically, before any symlink
  expansion, while `..` inside symlink targets binds physically to the
  resolved prefix; the canonical output never keeps a `..`.
* With `-s` and `-m` together only the file names are processed, touching
  no file.
* When GNU keeps `-s`, `--strip` and `--no-symlinks` on one option line,
  the usage table here splits them, as the hanging layout takes one option
  per line.

### sed

* Supports BRE and ERE (`-E`) patterns, but not backreferences (`\1` to
  `\9`) in patterns, nor the multi-character collating symbols of bracket
  expressions: a collating symbol or equivalence class of a single
  character stands for that character, as the C locale orders them.
  Backreferences in the replacement of `s` work as specified.
* The `e` command and the `e` flag of `s` are not implemented: they run
  their command through a shell, and the runtime only offers an `Exec` that
  takes a program name with an argument list, so there is no shell to hand
  the text to.
* `--posix` rejects the GNU script syntax this implementation parses (the
  `T`, `Q`, `z`, `F`, `W` and `R` commands, the `first~step`, `addr1,+N`,
  `addr1,~N` and `0,/re/` addresses with their `/re/I` and `/re/M`
  modifiers, the `I` and `M` flags and a number with the `g` flag of `s`,
  the `\U \L \u \l \E` replacement escapes, the one-line texts of `a`, `i`
  and `c`, the exit status argument of `q` and `Q`, the width argument of
  `l`, and the range GNU lets `a`, `i`, `r` and `=` take), while keeping
  the GNU conveniences it has always accepted, like `-i` and `-E`.
* `-u` is accepted and does nothing, since the runtime writes each record
  as soon as it is produced.
* `--follow-symlinks` only works when the runtime file system reports
  symbolic links, as the OS one does, and it follows a link whatever
  directory its target is in, where GNU sed keeps to targets beside the
  link; a dangling link is reported as an input file that can't be read
  rather than edited as an empty file.

### shasum

* Follows the Perl script of the `Digest::SHA` distribution, but it doesn't
  take its `-U` (Universal Newlines) and `-0` (BITS) modes.
* When verifying, it hashes with the algorithm `-a` selects, instead of the
  one each checksum line implies.

### shuf

* Consumes the bytes of `--random-source` as it needs them, so the same
  source is deterministic, but the permutation can differ from GNU's for
  the same file.
* The whole input is read into memory, where GNU samples small counts of
  large inputs with reservoir sampling, and ranges beyond what the machine
  can hold are refused with `too many input lines` where GNU dies with
  `memory exhausted`.
* The overflow diagnostics lack GNU's errno suffixes.

### stat

* Prints `?` for every file system statistic of `-f` except the name, since
  the file system interface reports no statfs data.
* On a memory file system the device, inode and rdev numbers are 0, the
  link count 1, the blocks the size rounded up to 512 bytes, and the
  identifiers 0 when the owner isn't known, as `ls` writes them.
* The mount point (`%m`) and the security context (`%C`) print `?` and mark
  the file failed, as GNU does when their reads fail.
* `TZ` is honored only as a zone name, the `'` grouping flag and the `I`
  and `+` flags of printf are not applied, and `--cached` is accepted and
  does nothing.

### sync

* Flushes through the runtime file system, so the whole system is only
  flushed when the FS maps the real one, and the `-d` and `-f` flags
  degrade to the closest operation the platform offers.

### tar

* Supports the `-c`, `-r`, `-t`, `-u` and `-x` operations and the `-b`,
  `-e`, `-f`, `-h`, `-m`, `-o`, `-p`, `-v` and `-w` options, in both the
  dashed and the old option styles.
* Member operands of `-t` and `-x` are literal names or directory prefixes,
  not glob patterns; ownership is never changed, making `-o` a no-op; `-e`
  only ignores files which can't be read while an archive is created;
  extraction refuses to write through symbolic links; and the archive is
  the standard input or output (or the name in the `TAPE` environment
  variable) when `-f` is not given.

### tee

* Implements `-p` and `--output-error` with their four modes, telling a
  pipe from a regular output by the file type the runtime reports, so
  broken pipes are noticed only when a write fails, not polled ahead as GNU
  does: the file of a process substitution may receive the pipe buffer's
  worth of extra data before the broken pipe is seen.
* A broken standard output is fatal rather than ignorable, in every mode,
  since Go raises SIGPIPE for it instead of reporting the error; only the
  `nopipe` modes are honored for the other outputs, and the default mode
  leaves with the status 1 where GNU dies of the signal.
* `-i` is accepted and does nothing, signals never reaching the in-process
  runtime, and the mode of `--output-error` must be given whole, without
  the unambiguous abbreviations GNU accepts.

### time

* The runtime reports the resource usage of the executed children as
  cumulative `getrusage(RUSAGE_CHILDREN)` snapshots, so the average
  unshared data and text sizes (`%D`, `%X`, `%K`, `%t`, `%p`), which modern
  kernels no longer track, are always zero.
* The counts degrade to zeros when the runtime carries no usage hook, where
  the default output falls back to the three-line format the `-p` option
  selects, which POSIX leaves unspecified.
* When the utility cannot be found, cannot be invoked or is terminated by a
  signal, only the diagnostic is written, without timing statistics.
* The termination information of `%Tn`, `%Ts` and the `signalled` value of
  `%Tt` stays empty, since the runtime exposes no signal statuses.
* The verbs of the usage errors follow the runtime's executor rather than
  the child-side wording of GNU.
* The percentage of `%P` is computed from the full time durations, which
  records more precision than GNU's truncated math.

### timeout

* Cannot signal process groups: `-s/--signal` is validated and reported
  with `-v`, but the only stop mechanism is the cancellation of the child's
  execution context, which kills the process (so `-s` never actually
  delivers its signal).
* Consequently `-k/--kill-after` gives no grace period of a soft signal; it
  only stops waiting, reporting 137 after the kill duration.
* `--preserve-status` mimics a death from the configured signal (128+SIG)
  on a timeout, since the actual status is unobservable, and
  `-f/--foreground` is a no-op.
* When the command dies from an outside signal, the 125 status and error of
  the runtime are propagated instead of re-raising the signal.
* Usage errors exit with 1, like the other utilities here.
* Duration parsing clamps any positive underflow (decimal or hexadecimal)
  to one nanosecond, so it still times out, where GNU would keep a tiny
  positive interval.

### uname

* The processor type (`-p`) is never reported and prints `unknown` (as GNU
  on Linux does); the hardware platform (`-i`) repeats the machine name, as
  GNU reports on Linux.
* On darwin and the BSDs the standard library has no `uname(2)`, so the
  fields come from the kernel's sysctl interface instead; on Windows
  release/version come from `syscall.GetVersion`, which Windows reports as
  6.2.9200 for processes without an application manifest, and `-p`/`-i`
  print `unknown`; on the smaller ports (plan9, wasm, solaris, aix) only
  what `runtime` and `os.Hostname` expose is reported, and unavailable
  fields print `unknown`.
* On Android, `-o` reports `Android`.
* GNU 9.12's `-A/--all-labeled` is not implemented.

### which

* Models no aliases or shell functions, so `--read-alias`, `--skip-alias`,
  `--read-functions` and `--skip-functions` are accepted and ignored.
* `--skip-dot` follows the code of GNU which, skipping every non-absolute
  `PATH` entry, beyond what its manual wording suggests.
* The `~` expansion of `PATH` entries is not done, so `--skip-tilde` only
  skips such entries as typed, and `--show-tilde` is never disabled for
  root.
* Without `HOME`, the tilde options are inactive; without `PATH`, the
  diagnostic reports the default search path instead of GNU's `(null)`.

### whoami

* Takes the name from the `USER` or `LOGNAME` environment variables, since
  the isolated runtime exposes no user database.
