# stdbuf invocation (GNU Coreutils 9.12)
### 23.5 `stdbuf`: Run a command with modified I/O stream buffering

`stdbuf` allows one to modify the buffering operations of the
three standard I/O streams associated with a program. Synopsis:

```
stdbuf option... command
```

command must start with the name of a program that

1. uses the ISO C `FILE` streams for input/output, and
2. does not adjust the buffering of its standard streams.

Not every command operates in this way.
For example, `dd` does not use `FILE` streams,
and `tee` adjusts its streams’ buffering.

Any additional args are passed as additional arguments to the
command.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-i mode`’

‘`--input=mode`’
:   Adjust the standard input stream buffering.

‘`-o mode`’

‘`--output=mode`’
:   Adjust the standard output stream buffering.

‘`-e mode`’

‘`--error=mode`’
:   Adjust the standard error stream buffering.

The mode can be specified as follows:

‘`L`’
:   Set the stream to line buffered mode.
    In this mode data is coalesced until a newline is output or
    input is read from any stream attached to a terminal device.
    This option is invalid with standard input.

‘`0`’
:   Disable buffering of the selected stream.
    In this mode, data is output immediately and only the
    amount of data requested is read from input.
    Disabling buffering for input does not necessarily influence the responsiveness
    or blocking behavior of the stream input functions.
    For example, `fread` will still block until `EOF` or error
    or the amount requested is read,
    even if the underlying `read` returns less data than requested.

‘`size`’
:   Specify the size of the buffer to use in fully buffered mode.
    size may be, or may be an integer optionally followed by,
    one of the following multiplicative suffixes:

    ```
    ‘KB’ =>           1000 (KiloBytes)
    ‘K’  =>           1024 (KibiBytes)
    ‘MB’ =>      1000*1000 (MegaBytes)
    ‘M’  =>      1024*1024 (MebiBytes)
    ‘GB’ => 1000*1000*1000 (GigaBytes)
    ‘G’  => 1024*1024*1024 (GibiBytes)
    ```

    and so on for ‘`T`’, ‘`P`’, ‘`E`’, ‘`Z`’, ‘`Y`’,
    ‘`R`’, and ‘`Q`’.
    Binary prefixes can be used, too: ‘`KiB`’=‘`K`’, ‘`MiB`’=‘`M`’,
    and so on.

`stdbuf` is installed only on platforms that use the
Executable and Linkable Format (ELF) and support the
`constructor` attribute, so portable scripts should not rely on
its existence.

Exit status:

```
125 if stdbuf itself fails
126 if command is found but cannot be invoked
127 if command cannot be found
the exit status of command otherwise
```
