# fg
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

fg — run jobs in the foreground

#### SYNOPSIS

`[UP] ![[Option Start]](../images/opt-start.gif) fg [job_id] ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

If job control is enabled (see the description of [*set*](../utilities/V3_chap02.html#set) **-m**), the shell
is interactive, and the current shell execution environment (see [*2.13 Shell
Execution Environment*](../utilities/V3_chap02.html#tag_19_13)) is not a subshell environment, the *fg* utility shall move a background job in the current
execution environment into the foreground, as described in [*2.11 Job
Control*](../utilities/V3_chap02.html#tag_19_11); it may also do so if the shell is non-interactive or the current shell execution environment is a subshell
environment.

Using *fg* to place a job into the foreground shall remove its process ID from the list of those "known in the current
shell execution environment"; see [*2.9.3.1 Asynchronous AND-OR
Lists*](../utilities/V3_chap02.html#tag_19_09_03_02).

#### OPTIONS

None.

#### OPERANDS

The following operand shall be supported:

*job_id*
:   Specify the job to be run as a foreground job. If no *job_id* operand is given, the *job_id* for the job that was
    most recently suspended, placed in the background, or run as a background job shall be used. The format of *job_id* is
    described in XBD [*3.182 Job ID*](../basedefs/V1_chap03.html#tag_03_182).

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *fg*:

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

The *fg* utility shall write the command line of the job to standard output in the following format:

```

"%s\n", <command>
```

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

If the *fg* utility succeeds, it does not return an exit status. Instead, the shell waits for the job that *fg* moved
into the foreground.

If *fg* does not move a job into the foreground, the following exit value shall be returned:

>0
:   An error occurred.

#### CONSEQUENCES OF ERRORS

If job control is disabled, the *fg* utility shall exit with an error and no job shall be placed in the foreground.

*The following sections are informative.*

#### APPLICATION USAGE

This utility is required to be intrinsic. See [*1.7 Intrinsic Utilities*](../utilities/V3_chap01.html#tag_18_07)
for details.

The *fg* utility does not work as expected when it is operating in its own utility execution environment because that
environment has no applicable jobs to manipulate. See the APPLICATION USAGE section for [*bg*](../utilities/bg.html#). For this reason, *fg* is generally implemented as a shell regular built-in.

#### EXAMPLES

None.

#### RATIONALE

The extensions to the shell specified in this volume of POSIX.1-2024 have mostly been based on features provided by the
KornShell. The job control features provided by [*bg*](../utilities/bg.html), *fg*, and [*jobs*](../utilities/jobs.html) are also based on the KornShell. The standard developers examined the characteristics of
the C shell versions of these utilities and found that differences exist. Despite widespread use of the C shell, the KornShell
versions were selected for this volume of POSIX.1-2024 to maintain a degree of uniformity with the rest of the KornShell features
selected (such as the very popular command line editing features).

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*2.9.3.1 Asynchronous AND-OR Lists*](../utilities/V3_chap02.html#tag_19_09_03_02), [*2.13 Shell Execution Environment*](../utilities/V3_chap02.html#tag_19_13), [*bg*](../utilities/bg.html#)
, [*kill*](../utilities/kill.html#tag_20_64), [*jobs*](../utilities/jobs.html#), [*wait*](../utilities/wait.html#tag_20_147)

XBD [*3.182 Job ID*](../basedefs/V1_chap03.html#tag_03_182), [*8.
Environment Variables*](../basedefs/V1_chap08.html#tag_08)

#### CHANGE HISTORY

First released in Issue 4.

#### Issue 6

This utility is marked as part of the User Portability Utilities option.

The APPLICATION USAGE section is added.

The JC marking is removed from the SYNOPSIS since job control is mandatory is this version.

#### Issue 8

Austin Group Defect 854 is applied, adding a note to the APPLICATION USAGE section that this utility is required to be
intrinsic.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1254 is applied, updating the DESCRIPTION to account for the addition of [*2.11 Job Control*](../utilities/V3_chap02.html#tag_19_11) and changing the EXIT STATUS section.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
