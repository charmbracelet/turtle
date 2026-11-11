# What information is listed (GNU Coreutils 9.12)
#### 10.1.2 What information is listed

These options affect the information that `ls` displays. By
default, only file names are shown.

‘`--author`’
:   In long format, list each file’s author.
    In GNU/Hurd, file authors can differ from their owners, but in other
    operating systems the two are the same.

‘`-D`’

‘`--dired`’
:   Print an additional line after the main output:

    ```
    //DIRED// beg1 end1 beg2 end2 ...
    ```

    The begn and endn are unsigned integers that record the
    byte position of the beginning and end of each file name in the output.
    This makes it easy for Emacs to find the names, even when they contain
    unusual characters such as space or newline, without fancy searching.

    If directories are being listed recursively via
    `--recursive` (`-R`), output a similar
    line with offsets for each subdirectory name:

    ```
    //SUBDIRED// beg1 end1 ...
    ```

    Finally, output a line of the form:

    ```
    //DIRED-OPTIONS// --quoting-style=word
    ```

    where word is the quoting style (see [Formatting the file names](Formatting-the-file-names.md)).

    Here is an actual example:

    ```
    $ mkdir -p a/sub/deeper a/sub2
    $ touch a/f1 a/f2
    $ touch a/sub/deeper/file
    $ ls -gloRF --dired a
      a:
      total 8
      -rw-r--r-- 1    0 Jun 10 12:27 f1
      -rw-r--r-- 1    0 Jun 10 12:27 f2
      drwxr-xr-x 3 4096 Jun 10 12:27 sub/
      drwxr-xr-x 2 4096 Jun 10 12:27 sub2/

      a/sub:
      total 4
      drwxr-xr-x 2 4096 Jun 10 12:27 deeper/

      a/sub/deeper:
      total 0
      -rw-r--r-- 1 0 Jun 10 12:27 file

      a/sub2:
      total 0
    //DIRED// 48 50 84 86 120 123 158 162 217 223 282 286
    //SUBDIRED// 2 3 167 172 228 240 290 296
    //DIRED-OPTIONS// --quoting-style=literal
    ```

    The pairs of offsets on the ‘`//DIRED//`’ line above delimit
    these names: `f1`, `f2`, `sub`, `sub2`, `deeper`,
    `file`.
    The offsets on the ‘`//SUBDIRED//`’ line delimit the following
    directory names: `a`, `a/sub`, `a/sub/deeper`, `a/sub2`.

    Here is an example of how to extract the fifth entry name, ‘`deeper`’,
    corresponding to the pair of offsets, 222 and 228:

    ```
    $ ls -gloRF --dired a > out
    $ dd bs=1 skip=222 count=6 < out 2>/dev/null; echo
    deeper
    ```

    Although the listing above includes a trailing slash
    for the ‘`deeper`’ entry, the offsets select the name without
    the trailing slash. However, if you invoke `ls` with `--dired`
    (`-D`) along with an option like
    `--escape` (`-b`) and operate
    on a file whose name contains special characters, the backslash
    *is* included:

    ```
    $ touch 'a b'
    $ ls -blog --dired 'a b'
      -rw-r--r-- 1 0 Jun 10 12:28 a\ b
    //DIRED// 30 34
    //DIRED-OPTIONS// --quoting-style=escape
    ```

    If you use a quoting style like `--quoting-style=c` (`-Q`)
    that adds quote marks, then the offsets include the quote marks.
    So beware that the user may select the quoting style via the environment
    variable `QUOTING_STYLE`. Hence, applications using `--dired`
    should either specify an explicit `--quoting-style=literal`
    (`-N`) option on the command line, or else be
    prepared to parse the escaped names.

    The `--dired` (`-D`) option implies long format output
    with hyperlinks disabled, and takes precedence over previously specified
    output formats or hyperlink mode.

‘`--full-time`’
:   Produce long format, and list times in full. It is
    equivalent to using `--format=long` (`-l`) with
    `--time-style=full-iso` (see [Formatting file timestamps](Formatting-file-timestamps.md)).

‘`-g`’
:   Produce long format, but omit owner information.

‘`-G`’

‘`--no-group`’
:   Inhibit display of group information in long format.
    (This is the default in some non-GNU versions of `ls`, so we
    provide this option for compatibility.)

‘`--block-size=size`’
:   With `-l`, scale sizes by size before printing them
    (see [Block size](Block-size.md)). For example,
    `--block-size=G` prints sizes in units of 1,073,741,824 bytes.

‘`-h`’

‘`--human-readable`’
:   Append a size letter to each size, such as ‘`M`’ for mebibytes.
    Powers of 1024 are used, not 1000; ‘`M`’ stands for 1,048,576 bytes.
    This option is equivalent to `--block-size=human-readable`.
    Use the `--si` option if you prefer powers of 1000.

‘`-i`’

‘`--inode`’
:   Print the inode number (also called the file serial number and index
    number) of each file to the left of the file name. (This number
    uniquely identifies each file within a particular file system.)

