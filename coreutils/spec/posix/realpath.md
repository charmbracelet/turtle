# realpath
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

realpath — resolve a pathname

#### SYNOPSIS

`realpath [-E|-e] file`

#### DESCRIPTION

The *realpath* utility shall canonicalize the pathname specified by the *file* operand as follows:

If a call to the [*realpath*()](../functions/realpath.html) function with the specified pathname as its first
argument would succeed, the canonicalized pathname shall be the pathname that would be returned by that [*realpath*()](../functions/realpath.html) call. Otherwise:

* If the **-e** option is specified, the canonicalization shall fail.
* If the **-E** option is specified, then if a call to the [*realpath*()](../functions/realpath.html) function
  with the specified pathname as its first argument would encounter an error condition other than [ENOENT], the canonicalization
  shall fail; if the call would encounter an [ENOENT] error, *realpath* shall expand all symbolic links that would be
  encountered in an attempt to resolve the specified pathname using the algorithm specified in XBD [*4.16 Pathname Resolution*](../basedefs/V1_chap04.html#tag_04_16), except that any trailing <slash> characters that
  are not also leading <slash> characters shall be ignored. If this expansion succeeds and the path prefix of the expanded
  pathname resolves to an existing directory, the canonicalized pathname shall be the expanded pathname. In all other cases, the
  canonicalization shall fail. If the expanded pathname is not empty, does not begin with a <slash>, and has exactly one
  pathname component, it shall be treated as if it had a path prefix of "./".
* If no options are specified, *realpath* shall canonicalize the specified pathname in an unspecified manner such that the
  resulting absolute pathname does not contain any components that refer to files of type symbolic link and does not contain any
  components that are dot or dot-dot.

Upon successful canonicalization, *realpath* shall write the canonicalized pathname, followed by a <newline>
character, to standard output.

If canonicalization fails, or the canonicalized pathname is empty, nothing shall be written to standard output, a diagnostic
message shall be written to standard error, and *realpath* shall exit with non-zero status.

#### OPTIONS

The *realpath* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following options shall be supported:

**-E**
:   Do not treat it as an error if attempting to resolve the last component of the canonicalized form of the *file* operand
    results in an [ENOENT] error condition.

**-e**
:   Treat it as an error if attempting to resolve the last component of the canonicalized form of the *file* operand results
    in an [ENOENT] error condition.

Specifying more than one of the mutually-exclusive options **-E** and **-e** shall not be considered an error. The last
option specified shall determine the behavior of the utility.

#### OPERANDS

The following operand shall be supported:

*file*
:   A pathname to be canonicalized.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *realpath*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments and input files).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

See DESCRIPTION.

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

The following exit values shall be returned:

0
:   Successful completion.

>0
:   An error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

If neither the **-e** nor the **-E** option is specified, some implementations behave as if **-e** had been specified
and others as if **-E** had been specified, but there are also implementations where the behavior differs from both of these.
For example, the *mksh* shell has an internal implementation of *realpath* that canonicalizes **/dir/regular_file/..**
to **/dir**, whereas the [*realpath*()](../functions/realpath.html) function would return an [ENOTDIR] error in
this case. Portable applications should always specify either **-e** or **-E**.

#### EXAMPLES

None.

#### RATIONALE

The *realpath* utility was added in preference to a **-f** option found in some implementations of the [*readlink*](../utilities/readlink.html) utility because it allows the application to specify whether or not a missing
final component is to be treated as an error.

The behavior with the **-E** option when *file* does not resolve (with symbolic links followed) to an existing file is
not the same as simply calling [*realpath*()](../functions/realpath.html) with the path prefix of the *file*
operand and writing the resulting pathname, a <slash>, and the last component of *file* to standard output. For example,
if **/tmp/nofile** does not exist, and *file* is **A/B** where **A** is an existing directory and **B** is a
symbolic link to **/tmp/nofile**, *realpath* with **-E** will output **/tmp/nofile**, but if **B** is a symbolic
link to **/tmp/nofile/foo**, *realpath* with **-E** will treat this as an error. In both cases realpath("A/B")
would fail with *errno* set to [ENOENT]. Even though realpath("A") would succeed, in neither case is anything ending
**/B** the result.

Trailing <slash> characters (that follow a non-<slash>) are handled differently with **-E** than with **-e**.
With **-e** they are handled as for the [*realpath*()](../functions/realpath.html) function. With **-E** they
are sometimes effectively ignored, and they are never included in the output. For example, if **/tmp/nofile** does not exist and
**/tmp/regfile** is an existing regular file:

```

$ realpath -E /tmp/nofile/
/tmp/nofile
$ realpath -E /tmp/regfile/
realpath: /tmp/regfile/: Not a directory
```

#### FUTURE DIRECTIONS

If this utility is directed to display a pathname that contains any bytes that have the encoded value of a <newline>
character when <newline> is a terminator or separator in the output format being used, implementations are encouraged to
treat this as an error. A future version of this standard may require implementations to treat this as an error.

#### SEE ALSO

[*ln*](../utilities/ln.html#), [*ls*](../utilities/ls.html#), [*pwd*](../utilities/pwd.html#), [*readlink*](../utilities/readlink.html#tag_20_101)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

XSH [*2.3 Error Numbers*](../functions/V2_chap02.html#tag_16_03), [*realpath*()](../functions/realpath.html#)

#### CHANGE HISTORY

First released in Issue 8.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
