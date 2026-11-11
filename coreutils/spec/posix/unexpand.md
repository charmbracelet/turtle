# unexpand
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

unexpand — convert spaces to tabs

#### SYNOPSIS

`unexpand [-a|-t tablist] [file...]`

#### DESCRIPTION

The *unexpand* utility shall copy files or standard input to standard output, converting <blank> characters at the
beginning of each line into the maximum number of <tab> characters followed by the minimum number of <space> characters
needed to fill the same column positions originally filled by the translated <blank> characters. By default, tabstops shall
be set at every eighth column position. Each <backspace> shall be copied to the output, and shall cause the column position
count for tab calculations to be decremented; the count shall never be decremented to a value less than one.

#### OPTIONS

The *unexpand* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following options shall be supported:

**-a**
:   In addition to translating <blank> characters at the beginning of each line, translate all sequences of two or more
    <blank> characters immediately preceding a tab stop to the maximum number of <tab> characters followed by the minimum
    number of <space> characters needed to fill the same column positions originally filled by the translated <blank>
    characters.

**-t***tablist*
:   Specify the tab stops. The application shall ensure that the *tablist* option-argument is a single argument consisting of
    a single positive decimal integer or multiple positive decimal integers, separated by <blank> or <comma> characters, in
    ascending order. If a single number is given, tabs shall be set *tablist* column positions apart instead of the default 8. If
    multiple numbers are given, the tabs shall be set at those specific column positions.

    The application shall ensure that each tab-stop position *N* is an integer value greater than zero, and the list shall be
    in strictly ascending order. This is taken to mean that, from the start of a line of output, tabbing to position *N* shall
    cause the next character output to be in the (*N*+1)th column position on that line. When the **-t** option is not
    specified, the default shall be the equivalent of specifying **-t 8** (except for the interaction with **-a**, described
    below).

    No <space>-to-<tab> conversions shall occur for characters at positions beyond the last of those specified in a
    multiple tab-stop list.

    When **-t** is specified, the presence or absence of the **-a** option shall be ignored; conversion shall not be limited
    to the processing of leading <blank> characters.

#### OPERANDS

The following operand shall be supported:

*file*
:   A pathname of a text file to be used as input.

#### STDIN

See the INPUT FILES section.

#### INPUT FILES

The input files shall be text files.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *unexpand*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as
    opposed to multi-byte characters in arguments and input files), the processing of <tab> and <space> characters, and for
    the determination of the width in column positions each character would occupy on an output device.

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The standard output shall be equivalent to the input files with the specified <space>-to-<tab> conversions.

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

One non-intuitive aspect of *unexpand* is its restriction to leading <space> characters when neither **-a** nor
**-t** is specified. Users who always want to convert all <space> characters in a file can easily alias *unexpand* to
use the **-a** or **-t 8** option.

#### EXAMPLES

None.

#### RATIONALE

On several occasions, consideration was given to adding a **-t** option to the *unexpand* utility to complement the
**-t** in [*expand*](../utilities/expand.html) (see [*expand*](../utilities/expand.html#)). The
historical intent of *unexpand* was to translate multiple <blank> characters into tab stops, where tab stops were a
multiple of eight column positions on most UNIX systems. An early proposal omitted **-t** because it seemed outside the scope of
the User Portability Utilities option; it was not described in any of the base documents for IEEE Std 1003.2-1992.
However, hard-coding tab stops every eight columns was not suitable for the international community and broke historical precedents
for some vendors in the FORTRAN community, so **-t** was restored in conjunction with the list of valid extension categories
considered by the standard developers. Thus, *unexpand* is now the logical converse of [*expand*](../utilities/expand.html).

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*expand*](../utilities/expand.html#), [*tabs*](../utilities/tabs.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 4.

#### Issue 6

This utility is marked as part of the User Portability Utilities option.

The definition of the *LC_CTYPE* environment variable is changed to align with the IEEE P1003.2b draft standard.

The normative text is reworded to avoid use of the term "must" for application requirements.

#### Issue 7

The *unexpand* utility is moved from the User Portability Utilities option to the Base. User Portability Utilities is now
an option for interactive utilities.

SD5-XCU-ERN-97 is applied, updating the SYNOPSIS.

POSIX.1-2008, Technical Corrigendum 2, XCU/TC2-2008/0198 [885] is applied.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
