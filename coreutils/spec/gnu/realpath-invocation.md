# realpath invocation (GNU Coreutils 9.12)
### 18.5 `realpath`: Print the resolved file name.

The `realpath` command outputs a file’s canonicalized name,
which is an absolute file name that contains neither unnecessary
‘`/`’ nor components that are ‘`.`’, ‘`..`’, or a symbolic link.

Synopsis:

```
realpath [option]... file...
```

By default, `realpath` succeeds if every file name component
but the last names an existing directory. It ignores trailing slashes
that are not also leading slashes.

The file name canonicalization functionality overlaps with that of the
`readlink` command. This is the preferred command for
canonicalization as it’s a more suitable and standard name. In addition
this command supports relative file name processing functionality.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-E`’

‘`--canonicalize`’
:   Do not fail merely because the named file does not exist.
    Although this is the default behavior of GNU `realpath`,
    POSIX does not require this default, so portable applications should
    use an explicit `-E`.

‘`-e`’

‘`--canonicalize-existing`’
:   Fail if the named file does not exist.
    A trailing slash requires that the name resolve to a directory.

‘`-m`’

‘`--canonicalize-missing`’
:   If any component of a specified file name is missing or unavailable,
    treat it as a directory.

‘`-L`’

‘`--logical`’
:   Symbolic links are resolved in the specified file names,
    but they are resolved after any subsequent ‘`..`’ components are processed.

‘`-P`’

‘`--physical`’
:   Symbolic links are resolved in the specified file names,
    and they are resolved before any subsequent ‘`..`’ components are processed.
    This is the default mode of operation.

‘`-q`’

‘`--quiet`’
:   Suppress diagnostic messages for specified file names.

‘`--relative-to=dir`’
:   Print the resolved file names relative to the specified directory.
    This option honors the `-m` and `-e` options
    pertaining to file existence.

‘`--relative-base=dir`’
:   Print the resolved file names as relative *if* the files
    are descendants of dir.
    Otherwise, print the resolved file names as absolute.
    This option honors the `-m` and `-e` options
    pertaining to file existence.
    For details about combining `--relative-to` and `--relative-base`,
    see [Realpath usage examples](Realpath-usage-examples.md).

‘`-s`’

‘`--strip`’

‘`--no-symlinks`’
:   Do not resolve symbolic links. Only resolve references to
    ‘`/./`’, ‘`/../`’ and remove extra ‘`/`’ characters.
    When combined with the `-m` option, realpath operates
    only on the file name, and does not touch any actual file.

‘`-z`’

‘`--zero`’
:   Output a zero byte (ASCII NUL) at the end of each line,
    rather than a newline. This option enables other programs to parse the
    output even when that output would contain data with embedded newlines.

When standard output is a terminal, file names are quoted using
the ‘`shell-escape`’ style. The environment variable
`QUOTING_STYLE` can select the quoting style. Valid quoting styles are:

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

Exit status:

```
0 if all file names were printed without issue.
1 otherwise.
```

* [Realpath usage examples](Realpath-usage-examples.md)
