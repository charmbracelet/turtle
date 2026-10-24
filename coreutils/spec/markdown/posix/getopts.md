# getopts
The Open Group Base Specifications Issue 8
IEEE Std 1003.1-2024
Copyright © 2001-2024 The IEEE and The Open Group

#### NAME

getopts — parse utility options

#### SYNOPSIS

`getopts optstring name [param...]`

#### DESCRIPTION

The *getopts* utility shall retrieve options and option-arguments from a list of parameters. It shall support the Utility
Syntax Guidelines 3 to 10, inclusive, described in XBD [*12.2 Utility Syntax
Guidelines*](../basedefs/V1_chap12.html#tag_12_02).

When the shell is first invoked, the shell variable *OPTIND* shall be initialized to 1. Each time *getopts* is
invoked, it shall place the value of the next option found in the parameter list in the shell variable specified by the *name*
operand and the shell variable *OPTIND* shall be set as follows:

* When *getopts* successfully parses an option that takes an option-argument (that is, a character followed by <colon>
  in *optstring*, and exit status is 0), the value of *OPTIND* shall be the integer index of the next element of the
  parameter list (if any; see OPERANDS below) to be searched for an option character. Index 1 identifies the first element of the
  parameter list.
* When *getopts* reports end of options (that is, when exit status is 1), the value of *OPTIND* shall be the integer
  index of the next element of the parameter list (if any).
* In all other cases, the value of *OPTIND* is unspecified, but shall encode the information needed for the next invocation
  of *getopts* to resume parsing options after the option just parsed.

When the option requires an option-argument, the *getopts* utility shall place it in the shell variable *OPTARG .* If
no option was found, or if the option that was found does not have an option-argument, *OPTARG* shall be unset.

If an option character not contained in the *optstring* operand is found where an option character is expected, the shell
variable specified by *name* shall be set to the <question-mark> ('?') character. In this case, if the first
character in *optstring* is a <colon> (':'), the shell variable *OPTARG* shall be set to the option
character found, but no output shall be written to standard error; otherwise, the shell variable *OPTARG* shall be unset and a
diagnostic message shall be written to standard error. This condition shall be considered to be an error detected in the way
arguments were presented to the invoking application, but shall not be an error in *getopts* processing.

If an option-argument is missing:

* If the first character of *optstring* is a <colon>, the shell variable specified by *name* shall be set to the
  <colon> character and the shell variable *OPTARG* shall be set to the option character found.
* Otherwise, the shell variable specified by *name* shall be set to the <question-mark> character, the shell variable
  *OPTARG* shall be unset, and a diagnostic message shall be written to standard error. This condition shall be considered to be
  an error detected in the way arguments were presented to the invoking application, but shall not be an error in *getopts*
  processing; a diagnostic message shall be written as stated, but the exit status shall be zero.

When the end of options is encountered, the *getopts* utility shall exit with a return value of one; the shell variable
*OPTIND* shall be set to the index of the argument containing the first operand in the parameter list, or the value 1 plus the
number of elements in the parameter list if there are no operands in the parameter list; the *name* variable shall be set to
the <question-mark> character. Any of the following shall identify the end of options: the first "--" element of the
parameter list that is not an option-argument, finding an element of the parameter list that is not an option-argument and does not
begin with a '-', or encountering an error.

The shell variables *OPTIND* and *OPTARG* shall not be exported by default. An error in setting any of these variables
(such as if *name* has previously been marked *readonly*) shall be considered an error of *getopts* processing, and
shall result in a return value greater than one.

The *getopts* utility can affect *OPTIND ,* *OPTARG ,* and the shell variable specified by the *name*
operand, within the current shell execution environment; see [*2.13 Shell
Execution Environment*](../utilities/V3_chap02.html#tag_19_13).

If the application sets *OPTIND* to the value 1, a new set of parameters can be used: either the current positional
parameters or new *param* values. Any other attempt to invoke *getopts* multiple times in a single shell execution
environment with parameters (positional parameters or *param* operands) that are not the same in all invocations, or with an
*OPTIND* value modified by the application to be a value other than 1, produces unspecified results.

#### OPTIONS

None.

#### OPERANDS

The following operands shall be supported:

*optstring*
:   A string containing the option characters recognized by the utility invoking *getopts*. If a character is followed by a
    <colon>, the option shall be expected to have an argument, which should be supplied as a separate argument. Applications
    should specify an option character and its option-argument as separate arguments, but *getopts* shall interpret the characters
    following an option character requiring arguments as an argument whether or not this is done. An explicit null option-argument need
    not be recognized if it is not supplied as a separate argument when *getopts* is invoked. (See also the [*getopt*()](../functions/getopt.html) function defined in the System Interfaces volume of POSIX.1-2024.) The characters
    <question-mark> and <colon> shall not be used as option characters by an application. The use of other option
    characters that are not alphanumeric produces unspecified results. Whether or not the option-argument is supplied as a separate
    argument from the option character, the value in *OPTARG* shall only be the characters of the option-argument. The first
    character in *optstring* determines how *getopts* behaves if an option character is not known or an option-argument is
    missing.

*name*
:   The name of a shell variable that shall be set by the *getopts* utility to the option character that was found.

By default, the list of parameters parsed by the *getopts* utility shall be the positional parameters currently set in the
invoking shell environment ("$@"). If *param* operands are given, they shall be parsed instead of the positional
parameters. Note that the next element of the parameter list need not exist; in this case, *OPTIND* will be set to
$#+1 or the number of *param* operands plus 1.

#### STDIN

Not used.

#### INPUT FILES

None.

#### ENVIRONMENT VARIABLES

The following environment variables shall affect the execution of *getopts*:

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

*OPTIND*
:   This variable shall be used by the *getopts* utility as the index of the next argument to be processed.

#### ASYNCHRONOUS EVENTS

Default.

#### STDOUT

Not used.

#### STDERR

Whenever an error is detected and the first character in the *optstring* operand is not a <colon> (':'), a
diagnostic message shall be written to standard error with the following information in an unspecified format:

* The invoking program name shall be identified in the message. The invoking program name shall be the value of the shell special
  parameter 0 (see [*2.5.2 Special Parameters*](../utilities/V3_chap02.html#tag_19_05_02)) at the time the
  *getopts* utility is invoked. A name equivalent to:

  ```

  basename "$0"
  ```

  may be used.
* If an option is found that was not specified in *optstring*, this error is identified and the invalid option character
  shall be identified in the message.
* If an option requiring an option-argument is found, but an option-argument is not found, this error shall be identified and the
  invalid option character shall be identified in the message.

#### OUTPUT FILES

None.

#### EXTENDED DESCRIPTION

None.

#### EXIT STATUS

The following exit values shall be returned:

0
:   An option, specified or unspecified by *optstring*, was found.

1
:   The end of options was encountered.

>1
:   An error occurred.

#### CONSEQUENCES OF ERRORS

Default.

*The following sections are informative.*

#### APPLICATION USAGE

This utility is required to be intrinsic. See [*1.7 Intrinsic Utilities*](../utilities/V3_chap01.html#tag_18_07)
for details.

Since *getopts* affects the current shell execution environment, it is generally provided as a shell regular built-in. If
it is called in a subshell or separate utility execution environment, such as one of the following:

```

(getopts abc value "$@")
nohup getopts ...
find . -exec getopts ... \;
```

it does not affect the shell variables in the caller's environment.

Note that shell functions share *OPTIND* with the calling shell even though the positional parameters are changed. If the
calling shell and any of its functions uses *getopts* to parse arguments, the results are unspecified.

#### EXAMPLES

The following example script parses and displays its arguments:

```

aflag=
bflag=
while getopts ab: name
do
    case $name in
    a)    aflag=1;;
    b)    bflag=1
          bval="$OPTARG";;
    ?)   printf "Usage: %s: [-a] [-b value] args\n" $0
          exit 2;;
    esac
done
if [ -n "$aflag" ]; then
    printf "Option -a specified\n"
fi
if [ -n "$bflag" ]; then
    printf 'Option -b "%s" specified\n' "$bval"
fi
shift $(($OPTIND - 1))
printf "Remaining arguments are: %s\n" "$*"
```

#### RATIONALE

The *getopts* utility was chosen in preference to the System V *getopt* utility because *getopts* handles
option-arguments containing <blank> characters.

The *OPTARG* variable is not mentioned in the ENVIRONMENT VARIABLES section because it does not affect the execution of
*getopts*; it is one of the few "output-only" variables used by the standard utilities.

The <colon> is not allowed as an option character because that is not historical behavior, and it violates the Utility
Syntax Guidelines. The <colon> is now specified to behave as in the KornShell version of the *getopts* utility; when
used as the first character in the *optstring* operand, it disables diagnostics concerning missing option-arguments and
unexpected option characters. This replaces the use of the *OPTERR* variable that was specified in an early proposal.

Although a leading <plus-sign> in *optstring* is required to have no effect on the behavior of [*getopt*()](../functions/getopt.html), this standard intentionally allows implementations of the *getopts* utility to
use a leading <plus-sign> as an extension that alters behavior. In fact, a <plus-sign> anywhere in the *optstring*
in the *getopts* utility produces unspecified behavior.

The formats of the diagnostic messages produced by the *getopts* utility and the [*getopt*()](../functions/getopt.html) function are not fully specified because implementations with superior
("friendlier") formats objected to the formats used by some historical implementations. The standard developers considered it
important that the information in the messages used be uniform between *getopts* and [*getopt*()](../functions/getopt.html). Exact duplication of the messages might not be possible, particularly if a utility
is built on another system that has a different [*getopt*()](../functions/getopt.html) function, but the messages
must have specific information included so that the program name, invalid option character, and type of error can be distinguished
by a user.

Only a rare application program intercepts a *getopts* standard error message and wants to parse it. Therefore,
implementations are free to choose the most usable messages they can devise. The following formats are used by many historical
implementations:

```

"%s: illegal option -- %c\n", <program name>, <option character>

"%s: option requires an argument -- %c\n", <program name>, \
    <option character>
```

Historical shells with built-in versions of [*getopt*()](../functions/getopt.html) or *getopts* have used
different formats, frequently not even indicating the option character found in error.

#### FUTURE DIRECTIONS

None.

#### SEE ALSO

[*2.5.2 Special Parameters*](../utilities/V3_chap02.html#tag_19_05_02)

XBD [*8. Environment Variables*](../basedefs/V1_chap08.html#tag_08), [*12.2 Utility Syntax Guidelines*](../basedefs/V1_chap12.html#tag_12_02)

XSH [*getopt*()](../functions/getopt.html#)

#### CHANGE HISTORY

First released in Issue 4.

#### Issue 6

The normative text is reworded to avoid use of the term "must" for application requirements.

#### Issue 7

POSIX.1-2008, Technical Corrigendum 1, XCU/TC1-2008/0092 [159] is applied.

#### Issue 8

Austin Group Defect 191 is applied, adding a paragraph about leading <plus-sign> to the RATIONALE section.

Austin Group Defect 367 is applied, requiring that *getopts* distinguishes between encountering the end of options and an
error occurring, setting its exit status to one and greater than one, respectively.

Austin Group Defect 854 is applied, adding a note to the APPLICATION USAGE section that this utility is required to be
intrinsic.

Austin Group Defect 1122 is applied, changing the description of *NLSPATH .*

Austin Group Defect 1442 is applied, changing the EXAMPLES section.

Austin Group Defect 1784 is applied, clarifying several aspects of *getopts* behavior and changing the value of
*OPTIND* to be unspecified in some circumstances.

*End of informative text.*

[return to top of page](#top)

UNIX® is a registered Trademark of The Open Group.
POSIX™ is a Trademark of The IEEE.
Copyright © 2001-2024 The IEEE and The Open Group, All Rights Reserved
[ [Main Index](../mindex.html) | [XBD](../basedefs/contents.html) | [XSH](../functions/contents.html) | [XCU](../utilities/contents.html) | [XRAT](../xrat/contents.html)
]
