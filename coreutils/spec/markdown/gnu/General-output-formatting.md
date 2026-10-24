# General output formatting (GNU Coreutils 9.12)
#### 10.1.4 General output formatting

These options affect the appearance of the overall output.

‘`--format=single-column`’
:   List one file name per line, with no other information.
    This is the default for `ls` when standard
    output is not a terminal. See also the `--escape` (`-b`),
    `--hide-control-chars` (`-q`), and `--zero` options
    to disambiguate output of file names containing newline characters.

‘`-1`’
:   List one file per line. This is like `--format=single-column`
    except that it has no effect if long format is also in effect.

‘`-C`’

‘`--format=vertical`’
:   List files in columns, sorted vertically, with no other information.
    This is the default for `ls` if standard output is a terminal.
    It is always the default for the `dir` program.
    GNU `ls` uses variable width columns to display as many files as
    possible in the fewest lines.

‘`--color[=when]`’
:   Specify whether to use color for distinguishing file types; when
    may be omitted, or one of:

    * none
      - Do not use color at all. This is the default.
    * auto

      - Only use color if standard output is a terminal.
    * always
      - Always use color.

    Specifying `--color` and no when is equivalent to
    `--color=always`.
    If piping a colored listing through a pager like `less`,
    use the pager’s `-R` option to pass the color codes to the terminal.
    See [`dircolors`: Color setup for `ls`](dircolors-invocation.md) to configure the colors used.

    Using the `--color` option may incur a noticeable
    performance penalty when run in a large directory,
    because the default settings require that `ls` `stat` every
    single file it lists.
    However, if you would like most of the file-type coloring
    but can live without the other coloring options (e.g.,
    executable, orphan, sticky, other-writable, capability), use
    `dircolors` to set the `LS_COLORS` environment variable like this,

    ```
    eval $(dircolors -p | perl -pe \
      's/^((CAP|S[ET]|O[TR]|M|E)\w+).*/$1 00/' | dircolors -)
    ```

    and on a `dirent.d_type`-capable file system, `ls`
    will perform only one `stat` call per command line argument.

‘`-F`’

‘`--classify[=when]`’

‘`--indicator-style=classify`’
:   Append a character to each file name indicating the file type. Also,
    for regular files that are executable, append ‘`*`’. The file type
    indicators are ‘`/`’ for directories, ‘`@`’ for symbolic links,
    ‘`|`’ for FIFOs, ‘`=`’ for sockets, ‘`>`’ for doors,
    and nothing for regular files.
    when may be omitted, or one of:

    * none
      - Do not classify. This is the default.
    * auto

      - Only classify if standard output is a terminal.
    * always
      - Always classify.

    Specifying `--classify` and no when is equivalent to
    `--classify=always`.
    Do not follow symbolic links listed on the
    command line unless the `--dereference-command-line` (`-H`),
    `--dereference` (`-L`), or
    `--dereference-command-line-symlink-to-dir` options are specified.

‘`--file-type`’

‘`--indicator-style=file-type`’
:   Append a character to each file name indicating the file type. This is
    like `--classify` (`-F`, except that executables are not marked.

‘`--hyperlink[=when]`’
:   Output codes recognized by some terminals to link
    to files using the ‘`file://`’ URI format.
    when may be omitted, or one of:

    * none
      - Do not use hyperlinks at all. This is the default.
    * auto

      - Only use hyperlinks if standard output is a terminal.
    * always
      - Always use hyperlinks.

    Specifying `--hyperlink` and no when is equivalent to
    `--hyperlink=always`.

‘`--indicator-style=word`’
:   Append a character indicator with style word to file names,
    as follows:

    ‘`none`’
    :   Do not append any character indicator; this is the default.

    ‘`slash`’
    :   Append ‘`/`’ for directories. This is the same as the `-p`
        option.

    ‘`file-type`’
    :   Append ‘`/`’ for directories, ‘`@`’ for symbolic links, ‘`|`’
        for FIFOs, ‘`=`’ for sockets, and nothing for regular files. This is
        the same as the `--file-type` option.

    ‘`classify`’
    :   Append ‘`*`’ for executable regular files, otherwise behave as for
        ‘`file-type`’. This is the same as the `--classify`
        (`-F`) option.

‘`-k`’

‘`--kibibytes`’
:   Set the default block size to its normal value of 1024 bytes,
    overriding any contrary specification in environment variables
    (see [Block size](Block-size.md)). If `--block-size`,
    `--human-readable` (`-h`), or `--si` options are used,
    they take precedence even if `--kibibytes` (`-k`) is placed after
    the other options.

    The `--kibibytes` (`-k`) option affects the
    per-directory block count written in long format,
    and the file system allocation written by the `--size` (`-s`)
    option. It does not affect the file size in bytes that is written in
    long format.

‘`-m`’

‘`--format=commas`’
:   List files horizontally, with as many as will fit on each line,
    separated by ‘`,` ’ (a comma and a space),
    and with no other information.

‘`-p`’

‘`--indicator-style=slash`’
:   Append a ‘`/`’ to directory names.

‘`-x`’

‘`--format=across`’

‘`--format=horizontal`’
:   List the files in columns, sorted horizontally.

‘`-T cols`’

‘`--tabsize=cols`’
:   Assume that each tab stop is cols columns wide. The default is 8.
    `ls` uses tabs where possible in the output, for efficiency. If
    cols is zero, do not use tabs at all.

    Some terminal emulators might not properly align columns to the right of a
    TAB following a non-ASCII byte. You can avoid that issue by using the
    `-T0` option or put `TABSIZE=0` in your environment, to tell
    `ls` to align using spaces, not tabs.

    If you set a terminal’s hardware tabs to anything other than the default,
    you should also use a `--tabsize` option or `TABSIZE`
    environment variable either to match the hardware tabs, or to disable
    the use of hardware tabs. Otherwise, the output of `ls` may
    not line up. For example, if you run the shell command ‘`tabs -4`’
    to set hardware tabs to every four columns, you should also run
    ‘`export TABSIZE=4`’ or ‘`export TABSIZE=0`’, or use the
    corresponding `--tabsize` options.

‘`-w cols`’

‘`--width=cols`’
:   Assume the screen is cols columns wide. The default is taken
    from the terminal settings if possible; otherwise the environment
    variable `COLUMNS` is used if it is set; otherwise the default
    is 80. With a cols value of ‘`0`’, there is no limit on
    the length of the output line, and that single output line will
    be delimited with spaces, not tabs.

‘`--zero`’
:   Output a zero byte (ASCII NUL) at the end of each line,
    rather than a newline. This option enables other programs to parse the
    output even when that output would contain data with embedded newlines.
    This option is incompatible with the `--dired` (`-D`) option.
    This option also implies the options `--show-control-chars`,
    `-1`, `--color=none`, and
    `--quoting-style=literal` (`-N`).
