# tty invocation (GNU Coreutils 9.12)
### 19.4 `tty`: Print file name of terminal on standard input

`tty` prints the file name of the terminal connected to its standard
input. It prints ‘`not a tty`’ if standard input is not a terminal.
Synopsis:

```
tty [option]...
```

The program accepts the following option. Also see [Common options](Common-options.md).

‘`-s`’

‘`--silent`’

‘`--quiet`’
:   Print nothing; only return an exit status.

    POSIX.1-2001 removed the `-s` option. Portable shell scripts
    should redirect standard output to `/dev/null` instead.

Exit status:

```
0 if standard input is a terminal
1 if standard input is a non-terminal file
2 if given incorrect arguments
3 if a write error occurs
4 if the terminal’s name cannot be determined
```
