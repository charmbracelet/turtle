# head
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

head — copy the first part of files

#### SYNOPSIS

`head [-c number|-n number] [file...]`

#### DESCRIPTION

The *head* utility shall copy its input files to the standard output, ending the output for each file at a designated
point.

Copying shall end at the point in the file indicated by the **-c** *number* or **-n** *number* options. The
option-argument *number* shall be counted in units of lines or bytes, according to the options **-n** and **-c**. Both
line and byte counts start from 1.

#### OPTIONS

The *head* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following options shall be supported:

**-c***number*
:   The first *number* bytes of each input file shall be copied to standard output. The application shall ensure that the
    *number* option-argument is a positive decimal integer.

**-n***number*
:   This option shall be equivalent to **-c** *number*, except that the ending location in the file shall be measured in
    lines instead of bytes.

When a file contains less than *number* bytes or lines, it shall be copied to standard output in its entirety. This shall
not be an error.

If no options are specified, *head* shall act as if **-n 10** had been specified.

#### OPERANDS

The following operand shall be supported:

*file*
:   A pathname of an input file. If no *file* operands are specified, the standard input shall be used.

#### STDIN

The standard input shall be used if no *file* operands are specified, and shall be used if a *file* operand is
'-' and the implementation treats the '-' as meaning standard input. Otherwise, the standard input shall not be
used. See the INPUT FILES section.

#### INPUT FILES

If the **-c** option is specified, the input files can contain arbitrary data; otherwise, the input files shall be text
files, but the line length shall not be restricted to {LINE_MAX} bytes.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *head*:

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
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The standard output shall contain designated portions of the input files.

If multiple *file* operands are specified, *head* shall precede the output for each with the header:

```

"\n==> %s <==\n", <pathname>
```

except that the first header written shall not include the initial <newline>.

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

When using *head* to process pathnames, it is recommended that LC_ALL, or at least LC_CTYPE and LC_COLLATE, are set to
POSIX or C in the environment, since pathnames can contain byte sequences that do not form valid characters in some locales, in
which case the utility's behavior would be undefined. In the POSIX locale each byte is a valid single-byte character, and therefore
this problem is avoided.

#### EXAMPLES

To write the first ten lines of all files (except those with a leading period) in the directory:

```

head -- *
```

#### RATIONALE

Although it is possible to simulate *head* with [*sed*](../utilities/sed.html) 10q for a single file, the
standard developers decided that the popularity of *head* on historical BSD systems warranted its inclusion alongside [*tail*](../utilities/tail.html).

POSIX.1-2024 version of *head* follows the Utility Syntax Guidelines. The **-n** option was added to this new interface
so that *head* and [*tail*](../utilities/tail.html) would be more logically related. Earlier versions of this
standard allowed a **-number** option. This form is no longer specified by POSIX.1-2024 but may be present in some
implementations.

The *head* and [*tail*](../utilities/tail.html) utilities have not historically been symmetric. For
example, this standard only requires [*tail*](../utilities/tail.html) to support at most one file operand, while
*head* must operate on multiple files. Conversely, this standard requires [*tail*](../utilities/tail.html) to
be able to start at a position relative to the start of a file, but *head* need not support stopping at a position relative to
the end of the file. Implementations may choose to make *head* and [*tail*](../utilities/tail.html) symmetric
as an extension, but applications should not rely on this.

Older implementations of *head* did not support **-c** *number*, but emulating this via dd ibs=1
count=number is much less efficient and emulating via dd obs=pipe_buf | dd ibs=size count=number_of_blocks is
cumbersome, somewhat less efficient, and can only be used if the number of bytes to be copied is a multiple of a suitable block
size less than or equal to {PIPE_BUF}.

#### FUTURE DIRECTIONS

If this utility is directed to display a pathname that contains any bytes that have the encoded value of a <newline>
character when <newline> is a terminator or separator in the output format being used, implementations are encouraged to
treat this as an error. A future version of this standard may require implementations to treat this as an error.

#### SEE ALSO

[*dd*](../utilities/dd.html#), [*sed*](../utilities/sed.html#), [*tail*](../utilities/tail.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 4.

#### Issue 6

The obsolescent **-number** form is removed.

The normative text is reworded to avoid use of the term "must" for application requirements.

The DESCRIPTION is updated to clarify that when a file contains less than the number of lines requested, the entire file is
copied to standard output.

#### Issue 7

Austin Group Interpretations 1003.1-2001 #027 and #092 are applied.

SD5-XCU-ERN-97 is applied, updating the SYNOPSIS.

The APPLICATION USAGE section is removed and the EXAMPLES section is corrected.

POSIX.1-2008, Technical Corrigendum 2, XCU/TC2-2008/0107 [663] is applied.

#### Issue 8

Austin Group Defect 251 is applied, encouraging implementations to report an error if a utility is directed to display a
pathname that contains any bytes that have the encoded value of a <newline> character when <newline> is a terminator or
separator in the output format being used.

Austin Group Defect 407 is applied, adding the **-c** *number* option.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
