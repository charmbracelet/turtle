# kill invocation (GNU Coreutils 9.12)
### 24.1 `kill`: Send a signal to processes

The `kill` command sends a signal to processes, causing them
to terminate or otherwise act upon receiving the signal in some way.
Alternatively, it lists information about signals. Synopses:

```
kill [-s signal | --signal signal | -signal] pid...
kill [-l | --list | -t | --table] [signal]...
```

Due to shell aliases and built-in `kill` functions, using an
unadorned `kill` interactively or in a script may get you
different functionality than that described here. Invoke it via
`env` (i.e., `env kill …`) to avoid interference
from the shell.

The first form of the `kill` command sends a signal to all
pid arguments. The default signal to send if none is specified
is ‘`TERM`’. The special signal number ‘`0`’ does not denote a
valid signal, but can be used to test whether the pid arguments
specify processes to which a signal could be sent.

If pid is positive, the signal is sent to the process with the
process ID pid. If pid is zero, the signal is sent to all
processes in the process group of the current process. If pid
is −1, the signal is sent to all processes for which the user has
permission to send a signal. If pid is less than −1, the signal
is sent to all processes in the process group that equals the absolute
value of pid.

If pid is not positive, a system-dependent set of system
processes is excluded from the list of processes to which the signal
is sent.

If a negative pid argument is desired as the first one, it
should be preceded by `--`. However, as a common extension to
POSIX, `--` is not required with ‘`kill
-signal -pid`’. The following commands are equivalent:

```
kill -15 -1
kill -TERM -1
kill -s TERM -- -1
kill -- -1
```

The first form of the `kill` command succeeds if every pid
argument specifies at least one process that the signal was sent to.

The second form of the `kill` command lists signal information.
This form of the `kill` command succeeds if all signal
arguments are valid and if there is no output error.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-s`’

‘`--signal`’
:   Specify the name or number of the signal to be sent.

‘`-l`’

‘`--list`’
:   List the supported signal names one per line if the signal option
    is omitted. If one or more signal options are provided, they are
    converted between signal names and signal numbers.

‘`-t`’

‘`--table`’
:   This option behaves the same way as `--list` except it prints a
    table of signal numbers, names, and descriptions.

A signal may be a signal name like ‘`HUP`’, or a signal
number like ‘`1`’, or an exit status of a process terminated by the
signal. A signal name can be given in canonical form or prefixed by
‘`SIG`’. The case of the letters is ignored, except for the
`-signal` option which must use upper case to avoid
ambiguity with lower case option letters.
See [Signal specifications](Signal-specifications.md), for a list of supported
signal names and numbers.
