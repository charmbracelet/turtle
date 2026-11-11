# mesg
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

mesg — permit or deny messages

#### SYNOPSIS

`mesg [y|n]`

#### DESCRIPTION

The *mesg* utility shall control whether other users are allowed to send messages via [*write*](../utilities/write.html), [*talk*](../utilities/talk.html), or other utilities to a terminal
device. The terminal device affected shall be determined by searching for the first terminal in the sequence of devices associated
with standard input, standard output, and standard error, respectively. With no arguments, *mesg* shall report the current
state without changing it. Processes with appropriate privileges may be able to send messages to the terminal independent of the
current state.

#### OPTIONS

None.

#### OPERANDS

The following operands shall be supported in the POSIX locale:

*y*
:   Grant permission to other users to send messages to the terminal device.

*n*
:   Deny permission to other users to send messages to the terminal device.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *mesg*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written (by *mesg*) to
    standard error.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

If no operand is specified, *mesg* shall display the current terminal state in an unspecified format.

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

The following exit values shall be returned:

0
:   Receiving messages is allowed.

1
:   Receiving messages is not allowed.

>1
:   An error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

The mechanism by which the message status of the terminal is changed is unspecified. Therefore, unspecified actions may cause
the status of the terminal to change after *mesg* has successfully completed. These actions may include, but are not limited
to: another invocation of the *mesg* utility, login procedures; invocation of the [*stty*](../utilities/stty.html) utility, invocation of the [*chmod*](../utilities/chmod.html) utility or
[*chmod*()](../functions/chmod.html) function, and so on.

#### EXAMPLES

None.

#### RATIONALE

The terminal changed by *mesg* is that associated with the standard input, output, or error, rather than the controlling
terminal for the session. This is because users logged in more than once should be able to change any of their login terminals
without having to stop the job running in those sessions. This is not a security problem involving the terminals of other users
because appropriate privileges would be required to affect the terminal of another user.

The method of checking each of the first three file descriptors in sequence until a terminal is found was adopted from System
V.

The file **/dev/tty** is not specified for the terminal device because it was thought to be too restrictive. Typical
environment changes for the *n* operand are that write permissions are removed for *others* and *group* from the
appropriate device. It was decided to leave the actual description of what is done as unspecified because of potential differences
between implementations.

The format for standard output is unspecified because of differences between historical implementations. This output is
generally not useful to shell scripts (they can use the exit status), so exact parsing of the output is unnecessary.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*talk*](../utilities/talk.html#), [*write*](../utilities/write.html#tag_20_151)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 6

This utility is marked as part of the User Portability Utilities option.

#### Issue 7

The *mesg* utility is moved from the User Portability Utilities option to the Base. User Portability Utilities is now an
option for interactive utilities.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
