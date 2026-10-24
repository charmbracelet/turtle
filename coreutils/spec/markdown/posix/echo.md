# echo
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

echo — write arguments to standard output

#### SYNOPSIS

`echo [string...]`

#### DESCRIPTION

The *echo* utility writes its arguments to standard output, followed by a <newline>. If there are no arguments, only
the <newline> is written.

#### OPTIONS

The *echo* utility shall not recognize the "--" argument in the manner specified by Guideline 10 of XBD [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02); "--" shall be recognized as a string
operand.

Implementations shall not support any options.

#### OPERANDS

The following operands shall be supported:

*string*
:   A string to be written to standard output. If the first operand consists of a '-' followed by one or more characters
    from the set {'e', 'E', 'n'}, or if any of the operands contain a <backslash> character, the
    results are implementation-defined.

    [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    On XSI-conformant systems, if the first operand consists of a '-' followed by one or more characters from the set
    {'e', 'E', 'n'}, it shall be treated as a string to be written. The following character sequences shall
    be recognized on XSI-conformant systems within any of the arguments:

    \a
    :   Write an <alert>.

    \b
    :   Write a <backspace>.

    \c
    :   Suppress the <newline> that otherwise follows the final argument in the output. All characters following the
        '\c' in the arguments shall be ignored.

    \f
    :   Write a <form-feed>.

    \n
    :   Write a <newline>.

    \r
    :   Write a <carriage-return>.

    \t
    :   Write a <tab>.

    \v
    :   Write a <vertical-tab>.

    \\
    :   Write a <backslash> character.

    \0*num*
    :   Write an 8-bit value that is the zero, one, two, or three-digit octal number *num*.

![[Option End]](../images/opt-end.gif)

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *echo*:

*LANG*
:   Provide a default value for the internationalization variables that are unset or null. (See XBD [*8.2 Internationalization Variables*](../basedefs/V1_chap08.html#tag_08_02) for the precedence of internationalization
    variables used to determine the values of locale categories.)

*LC_ALL*
:   If set to a non-empty string value, override the values of all the other internationalization variables.

*LC_CTYPE*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the locale for the interpretation of sequences of bytes of text data as characters (for example, single-byte as opposed
    to multi-byte characters in arguments). ![[Option End]](../images/opt-end.gif)

*LC_MESSAGES*
:   Determine the locale that should be used to affect the format and contents of diagnostic messages written to standard error.

*NLSPATH*
:   [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)
    Determine the location of messages objects and message catalogs. ![[Option End]](../images/opt-end.gif)

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

The *echo* utility arguments shall be separated by single <space> characters and a <newline> character shall
follow the last argument. [[XSI](javascript:open_code('XSI'))] ![[Option Start]](../images/opt-start.gif)  Output transformations shall occur based on the escape sequences in the input. See the OPERANDS
section. ![[Option End]](../images/opt-end.gif)

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

It is not possible to use *echo* portably across all POSIX systems unless escape sequences are omitted, and the first
argument does not consist of a '-' followed by one or more characters from the set {'e', 'E',
'n'}.

The [*printf*](../utilities/printf.html) utility can be used portably to emulate any of the traditional
behaviors of the *echo* utility as follows (assuming that *IFS* has its standard value or is unset):

* The historic System V *echo* and the requirements on XSI implementations in this volume of POSIX.1-2024 are equivalent
  to:

  ```

  printf "%b\n" "$*"
  ```
* The BSD *echo* is equivalent to:

  ```

  if [ "X$1" = "X-n" ]
  then
      shift
      printf "%s" "$*"
  else
      printf "%s\n" "$*"
  fi
  ```

New applications are encouraged to use [*printf*](../utilities/printf.html) instead of *echo*.

#### EXAMPLES

None.

#### RATIONALE

The *echo* utility has not been made obsolescent because of its extremely widespread use in historical applications.
Conforming applications that wish to do prompting without <newline> characters or that could possibly be expecting to echo a
string consisting of a '-' followed by one or more characters from the set {'e', 'E', 'n'}
should use the [*printf*](../utilities/printf.html) utility.

At the time that the IEEE Std 1003.2-1992 standard was being developed, the two different historical versions of
*echo* that were considered for standardization varied in incompatible ways.

The BSD *echo* checked the first argument for the string **-n** which caused it to suppress the <newline> that
would otherwise follow the final argument in the output.

The System V *echo* treated all arguments as strings to be written, but allowed escape sequences within them, as described
for XSI implementations in the OPERANDS section, including \c to suppress a trailing <newline>.

Thus the IEEE Std 1003.2-1992 standard said that the behavior was implementation-defined if the first operand is
**-n** or if any of the operands contain a <backslash> character. It also specified that the *echo* utility does not
support Utility Syntax Guideline 10 because historical applications depended on *echo* to echo *all* of its arguments,
except for the **-n** first argument in the BSD version.

The Single UNIX Specification, Version 1 required the System V behavior, and this became the XSI requirement when Version 2 and
POSIX.2 were merged with POSIX.1 to form the joint IEEE Std 1003.1-2001 / Single UNIX Specification, Version 3
standard.

This standard now treats a first operand of **-e** or **-E** the same as **-n** in recognition that support for them
has become more widespread in non-XSI implementations. Where supported, **-e** enables processing of escape sequences in the
remaining operands (in situations where it is disabled by default), and **-E** disables it (in situations where it is enabled by
default). A first operand containing a combination of these three letters, in the same manner as option grouping, also results in
implementation-defined behavior.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*printf*](../utilities/printf.html#tag_20_96)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

#### CHANGE HISTORY

First released in Issue 2.

#### Issue 5

In the OPTIONS section, the last sentence is changed to indicate that implementations "do not" support any options; in the
previous issue this said "need not".

#### Issue 6

The following new requirements on POSIX implementations derive from alignment with the Single UNIX Specification:

* A set of character sequences is defined as *string* operands.
* *LC_CTYPE* is added to the list of environment variables affecting *echo*.
* In the OPTIONS section, implementations shall not support any options.

IEEE Std 1003.1-2001/Cor 1-2002, item XCU/TC1/D6/21 is applied, so that the *echo* utility can accommodate
historical BSD behavior.

#### Issue 7

SD5-XCU-ERN-97 is applied, updating the SYNOPSIS.

#### Issue 8

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1222 is applied, making the results implementation-defined, on systems that are not XSI-conformant, if the
first operand consists of a '-' followed by one or more characters from the set {'e', 'E',
'n'}.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
