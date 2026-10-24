# logname
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

logname — return the user's login name

#### SYNOPSIS

`logname`

#### DESCRIPTION

The *logname* utility shall write the user's login name to standard output. The login name shall be the string that would
be returned by the [*getlogin*()](../functions/getlogin.html) function defined in the System Interfaces volume of
POSIX.1-2024. Under the conditions where the [*getlogin*()](../functions/getlogin.html) function would fail, the
*logname* utility shall write a diagnostic message to standard error and exit with a non-zero exit status.

#### OPTIONS

None.

#### OPERANDS

None.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *logname*:

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

The *logname* utility output shall be a single line consisting of the user's login name:

```

"%s\n", <login name>
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
:   Successful completion.

>0
:   An error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

The *logname* utility explicitly ignores the *LOGNAME* environment variable because environment changes could produce
erroneous results.

#### EXAMPLES

None.

#### RATIONALE

The **passwd** file is not listed as required because the implementation may have other means of mapping login names.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*id*](../utilities/id.html#), [*who*](../utilities/who.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08)

XSH [*getlogin*()](../functions/getlogin.html#)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
