# sleep invocation (GNU Coreutils 9.12)
### 25.1 `sleep`: Delay for a specified time

`sleep` pauses for an amount of time specified by the sum of
the values of the command line arguments.
Synopsis:

```
sleep number[smhd]...
```

Each argument is a non-negative number followed by an optional unit; the default
is seconds. The units are:

‘`s`’
:   seconds

‘`m`’
:   minutes

‘`h`’
:   hours

‘`d`’
:   days

Although portable POSIX scripts must give `sleep` a single
non-negative integer argument without a suffix, GNU `sleep`
also accepts two or more arguments, unit suffixes, and floating-point
numbers in either the current or the C locale. See [Floating point numbers](Floating-point.md).

A hexadecimal number can precede a ‘`d`’ suffix only if the number
has a ‘`p`’ style exponent, e.g., ‘`0x1p0d`’ means one day.

For instance, the following could be used to `sleep` for
1 second, 234 milli-, 567 micro- and 890 nanoseconds:

```
sleep 1234e-3 567.89e-6
```

Also one could sleep indefinitely like:

```
sleep inf
```

The only options are `--help` and `--version`. See [Common options](Common-options.md).

Due to shell aliases and built-in `sleep` functions, using an
unadorned `sleep` interactively or in a script may get you
different functionality than that described here. Invoke it via
`env` (i.e., `env sleep …`) to avoid interference
from the shell.

An exit status of zero indicates success,
and a nonzero value indicates failure.
