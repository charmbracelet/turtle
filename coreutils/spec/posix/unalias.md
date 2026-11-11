# unalias
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

unalias — remove alias definitions

#### SYNOPSIS

`unalias alias-name...

unalias -a`

#### DESCRIPTION

The *unalias* utility shall remove the definition for each alias name specified. See [*2.3.1 Alias Substitution*](../utilities/V3_chap02.html#tag_19_03_01). The aliases shall be removed from the current
shell execution environment; see [*2.13 Shell Execution Environment*](../utilities/V3_chap02.html#tag_19_13).

#### OPTIONS

The *unalias* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following option shall be supported:

**-a**
:   Remove all alias definitions from the current shell execution environment.

#### OPERANDS

The following operand shall be supported:

*alias-name*
:   The name of an alias to be removed.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *unalias*:

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

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

The following exit values shall be returned:

0
:   Successful completion.

>0
:   One of the *alias-name* operands specified did not represent a valid alias definition, or an error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

This utility is required to be intrinsic. See [*1.7 Intrinsic Utilities*](../utilities/V3_chap01.html#tag_18_07)
for details.

Since *unalias* affects the current shell execution environment, it is generally provided as a shell regular built-in.

#### EXAMPLES

None.

#### RATIONALE

The *unalias* description is based on that from historical KornShell implementations. Known differences exist between that
and the C shell. The KornShell version was adopted to be consistent with all the other KornShell features in this volume of
POSIX.1-2024, such as command line editing.

The **-a** option is the equivalent of the *unalias* \* form of the C shell and is provided to address security concerns
about unknown aliases entering the environment of a user (or application) through the allowable implementation-defined predefined
alias route or as a result of an *ENV* file. (Although *unalias* could be used to simplify the "secure" shell script
shown in the [*command*](../utilities/command.html) rationale, it does not obviate the need to quote all command
names. An initial call to *unalias* **-a** would have to be quoted in case there was an alias for *unalias*.)

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*2. Shell Command Language*](../utilities/V3_chap02.html#tag_19), [*alias*](../utilities/alias.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 4.

#### Issue 6

This utility is marked as part of the User Portability Utilities option.

#### Issue 7

The *unalias* utility is moved from the User Portability Utilities option to the Base. User Portability Utilities is now an
option for interactive utilities.

#### Issue 8

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
