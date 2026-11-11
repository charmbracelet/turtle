# rmdel
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

rmdel — remove a delta from an SCCS file (**DEVELOPMENT**)

#### SYNOPSIS

`[XSI] ![[Option Start]](../images/opt-start.gif) rmdel -r SID file... ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *rmdel* utility shall remove the delta specified by the SID from each named SCCS file. The delta to be removed shall be
the most recent delta in its branch in the delta chain of each named SCCS file. In addition, the application shall ensure that the
SID specified is not that of a version being edited for the purpose of making a delta; that is, if a *p-file* (see [*get*](../utilities/get.html#)) exists for the named SCCS file, the SID specified shall not appear in any entry of the
*p-file*.

Removal of a delta shall be restricted to:

1. The user who made the delta
2. The owner of the SCCS file
3. The owner of the directory containing the SCCS file

#### OPTIONS

The *rmdel* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following option shall be supported:

**-r***SID*
:   Specify the SCCS identification string (*SID*) of the delta to be deleted.

#### OPERANDS

The following operand shall be supported:

*file*
:   A pathname of an existing SCCS file or a directory. If *file* is a directory, the *rmdel* utility shall behave as
    though each file in the directory were specified as a named file, except that non-SCCS files (last component of the pathname does
    not begin with **s.**) and unreadable files shall be silently ignored.

    If exactly one *file* operand appears, and it is '-', the standard input shall be read; each line of the standard
    input is taken to be the name of an SCCS file to be processed. Non-SCCS files and unreadable files shall be silently ignored.

#### STDIN

The standard input shall be a text file used only when the *file* operand is specified as '-'. Each line of the
text file shall be interpreted as an SCCS pathname.

#### INPUT FILES

The SCCS files shall be files of unspecified format.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *rmdel*:

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

Not used.

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

The SCCS files shall be files of unspecified format. During processing of a *file*, a temporary *x-file*, as described
in [*admin*](../utilities/admin.html#), may be created and deleted; a locking *z-file*, as described in
[*get*](../utilities/get.html#), may be created and deleted.

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

If this utility is directed to create a new directory entry that contains any bytes that have the encoded value of a
<newline> character, implementations are encouraged to treat this as an error. A future version of this standard may require
implementations to treat this as an error.

#### SEE ALSO

[*admin*](../utilities/admin.html#), [*delta*](../utilities/delta.html#), [*get*](../utilities/get.html#), [*prs*](../utilities/prs.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 6

The normative text is reworded to avoid use of the term "must" for application requirements.

#### Issue 8

Austin Group Defect 251 is applied, encouraging implementations to disallow the creation of filenames containing any bytes that
have the encoded value of a <newline> character.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