‘`-l`’

‘`--format=long`’

‘`--format=verbose`’
:   Produce long format.
    In addition to the name of each file, print the file type, file mode bits,
    number of hard links, owner name, group name, size, and
    timestamp (see [Formatting file timestamps](Formatting-file-timestamps.md)), normally
    the modification timestamp (the mtime, see [File timestamps](File-timestamps.md)).
    If the owner or group name cannot be determined, print
    the owner or group ID instead, right-justified as a cue
    that it is a number rather than a textual name.
    Print question marks for other information that
    cannot be determined.

    For block special files and character special files, the size field is
    replaced by the corresponding device major and minor numbers as two decimal
    numbers separated by a comma and at least one space.

    Normally the size is printed as a byte count without punctuation, but
    this can be overridden (see [Block size](Block-size.md)).
    For example, `--human-readable` (`-h`)
    prints an abbreviated, human-readable count, and
    ‘`--block-size="'1"`’ prints a byte count with the thousands
    separator of the current locale.

    For each directory that is listed, preface the files with a line
    ‘`total blocks`’, where blocks is the file system allocation
    for all files in that directory. The block size currently defaults to 1024
    bytes, but this can be overridden (see [Block size](Block-size.md)).
    The blocks computed counts each hard link separately;
    this is arguably a deficiency.

    The file type is one of the following characters:

    ‘`-`’
    :   regular file

    ‘`b`’
    :   block special file

    ‘`c`’
    :   character special file

    ‘`C`’
    :   high performance (“contiguous data”) file

    ‘`d`’
    :   directory

    ‘`D`’
    :   door (Solaris)

    ‘`l`’
    :   symbolic link

    ‘`M`’
    :   off-line (“migrated”) file (Cray DMF)

    ‘`n`’
    :   network special file (HP-UX)

    ‘`p`’
    :   FIFO (named pipe)

    ‘`P`’
    :   port (Solaris)

    ‘`s`’
    :   socket

    ‘`?`’
    :   some other file type

    The file mode bits listed are similar to symbolic mode specifications
    (see [Symbolic Modes](Symbolic-Modes.md)). But `ls` combines multiple bits into the
    third character of each set of permissions as follows:

    ‘`s`’
    :   If the set-user-ID or set-group-ID bit and the corresponding executable bit
        are both set.

    ‘`S`’
    :   If the set-user-ID or set-group-ID bit is set but the corresponding
        executable bit is not set.

    ‘`t`’
    :   If the restricted deletion flag or sticky bit, and the
        other-executable bit, are both set. The restricted deletion flag is
        another name for the sticky bit. See [Structure of File Mode Bits](Mode-Structure.md).

    ‘`T`’
    :   If the restricted deletion flag or sticky bit is set but the
        other-executable bit is not set.

    ‘`x`’
    :   If the executable bit is set and none of the above apply.

    ‘`-`’
    :   Otherwise.

    Following the file mode bits is a single character that specifies
    whether an alternate access method such as an access control list
    applies to the file. When the character following the file mode bits is a
    space, there is no alternate access method. When it is a printing
    character, then there is such a method.

    GNU `ls` uses a ‘`.`’ character to indicate a file
    with a security context, but no other alternate access method.

    A file with any other combination of alternate access methods
    is marked with a ‘`+`’ character.

    `ls` uses a ‘`?`’ character to indicate it is unable to determine
    whether alternate access methods apply to the file, which may happen for
    example with some NFS setups with files without read permission.

‘`-n`’

‘`--numeric-uid-gid`’
:   Produce long format, but
    display right-justified numeric user and group IDs
    instead of left-justified owner and group names.

‘`-o`’
:   Produce long format, but omit group information.
    It is equivalent to using `--format=long` (`-l`)
    with `--no-group` (`-G`).

‘`-s`’

‘`--size`’
:   Print the file system allocation of each file to the left of the file name.
    This is the amount of file system space used by the file, which is usually a
    bit more than the file’s size, but it can be less if the file has holes.

    Normally the allocation is printed in units of
    1024 bytes, but this can be overridden (see [Block size](Block-size.md)).

    For files that are NFS-mounted from an HP-UX system to a BSD system,
    this option reports sizes that are half the correct values. On HP-UX
    systems, it reports sizes that are twice the correct values for files
    that are NFS-mounted from BSD systems. This is due to a flaw in HP-UX;
    it also affects the HP-UX `ls` program.

‘`--si`’
:   Append an SI-style abbreviation to each size, such as ‘`M`’ for
    megabytes. Powers of 1000 are used, not 1024; ‘`M`’ stands for
    1,000,000 bytes. This option is equivalent to
    `--block-size=si`. Use the `-h` or
    `--human-readable` option if
    you prefer powers of 1024.

‘`-Z`’

‘`--context`’
:   Display the SELinux security context or ‘`?`’ if none is found.
    In long format, print the security context to the left of the size column.
