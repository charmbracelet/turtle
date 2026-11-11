# uustat
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

uustat — uucp status enquiry and job control

#### SYNOPSIS

`[UU] ![[Option Start]](../images/opt-start.gif) uustat [-q|-k jobid|-r jobid]

uustat [-s system] [-u user] ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *uustat* utility shall display the status of, or cancel, previously specified [*uucp*](../utilities/uucp.html) requests, or provide general status on [*uucp*](../utilities/uucp.html)
connections to other systems.

When no options are given, *uustat* shall write to standard output the status of all [*uucp*](../utilities/uucp.html) requests issued by the current user.

Typical implementations of this utility require a communications line configured to use XBD [*11. General Terminal Interface*](../basedefs/V1_chap11.html#tag_11), but other communications means may be used. On
systems where there are no available communications means (either temporarily or permanently), this utility shall write an error
message describing the problem and exit with a non-zero exit status.

#### OPTIONS

The *uustat* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following options shall be supported:

**-q**
:   Write the jobs queued for each machine.

**-k***jobid*
:   Kill the [*uucp*](../utilities/uucp.html) request whose job identification is *jobid*. The application
    shall ensure that the killed [*uucp*](../utilities/uucp.html) request belongs to the person invoking *uustat*
    unless that user has appropriate privileges.

**-r***jobid*
:   Rejuvenate *jobid*. The files associated with *jobid* are touched so that their modification time is set to the
    current time. This prevents the cleanup program from deleting the job until the jobs modification time reaches the limit imposed by
    the program.

**-s***system*
:   Write the status of all [*uucp*](../utilities/uucp.html) requests for remote system *system*.

**-u***user*
:   Write the status of all [*uucp*](../utilities/uucp.html) requests issued by *user*.

#### OPERANDS

None.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *uustat*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error, and
    informative messages written to standard output.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The standard output shall consist of information about each job selected, in an unspecified format. The information shall
include at least the job ID, the user ID or name, and the remote system name.

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

This utility is part of the UUCP Utilities option and need not be supported by all implementations.

#### EXAMPLES

None.

#### RATIONALE

None.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*uucp*](../utilities/uucp.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*11. General Terminal Interface*](../basedefs/V1_chap11.html#tag_11), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 6

The normative text is reworded to avoid use of the term "must" for application requirements.

The *LC_TIME* and *TZ* entries are removed from the ENVIRONMENT VARIABLES section.

The UN margin code and associated shading are removed from the **-q** option in response to The Open Group Base Resolution
bwg2001-003.

#### Issue 7

SD5-XCU-ERN-46 is applied, moving this utility to the UUCP Utilities Option Group.

SD5-XCU-ERN-97 is applied, updating the SYNOPSIS.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1516 is applied, adding XSI shading to text relating to *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
