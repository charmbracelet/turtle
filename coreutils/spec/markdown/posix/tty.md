# tty
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

tty — return user's terminal name

#### SYNOPSIS

`tty`

#### DESCRIPTION

The *tty* utility shall write to the standard output the name of the terminal that is open as standard input. The name that
is used shall be equivalent to the string that would be returned by the [*ttyname*()](../functions/ttyname.html)
function defined in the System Interfaces volume of POSIX.1-2024.

#### OPTIONS

The *tty* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

#### OPERANDS

None.

#### STDIN

While no input is read from standard input, standard input shall be examined to determine whether or not it is a terminal, and,
if so, to determine the name of the terminal.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *tty*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error and
    informative messages written to standard output.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

If standard input is a terminal device, a pathname of the terminal as specified by the [*ttyname*()](../functions/ttyname.html) function defined in the System Interfaces volume of POSIX.1-2024 shall be written
in the following format:

```

"%s\n", <terminal name>
```

Otherwise, a message shall be written indicating that standard input is not connected to a terminal. In the POSIX locale, the
*tty* utility shall use the format:

```

"not a tty\n"
```

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

The following exit values shall be returned:

0
:   Standard input is a terminal, and the output specified in STDOUT was successfully written to standard output.

1
:   Standard input is not a terminal, and the output specified in STDOUT was successfully written to standard output.

>1
:   An error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

This utility checks the status of the file open as standard input against that of an implementation-defined set of files. It is
possible that no match can be found, or that the match found need not be the same file as that which was opened for standard input
(although they are the same device).

#### EXAMPLES

None.

#### RATIONALE

None.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

XSH [*isatty*()](../functions/isatty.html#), [*ttyname*()](../functions/ttyname.html#)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 5

The SYNOPSIS is changed to indicate two forms of the command, with the second form marked as obsolete. This is a clarification
and does not change the functionality published in previous issues.

#### Issue 6

The obsolescent **-s** option is removed.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1509 is applied, changing the EXIT STATUS section.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
