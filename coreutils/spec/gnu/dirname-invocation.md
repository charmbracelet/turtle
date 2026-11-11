# dirname invocation (GNU Coreutils 9.12)
### 18.2 `dirname`: Strip last file name component

`dirname` prints all but the final slash-delimited component
of each name. Slashes on either side of the final component are
also removed. If the string contains no slash, `dirname`
prints ‘`.`’ (meaning the current directory). Synopsis:

```
dirname [option] name...
```

name need not be a file name, but if it is, this operation
effectively lists the directory that contains the final component,
including the case when the final component is itself a directory.

Together, `basename` and `dirname` are designed such
that if ‘`ls "$name"`’ succeeds, then the command sequence ‘`cd
"$(dirname "$name")"; ls "$(basename "$name")"`’ will, too. This works
for everything except file names containing a trailing newline.

POSIX allows the implementation to define the results if
name is ‘`//`’. With GNU `dirname`, the
result is ‘`//`’ on platforms where // is distinct from
/, and ‘`/`’ on platforms where there is no difference.

The program accepts the following option. Also see [Common options](Common-options.md).

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

An exit status of zero indicates success,
and a nonzero value indicates failure.

Examples:

```
# Output "/usr/bin".
dirname /usr/bin/sort
dirname /usr/bin//.//

# Output "dir1" followed by "dir2"
dirname dir1/str dir2/str

# Output ".".
dirname stdio.h
```
