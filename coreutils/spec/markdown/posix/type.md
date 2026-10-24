# type
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

type — write a description of command type

#### SYNOPSIS

`[XSI] ![[Option Start]](../images/opt-start.gif) type name... ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *type* utility shall indicate how each argument would be interpreted if used as a command name.

#### OPTIONS

None.

#### OPERANDS

The following operand shall be supported:

*name*
:   A name to be interpreted.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *type*:

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

*PATH*
:   Determine the location of *name*, as described in XBD [*8. Environment
    Variables*](../basedefs/V1_chap08.html#tag_08).

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The standard output of *type* contains information about each operand in an unspecified format. The information provided
typically identifies the operand as a shell built-in, function, alias, or keyword, and where applicable, may display the operand's
pathname.

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

This utility is required to be intrinsic. See [*1.7 Intrinsic Utilities*](../utilities/V3_chap01.html#tag_18_07)
for details.

Since *type* must be aware of the contents of the current shell execution environment (such as the lists of commands,
functions, and built-ins processed by [*hash*](../utilities/hash.html)), it is always provided as a shell regular
built-in. If it is called in a separate utility execution environment, such as one of the following:

```

nohup type writer
find . -type f -exec type {} +
```
it might not produce accurate results.

#### EXAMPLES

None.

#### RATIONALE

None.

#### FUTURE DIRECTIONS

If this utility is directed to display a pathname that contains any bytes that have the encoded value of a <newline>
character when <newline> is a terminator or separator in the output format being used, implementations are encouraged to
treat this as an error. A future version of this standard may require implementations to treat this as an error.

#### SEE ALSO

[*command*](../utilities/command.html#), [*hash*](../utilities/hash.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 8

Austin Group Defect 248 is applied, changing a command line in the APPLICATION USAGE section.

Austin Group Defect 251 is applied, encouraging implementations to report an error if a utility is directed to display a
pathname that contains any bytes that have the encoded value of a <newline> character when <newline> is a terminator or
separator in the output format being used.

Austin Group Defect 854 is applied, adding a note to the APPLICATION USAGE section that this utility is required to be
intrinsic.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
