# readlink invocation (GNU Coreutils 9.12)
### 12.6 `readlink`: Print value of a symlink or canonical file name

`readlink` may work in one of two supported modes:

‘`Readlink mode`’
:   `readlink` outputs the value of the given symbolic links.
    If `readlink` is invoked with an argument other than the name
    of a symbolic link, it produces no output and exits with a nonzero exit code.

‘`Canonicalize mode`’
:   `readlink` outputs the absolute name of the given files which contain
    no `.`, `..` components nor any repeated separators
    (`/`) or symbolic links. The `realpath` command is the
    preferred command to use for canonicalization. See [`realpath`: Print the resolved file name.](realpath-invocation.md).

```
readlink [option]... file...
```

By default, `readlink` operates in readlink mode.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-f`’

‘`--canonicalize`’
:   Activate canonicalize mode.
    If any component of the file name except the last one is missing or unavailable,
    `readlink` produces no output and exits with a nonzero exit
    code. A trailing slash is ignored.

‘`-e`’

‘`--canonicalize-existing`’
:   Activate canonicalize mode.
    If any component is missing or unavailable, `readlink` produces
    no output and exits with a nonzero exit code. A trailing slash
    requires that the name resolve to a directory.

‘`-m`’

‘`--canonicalize-missing`’
:   Activate canonicalize mode.
    If any component is missing or unavailable, `readlink` treats it
    as a directory.

‘`-n`’

‘`--no-newline`’
:   Do not print the output delimiter, when a single file is specified.
    Print a warning if specified along with multiple files.

‘`-q`’

‘`-s`’

‘`--quiet`’

‘`--silent`’
:   Suppress most error messages.

    This option is on by default if the `POSIXLY_CORRECT` environment
    variable is not set.

‘`-v`’

‘`--verbose`’
:   Report error messages.

    This option is on by default if the `POSIXLY_CORRECT` environment
    variable is set.

‘`-z`’

‘`--zero`’
:   Output a zero byte (ASCII NUL) at the end of each line,
    rather than a newline. This option enables other programs to parse the
    output even when that output would contain data with embedded newlines.

When standard output is a terminal, output is quoted using the
‘`shell-escape`’ style. The environment variable
`QUOTING_STYLE` specifies the quoting style. Valid quoting styles
are:

‘`literal`’
:   Output strings as-is;

‘`shell`’
:   Quote strings for the shell if they contain shell metacharacters or would
    cause ambiguous output.
    The quoting is suitable for POSIX-compatible shells like
    `bash`, but it does not always work for incompatible shells
    like `csh`.

‘`shell-always`’
:   Quote strings for the shell, even if they would normally not require quoting.

‘`shell-escape`’
:   Like ‘`shell`’, but also quote non-printable characters using the POSIX
    ‘`$''`’ syntax suitable for most shells. This is the most general format
    as any file name is represented unambiguously and safely. I.e., the full
    quoted string can be pasted back to the shell to refer to any file.
    Note also that specifying the C locale with `LC_ALL=C` can be useful
    with this output format, to avoid character set conversion issues
    with some terminals; e.g., xterm always converting to unicode composed form.
    See also
    [Quoting file names](https://www.gnu.org/software/coreutils/quotes.html).

‘`shell-escape-always`’
:   Like ‘`shell-escape`’, but quote strings even if they would
    normally not require quoting.

‘`c`’
:   Quote strings as for C character string literals, including the
    surrounding double-quote characters;

‘`c-maybe`’
:   Quote strings as for C character string literals, except omit the
    surrounding double-quote if no escaping is required.

‘`escape`’
:   Quote strings as for C character string literals, except omit the
    surrounding double-quote characters;

‘`clocale`’
:   Quote strings as for C character string literals, except use
    surrounding quotation marks appropriate for the
    locale.

‘`locale`’
:   Quote strings as for C character string literals, except use
    surrounding quotation marks appropriate for the locale, and quote
    `'like this'` instead of `"like
    this"` in the default C locale. This looks nicer on many displays.

The `readlink` utility first appeared in OpenBSD 2.1.

The `realpath` command without options, operates like
`readlink` in canonicalize mode.

An exit status of zero indicates success,
and a nonzero value indicates failure.
