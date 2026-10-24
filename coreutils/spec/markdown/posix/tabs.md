# tabs
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

tabs — set terminal tabs

#### SYNOPSIS

`[XSI] tabs [-n|![](../images/opt-start.gif)-a|-a2|-c|-c2|-c3|-f|-p|-s|-u![](../images/opt-end.gif)]
[-T type]

tabs [-T type]
n[[sep[+]n]...]`

#### DESCRIPTION

The *tabs* utility shall display a series of characters that first clears the hardware terminal tab settings and then
initializes the tab stops at the specified positions [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)  and optionally adjusts the margin. ![[Option End]](../images/opt-end.gif)

The phrase "tab-stop position *N*" shall be taken to mean that, from the start of a line of output, tabbing to position
*N* shall cause the next character output to be in the (*N*+1)th column position on that line. The maximum number of tab
stops allowed is terminal-dependent.

It need not be possible to implement *tabs* on certain terminals. If the terminal type obtained from the *TERM*
environment variable or **-T** option represents such a terminal, an appropriate diagnostic message shall be written to standard
error and *tabs* shall exit with a status greater than zero.

#### OPTIONS

The *tabs* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02), [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)  except for various extensions: the options **-a2**, **-c2**, and **-c3** are
multi-character. ![[Option End]](../images/opt-end.gif)

The following options shall be supported:

**-***n*
:   Specify repetitive tab stops separated by a uniform number of column positions, *n*, where *n* is a single-digit
    decimal number. The default usage of *tabs* with no arguments shall be equivalent to *tabs* -8. When **-0** is used,
    the tab stops shall be cleared and no new ones set.

**-a**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,10,16,36,72
    Assembler, applicable to some mainframes. ![[Option End]](../images/opt-end.gif)

**-a2**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,10,16,40,72
    Assembler, applicable to some mainframes. ![[Option End]](../images/opt-end.gif)

**-c**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,8,12,16,20,55
    COBOL, normal format. ![[Option End]](../images/opt-end.gif)

**-c2**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,6,10,14,49
    COBOL, compact format (columns 1 to 6 omitted). ![[Option End]](../images/opt-end.gif)

**-c3**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,6,10,14,18,22,26,30,34,38,42,46,50,54,58,62,67
    COBOL compact format (columns 1 to 6 omitted), with more tabs than **-c2**. ![[Option End]](../images/opt-end.gif)

**-f**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,7,11,15,19,23
    FORTRAN ![[Option End]](../images/opt-end.gif)

**-p**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,5,9,13,17,21,25,29,33,37,41,45,49,53,57,61
    PL/1 ![[Option End]](../images/opt-end.gif)

**-s**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,10,55
    SNOBOL ![[Option End]](../images/opt-end.gif)

**-u**
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    1,12,20,44
    Assembler, applicable to some mainframes. ![[Option End]](../images/opt-end.gif)

**-T***type*
:   Indicate the type of terminal. If this option is not supplied and the *TERM* variable is unset or null, an unspecified
    default terminal type shall be used. The setting of *type* shall take precedence over the value in *TERM .*

#### OPERANDS

The following operand shall be supported:

*n***[[***sep***[**+**]***n***]**...**]**
:   A single command line argument that consists of one or more tab-stop values (*n*) separated by a separator character
    (*sep*) which is either a <comma> or a <blank> character. The application shall ensure that the tab-stop values
    are positive decimal integers in strictly ascending order. If any tab-stop value (except the first one) is preceded by a
    <plus-sign>, it is taken as an increment to be added to the previous value. For example, the tab lists 1,10,20,30 and "1
    10 +10 +10" are considered to be identical.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *tabs*:

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

*TERM*
:   Determine the terminal type. If this variable is unset or null, and if the **-T** option is not specified, an unspecified
    default terminal type shall be used.

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

If standard output is a terminal, the appropriate sequence to clear and set the tab stops may be written to standard output in
an unspecified format. If standard output is not a terminal, undefined results occur.

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

This utility makes use of the terminal's hardware tabs and the [*stty*](../utilities/stty.html) *tabs*
option.

This utility is not recommended for application use.

Some integrated display units might not have escape sequences to set tab stops, but may be set by internal system calls. On
these terminals, *tabs* works if standard output is directed to the terminal; if output is directed to another file, however,
*tabs* fails.

#### EXAMPLES

None.

#### RATIONALE

Consideration was given to having the [*tput*](../utilities/tput.html) utility handle all of the functions
described in *tabs*. However, the separate *tabs* utility was retained because it seems more intuitive to use a command
named *tabs* than [*tput*](../utilities/tput.html) with a new option. The [*tput*](../utilities/tput.html) utility does not support setting or clearing tabs, and no known historical version of
*tabs* supports the capability of setting arbitrary tab stops.

The System V *tabs* interface is very complex; the version in this volume of POSIX.1-2024 has a reduced feature list, but
many of the features omitted were restored as part of the XSI option even though the supported languages and coding styles are
primarily historical.

There was considerable sentiment for specifying only a means of resetting the tabs back to a known state—presumably the
"standard" of tabs every eight positions. The following features were omitted:

* Setting tab stops via the first line in a file, using --*file*. Since even the SVID has no complete explanation of this
  feature, it is doubtful that it is in widespread use.

In an early proposal, a **-t** *tablist* option was added for consistency with [*expand*](../utilities/expand.html); this was later removed when inconsistencies with the historical list of tabs were
identified.

Consideration was given to adding a **-p** option that would output the current tab settings so that they could be saved and
then later restored. This was not accepted because querying the tab stops of the terminal is not a capability in historical
*terminfo* or *termcap* facilities and might not be supported on a wide range of terminals.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*expand*](../utilities/expand.html#), [*stty*](../utilities/stty.html#), [*tput*](../utilities/tput.html#), [*unexpand*](../utilities/unexpand.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 6

This utility is marked as part of the User Portability Utilities option.

The normative text is reworded to avoid use of the term "must" for application requirements.

#### Issue 7

The *tabs* utility is removed from the User Portability Utilities option. User Portability Utilities is now an option for
interactive utilities.

SD5-XCU-ERN-97 is applied, updating the SYNOPSIS.

The SYNOPSIS and OPERANDS sections are updated.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
