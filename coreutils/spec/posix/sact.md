# sact
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

sact — print current SCCS file-editing activity (**DEVELOPMENT**)

#### SYNOPSIS

`[XSI] ![[Option Start]](../images/opt-start.gif) sact file... ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *sact* utility shall inform the user of any impending deltas to a named SCCS file by writing a list to standard output.
This situation occurs when [*get*](../utilities/get.html) **-e** has been executed previously without a
subsequent execution of [*delta*](../utilities/delta.html), [*unget*](../utilities/unget.html), or
[*sccs*](../utilities/sccs.html) **unedit**.

#### OPTIONS

None.

#### OPERANDS

The following operand shall be supported:

*file*
:   A pathname of an existing SCCS file or a directory. If *file* is a directory, the *sact* utility shall behave as
    though each file in the directory were specified as a named file, except that non-SCCS files (last component of the pathname does
    not begin with **s.**) and unreadable files shall be silently ignored.

    If exactly one *file* operand appears, and it is '-', the standard input shall be read; each line of the standard
    input shall be taken to be the name of an SCCS file to be processed. Non-SCCS files and unreadable files shall be silently
    ignored.

#### STDIN

The standard input shall be a text file used only when the *file* operand is specified as '-'. Each line of the
text file shall be interpreted as an SCCS pathname.

#### INPUT FILES

Any SCCS files interrogated are files of an unspecified format.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *sact*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments and input files).

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error.

*NLSPATH*
:   Determine the location of messages objects and message catalogs.

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The output for each named file shall consist of a line in the following format:

```

"%sΔ%sΔ%sΔ%sΔ%s\n", <SID>, <new SID>, <login>, <date>, <time>
```

<*SID*>
:   Specifies the SID of a delta that currently exists in the SCCS file to which changes are made to make the new delta.

<*new SID*>
:   Specifies the SID for the new delta to be created.

<*login*>
:   Contains the login name of the user who makes the delta (that is, who executed a [*get*](../utilities/get.html)
    for editing).

<*date*>
:   Contains the date that [*get*](../utilities/get.html) **-e** was executed, in the format used by the
    [*prs*](../utilities/prs.html) **:D:** data keyword.

<*time*>
:   Contains the time that [*get*](../utilities/get.html) **-e** was executed, in the format used by the
    [*prs*](../utilities/prs.html) **:T:** data keyword.

If there is more than one named file or if a directory or standard input is named, each pathname shall be written before each of
the preceding lines:

```

"\n%s:\n", <pathname>
```

#### STDERR

The standard error shall be used only for optional informative messages concerning SCCS files with no impending deltas, and for
diagnostic messages.

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

If this utility is directed to display a pathname that contains any bytes that have the encoded value of a <newline>
character when <newline> is a terminator or separator in the output format being used, implementations are encouraged to
treat this as an error. A future version of this standard may require implementations to treat this as an error.

#### SEE ALSO

[*delta*](../utilities/delta.html#), [*get*](../utilities/get.html#), [*sccs*](../utilities/sccs.html#), [*unget*](../utilities/unget.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 8

Austin Group Defect 251 is applied, encouraging implementations to report an error if a utility is directed to display a
pathname that contains any bytes that have the encoded value of a <newline> character when <newline> is a terminator or
separator in the output format being used.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
