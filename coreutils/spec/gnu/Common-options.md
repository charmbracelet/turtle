# Common options (GNU Coreutils 9.12)
## 2 Common options

Certain options are available in all of these programs. Rather than
writing identical descriptions for each of the programs, they are
described here. (In fact, every GNU program accepts (or should accept)
these options.)

Normally options and operands can appear in any order, and programs act
as if all the options appear before any operands. For example,
‘`sort -r passwd -t :`’ acts like ‘`sort -r -t : passwd`’, since
‘`:`’ is an option-argument of `-t`. However, if the
`POSIXLY_CORRECT` environment variable is set, options must appear
before operands, unless otherwise specified for a particular command.

A few programs can usefully have trailing operands with leading
‘`-`’. With such a program, options must precede operands even if
`POSIXLY_CORRECT` is not set, and this fact is noted in the
program description. For example, the `env` command’s options
must appear before its operands, since in some cases the operands
specify a command that itself contains options.

Most programs that accept long options recognize unambiguous
abbreviations of those options. For example, ‘`rmdir
--ignore-fail-on-non-empty`’ can be invoked as ‘`rmdir
--ignore-fail`’ or even ‘`rmdir --i`’. Ambiguous options, such as
‘`ls --h`’, are identified as such.

Some of these programs recognize the `--help` and `--version`
options only when one of them is the sole command line argument. For
these programs, abbreviations of the long options are not always recognized.

‘`--help`’
:   Print a usage message listing all available options, then exit successfully.
    Help output may be marked up with terminal codes for formatting or
    hyperlinks, which can be disabled by unsetting the `TERM` environment
    variable, or setting it to the value ‘`dumb`’.

‘`--version`’
:   Print the version number, then exit successfully.

‘`--`’
:   Delimit the option list. Later arguments, if any, are treated as
    operands even if they begin with ‘`-`’. For example, ‘`sort --
    -r`’ reads from the file named `-r`.

A single ‘`-`’ operand is not really an option, though it looks like one. It
stands for a file operand, and some tools treat it as standard input, or as
standard output if that is clear from the context. For example, ‘`sort -`’
reads from standard input, and is equivalent to plain ‘`sort`’. Unless
otherwise specified, a ‘`-`’ can appear as any operand that requires a file
name.

* [Backup options](Backup-options.md)
* [Block size](Block-size.md)
* [Signal specifications](Signal-specifications.md)
* [chown, chgrp, chroot, id: Disambiguating user names and IDs](Disambiguating-names-and-IDs.md)
* [Sources of random data](Random-sources.md)
* [Target directory](Target-directory.md)
* [Trailing slashes](Trailing-slashes.md)
* [Traversing symlinks](Traversing-symlinks.md)
* [Treating `/` specially](Treating-_002f-specially.md)
* [Special built-in utilities](Special-built_002din-utilities.md)
* [Exit status](Exit-status.md)
* [Floating point numbers](Floating-point.md)
* [Hardware Acceleration Configuration](Hardware-Acceleration.md)
* [Standards conformance](Standards-conformance.md)
* [`coreutils`: Multi-call program](Multi_002dcall-invocation.md)
