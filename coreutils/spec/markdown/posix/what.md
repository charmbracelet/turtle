# what
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

what — identify SCCS files (**DEVELOPMENT**)

#### SYNOPSIS

`[XSI] ![[Option Start]](../images/opt-start.gif) what [-s] file... ![[Option End]](../images/opt-end.gif)`

#### DESCRIPTION

The *what* utility shall search the given files for all occurrences of the pattern that [*get*](../utilities/get.html) (see [*get*](../utilities/get.html#)) substitutes for the %**Z**%
keyword ("@(#)"). The *what* utility shall write to standard output the identification string that follows up to, but
not including, the first occurrence of one of the following: <double-quote> ('"' ), <greater-than-sign>
('>'), <newline>, <backslash> ('\\'), <NUL> ('\0'), or an end-of-file condition on
the input file. If not at end-of-file, the *what* utility shall then look for the next occurrence of "@(#)" after one
of those characters.

#### OPTIONS

The *what* utility shall conform to XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

The following option shall be supported:

**-s**
:   Write at most one identification string for each file. After locating and writing to standard output the identification string
    following the first pattern (if any) in a file, no further data shall be read from that file and the search shall recommence from
    the beginning of the next file, if any.

#### OPERANDS

The following operands shall be supported:

*file*
:   A pathname of a file to search.

#### STDIN

Not used.

#### INPUT FILES

The input files shall be of any file type.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *what*:

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

For each *file* operand, the standard output shall consist of:

```

"%s:\n", <pathname>
```

followed by zero or more of:

```

"\t%s\n", <identification string>
```

one for each identification string located.

#### STDERR

The standard error shall be used only for diagnostic messages.

#### OUTPUT FILES

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

The following exit values shall be returned:

0
:   One or more matches were found and the output specified in STDOUT was successfully written to standard output.

1
:   No matches were found or an error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

The *what* utility is intended to be used in conjunction with the SCCS command [*get*](../utilities/get.html), which automatically inserts identifying information, but it can also be used where the
information is inserted by any other means.

When the string "@(#)" is included in a library routine in a shared library, it might not be found in an **a.out**
file using that library routine.

#### EXAMPLES

If the C-language program in file **f.c** contains:

```

char ident[] = "@(#)identification information";
```

and **f.c** is compiled to yield **f.o** and **a.out**, then the command:

```

what f.c f.o a.out
```

writes:

```

f.c:
    identification information
    ...
f.o:
    identification information
    ...
a.out:
    identification information
    ...
```

#### RATIONALE

This standard requires that when the **-s** option is used, *what* does not continue reading from the current file after
writing the first identification string. This might seem an unimportant detail, but applications would experience different
behavior if a *file* operand named a FIFO special file and *what* waited for an end-of-file condition rather than closing
the file straight away.

#### FUTURE DIRECTIONS

If this utility is directed to display a pathname that contains any bytes that have the encoded value of a <newline>
character when <newline> is a terminator or separator in the output format being used, implementations are encouraged to
treat this as an error. A future version of this standard may require implementations to treat this as an error.

#### SEE ALSO

[*get*](../utilities/get.html#)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 8

Austin Group Defect 251 is applied, encouraging implementations to report an error if a utility is directed to display a
pathname that contains any bytes that have the encoded value of a <newline> character when <newline> is a terminator or
separator in the output format being used.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1512 is applied, changing the EXIT STATUS section.

Austin Group Defect 1538 is applied, clarifying the **-s** option.

Austin Group Defect 1563 is applied, clarifying the output format when the *what* utility finds multiple identification
strings in one file.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
