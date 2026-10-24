# strip
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

strip — remove unnecessary information from strippable files (**DEVELOPMENT**)

#### SYNOPSIS

`[SD] ![[Option Start]](../images/opt-start.gif) strip file... ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

A strippable file is defined as a relocatable, object, or executable file. [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)  On
XSI-conformant systems, a strippable file can also be an archive of object or relocatable files. ![[Option End]](../images/opt-end.gif)

The *strip* utility shall remove from strippable files named by the *file* operands any information the implementor
deems unnecessary for execution of those files. The nature of that information is unspecified. The effect of *strip* on object
and executable files shall be similar to the use of the **-s** option to [*c17*](../utilities/c17.html).
[[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
 The effect of *strip* on an archive of object files shall be similar to the use of the **-s** option to [*c17*](../utilities/c17.html) for each object file in the archive. ![[Option End]](../images/opt-end.gif)

#### OPTIONS

None.

#### OPERANDS

The following operand shall be supported:

*file*
:   A pathname referring to a strippable file.

#### STDIN

Not used.

#### INPUT FILES

The input files shall be in the form of strippable files successfully produced by any compiler defined by this volume of
POSIX.1-2024 [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)  or produced by creating or updating an archive of such files using the [*ar*](../utilities/ar.html) utility. ![[Option End]](../images/opt-end.gif)

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *strip*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

Not used.

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

The *strip* utility shall produce strippable files of unspecified format.

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

None.

#### EXAMPLES

None.

#### RATIONALE

Historically, this utility has been used to remove the symbol table from a strippable file. It was included since it is known
that the amount of symbolic information can amount to several megabytes; the ability to remove it in a portable manner was deemed
important, especially for smaller systems.

The behavior of *strip* on object and executable files is said to be the same as the **-s** option to a compiler. While
the end result is essentially the same, it is not required to be identical.

XSI-conformant systems support use of *strip* on archive files containing object files or relocatable files.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*ar*](../utilities/ar.html#), [*c17*](../utilities/c17.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 6

This utility is marked as part of the Software Development Utilities option.

#### Issue 7

Austin Group Interpretation 1003.1-2001 #103 is applied.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1330 is applied, removing obsolescent interfaces.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
