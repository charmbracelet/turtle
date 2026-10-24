# stat invocation (GNU Coreutils 9.12)
### 14.3 `stat`: Report file or file system status

`stat` displays information about the specified file(s). Synopsis:

```
stat [option]... [file]...
```

With no option, `stat` reports all information about the given files.
But it also can be used to report the information of the file systems the
given files are located on. If the files are links, `stat` can
also give information about the files the links point to.

Due to shell aliases and built-in `stat` functions, using an
unadorned `stat` interactively or in a script may get you
different functionality than that described here. Invoke it via
`env` (i.e., `env stat …`) to avoid interference
from the shell.

‘`-L`’

‘`--dereference`’
:   Change how `stat` treats symbolic links.
    With this option, `stat` acts on the file referenced
    by each symbolic link argument.
    Without it, `stat` acts on any symbolic link argument directly.

‘`-f`’

‘`--file-system`’
:   Report information about the file systems where the given files are located
    instead of information about the files themselves.
    This option implies the `-L` option.

‘`--cached=mode`’
:   Control how attributes are read from the file system;
    if supported by the system. This allows one to
    control the trade-off between freshness and efficiency
    of attribute access, especially useful with remote file systems.
    mode can be:

    ‘`always`’
    :   Always read the already cached attributes if available.

    ‘`never`’
    :   Always synchronize with the latest file system attributes.
        This also mounts automounted files.

    ‘`default`’
    :   Leave the caching behavior to the underlying file system.

‘`-c`’

‘`--format=format`’
:   Use format rather than the default format.
    format is automatically newline-terminated, so
    running a command like the following with two or more file
    operands produces a line of output for each operand:

    ```
    $ stat --format=%d:%i / /usr
    2050:2
    2057:2
    ```

‘`--printf=format`’
:   Use format rather than the default format.
    Like `--format`, but interpret backslash escapes,
    and do not output a mandatory trailing newline.
    If you want a newline, include ‘`\n`’ in the format.
    Here’s how you would use `--printf` to print the device
    and inode numbers of `/` and `/usr`:

    ```
    $ stat --printf='%d:%i\n' / /usr
    2050:2
    2057:2
    ```

‘`-t`’

‘`--terse`’
:   Print the information in terse form, suitable for parsing by other programs.

    The output of the following commands are identical and the `--format`
    also identifies the items printed (in fuller form) in the default format.
    The format string would include another ‘`%C`’ at the end with an
    active SELinux security context.

    ```
    $ stat --format="%Qn %s %b %f %u %g %D %i %h %t %T %X %Y %Z %W %o" ...
    $ stat --terse ...
    ```

    The same illustrating terse output in `--file-system` mode:

    ```
    $ stat -f --format="%Qn %i %l %t %s %S %b %f %a %c %d" ...
    $ stat -f --terse ...
    ```

The valid format directives for files with `--format` and
`--printf` are:

* %a – Permission bits in octal (see ‘`#`’ and ‘`0`’ printf flags)
* %A – Permission bits in symbolic form (similar to `ls -ld`)
* %b – Number of blocks allocated (see ‘`%B`’)
* %B – The size in bytes of each block reported by ‘`%b`’
* %C – The SELinux security context of a file, if available
* %d – Device number in decimal (st_dev)
* %D – Device number in hex (st_dev)
* %Hd – Major device number in decimal
* %Ld – Minor device number in decimal
* %f – Raw mode in hex
* %F – File type
* %g – Group ID of owner
* %G – Group name of owner
* %h – Number of hard links
* %i – Inode number
* %m – Mount point (see selow)
* %n – File name
* %Qn – Quoted file name (see below)
* %N – Quoted file name with dereference if symbolic link (see below)
* %o – Optimal I/O transfer size hint
* %s – Total size, in bytes
* %r – Device type in decimal (st_rdev)
* %R – Device type in hex (st_rdev)
* %Hr – Major device type in decimal (see below)
* %Lr – Minor device type in decimal (see below)
* %t – Major device type in hex (see below)
* %T – Minor device type in hex (see below)
* %u – User ID of owner
* %U – User name of owner
* %w – Time of file birth, or ‘`-`’ if unknown
* %W – Time of file birth as seconds since Epoch, or ‘`0`’
* %x – Time of last access
* %X – Time of last access as seconds since Epoch
* %y – Time of last data modification
* %Y – Time of last data modification as seconds since Epoch
* %z – Time of last status change
* %Z – Time of last status change as seconds since Epoch

The ‘`%a`’ format prints the octal mode, and so it is useful
to control the zero padding of the output with the ‘`#`’ and ‘`0`’
printf flags. For example to pad to at least 3 wide while making larger
numbers unambiguously octal, you can use ‘`%#03a`’.

The ‘`%N`’ and ‘`%Qn`’ formats can be set with the environment variable
`QUOTING_STYLE`. If that environment variable is not set,
the default value is ‘`shell-escape`’. Valid quoting styles are:

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

The ‘`r`’, ‘`R`’, ‘`%t`’, and ‘`%T`’ formats operate on the st_rdev
member of the stat(2) structure, i.e., the represented device rather than
the containing device, and so are only defined for character and block
special files. On some systems or file types, st_rdev may be used to
represent other quantities.

The ‘`%W`’, ‘`%X`’, ‘`%Y`’, and ‘`%Z`’ formats accept a
precision preceded by a period to specify the number of digits to
print after the decimal point. For example, ‘`%.3X`’ outputs the
access timestamp to millisecond precision. If a period is given but no
precision, `stat` uses 9 digits, so ‘`%.X`’ is equivalent to
‘`%.9X`’. When discarding excess precision, timestamps are truncated
toward minus infinity.

```
zero pad:
  $ stat -c '[%015Y]' /usr
  [000001288929712]
space align:
  $ stat -c '[%15Y]' /usr
  [     1288929712]
  $ stat -c '[%-15Y]' /usr
  [1288929712     ]
precision:
  $ stat -c '[%.3Y]' /usr
  [1288929712.114]
  $ stat -c '[%.Y]' /usr
  [1288929712.114951834]
```

The mount point printed by ‘`%m`’ is similar to that output
by `df`, except that:

* stat does not dereference symlinks by default
  (unless `-L` is specified)
* stat does not search for specified device nodes in the
  file system list, instead operating on them directly
* stat outputs the alias for a bind mounted file, rather than
  the initial mount point of its backing device.
  One can recursively call stat until there is no change in output,
  to get the current base mount point

When listing file system information (`--file-system` (`-f`)),
you must use a different set of format directives:

* %a – Free blocks available to non-super-user
* %b – Total data blocks in file system
* %c – Total file nodes in file system
* %d – Free file nodes in file system
* %f – Free blocks in file system
* %i – File System ID in hex
* %l – Maximum length of file names
* %n – File name
* %Qn – Quoted file name
* %s – Block size (for faster transfers)
* %S – Fundamental block size (for block counts)
* %t – Type in hex
* %T – Type in human readable form

Timestamps are listed according to the time zone rules specified by
the `TZ` environment variable, or by the system default rules if
`TZ` is not set. See [Specifying the Time Zone
with `TZ`](https://www.gnu.org/software/libc/manual/html_node/TZ-Variable.html#TZ-Variable) in The GNU C Library Reference Manual.

An exit status of zero indicates success,
and a nonzero value indicates failure.
