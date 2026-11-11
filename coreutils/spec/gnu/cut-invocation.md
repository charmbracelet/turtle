# cut invocation (GNU Coreutils 9.12)
### 8.1 `cut`: Print selected parts of lines

`cut` writes to standard output selected parts of each line of each
input file, or standard input if no files are given or for a file name of
‘`-`’. Synopsis:

```
cut option... [file]...
```

In the table which follows, the byte-list, character-list,
and field-list are one or more numbers or ranges (two numbers
separated by a dash) separated by commas. Bytes, characters, and
fields are numbered starting at 1. Incomplete ranges may be
given: `-m` means ‘`1-m`’; ‘`n-`’ means
‘`n`’ through end of line or last field. The list elements
can be repeated, can overlap, and can be specified in any order; but
the selected input is written in the same order that it is read, and
is written exactly once.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-b byte-list`’

‘`--bytes=byte-list`’
:   Select for printing only the bytes in positions listed in
    byte-list. Tabs and backspaces are treated like any other
    character; they take up 1 byte. If an output delimiter is specified,
    (see the description of `--output-delimiter`), then output that
    string between ranges of selected bytes.

‘`-c character-list`’

‘`--characters=character-list`’
:   Select for printing only the characters in positions listed in
    character-list. Tabs and backspaces are
    treated like any other character; they take up 1 character.
    Combining characters are considered as separate characters.
    If an output delimiter is specified, (see the description of
    `--output-delimiter`), then output that string between ranges
    of selected characters.

‘`--complement`’
:   This option is a GNU extension.
    Select for printing the complement of the bytes, characters or fields
    selected with the `-b`, `-c` or `-f` options.
    In other words, do *not* print the bytes, characters or fields
    specified via those options. This option is useful when you have
    many fields and want to print all but a few of them.

‘`-d character`’

‘`--delimiter=character`’
:   With `-f`, use character as
    the input field separator (default is TAB).
    If an empty delimiter character is specified then the ASCII NUL
    character is used to delimit the fields.
    Note the delimiter character can be the same as the
    line terminator character, which can be used to select ranges of lines.
    In this case the last line terminator in the input is not treated
    as a field separator, which is significant with the `-s` option.

‘`-f field-list`’

‘`--fields=field-list`’
:   Select for printing only the fields listed in field-list.
    Fields are separated by a TAB character by default. Also print any
    line that contains no delimiter character, unless the
    `--only-delimited` (`-s`) option is specified.

    The `awk` command supports more sophisticated field processing,
    like reordering fields, and handling fields aligned with blank characters.
    By default `awk` uses (and discards) runs of blank characters
    to separate fields, and ignores leading and trailing blanks.

    ```
    awk '{print $2}'      # print the second field
    awk '{print $(NF-1)}' # print the penultimate field
    awk '{print $2,$1}'   # reorder the first two fields
    ```

    While `cut` accepts field specifications in
    arbitrary order, output is always in the order encountered in the file.

    In the unlikely event that `awk` is unavailable,
    one can use the `join` command, to process blank
    characters as `awk` does above.

    ```
    join -a1 -o 1.2     - /dev/null # print the second field
    join -a1 -o 1.2,1.1 - /dev/null # reorder the first two fields
    ```

‘`-F`’
:   Like the `-f,--fields` option, but also implies
    `-w` and `--output-delimitor=' '`.
    I.e., fields are separated by runs of blank characters,
    and output fields are separated by a single ASCII space.

‘`-n`’

‘`--no-partial`’
:   With `--bytes`, do not split multi-byte characters.
    A byte range must encompass the end of a multi-byte character
    for it to be selected.

‘`-O string`’

‘`--output-delimiter=string`’
:   With `-f`, output fields are separated by string.
    The default with `-f` is to use the input delimiter,
    or with `-w` the TAB character.
    When using `-b` or `-c` to select ranges of byte or
    character offsets (as opposed to ranges of fields),
    output output_delim_string between non-overlapping
    ranges of selected bytes.

‘`-s`’

‘`--only-delimited`’
:   For `-f`, do not print lines that do not contain field separators.
    Normally, any line without a field separator is printed verbatim.
    With `--whitespace-delimited=trimmed`, lines that contain
    only leading or trailing blanks will be suppressed.

‘`-w`’

‘`--whitespace-delimited[=trimmed]`’
:   With `-f`, separate fields with a run of blank characters
    (usually space or TAB). With ‘`trimmed`’ do not consider
    leading and trailing blanks as field separators.
    `-w` is implied with the `-F` option.
    Note efficiency of this mode can be improved if the input
    fields are separated by runs of only ASCII blank characters (space or TAB).
    Even if fields contain arbitrary other characters, you can efficiently
    search for ASCII blanks by invoking `cut` like ‘`LC_ALL=C cut`’.

‘`-z`’

‘`--zero-terminated`’
:   Delimit items with a zero byte rather than a newline (ASCII LF).
    I.e., treat input as items separated by ASCII NUL
    and terminate output items with ASCII NUL.
    This option can be useful in conjunction with ‘`perl -0`’ or
    ‘`find -print0`’ and ‘`xargs -0`’ which do the same in order to
    reliably handle arbitrary file names (even those containing blanks
    or other special characters).

An exit status of zero indicates success,
and a nonzero value indicates failure.
