# ipcrm
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

ipcrm — remove an XSI message queue, semaphore set, or shared memory segment identifier

#### SYNOPSIS

`[XSI] ![[Option Start]](../images/opt-start.gif) ipcrm [-q msgid|-Q msgkey|-s semid|-S semkey|-m shmid|-M shmkey]...
![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *ipcrm* utility shall remove zero or more message queues, semaphore sets, or shared memory segments. The interprocess
communication facilities to be removed are specified by the options.

Only a user with appropriate privileges shall be allowed to remove an interprocess communication facility that was not created
by or owned by the user invoking *ipcrm*.

#### OPTIONS

The *ipcrm* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following options shall be supported:

**-q***msgid*
:   Remove the message queue identifier *msgid* from the system and destroy the message queue and data structure associated
    with it.

**-m***shmid*
:   Remove the shared memory identifier *shmid* from the system. The shared memory segment and data structure associated with
    it shall be destroyed when all processes with the segment attached have either detached the segment or terminated. If the segment
    is not attached to any process, it shall be destroyed immediately.

**-s***semid*
:   Remove the semaphore identifier *semid* from the system and destroy the set of semaphores and data structure associated
    with it.

**-Q***msgkey*
:   Remove the message queue identifier, created with key *msgkey*, from the system and destroy the message queue and data
    structure associated with it.

**-M***shmkey*
:   Remove the shared memory identifier, created with key *shmkey*, from the system. The shared memory segment and data
    structure associated with it shall be destroyed after the last detach.

**-S***semkey*
:   Remove the semaphore identifier, created with key *semkey*, from the system and destroy the set of semaphores and data
    structure associated with it.

#### OPERANDS

None.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *ipcrm*:

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

Not used.

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

[*ipcs*](../utilities/ipcs.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

XSH [*msgctl*()](../functions/msgctl.html#), [*semctl*()](../functions/semctl.html#), [*shmctl*()](../functions/shmctl.html#)

#### CHANGE HISTORY

First released in Issue 5.

#### Issue 7

SD5-XCU-ERN-97 is applied, updating the SYNOPSIS.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1240 is applied, clarifying the description of the **-m** option.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
