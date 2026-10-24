# printenv invocation (GNU Coreutils 9.12)
### 19.3 `printenv`: Print all or some environment variables

`printenv` prints environment variable values. Synopsis:

```
printenv [option] [variable]...
```

If no variables are specified, `printenv` prints the value of
every environment variable. Otherwise, it prints the value of each
variable that is set, and nothing for those that are not set.

The program accepts the following option. Also see [Common options](Common-options.md).

‘`-0`’

‘`--null`’
:   Output a zero byte (ASCII NUL) at the end of each line,
    rather than a newline. This option enables other programs to parse the
    output even when that output would contain data with embedded newlines.

If no variables are specified, `printenv` quotes
environment variables and their values using the ‘`shell-escape`’
style. If variables are specified, `printenv` only quotes
their values using the ‘`shell-escape`’ style if standard output is a
terminal.

The environment variable `QUOTING_STYLE`
specifies the quoting style. Valid quoting styles are:

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
0 if all variables specified were found
1 if at least one specified variable was not found
2 if a write error occurred
```
