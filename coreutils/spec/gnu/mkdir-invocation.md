# mkdir invocation (GNU Coreutils 9.12)
### 12.3 `mkdir`: Make directories

`mkdir` creates directories with the specified names. Synopsis:

```
mkdir [option]... name...
```

`mkdir` creates each directory name in the order given.
It reports an error if name already exists, unless the
`-p` option is given and name is a directory.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-m mode`’

‘`--mode=mode`’
:   Set the file permission bits of created directories to mode,
    which uses the same syntax as
    in `chmod` and uses ‘`a=rwx`’ (read, write and execute allowed for
    everyone) for the point of the departure. See [File permissions](File-permissions.md).
    This option affects only directories given on the command line;
    it does not affect any parents that may be created via the `-p` option.

    Normally the directory has the desired file mode bits at the moment it
    is created. As a GNU extension, mode may also mention
    special mode bits, but in this case there may be a temporary window
    during which the directory exists but its special mode bits are
    incorrect. See [Directories and the Set-User-ID and Set-Group-ID Bits](Directory-Setuid-and-Setgid.md), for how the
    set-user-ID and set-group-ID bits of directories are inherited unless
    overridden in this way.

‘`-p`’

‘`--parents`’
:   Make any missing parent directories for each argument, setting their
    file permission bits to ‘`=rwx,u+wx`’,
    that is, with the umask modified by ‘`u+wx`’. Ignore
    existing parent directories, and do not change their file permission
    bits.

    If the `-m` option is also given, it does not affect
    file permission bits of any newly-created parent directories.
    To control these bits, set the
    umask before invoking `mkdir`. For example, if the shell
    command ‘`(umask u=rwx,go=rx; mkdir -p P/Q)`’ creates the parent
    `P` it sets the parent’s file permission bits to ‘`u=rwx,go=rx`’.
    (The umask must include ‘`u=wx`’ for this method to work.)
    To set a parent’s special mode bits as well, you can invoke
    `chmod` after `mkdir`. See [Directories and the Set-User-ID and Set-Group-ID Bits](Directory-Setuid-and-Setgid.md), for how the set-user-ID and set-group-ID bits of
    newly-created parent directories are inherited.

‘`-v`’

‘`--verbose`’
:   Print a message for each created directory. This is most useful with
    `--parents`.

‘`-Z`’

‘`--context[=context]`’
:   Without a specified context, adjust the SELinux security context according
    to the system default type for destination files, similarly to the
    `restorecon` command.
    The long form of this option with a specific context specified,
    will set the context for newly created files only.
    With a specified context, if both SELinux and SMACK are disabled, a warning is
    issued.

An exit status of zero indicates success,
and a nonzero value indicates failure.
