# Formatting the file names (GNU Coreutils 9.12)
#### 10.1.6 Formatting the file names

These options change how file names themselves are printed.

You can specify the default value of the `--quoting-style` option
with the environment variable `QUOTING_STYLE`. If that environment
variable is not set, the default value is ‘`shell-escape`’ when the
output is a terminal, and ‘`literal`’ otherwise.

‘`-b`’

‘`--escape`’

‘`--quoting-style=escape`’
:   Quote nongraphic characters in file names using alphabetic and octal
    backslash sequences like those used in C.

‘`-N`’

‘`--literal`’

‘`--quoting-style=literal`’
:   Do not quote file names. However, with `ls` nongraphic
    characters are still printed as question marks if the output is a
    terminal and you do not specify the `--show-control-chars` option.
    Also newline characters in file names are printed as question marks
    if ambiguous with separators, irrespective of whether output
    is a terminal or not.

‘`-q`’

‘`--hide-control-chars`’
:   Print question marks instead of nongraphic characters in file names.
    This is the default if the output is a terminal and the program is
    `ls`.

‘`-Q`’

‘`--quote-name`’

‘`--quoting-style=c`’
:   Enclose file names in double quotes and quote nongraphic characters as
    in C.

‘`--quoting-style=word`’
:   Use style word to quote file names and other strings that may
    contain arbitrary characters. The word should
    be one of the following:

    ‘`literal`’
    :   Output strings as-is;
        this is the same as the `--literal` (`-N`) option.

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
        Consequently, this is the default format used when `ls`
        is outputting to a tty.
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
        this is the same as the `--quote-name` (`-Q`) option.

    ‘`c-maybe`’
    :   Quote strings as for C character string literals, except omit the
        surrounding double-quote if no escaping is required.

    ‘`escape`’
    :   Quote strings as for C character string literals, except omit the
        surrounding double-quote characters;
        this is the same as the `--escape` (`-b`) option.

    ‘`clocale`’
    :   Quote strings as for C character string literals, except use
        surrounding quotation marks appropriate for the
        locale.

    ‘`locale`’
    :   Quote strings as for C character string literals, except use
        surrounding quotation marks appropriate for the locale, and quote
        `'like this'` instead of `"like
        this"` in the default C locale. This looks nicer on many displays.

‘`--show-control-chars`’
:   Print nongraphic characters as-is in file names.
    This is the default unless the output is a terminal and the program is
    `ls`.
