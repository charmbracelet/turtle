# paste invocation (GNU Coreutils 9.12)
### 8.2 `paste`: Merge lines of files

`paste` writes to standard output lines consisting of sequentially
corresponding lines of each given file, separated by a delimiter character
(default is the TAB character).
The newline of every line except the line from the last input file
is replaced with a delimiter character.
Standard input is used for a file name of ‘`-`’ or if no input files
are given.

Synopsis:

```
paste [option]... [file]...
```

For example, with:

```
$ cat num2
1
2
$ cat let3
a
b
c
```

Take lines sequentially from each file:

```
$ paste num2 let3
1       a
2       b
        c
```

Duplicate lines from a file:

```
$ paste num2 let3 num2
1       a      1
2       b      2
        c
```

Intermix lines from standard input:

```
$ paste - let3 - < num2
1       a      2
        b
        c
```

Join consecutive lines with a space:

```
$ seq 4 | paste -d ' ' - -
1 2
3 4
```

Comma separate data:

```
$ seq 4 | paste -s -d ','
1,2,3,4
```

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-s`’

‘`--serial`’
:   Paste the lines of one file at a time rather than one line from each file.
    The newline of every line except the last line in each input file
    is replaced with a delimiter character.
    Using the above example data:

    ```
    $ paste -s num2 let3
    1       2
    a       b       c
    ```

‘`-d delim-list`’

‘`--delimiters=delim-list`’
:   Consecutively use the characters in delim-list instead of
    TAB to separate merged lines. When delim-list is
    exhausted, start again at its beginning.

    The following backslash escape sequences are recognized
    in delim-list:

    ‘`\0 The empty string (not the NUL character)`’

    ‘`\n newline`’

    ‘`\t TAB`’

    ‘`\\ A backslash`’

    ‘`\b backspace (GNU extension)`’

    ‘`\f form feed (GNU extension)`’

    ‘`\r carriage return (GNU extension)`’

    ‘`\v vertical tab (GNU extension)`’

    It is an error if no character follows an unescaped backslash.
    As a GNU extension, a backslash followed by a character not listed
    above is interpreted as that character.

    Using the above example data:

    ```
    $ paste -d '%_' num2 let3 num2
    1%a_1
    2%b_2
    %c_
    ```

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
