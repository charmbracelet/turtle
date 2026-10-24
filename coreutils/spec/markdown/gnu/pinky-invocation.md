# pinky invocation (GNU Coreutils 9.12)
### 20.7 `pinky`: Print information about users

`pinky` is a lightweight implementation of the `finger` command.
Synopsis:

```
pinky [option]... [username]...
```

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-l`’
:   Produce long format output.

    When producing long output at least one username must be given.
    If username cannot be found, the real name is printed as
    ‘`???`’ and the home directory and shell are omitted.

‘`-b`’
:   Omit the user’s home directory and shell when printing in long format.

‘`-h`’
:   Omit the user’s project file when printing in long format.

‘`-p`’
:   Omit the user’s plan file when printing in long format.

‘`-s`’
:   Produce short format output. This is the default behavior when no
    options are given.

‘`-f`’
:   Omit the column headings when printing in short format.

‘`-w`’
:   Omit the user’s full name when printing in short format.

‘`-i`’
:   Omit the user’s full name and remote host when printing in short
    format.

‘`-q`’
:   Omit the user’s full name, remote host, and idle time when printing in
    short format.

‘`--lookup`’
:   Attempt to canonicalize hostnames found in utmp through a DNS lookup.
    This is not the default because of potential delays.

An exit status of zero indicates success,
and a nonzero value indicates failure.
