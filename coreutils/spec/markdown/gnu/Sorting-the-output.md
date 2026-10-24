# Sorting the output (GNU Coreutils 9.12)
#### 10.1.3 Sorting the output

These options change the order in which `ls` outputs
information for the files in a directory.
(Command-line operands are always processed left to right.)
By default, files are sorted alphabetically by name, using the
character collating sequence specified by the `LC_COLLATE` locale.

‘`-c`’

‘`--time=ctime`’

‘`--time=status`’
:   In long format,
    print the status change timestamp (the ctime) instead of the mtime.
    When sorting by time or when not using long format,
    sort according to the ctime. See [File timestamps](File-timestamps.md).

‘`-f`’
:   Do not sort, and list all files.
    This is like `--sort=none` (`-U`) combined
    with `--all` (`-a`).

‘`--group-directories-first`’
:   When listing a directory’s files,
    group all subdirectories before non-directories
    and then sort the subdirectories and the non-directories separately.
    That is, this option specifies a primary sort key,
    and the other sorting options specify a secondary key.
    However, any use of `--sort=none`
    (`-U`) disables this option altogether.

‘`-r`’

‘`--reverse`’
:   Reverse whatever the sorting method is – e.g., list files in reverse
    alphabetical order, youngest first, smallest first, or whatever.
    This option has no effect when `--sort=none` (`-U`)
    is in effect.

‘`-S`’

‘`--sort=size`’
:   Sort by file size, largest first.

‘`-t`’

‘`--sort=time`’
:   Sort by modification timestamp (mtime) by default, newest first.
    The timestamp to order by can be changed with the `--time` option.
    See [File timestamps](File-timestamps.md).

‘`-u`’

‘`--time=atime`’

‘`--time=access`’

‘`--time=use`’
:   In long format, print the last access timestamp (the atime).
    When sorting by time or when not using long format,
    sort according to the atime.
    See [File timestamps](File-timestamps.md).

‘`--time=mtime`’

‘`--time=modification`’
:   This is the default timestamp display and sorting mode.
    In long format, print the last data modification timestamp (the mtime).
    When sorting by time or when not using long format,
    sort according to the mtime.
    See [File timestamps](File-timestamps.md).

‘`--time=birth`’

‘`--time=creation`’
:   In long format, print the file creation timestamp if available,
    falling back to the file modification timestamp (mtime) if not.
    When sorting by time or when not using long format,
    sort according to the birth time.
    See [File timestamps](File-timestamps.md).

‘`-U`’

‘`--sort=none`’
:   Do not sort; list the files in whatever order they are
    stored in the directory. This can be useful when listing large
    directories, where sorting can take some time.

    Unlike `-f`, this option does not imply `--all`
    (`-a`).

‘`-v`’

‘`--sort=version`’
:   Sort by version name and number, lowest first. It behaves like a default
    sort, except that each sequence of decimal digits is treated numerically
    as an index/version number. See [Version sort ordering](Version-sort-ordering.md).

‘`--sort=name`’
:   Sort by file name. This is the default operation of `ls`,
    and can be explicitly specified to override any
    previously specified sorting option.

‘`--sort=width`’
:   Sort by printed width of file names.
    This can be useful with the `--format=vertical` (`-C`)
    output format, to most densely display the listed files.

‘`-X`’

‘`--sort=extension`’
:   Sort directory contents alphabetically by file extension (characters
    after the last ‘`.`’); files with no extension are sorted first.
