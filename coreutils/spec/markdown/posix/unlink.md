# unlink
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

unlink — call the unlink function

#### SYNOPSIS

`[XSI] ![[Option Start]](../images/opt-start.gif) unlink file ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *unlink* utility shall perform the function call:

```

unlink(file);
```

A user may need appropriate privileges to invoke the *unlink* utility.

#### OPTIONS

None.

#### OPERANDS

The following operands shall be supported:

*file*
:   The pathname of an existing file.

#### STDIN

Not used.

#### INPUT FILES

Not used.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *unlink*:

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
:   Determine the location of messages objects and message catalogs.

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

None.

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

None.

#### EXAMPLES

None.

#### RATIONALE

None.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*link*](../utilities/link.html#tag_20_66), [*rm*](../utilities/rm.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08)

XSH [*unlink*()](../functions/unlink.html#tag_17_649)

#### CHANGE HISTORY

First released in Issue 5.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
