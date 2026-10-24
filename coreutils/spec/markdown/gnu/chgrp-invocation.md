# chgrp invocation (GNU Coreutils 9.12)
### 13.2 `chgrp`: Change group ownership

`chgrp` changes the group ownership of each given file
to group (which can be either a group name or a numeric group ID)
or to the group of an existing reference file. See [`chown`: Change file owner and group](chown-invocation.md).
Synopsis:

```
chgrp [option]... {group | --reference=ref_file} file...
```

If group is intended to represent a
numeric group ID, then you may specify it with a leading ‘`+`’.
See [chown, chgrp, chroot, id: Disambiguating user names and IDs](Disambiguating-names-and-IDs.md).

It is system dependent whether a user can change the group to an arbitrary one,
or the more portable behavior of being restricted to setting a group of
which the user is a member.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-c`’

‘`--changes`’
:   Verbosely describe the action for each file whose group actually
    changes.

‘`-f`’

‘`--silent`’

‘`--quiet`’
:   Do not print error messages about files whose group cannot be
    changed.

‘`--from=old-owner`’
:   Change a file’s ownership only if it has current attributes specified
    by old-owner. old-owner has the same form as new-owner
    described above.
    This option is useful primarily from a security standpoint in that
    it narrows considerably the window of potential abuse.
    For example, to reflect a user ID numbering change for one user’s files
    without an option like this, `root` might run

    ```
    find / -owner OLDUSER -print0 | xargs -0 chgrp -h NEWUSER
    ```

    But that is dangerous because the interval between when the `find`
    tests the existing file’s owner and when the `chgrp` is actually run
    may be quite large.
    One way to narrow the gap would be to invoke chgrp for each file
    as it is found:

    ```
    find / -owner OLDUSER -exec chgrp -h NEWUSER {} \;
    ```

    But that is very slow if there are many affected files.
    With this option, it is safer (the gap is narrower still)
    though still not perfect:

    ```
    chgrp -h -R --from=OLDUSER NEWUSER /
    ```

‘`--dereference`’
:   Do not act on symbolic links themselves but rather on what they point to.
    This is the default when not operating recursively.

    Combining this dereferencing option with the `--recursive` option
    may create a security risk:
    During the traversal of the directory tree, an attacker may be able to
    introduce a symlink to an arbitrary target; when the tool reaches that,
    the operation will be performed on the target of that symlink,
    possibly allowing the attacker to escalate privileges.

‘`-h`’

‘`--no-dereference`’
:   Act on symbolic links themselves instead of what they point to.
    This mode relies on the `lchown` system call.
    On systems that do not provide the `lchown` system call,
    no diagnostic is issued, but see `--verbose`.

‘`--preserve-root`’
:   Fail upon any attempt to recursively change the root directory, `/`.
    Without `--recursive`, this option has no effect.
    See [Treating `/` specially](Treating-_002f-specially.md).

‘`--no-preserve-root`’
:   Cancel the effect of any preceding `--preserve-root` option.
    See [Treating `/` specially](Treating-_002f-specially.md).

‘`--reference=ref_file`’
:   Change the group of each file to be the same as that of
    ref_file. If ref_file is a symbolic link, do not use the
    group of the symbolic link, but rather that of the file it refers to.

‘`-v`’

‘`--verbose`’
:   Output a diagnostic for every file processed.
    If a symbolic link is encountered during a recursive traversal
    on a system without the `lchown` system call, and `--no-dereference`
    is in effect, then issue a diagnostic saying neither the symbolic link nor
    its referent is being changed.

‘`-R`’

‘`--recursive`’
:   Recursively change the group ownership of directories and their contents.

‘`-H`’
:   If `--recursive` (`-R`) is specified and
    a command line argument is a symbolic link to a directory, traverse it.
    See [Traversing symlinks](Traversing-symlinks.md).

‘`-L`’
:   In a recursive traversal, traverse every symbolic link to a directory
    that is encountered.

    Combining this dereferencing option with the `--recursive` option
    may create a security risk:
    During the traversal of the directory tree, an attacker may be able to
    introduce a symlink to an arbitrary target; when the tool reaches that,
    the operation will be performed on the target of that symlink,
    possibly allowing the attacker to escalate privileges.

    See [Traversing symlinks](Traversing-symlinks.md).

‘`-P`’
:   Do not traverse any symbolic links.
    This is the default if none of `-H`, `-L`,
    or `-P` is specified.
    See [Traversing symlinks](Traversing-symlinks.md).

An exit status of zero indicates success,
and a nonzero value indicates failure.

Examples:

```
# Change the group of /u to "staff".
chgrp staff /u

# Change the group of /u and subfiles to "staff".
chgrp -hR staff /u
```
