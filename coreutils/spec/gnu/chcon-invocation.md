# chcon invocation (GNU Coreutils 9.12)
### 22.1 `chcon`: Change SELinux context of file

`chcon` changes the SELinux security context of the selected files.
Synopses:

```
chcon [option]... context file...
chcon [option]... [-u user] [-r role] [-l range] [-t type] file...
chcon [option]... --reference=rfile file...
```

Change the SELinux security context of each file to context.
With `--reference`, change the security context of each file
to that of rfile.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`--dereference`’
:   Do not affect symbolic links but what they refer to; this is the default.

‘`-h`’

‘`--no-dereference`’
:   Affect the symbolic links themselves instead of any referenced file.

‘`--reference=rfile`’
:   Use rfile’s security context rather than specifying a context value.

‘`-R`’

‘`--recursive`’
:   Operate on files and directories recursively.

‘`--preserve-root`’
:   Refuse to operate recursively on the root directory, `/`,
    when used together with the `--recursive` option.
    See [Treating `/` specially](Treating-_002f-specially.md).

‘`--no-preserve-root`’
:   Do not treat the root directory, `/`, specially when operating
    recursively; this is the default.
    See [Treating `/` specially](Treating-_002f-specially.md).

‘`-H`’
:   If `--recursive` (`-R`) is specified and
    a command line argument is a symbolic link to a directory, traverse it.
    See [Traversing symlinks](Traversing-symlinks.md).

‘`-L`’
:   In a recursive traversal, traverse every symbolic link to a directory
    that is encountered.
    See [Traversing symlinks](Traversing-symlinks.md).

‘`-P`’
:   Do not traverse any symbolic links.
    See [Traversing symlinks](Traversing-symlinks.md).

‘`-v`’

‘`--verbose`’
:   Output a diagnostic for every file processed.

‘`-u user`’

‘`--user=user`’
:   Set user user in the target security context.

‘`-r role`’

‘`--role=role`’
:   Set role role in the target security context.

‘`-t type`’

‘`--type=type`’
:   Set type type in the target security context.

‘`-l range`’

‘`--range=range`’
:   Set range range in the target security context.

An exit status of zero indicates success,
and a nonzero value indicates failure.
