# uname invocation (GNU Coreutils 9.12)
### 21.4 `uname`: Print system information

`uname` prints information about the machine and operating system
it is run on. If no options are given, `uname` acts as if the
`-s` option were given. Synopsis:

```
uname [option]...
```

If multiple options or `-a` are given, the selected information is
printed in this order:

```
kernel-name nodename kernel-release kernel-version
machine processor hardware-platform operating-system
```

The information may contain internal spaces, so such output cannot be
parsed reliably. In the following example, kernel-version is
‘`#1 SMP Fri Jul 17 17:18:38 UTC 2020`’:

```
uname -a
⇒ Linux dumdum.example.org 5.9.16-200.fc33.x86_64 #1 SMP Mon Dec 21 14:08:22 UTC 2020 x86_64 x86_64 x86_64 GNU/Linux
```

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-a`’

‘`--all`’
:   Print all of the below information, except omit the processor type
    and the hardware platform name if they are unknown.

‘`-A`’

‘`--all-labeled`’
:   Like `-a`, print all of the below information,
    in a vertical, labeled format: label: value.

‘`-i`’

‘`--hardware-platform`’
:   Print the hardware platform name
    (sometimes called the hardware implementation).
    Print ‘`unknown`’ if this information is not available.
    This is non-portable, even across GNU/Linux distributions.

‘`-m`’

‘`--machine`’
:   Print the machine hardware name (sometimes called the hardware class
    or hardware type).

‘`-n`’

‘`--nodename`’
:   Print the network node hostname.

‘`-p`’

‘`--processor`’
:   Print the processor type (sometimes called the instruction set
    architecture or ISA).
    Print ‘`unknown`’ if this information is not available.
    This is non-portable, even across GNU/Linux distributions.

‘`-o`’

‘`--operating-system`’
:   Print the name of the operating system.

‘`-r`’

‘`--kernel-release`’
:   Print the kernel release.

‘`-s`’

‘`--kernel-name`’
:   Print the kernel name.
    POSIX 1003.1-2001 (see [Standards conformance](Standards-conformance.md)) calls this
    “the implementation of the operating system”, because the
    POSIX specification itself has no notion of “kernel”.
    The kernel name might be the same as the operating system name printed
    by the `-o` or `--operating-system` option, but it might
    differ. Some operating systems (e.g., FreeBSD, HP-UX) have the same
    name as their underlying kernels; others (e.g., GNU/Linux, Solaris)
    do not.

‘`-v`’

‘`--kernel-version`’
:   Print the kernel version.

An exit status of zero indicates success,
and a nonzero value indicates failure.
